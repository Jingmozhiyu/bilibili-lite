package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/media"

	"github.com/go-kratos/kratos/v3/log"
	"gorm.io/gorm"
)

// PublishVideoFromMP4 streams an upload into media processing, then persists its video and DASH stream.
func (r *videoRepo) PublishVideoFromMP4(ctx context.Context, input *biz.VideoUploadInput) (*biz.VideoUploadResult, error) {
	job, err := r.mediaManager.CreateUploadJob()
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	if err := r.mediaManager.StoreUpload(ctx, job, input.Content); err != nil {
		if errors.Is(err, media.ErrUploadTooLarge) {
			return nil, biz.ErrVideoUploadTooLarge
		}
		return nil, biz.ErrVideoUploadInterrupted
	}

	metadata, err := r.mediaManager.InspectMP4(ctx, job)
	if err != nil {
		return nil, biz.ErrVideoProcessing
	}
	if err := r.mediaManager.TranscodeDASH(ctx, job); err != nil {
		return nil, biz.ErrVideoProcessing
	}

	result, err := r.persistAndPublishUploadedVideo(ctx, input, metadata, job)
	if err != nil {
		log.Error("publish uploaded video", "error", err)
		return nil, err
	}
	if err := r.mediaManager.RemoveUploadJob(job); err != nil {
		log.Error("remove completed upload job", "error", err)
	}
	return result, nil
}

// persistAndPublishUploadedVideo obtains the auto-increment ID, publishes DASH files, and inserts the stream atomically.
func (r *videoRepo) persistAndPublishUploadedVideo(ctx context.Context, input *biz.VideoUploadInput, metadata *media.Metadata, job *media.UploadJob) (*biz.VideoUploadResult, error) {
	record := videoPO{
		OwnerID: input.OwnerID, Title: input.Title,
		Description: input.Description, DurationSeconds: metadata.DurationSeconds,
		PublishTime: time.Now(), Tags: append([]string(nil), input.Tags...),
	}
	var result *biz.VideoUploadResult
	var publishedDir string
	if err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		videoID := biz.VideoID(record.ID)
		bvid := videoID.BVID()
		manifestURL, finalDir, err := r.mediaManager.PublishDASH(job, bvid)
		if err != nil {
			return fmt.Errorf("publish media: %w", err)
		}
		publishedDir = finalDir
		stream := videoStreamPO{
			VideoID: record.ID, StreamKey: "dash-main", Label: "DASH",
			Codec: "avc1,mp4a", MimeType: "application/dash+xml", URL: manifestURL,
			Width: metadata.Width, Height: metadata.Height, Bandwidth: metadata.Bandwidth, DefaultStream: true,
		}
		result = &biz.VideoUploadResult{VideoID: videoID, ManifestURL: manifestURL}
		return tx.Create(&stream).Error
	}); err != nil {
		if cleanupErr := r.mediaManager.RemovePublished(publishedDir); cleanupErr != nil {
			log.Error("remove rolled-back DASH directory", "path", publishedDir, "error", cleanupErr)
		}
		return nil, biz.ErrVideoStorage
	}
	return result, nil
}
