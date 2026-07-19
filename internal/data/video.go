package data

import (
	"context"
	"errors"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/media"

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

// FindVideoByID loads video details with their owner and maps the persistence model to the domain model.
func (r *videoRepo) FindVideoByID(ctx context.Context, videoID biz.VideoID) (*biz.Video, error) {
	var record videoPO
	err := r.data.db.WithContext(ctx).
		Preload("Owner").
		First(&record, uint64(videoID)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	}
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	return toBizVideo(record), nil
}

// FindVideoPlayByID loads every DASH stream and timed danmaku item needed to initialize the player.
func (r *videoRepo) FindVideoPlayByID(ctx context.Context, videoID biz.VideoID) (*biz.VideoPlay, error) {
	numericVideoID := uint64(videoID)
	var record videoPO
	if err := r.data.db.WithContext(ctx).First(&record, numericVideoID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	} else if err != nil {
		return nil, biz.ErrVideoStorage
	}

	var streams []videoStreamPO
	if err := r.data.db.WithContext(ctx).
		Where("video_id = ? AND mime_type = ?", numericVideoID, "application/dash+xml").
		Order("id ASC").
		Find(&streams).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	var danmakus []danmakuPO
	if err := r.data.db.WithContext(ctx).Where("video_id = ?", numericVideoID).Order("time_seconds ASC").Find(&danmakus).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}

	return toBizVideoPlay(record, streams, danmakus), nil
}

// FindVideoLike loads both the user's active like record and the video's authoritative like count.
func (r *videoRepo) FindVideoLike(ctx context.Context, userID uint64, videoID biz.VideoID) (*biz.VideoLike, error) {
	numericVideoID := uint64(videoID)
	var video videoPO
	if err := r.data.db.WithContext(ctx).First(&video, numericVideoID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	} else if err != nil {
		return nil, biz.ErrVideoStorage
	}
	var like videoLikePO
	err := r.data.db.WithContext(ctx).Where("user_id = ? AND video_id = ?", userID, numericVideoID).First(&like).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoStorage
	}
	return &biz.VideoLike{VideoID: videoID, Liked: err == nil && like.Active, LikeCount: video.LikeCount}, nil
}

// SetVideoLike atomically applies an idempotent like state and recounts active likes under a video row lock.
func (r *videoRepo) SetVideoLike(ctx context.Context, userID uint64, videoID biz.VideoID, liked bool) (*biz.VideoLike, error) {
	numericVideoID := uint64(videoID)
	result := &biz.VideoLike{VideoID: videoID, Liked: liked}
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video videoPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&video, numericVideoID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoNotFound
		}
		if err != nil {
			return err
		}

		var like videoLikePO
		err = tx.Where("user_id = ? AND video_id = ?", userID, numericVideoID).First(&like).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound) && liked:
			if err := tx.Create(&videoLikePO{UserID: userID, VideoID: numericVideoID, Active: true}).Error; err != nil {
				return err
			}
		case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		case err == nil && like.Active != liked:
			if err := tx.Model(&like).Update("active", liked).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&videoLikePO{}).
			Where("video_id = ? AND active = ?", numericVideoID, true).
			Count(&result.LikeCount).Error; err != nil {
			return err
		}
		return tx.Model(&video).Update("like_count", result.LikeCount).Error
	})
	if err != nil {
		if errors.Is(err, biz.ErrVideoNotFound) {
			return nil, biz.ErrVideoNotFound
		}
		return nil, biz.ErrVideoStorage
	}
	return result, nil
}

// toBizVideo converts the GORM persistence model into a storage-independent video domain object.
func toBizVideo(v videoPO) *biz.Video {
	return &biz.Video{
		ID:              biz.VideoID(v.ID),
		Title:           v.Title,
		Description:     v.Description,
		OwnerName:       v.Owner.DisplayName,
		OwnerAvatarURL:  v.Owner.AvatarURL,
		CoverURL:        v.CoverURL,
		DurationSeconds: v.DurationSeconds,
		ViewCount:       v.ViewCount,
		DanmakuCount:    v.DanmakuCount,
		LikeCount:       v.LikeCount,
		CoinCount:       v.CoinCount,
		FavoriteCount:   v.FavoriteCount,
		ShareCount:      v.ShareCount,
		PublishTime:     v.PublishTime,
		Tags:            append([]string(nil), v.Tags...),
	}
}

// toBizVideoPlay combines persisted stream and danmaku records into one playback domain object.
func toBizVideoPlay(video videoPO, streams []videoStreamPO, danmakus []danmakuPO) *biz.VideoPlay {
	out := &biz.VideoPlay{
		VideoID: biz.VideoID(video.ID),
		Streams: make([]biz.VideoStream, 0, len(streams)),
		Danmaku: biz.DanmakuConfig{
			Enabled: true,
			Format:  "inline",
			Items:   make([]biz.DanmakuItem, 0, len(danmakus)),
		},
	}
	for _, stream := range streams {
		out.Streams = append(out.Streams, biz.VideoStream{
			ID:            stream.StreamKey,
			Label:         stream.Label,
			Codec:         stream.Codec,
			MimeType:      stream.MimeType,
			URL:           stream.URL,
			Width:         stream.Width,
			Height:        stream.Height,
			Bandwidth:     stream.Bandwidth,
			DefaultStream: stream.DefaultStream,
		})
	}
	for _, danmaku := range danmakus {
		out.Danmaku.Items = append(out.Danmaku.Items, biz.DanmakuItem{
			TimeSeconds: danmaku.TimeSeconds,
			Text:        danmaku.Text,
			Color:       danmaku.Color,
		})
	}
	return out
}
