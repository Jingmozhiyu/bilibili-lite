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
	// Insert the row first so its auto-increment ID (the BVID) exists before any file is written.
	record := videoPO{
		OwnerID: input.OwnerID, Title: "", Description: "",
		Status: string(biz.VideoStatusProcessing), Tags: []string{},
	}
	if err := r.data.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	videoID := biz.VideoID(record.ID)

	// Allocate a private job directory; nothing in it is publicly served until PublishDASH.
	job, err := r.mediaManager.CreateUploadJob()
	if err != nil {
		r.markVideoUploadFailed(ctx, videoID, "storage initialization failed")
		return nil, biz.ErrVideoStorage
	}

	// Every later failure marks the row failed and removes the partial job directory.
	fail := func(reason string, publicError error) (*biz.VideoUploadResult, error) {
		r.markVideoUploadFailed(ctx, videoID, reason)
		if cleanupErr := r.mediaManager.RemoveUploadJob(job); cleanupErr != nil {
			log.Error("remove failed upload job", "job_id", job.ID(), "error", cleanupErr)
		}
		return nil, publicError
	}

	// This is where the HTTP body is actually consumed: each Read pulls the next chunk off the
	// connection and writes it to disk, so receiving and persisting finish together.
	uploadBytes, err := r.mediaManager.StoreUpload(ctx, job, input.Content)
	if err != nil {
		if errors.Is(err, media.ErrUploadTooLarge) {
			return fail("video exceeds upload limit", biz.ErrVideoUploadTooLarge)
		}
		return fail("upload interrupted", biz.ErrVideoUploadInterrupted)
	}

	// The cover was already buffered by the transport, so this reads from memory, not the network.
	if input.Cover != nil {
		if err := r.mediaManager.StoreCover(ctx, job, input.Cover); err != nil {
			if errors.Is(err, media.ErrCoverTooLarge) {
				return fail("cover exceeds upload limit", biz.ErrVideoUploadTooLarge)
			}
			return fail("cover upload interrupted", biz.ErrVideoUploadInterrupted)
		}
	}

	// Check the quota and attach the job ID in one transaction; the job ID is what makes the row claimable.
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
	// An empty queue is not an error; the worker just sleeps until the next poll.
	record, err := r.claimVideoUpload(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, biz.ErrVideoStorage
	}
	videoID := biz.VideoID(record.ID)

	// The job directory can be gone if the janitor or a crash removed it; nothing is left to retry.
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

	// In each stage below, a cancelled ctx means shutdown, not a bad file: release the claim so
	// the job is retried instead of being marked failed.
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

	log.Info("start video transcode", "bvid", videoID.BVID(), "job_id", record.UploadJobID,
		"width", metadata.Width, "height", metadata.Height, "duration_seconds", metadata.DurationSeconds,
		"video_codec", metadata.VideoCodec, "pixel_format", metadata.PixelFormat, "frame_rate", metadata.FrameRate,
		"rotation", metadata.Rotation,
		"audio_codec", metadata.AudioCodec, "audio_sample_rate", metadata.AudioSampleRate,
		"audio_channels", metadata.AudioChannels, "audio_layout", metadata.AudioLayout)
	renditions, err := r.mediaManager.TranscodeDASH(ctx, job, metadata)
	if err != nil {
		if ctx.Err() != nil {
			return true, r.releaseVideoUploadClaim(videoID)
		}
		if errors.Is(err, media.ErrVideoResolutionTooLow) {
			return fail("video resolution is below 720p", err)
		}
		if errors.Is(err, media.ErrTranscodeTimeout) {
			return fail("DASH transcoding timed out", err)
		}
		return fail("DASH transcoding failed", err)
	}

	// Publish files before updating the database, so a ready row never points at missing media.
	manifestURL, publishedDir, err := r.mediaManager.PublishDASH(job, videoID.BVID())
	if err != nil {
		return fail("media publication failed", err)
	}
	coverURL := "/media/dash/" + videoID.BVID() + "/cover.jpg"

	// If the row changed meanwhile (e.g. the owner deleted it), undo the publish.
	if err := r.markVideoUploadReady(ctx, videoID, metadata, renditions, manifestURL, coverURL); err != nil {
		if cleanupErr := r.mediaManager.RemovePublished(publishedDir); cleanupErr != nil {
			log.Error("remove rolled-back DASH directory", "path", publishedDir, "error", cleanupErr)
		}
		return fail("media metadata persistence failed", err)
	}

	// The video is already live; a leftover job directory is only disk waste, so just log it.
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
		// Lock the owner row so concurrent uploads by the same user check the quota one at a time.
		var owner userPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, record.OwnerID).Error; err != nil {
			return err
		}

		// Administrators have no quota; everyone else is limited by the sum of their stored videos.
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

		// Setting upload_job_id enqueues the row; the status guard rejects a video deleted mid-upload.
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
		// SKIP LOCKED lets concurrent workers pick different rows instead of waiting on the same one.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND upload_job_id <> '' AND processing_started_at IS NULL", string(biz.VideoStatusProcessing)).
			Order("id ASC").First(&record).Error; err != nil {
			return err
		}

		// processing_started_at is the claim marker; the row lock is released when this short transaction commits.
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
	// The worker ctx is already cancelled at shutdown, so use a fresh short-lived one.
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
		// Lock the video row so a concurrent SubmitVideoForReview is serialized with this update.
		var record videoPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", uint64(videoID), string(biz.VideoStatusProcessing)).
			First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return biz.ErrVideoInvalidState
			}
			return err
		}

		// Already submitted goes straight to review; otherwise wait for the owner as ready.
		nextStatus := videoStatusAfterTranscode(record.SubmittedAt)
		result := tx.Model(&videoPO{}).
			Where("id = ? AND status = ?", uint64(videoID), string(biz.VideoStatusProcessing)).
			Updates(map[string]any{
				"status": string(nextStatus), "failure_reason": "",
				"duration_seconds": metadata.DurationSeconds, "cover_url": coverURL,
				"ready_at": &now, "updated_at": now, "upload_job_id": "", "processing_started_at": nil,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return biz.ErrVideoInvalidState
		}

		// Advertise the highest rendition's bandwidth for the adaptive DASH stream.
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

func videoStatusAfterTranscode(submittedAt *time.Time) biz.VideoStatus {
	if submittedAt != nil {
		return biz.VideoStatusPendingReview
	}
	return biz.VideoStatusReady
}

func (r *videoRepo) failQueuedVideoUpload(videoID biz.VideoID, jobID, reason string) {
	r.markVideoUploadFailed(context.Background(), videoID, reason)

	// Remove the job only if it can be reopened; either error is logged, never returned.
	job, err := r.mediaManager.OpenUploadJob(jobID)
	if err == nil {
		err = r.mediaManager.RemoveUploadJob(job)
	}
	if err != nil {
		log.Error("remove failed transcode job", "job_id", jobID, "error", err)
	}
}

func (r *videoRepo) markVideoUploadFailed(ctx context.Context, videoID biz.VideoID, reason string) {
	// Detach from the request ctx so the failure is still recorded after the client disconnects.
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
