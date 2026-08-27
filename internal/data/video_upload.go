package data

import (
	"context"
	"errors"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/media"

	"github.com/go-kratos/kratos/v3/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var quotaStatuses = []string{
	string(biz.VideoStatusProcessing),
	string(biz.VideoStatusReady),
	string(biz.VideoStatusPendingReview),
	string(biz.VideoStatusPublished),
	string(biz.VideoStatusRejected),
}

// ProcessVideoUpload stores a bounded source file and queues it for background transcoding.
func (r *videoRepo) ProcessVideoUpload(ctx context.Context, input *biz.VideoUploadInput) (*biz.VideoUploadResult, error) {
	record := videoPO{
		OwnerID: input.OwnerID, Title: "", Description: "",
		Status: string(biz.VideoStatusProcessing), Tags: []string{},
	}
	if err := r.data.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	videoID := biz.VideoID(record.ID)
	job, err := r.mediaManager.CreateUploadJob()
	if err != nil {
		r.markVideoUploadFailed(ctx, videoID, "storage initialization failed")
		return nil, biz.ErrVideoStorage
	}
	fail := func(reason string, publicError error) (*biz.VideoUploadResult, error) {
		r.markVideoUploadFailed(ctx, videoID, reason)
		if cleanupErr := r.mediaManager.RemoveUploadJob(job); cleanupErr != nil {
			log.Error("remove failed upload job", "job_id", job.ID(), "error", cleanupErr)
		}
		return nil, publicError
	}

	uploadBytes, err := r.mediaManager.StoreUpload(ctx, job, input.Content)
	if err != nil {
		if errors.Is(err, media.ErrUploadTooLarge) {
			return fail("video exceeds upload limit", biz.ErrVideoUploadTooLarge)
		}
		return fail("upload interrupted", biz.ErrVideoUploadInterrupted)
	}
	if input.Cover != nil {
		if err := r.mediaManager.StoreCover(ctx, job, input.Cover); err != nil {
			if errors.Is(err, media.ErrCoverTooLarge) {
				return fail("cover exceeds upload limit", biz.ErrVideoUploadTooLarge)
			}
			return fail("cover upload interrupted", biz.ErrVideoUploadInterrupted)
		}
	}
	if err := r.queueVideoUpload(ctx, &record, job.ID(), uploadBytes); err != nil {
		if errors.Is(err, biz.ErrVideoUploadQuotaExceeded) {
			return fail("user storage quota exceeded", err)
		}
		return fail("queue persistence failed", biz.ErrVideoStorage)
	}
	return &biz.VideoUploadResult{VideoID: videoID, Status: biz.VideoStatusProcessing}, nil
}

// ProcessNextVideoUpload claims one queued row so a bounded worker pool can transcode it.
func (r *videoRepo) ProcessNextVideoUpload(ctx context.Context) (bool, error) {
	record, err := r.claimVideoUpload(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, biz.ErrVideoStorage
	}
	videoID := biz.VideoID(record.ID)
	job, err := r.mediaManager.OpenUploadJob(record.UploadJobID)
	if err != nil {
		r.failQueuedVideoUpload(videoID, record.UploadJobID, "upload job is missing")
		return true, biz.ErrVideoProcessing
	}
	fail := func(reason string, cause error) (bool, error) {
		log.Error("process video upload failed", "bvid", videoID.BVID(), "job_id", record.UploadJobID, "stage", reason, "error", cause)
		r.failQueuedVideoUpload(videoID, record.UploadJobID, reason)
		return true, biz.ErrVideoProcessing
	}

	metadata, err := r.mediaManager.InspectMP4(ctx, job)
	if err != nil {
		if ctx.Err() != nil {
			return true, r.releaseVideoUploadClaim(videoID)
		}
		return fail("media inspection failed", err)
	}
	if err := r.mediaManager.GenerateCover(ctx, job, metadata, job.HasCover()); err != nil {
		if ctx.Err() != nil {
			return true, r.releaseVideoUploadClaim(videoID)
		}
		return fail("cover generation failed", err)
	}
	renditions, err := r.mediaManager.TranscodeDASH(ctx, job, metadata)
	if err != nil {
		if ctx.Err() != nil {
			return true, r.releaseVideoUploadClaim(videoID)
		}
		return fail("DASH transcoding failed", err)
	}
	manifestURL, publishedDir, err := r.mediaManager.PublishDASH(job, videoID.BVID())
	if err != nil {
		return fail("media publication failed", err)
	}
	coverURL := "/media/dash/" + videoID.BVID() + "/cover.jpg"
	if err := r.markVideoUploadReady(ctx, videoID, metadata, renditions, manifestURL, coverURL); err != nil {
		if cleanupErr := r.mediaManager.RemovePublished(publishedDir); cleanupErr != nil {
			log.Error("remove rolled-back DASH directory", "path", publishedDir, "error", cleanupErr)
		}
		return fail("media metadata persistence failed", err)
	}
	if err := r.mediaManager.RemoveUploadJob(job); err != nil {
		log.Error("remove completed upload job", "error", err)
	}
	return true, nil
}

