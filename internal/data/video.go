package data

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/conf"
	"bilibili-lite/internal/media"

	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type videoRepo struct {
	data                *Data
	mediaManager        *media.Manager
	maxUserStorageBytes int64
}

// NewVideoRepo creates a PostgreSQL-backed VideoRepo.
func NewVideoRepo(data *Data, mediaManager *media.Manager, dataConfig *conf.Data) biz.VideoRepo {
	return &videoRepo{data: data, mediaManager: mediaManager, maxUserStorageBytes: dataConfig.GetMedia().GetMaxUserStorageBytes()}
}

// ListVideos returns a keyset-paginated page of published videos, optionally filtered by owner.
func (r *videoRepo) ListVideos(ctx context.Context, options biz.VideoListOptions) (*biz.VideoList, error) {
	cursor, err := decodeVideoPageToken(options.PageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	query := r.data.db.WithContext(ctx).
		Preload("Owner").
		Where("status = ? AND deleted_at IS NULL", string(biz.VideoStatusPublished))
	if options.OwnerID != 0 {
		query = query.Where("owner_id = ?", options.OwnerID)
	}
	if cursor != 0 {
		query = query.Where("id < ?", cursor)
	}
	var records []videoPO
	if err := query.Order("id DESC").Limit(options.PageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	result := &biz.VideoList{Videos: make([]biz.Video, 0, min(len(records), options.PageSize))}
	if len(records) > options.PageSize {
		records = records[:options.PageSize]
		result.NextPageToken = encodeVideoPageToken(records[len(records)-1].ID)
	}
	for _, record := range records {
		result.Videos = append(result.Videos, *toBizVideo(record))
	}
	return result, nil
}

// ListPendingReviewVideos returns the oldest submitted videos first for deterministic moderation.
func (r *videoRepo) ListPendingReviewVideos(ctx context.Context, pageSize int, pageToken string) (*biz.VideoList, error) {
	cursor, err := decodeVideoPageToken(pageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	query := r.data.db.WithContext(ctx).Preload("Owner").
		Where("status = ? AND deleted_at IS NULL", string(biz.VideoStatusPendingReview))
	if cursor != 0 {
		query = query.Where("id > ?", cursor)
	}
	var records []videoPO
	if err := query.Order("id ASC").Limit(pageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	result := &biz.VideoList{Videos: make([]biz.Video, 0, min(len(records), pageSize))}
	if len(records) > pageSize {
		records = records[:pageSize]
		result.NextPageToken = encodeVideoPageToken(records[len(records)-1].ID)
	}
	for _, record := range records {
		result.Videos = append(result.Videos, *toBizVideo(record))
	}
	return result, nil
}

// ListAdminVideos returns newest-first records in one lifecycle state, including deleted audit rows.
func (r *videoRepo) ListAdminVideos(ctx context.Context, options biz.AdminVideoListOptions) (*biz.VideoList, error) {
	cursor, err := decodeVideoPageToken(options.PageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	query := r.data.db.WithContext(ctx).Preload("Owner").Where("status = ?", string(options.Status))
	if options.Status == biz.VideoStatusDeleted {
		query = query.Where("deleted_at IS NOT NULL")
	} else {
		query = query.Where("deleted_at IS NULL")
	}
	if cursor != 0 {
		query = query.Where("id < ?", cursor)
	}
	var records []videoPO
	if err := query.Order("id DESC").Limit(options.PageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	result := &biz.VideoList{Videos: make([]biz.Video, 0, min(len(records), options.PageSize))}
	if len(records) > options.PageSize {
		records = records[:options.PageSize]
		result.NextPageToken = encodeVideoPageToken(records[len(records)-1].ID)
	}
	for _, record := range records {
		result.Videos = append(result.Videos, *toBizVideo(record))
	}
	return result, nil
}

// FindVideoByID loads a published video detail with its owner.
func (r *videoRepo) FindVideoByID(ctx context.Context, videoID biz.VideoID) (*biz.Video, error) {
	record, err := findPublishedVideoPO(r.data.db.WithContext(ctx).Preload("Owner"), uint64(videoID), false)
	if err != nil {
		return nil, mapVideoReadError(err)
	}
	return toBizVideo(*record), nil
}

// FindAdminVideoByID loads pending or rejected detail for an administrator preview.
func (r *videoRepo) FindAdminVideoByID(ctx context.Context, videoID biz.VideoID) (*biz.Video, error) {
	var record videoPO
	err := r.data.db.WithContext(ctx).Preload("Owner").
		Where("id = ? AND status IN ? AND deleted_at IS NULL", uint64(videoID), adminPreviewStatuses()).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	}
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	return toBizVideo(record), nil
}

// FindVideoPlayByID loads DASH streams and timed danmaku for a published video.
func (r *videoRepo) FindVideoPlayByID(ctx context.Context, videoID biz.VideoID) (*biz.VideoPlay, error) {
	numericVideoID := uint64(videoID)
	record, err := findPublishedVideoPO(r.data.db.WithContext(ctx), numericVideoID, false)
	if err != nil {
		return nil, mapVideoReadError(err)
	}

	var streams []videoStreamPO
	var danmakus []danmakuPO
	// These read-only aggregates are independent and can use separate pool connections.
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return r.data.db.WithContext(groupCtx).
			Where("video_id = ? AND mime_type = ?", numericVideoID, "application/dash+xml").
			Order("id ASC").Find(&streams).Error
	})
	group.Go(func() error {
		return r.data.db.WithContext(groupCtx).
			Preload("User").
			Where("video_id = ?", numericVideoID).
			Order("time_seconds ASC").Find(&danmakus).Error
	})
	if err := group.Wait(); err != nil {
		return nil, biz.ErrVideoStorage
	}
	return toBizVideoPlay(*record, streams, danmakus), nil
}

// FindReviewVideoPlayByID loads media for a pending or rejected video visible only to administrators.
func (r *videoRepo) FindReviewVideoPlayByID(ctx context.Context, videoID biz.VideoID) (*biz.VideoPlay, error) {
	numericVideoID := uint64(videoID)
	var record videoPO
	err := r.data.db.WithContext(ctx).
		Where("id = ? AND status IN ? AND deleted_at IS NULL", numericVideoID, adminPreviewStatuses()).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	}
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	var streams []videoStreamPO
	if err := r.data.db.WithContext(ctx).
		Where("video_id = ? AND mime_type = ?", numericVideoID, "application/dash+xml").
		Order("id ASC").Find(&streams).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	return toBizVideoPlay(record, streams, nil), nil
}

func adminPreviewStatuses() []string {
	return []string{string(biz.VideoStatusPendingReview), string(biz.VideoStatusRejected)}
}

// FindVideoUploadStatus returns processing details only when the requester owns the video.
func (r *videoRepo) FindVideoUploadStatus(ctx context.Context, userID uint64, videoID biz.VideoID) (*biz.VideoUploadStatus, error) {
	var record videoPO
	err := r.data.db.WithContext(ctx).First(&record, uint64(videoID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	}
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	if record.OwnerID != userID {
		return nil, biz.ErrVideoForbidden
	}
	var stream videoStreamPO
	if record.Status == string(biz.VideoStatusReady) ||
		record.Status == string(biz.VideoStatusPendingReview) ||
		record.Status == string(biz.VideoStatusPublished) ||
		record.Status == string(biz.VideoStatusRejected) {
		err = r.data.db.WithContext(ctx).
			Where("video_id = ? AND mime_type = ?", record.ID, "application/dash+xml").
			Order("id ASC").First(&stream).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, biz.ErrVideoStorage
		}
	}
	coverURL := record.CoverURL
	if record.Status == string(biz.VideoStatusDeleted) {
		coverURL = ""
	}
	return &biz.VideoUploadStatus{
		VideoID: biz.VideoID(record.ID), Status: biz.VideoStatus(record.Status),
		FailureReason: record.FailureReason, ManifestURL: stream.URL, CoverURL: coverURL,
	}, nil
}

// SubmitVideoForReview stores final metadata immediately and enters moderation
// now or after an in-flight transcode becomes ready.
func (r *videoRepo) SubmitVideoForReview(ctx context.Context, input *biz.VideoReviewSubmission) (*biz.Video, error) {
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record videoPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, uint64(input.VideoID)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoNotFound
		}
		if err != nil {
			return err
		}
		if record.OwnerID != input.OwnerID {
			return biz.ErrVideoForbidden
		}
		if record.Status == string(biz.VideoStatusPendingReview) {
			return nil
		}
		if !canSubmitVideoStatus(biz.VideoStatus(record.Status)) {
			return biz.ErrVideoInvalidState
		}
		now := time.Now()
		record.Title = input.Title
		record.Description = input.Description
		record.Tags = append([]string(nil), input.Tags...)
		if record.Status != string(biz.VideoStatusProcessing) {
			record.Status = string(biz.VideoStatusPendingReview)
		}
		record.ReviewReason = ""
		record.SubmittedAt = &now
		record.ReviewedAt = nil
		record.ReviewedBy = nil
		record.PublishTime = nil
		record.UpdatedAt = now
		return tx.Select(
			"Title", "Description", "Tags", "Status", "ReviewReason", "SubmittedAt",
			"ReviewedAt", "ReviewedBy", "PublishTime", "UpdatedAt",
		).Updates(&record).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoForbidden), errors.Is(err, biz.ErrVideoInvalidState):
			return nil, err
		default:
			return nil, biz.ErrVideoStorage
		}
	}
	return r.findVideoByID(ctx, input.VideoID)
}

