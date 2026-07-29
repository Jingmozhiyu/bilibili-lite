package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	minimumQualifiedWatch = 5 * time.Second
	viewCountInterval     = time.Hour
	maxDailyVideoViews    = int64(10)
)

var shanghaiTime = time.FixedZone("Asia/Shanghai", 8*60*60)

// CreateVideoViewSession records when an authenticated viewer starts a published video.
func (r *videoRepo) CreateVideoViewSession(ctx context.Context, userID uint64, videoID biz.VideoID) (*biz.VideoViewSession, error) {
	var exists int64
	err := r.data.db.WithContext(ctx).Model(&videoPO{}).
		Where("id = ? AND status = ? AND deleted_at IS NULL", uint64(videoID), string(biz.VideoStatusPublished)).
		Count(&exists).Error
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	if exists == 0 {
		return nil, biz.ErrVideoNotFound
	}
	sessionID, err := newVideoViewSessionID()
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	now := time.Now()
	err = r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record := videoViewSessionPO{
			ID: sessionID, VideoID: uint64(videoID), UserID: userID,
			StartedAt: now, CreatedAt: now,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		history := videoWatchHistoryPO{
			UserID: userID, VideoID: uint64(videoID),
			WatchedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "video_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"watched_at": now,
				"updated_at": now,
			}),
		}).Create(&history).Error
	})
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	return &biz.VideoViewSession{ID: sessionID, StartedAt: now}, nil
}

// CompleteVideoViewSession qualifies five seconds of watching and enforces hourly and daily limits atomically.
func (r *videoRepo) CompleteVideoViewSession(ctx context.Context, userID uint64, videoID biz.VideoID, sessionID string) (*biz.VideoViewResult, error) {
	var result *biz.VideoViewResult
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var video videoPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND deleted_at IS NULL", string(biz.VideoStatusPublished)).
			First(&video, uint64(videoID)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoNotFound
		}
		if err != nil {
			return err
		}

		var session videoViewSessionPO
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND video_id = ? AND user_id = ?", sessionID, uint64(videoID), userID).
			First(&session).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoNotFound
		}
		if err != nil {
			return err
		}

		now := time.Now()
		if session.CompletedAt == nil && now.Sub(session.StartedAt) < minimumQualifiedWatch {
			return biz.ErrVideoViewTooEarly
		}
		dailyCount, latestCountedAt, err := countedViewStats(tx, userID, uint64(videoID), now)
		if err != nil {
			return err
		}
		if session.CompletedAt != nil {
			result = buildVideoViewResult(session.Counted, video.ViewCount, dailyCount, latestCountedAt, now)
			return nil
		}

		counted := dailyCount < maxDailyVideoViews && (!latestCountedAt.Valid || now.Sub(latestCountedAt.Time) >= viewCountInterval)
		session.CompletedAt = &now
		session.Counted = counted
		if err := tx.Model(&session).Updates(map[string]any{"completed_at": &now, "counted": counted}).Error; err != nil {
			return err
		}
		if counted {
			if err := tx.Model(&video).UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error; err != nil {
				return err
			}
			video.ViewCount++
			dailyCount++
			latestCountedAt = sql.NullTime{Time: now, Valid: true}
		}
		result = buildVideoViewResult(counted, video.ViewCount, dailyCount, latestCountedAt, now)
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoViewTooEarly):
			return nil, err
		default:
			return nil, biz.ErrVideoStorage
		}
	}
	if result.Counted {
		r.syncPublishedVideoToSearch(ctx, videoID)
	}
	return result, nil
}

func countedViewStats(tx *gorm.DB, userID, videoID uint64, now time.Time) (int64, sql.NullTime, error) {
	localNow := now.In(shanghaiTime)
	dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, shanghaiTime)
	dayEnd := dayStart.Add(24 * time.Hour)
	query := tx.Model(&videoViewSessionPO{}).
		Where("user_id = ? AND video_id = ? AND counted = ?", userID, videoID, true)
	var dailyCount int64
	if err := query.Where("completed_at >= ? AND completed_at < ?", dayStart, dayEnd).Count(&dailyCount).Error; err != nil {
		return 0, sql.NullTime{}, err
	}
	var latest sql.NullTime
	if err := tx.Model(&videoViewSessionPO{}).
		Select("MAX(completed_at)").
		Where("user_id = ? AND video_id = ? AND counted = ?", userID, videoID, true).
		Scan(&latest).Error; err != nil {
		return 0, sql.NullTime{}, err
	}
	return dailyCount, latest, nil
}

func buildVideoViewResult(counted bool, viewCount, dailyCount int64, latest sql.NullTime, now time.Time) *biz.VideoViewResult {
	remaining := maxDailyVideoViews - dailyCount
	if remaining < 0 {
		remaining = 0
	}
	nextEligible := now
	if latest.Valid && latest.Time.Add(viewCountInterval).After(nextEligible) {
		nextEligible = latest.Time.Add(viewCountInterval)
	}
	if dailyCount >= maxDailyVideoViews {
		localNow := now.In(shanghaiTime)
		nextDay := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, shanghaiTime)
		if nextDay.After(nextEligible) {
			nextEligible = nextDay
		}
	}
	return &biz.VideoViewResult{
		Counted: counted, ViewCount: viewCount,
		RemainingToday: int32(remaining), NextEligibleAt: nextEligible,
	}
}

func newVideoViewSessionID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