// RecoverStaleVideoUploads releases claims left behind by an interrupted worker.
func (r *videoRepo) RecoverStaleVideoUploads(ctx context.Context, before time.Time) (int64, error) {
	result := r.data.db.WithContext(ctx).Model(&videoPO{}).
		Where("status = ? AND processing_started_at < ?", string(biz.VideoStatusProcessing), before).
		Update("processing_started_at", nil)
	if result.Error != nil {
		return 0, biz.ErrVideoStorage
	}
	return result.RowsAffected, nil
}

// ActiveUploadJobIDs protects queued and claimed jobs from orphan cleanup.
func (r *videoRepo) ActiveUploadJobIDs(ctx context.Context) ([]string, error) {
	var jobIDs []string
	if err := r.data.db.WithContext(ctx).Model(&videoPO{}).
		Where("status = ? AND upload_job_id <> ''", string(biz.VideoStatusProcessing)).
		Pluck("upload_job_id", &jobIDs).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	return jobIDs, nil
}

func (r *videoRepo) queueVideoUpload(ctx context.Context, record *videoPO, jobID string, uploadBytes int64) error {
	return r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner userPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, record.OwnerID).Error; err != nil {
			return err
		}
		if owner.Role != "admin" {
			var usedBytes int64
			if err := tx.Model(&videoPO{}).
				Where("owner_id = ? AND id <> ? AND status IN ? AND deleted_at IS NULL", record.OwnerID, record.ID, quotaStatuses).
				Select("COALESCE(SUM(upload_bytes), 0)").Scan(&usedBytes).Error; err != nil {
				return err
			}
			if usedBytes+uploadBytes > r.maxUserStorageBytes {
				return biz.ErrVideoUploadQuotaExceeded
			}
		}
		result := tx.Model(&videoPO{}).
			Where("id = ? AND status = ?", record.ID, string(biz.VideoStatusProcessing)).
			Updates(map[string]any{"upload_job_id": jobID, "upload_bytes": uploadBytes, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return biz.ErrVideoInvalidState
		}
		return nil
	})
}

func (r *videoRepo) claimVideoUpload(ctx context.Context) (*videoPO, error) {
	var record videoPO
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND upload_job_id <> '' AND processing_started_at IS NULL", string(biz.VideoStatusProcessing)).
			Order("id ASC").First(&record).Error; err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&record).Updates(map[string]any{
			"processing_started_at": &now,
			"processing_attempts":   gorm.Expr("processing_attempts + 1"),
			"updated_at":            now,
		}).Error
	})
	return &record, err
}

func (r *videoRepo) releaseVideoUploadClaim(videoID biz.VideoID) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := r.data.db.WithContext(cleanupCtx).Model(&videoPO{}).
		Where("id = ? AND status = ?", uint64(videoID), string(biz.VideoStatusProcessing)).
		Updates(map[string]any{"processing_started_at": nil, "updated_at": time.Now()})
	if result.Error != nil || result.RowsAffected != 1 {
		return biz.ErrVideoStorage
	}
	return nil
}

func (r *videoRepo) markVideoUploadReady(ctx context.Context, videoID biz.VideoID, metadata *media.Metadata, renditions []media.Rendition, manifestURL, coverURL string) error {
	now := time.Now()
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&videoPO{}).
			Where("id = ? AND status = ?", uint64(videoID), string(biz.VideoStatusProcessing)).
			Updates(map[string]any{
				"status": string(biz.VideoStatusReady), "failure_reason": "",
				"duration_seconds": metadata.DurationSeconds, "cover_url": coverURL,
				"ready_at": &now, "updated_at": now, "upload_job_id": "", "processing_started_at": nil,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return biz.ErrVideoInvalidState
		}
		bandwidth := metadata.Bandwidth
		if len(renditions) > 0 {
			bandwidth = renditions[len(renditions)-1].Bandwidth
		}
		stream := videoStreamPO{
			VideoID: uint64(videoID), StreamKey: "dash-adaptive", Label: "自动",
			Codec: "avc1,mp4a", MimeType: "application/dash+xml", URL: manifestURL,
			Width: metadata.Width, Height: metadata.Height, Bandwidth: bandwidth,
			DefaultStream: true,
		}
		return tx.Create(&stream).Error
	})
	if err != nil {
		if errors.Is(err, biz.ErrVideoInvalidState) {
			return biz.ErrVideoInvalidState
		}
		return biz.ErrVideoStorage
	}
	return nil
}

func (r *videoRepo) failQueuedVideoUpload(videoID biz.VideoID, jobID, reason string) {
	r.markVideoUploadFailed(context.Background(), videoID, reason)
	job, err := r.mediaManager.OpenUploadJob(jobID)
	if err == nil {
		err = r.mediaManager.RemoveUploadJob(job)
	}
	if err != nil {
		log.Error("remove failed transcode job", "job_id", jobID, "error", err)
	}
}

func (r *videoRepo) markVideoUploadFailed(ctx context.Context, videoID biz.VideoID, reason string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := r.data.db.WithContext(cleanupCtx).Model(&videoPO{}).
		Where("id = ? AND status = ?", uint64(videoID), string(biz.VideoStatusProcessing)).
		Updates(map[string]any{
			"status": string(biz.VideoStatusFailed), "failure_reason": reason,
			"upload_job_id": "", "upload_bytes": 0, "processing_started_at": nil, "updated_at": time.Now(),
		}).Error
	if err != nil {
		log.Error("mark video upload failed", "bvid", videoID.BVID(), "error", err)
	}
}