func canSubmitVideoStatus(status biz.VideoStatus) bool {
	return status == biz.VideoStatusProcessing || status == biz.VideoStatusReady || status == biz.VideoStatusRejected
}

// ApproveVideo atomically publishes one pending submission.
func (r *videoRepo) ApproveVideo(ctx context.Context, decision biz.VideoReviewDecision) (*biz.Video, error) {
	return r.reviewVideo(ctx, decision, biz.VideoStatusPendingReview, biz.VideoStatusPublished, "")
}

// RejectVideo returns one pending submission to its owner.
func (r *videoRepo) RejectVideo(ctx context.Context, decision biz.VideoReviewDecision) (*biz.Video, error) {
	return r.reviewVideo(ctx, decision, biz.VideoStatusPendingReview, biz.VideoStatusRejected, decision.Reason)
}

// TakeDownVideo transitions a published video to rejected without deleting its media or BV record.
func (r *videoRepo) TakeDownVideo(ctx context.Context, decision biz.VideoReviewDecision) (*biz.Video, error) {
	return r.reviewVideo(ctx, decision, biz.VideoStatusPublished, biz.VideoStatusRejected, decision.Reason)
}

// DeleteAdminVideo removes playable media for any settled state and retains the BV audit row.
func (r *videoRepo) DeleteAdminVideo(ctx context.Context, decision biz.VideoReviewDecision) error {
	return r.deleteVideo(ctx, decision.VideoID, func(record *videoPO) error {
		if !biz.VideoStatus(record.Status).AllowsAdminDeletion() {
			return biz.ErrVideoInvalidState
		}
		return nil
	}, &decision)
}

