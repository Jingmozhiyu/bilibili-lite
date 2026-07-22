package data

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/media"

	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type videoRepo struct {
	data         *Data
	mediaManager *media.Manager
}

// NewVideoRepo creates a PostgreSQL-backed VideoRepo.
func NewVideoRepo(data *Data, mediaManager *media.Manager) biz.VideoRepo {
	return &videoRepo{data: data, mediaManager: mediaManager}
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

// FindVideoByID loads a published video detail with its owner.
func (r *videoRepo) FindVideoByID(ctx context.Context, videoID biz.VideoID) (*biz.Video, error) {
	record, err := findPublishedVideoPO(r.data.db.WithContext(ctx).Preload("Owner"), uint64(videoID), false)
	if err != nil {
		return nil, mapVideoReadError(err)
	}
	return toBizVideo(*record), nil
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
	if record.Status == string(biz.VideoStatusReady) || record.Status == string(biz.VideoStatusPublished) {
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

// PublishVideo atomically attaches final metadata and transitions a ready draft to published.
func (r *videoRepo) PublishVideo(ctx context.Context, input *biz.VideoPublishInput) (*biz.Video, error) {
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
		if record.Status == string(biz.VideoStatusPublished) {
			return nil
		}
		if record.Status != string(biz.VideoStatusReady) {
			return biz.ErrVideoInvalidState
		}
		now := time.Now()
		record.Title = input.Title
		record.Description = input.Description
		record.Tags = append([]string(nil), input.Tags...)
		record.Status = string(biz.VideoStatusPublished)
		record.FailureReason = ""
		record.PublishTime = &now
		record.UpdatedAt = now
		return tx.Select("Title", "Description", "Tags", "Status", "FailureReason", "PublishTime", "UpdatedAt").Updates(&record).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoForbidden), errors.Is(err, biz.ErrVideoInvalidState):
			return nil, err
		default:
			return nil, biz.ErrVideoStorage
		}
	}
	return r.FindVideoByID(ctx, input.VideoID)
}

// DeleteVideo preserves the BV record as deleted and removes any publicly served media.
func (r *videoRepo) DeleteVideo(ctx context.Context, userID uint64, videoID biz.VideoID) error {
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record videoPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, uint64(videoID)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoNotFound
		}
		if err != nil {
			return err
		}
		if record.OwnerID != userID {
			return biz.ErrVideoForbidden
		}
		now := time.Now()
		if err := tx.Model(&record).Updates(map[string]any{
			"status": string(biz.VideoStatusDeleted), "deleted_at": &now,
			"cover_url": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Where("video_id = ?", record.ID).Delete(&videoStreamPO{}).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoForbidden):
			return err
		default:
			return biz.ErrVideoStorage
		}
	}
	if err := r.mediaManager.RemoveVideo(videoID.BVID()); err != nil {
		return biz.ErrVideoStorage
	}
	return nil
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
		CreatedAt:    v.CreatedAt, UpdatedAt: v.UpdatedAt,
		Tags: append([]string(nil), v.Tags...),
	}
	if v.PublishTime != nil {
		out.PublishTime = *v.PublishTime
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
