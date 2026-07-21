package data

import (
	"context"
	"errors"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/media"

	"github.com/go-kratos/kratos/v3/log"
	"gorm.io/gorm"
)

// ProcessVideoUpload allocates a BV record first, then prepares its media as a ready draft.
func (r *videoRepo) ProcessVideoUpload(ctx context.Context, input *biz.VideoUploadInput) (*biz.VideoUploadResult, error) {
	record := videoPO{
		OwnerID: input.OwnerID, Title: "", Description: "",
		Status: string(biz.VideoStatusProcessing), Tags: []string{},
	}
	if err := r.data.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	videoID := biz.VideoID(record.ID)
	fail := func(reason string, publicError error) (*biz.VideoUploadResult, error) {
		r.markVideoUploadFailed(ctx, videoID, reason)
		return nil, publicError
	}

	job, err := r.mediaManager.CreateUploadJob()
	if err != nil {
		return fail("storage initialization failed", biz.ErrVideoStorage)
	}
	if err := r.mediaManager.StoreUpload(ctx, job, input.Content); err != nil {
		if errors.Is(err, media.ErrUploadTooLarge) {
			return fail("video exceeds upload limit", biz.ErrVideoUploadTooLarge)
		}
		return fail("upload interrupted", biz.ErrVideoUploadInterrupted)
	}
	customCover := input.Cover != nil
	if customCover {
		if err := r.mediaManager.StoreCover(ctx, job, input.Cover); err != nil {
			if errors.Is(err, media.ErrCoverTooLarge) {
				return fail("cover exceeds upload limit", biz.ErrVideoUploadTooLarge)
			}
			return fail("cover upload interrupted", biz.ErrVideoUploadInterrupted)
		}
	}

	metadata, err := r.mediaManager.InspectMP4(ctx, job)
	if err != nil {
		return fail("media inspection failed", biz.ErrVideoProcessing)
	}
	if err := r.mediaManager.GenerateCover(ctx, job, metadata, customCover); err != nil {
		return fail("cover generation failed", biz.ErrVideoProcessing)
	}
	if err := r.mediaManager.TranscodeDASH(ctx, job); err != nil {
		return fail("DASH transcoding failed", biz.ErrVideoProcessing)
	}

	manifestURL, publishedDir, err := r.mediaManager.PublishDASH(job, videoID.BVID())
	if err != nil {
		return fail("media publication failed", biz.ErrVideoStorage)
	}
	coverURL := "/media/dash/" + videoID.BVID() + "/cover.jpg"
	if err := r.markVideoUploadReady(ctx, videoID, metadata, manifestURL, coverURL); err != nil {
		if cleanupErr := r.mediaManager.RemovePublished(publishedDir); cleanupErr != nil {
			log.Error("remove rolled-back DASH directory", "path", publishedDir, "error", cleanupErr)
		}
		return fail("media metadata persistence failed", err)
	}
	if err := r.mediaManager.RemoveUploadJob(job); err != nil {
		log.Error("remove completed upload job", "error", err)
	}
	return &biz.VideoUploadResult{
		VideoID: videoID, Status: biz.VideoStatusReady,
		ManifestURL: manifestURL, CoverURL: coverURL,
	}, nil
}

func (r *videoRepo) markVideoUploadReady(ctx context.Context, videoID biz.VideoID, metadata *media.Metadata, manifestURL, coverURL string) error {
	now := time.Now()
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&videoPO{}).
			Where("id = ? AND status = ?", uint64(videoID), string(biz.VideoStatusProcessing)).
			Updates(map[string]any{
				"status": string(biz.VideoStatusReady), "failure_reason": "",
				"duration_seconds": metadata.DurationSeconds, "cover_url": coverURL,
				"ready_at": &now, "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return biz.ErrVideoInvalidState
		}
		stream := videoStreamPO{
			VideoID: uint64(videoID), StreamKey: "dash-main", Label: "DASH",
			Codec: "avc1,mp4a", MimeType: "application/dash+xml", URL: manifestURL,
			Width: metadata.Width, Height: metadata.Height, Bandwidth: metadata.Bandwidth,
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

func (r *videoRepo) markVideoUploadFailed(ctx context.Context, videoID biz.VideoID, reason string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := r.data.db.WithContext(cleanupCtx).Model(&videoPO{}).
		Where("id = ? AND status = ?", uint64(videoID), string(biz.VideoStatusProcessing)).
		Updates(map[string]any{
			"status": string(biz.VideoStatusFailed), "failure_reason": reason, "updated_at": time.Now(),
		}).Error
	if err != nil {
		log.Error("mark video upload failed", "bvid", videoID.BVID(), "error", err)
	}
}