// DeleteVideo preserves the BV record as deleted and removes any publicly served media.
func (r *videoRepo) DeleteVideo(ctx context.Context, userID uint64, videoID biz.VideoID) error {
	return r.deleteVideo(ctx, videoID, func(record *videoPO) error {
		if record.OwnerID != userID {
			return biz.ErrVideoForbidden
		}
		return nil
	}, nil)
}

func (r *videoRepo) deleteVideo(
	ctx context.Context,
	videoID biz.VideoID,
	authorize func(*videoPO) error,
	decision *biz.VideoReviewDecision,
) error {
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record videoPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, uint64(videoID)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoNotFound
		}
		if err != nil {
			return err
		}
		if err := authorize(&record); err != nil {
			return err
		}
		// Remove local media while the row lock is held so a failed cleanup leaves
		// the record visible and retryable instead of hiding orphaned files.
		if err := r.mediaManager.RemoveVideo(videoID.BVID()); err != nil {
			return err
		}
		now := time.Now()
		updates := map[string]any{
			"status": string(biz.VideoStatusDeleted), "deleted_at": &now,
			"cover_url": "", "updated_at": now,
		}
		if decision != nil {
			updates["review_reason"] = decision.Reason
			updates["reviewed_at"] = &now
			updates["reviewed_by"] = decision.AdminID
		}
		if err := tx.Model(&record).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("video_id = ?", record.ID).Delete(&videoStreamPO{}).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoForbidden), errors.Is(err, biz.ErrVideoInvalidState):
			return err
		default:
			return biz.ErrVideoStorage
		}
	}
	return nil
}

