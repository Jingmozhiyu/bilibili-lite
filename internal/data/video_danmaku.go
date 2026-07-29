package data

import (
	"context"
	"errors"
	"time"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
)

// CreateDanmaku persists a timed comment and increments the video's counter atomically.
func (r *videoRepo) CreateDanmaku(ctx context.Context, userID uint64, videoID biz.VideoID, timeSeconds float64, text, color string) (*biz.DanmakuItem, error) {
	var result *biz.DanmakuItem
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		if timeSeconds > float64(video.DurationSeconds)+1 {
			return biz.ErrVideoInvalidArgument
		}
		now := time.Now()
		record := danmakuPO{VideoID: video.ID, UserID: &userID, TimeSeconds: timeSeconds, Text: text, Color: color, CreatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Model(video).UpdateColumn("danmaku_count", gorm.Expr("danmaku_count + 1")).Error; err != nil {
			return err
		}
		var user userPO
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		result = toBizDanmaku(record, &user)
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoInvalidArgument):
			return nil, err
		default:
			return nil, biz.ErrVideoStorage
		}
	}
	r.syncPublishedVideoToSearch(ctx, videoID)
	return result, nil
}

// DeleteDanmaku allows its author or the video owner to remove it exactly once.
func (r *videoRepo) DeleteDanmaku(ctx context.Context, userID uint64, videoID biz.VideoID, danmakuID uint64) error {
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		var record danmakuPO
		err = tx.Where("id = ? AND video_id = ?", danmakuID, video.ID).First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoDanmakuNotFound
		}
		if err != nil {
			return err
		}
		if video.OwnerID != userID && (record.UserID == nil || *record.UserID != userID) {
			return biz.ErrVideoForbidden
		}
		if err := tx.Delete(&record).Error; err != nil {
			return err
		}
		return tx.Model(video).UpdateColumn("danmaku_count", gorm.Expr("GREATEST(danmaku_count - 1, 0)")).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoDanmakuNotFound), errors.Is(err, biz.ErrVideoForbidden):
			return err
		default:
			return biz.ErrVideoStorage
		}
	}
	r.syncPublishedVideoToSearch(ctx, videoID)
	return nil
}

func toBizDanmaku(record danmakuPO, user *userPO) *biz.DanmakuItem {
	item := &biz.DanmakuItem{
		ID: record.ID, TimeSeconds: record.TimeSeconds, Text: record.Text,
		Color: record.Color, CreatedAt: record.CreatedAt,
	}
	if record.UserID != nil {
		item.UserID = *record.UserID
	}
	if user != nil {
		item.UserName = user.DisplayName
	}
	return item
}