func (r *videoRepo) reviewVideo(
	ctx context.Context,
	decision biz.VideoReviewDecision,
	fromStatus biz.VideoStatus,
	toStatus biz.VideoStatus,
	reason string,
) (*biz.Video, error) {
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record videoPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, uint64(decision.VideoID)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoNotFound
		}
		if err != nil {
			return err
		}
		if record.Status != string(fromStatus) || record.DeletedAt != nil {
			return biz.ErrVideoInvalidState
		}
		now := time.Now()
		updates := map[string]any{
			"status":        string(toStatus),
			"review_reason": reason,
			"reviewed_at":   &now,
			"reviewed_by":   decision.AdminID,
			"updated_at":    now,
		}
		if toStatus == biz.VideoStatusPublished {
			updates["publish_time"] = &now
		}
		return tx.Model(&record).Updates(updates).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoInvalidState):
			return nil, err
		default:
			return nil, biz.ErrVideoStorage
		}
	}
	return r.findVideoByID(ctx, decision.VideoID)
}

func (r *videoRepo) findVideoByID(ctx context.Context, videoID biz.VideoID) (*biz.Video, error) {
	var record videoPO
	err := r.data.db.WithContext(ctx).Preload("Owner").First(&record, uint64(videoID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	}
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	return toBizVideo(record), nil
}

// findPublishedVideoPO centralizes public visibility checks and optional row locking.
func findPublishedVideoPO(db *gorm.DB, videoID uint64, lock bool) (*videoPO, error) {
	query := db.Where("status = ? AND deleted_at IS NULL", string(biz.VideoStatusPublished))
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var video videoPO
	err := query.First(&video, videoID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	}
	if err != nil {
		return nil, err
	}
	return &video, nil
}

func mapVideoReadError(err error) error {
	if errors.Is(err, biz.ErrVideoNotFound) {
		return biz.ErrVideoNotFound
	}
	return biz.ErrVideoStorage
}

func toBizVideo(v videoPO) *biz.Video {
	out := &biz.Video{
		ID: biz.VideoID(v.ID), OwnerID: v.OwnerID,
		Title: v.Title, Description: v.Description,
		OwnerName: v.Owner.DisplayName, OwnerAvatarURL: v.Owner.AvatarURL,
		CoverURL: v.CoverURL, Status: biz.VideoStatus(v.Status),
		DurationSeconds: v.DurationSeconds, ViewCount: v.ViewCount,
		DanmakuCount: v.DanmakuCount, LikeCount: v.LikeCount,
		CoinCount: v.CoinCount, FavoriteCount: v.FavoriteCount, ShareCount: v.ShareCount,
		CommentCount: v.CommentCount,
		ReviewReason: v.ReviewReason,
		CreatedAt:    v.CreatedAt, UpdatedAt: v.UpdatedAt,
		Tags: append([]string(nil), v.Tags...),
	}
	if v.PublishTime != nil {
		out.PublishTime = *v.PublishTime
	}
	if v.SubmittedAt != nil {
		out.SubmittedAt = *v.SubmittedAt
	}
	if v.ReviewedAt != nil {
		out.ReviewedAt = *v.ReviewedAt
	}
	return out
}

func toBizVideoPlay(video videoPO, streams []videoStreamPO, danmakus []danmakuPO) *biz.VideoPlay {
	out := &biz.VideoPlay{
		VideoID: biz.VideoID(video.ID),
		Streams: make([]biz.VideoStream, 0, len(streams)),
		Danmaku: biz.DanmakuConfig{Enabled: true, Format: "inline", Items: make([]biz.DanmakuItem, 0, len(danmakus))},
	}
	for _, stream := range streams {
		out.Streams = append(out.Streams, biz.VideoStream{
			ID: stream.StreamKey, Label: stream.Label, Codec: stream.Codec,
			MimeType: stream.MimeType, URL: stream.URL,
			Width: stream.Width, Height: stream.Height, Bandwidth: stream.Bandwidth,
			DefaultStream: stream.DefaultStream,
		})
	}
	for _, danmaku := range danmakus {
		out.Danmaku.Items = append(out.Danmaku.Items, *toBizDanmaku(danmaku, danmaku.User))
	}
	return out
}

func encodeVideoPageToken(videoID uint64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatUint(videoID, 10)))
}

func decodeVideoPageToken(token string) (uint64, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, err
	}
	videoID, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil || videoID == 0 {
		return 0, biz.ErrVideoInvalidArgument
	}
	return videoID, nil
}
