package data

import (
	"context"
	"errors"
	"time"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type videoEngagementRow struct {
	VideoID       uint64
	Liked         bool
	Favorited     bool
	MyCoinAmount  int32
	LikeCount     int64
	FavoriteCount int64
	CoinCount     int64
	ShareCount    int64
	CoinBalance   int64
}

// PostgreSQL is the source of truth for interaction state and counters. A future
// Redis cache can decorate these semantic repository methods without changing biz.

// FindVideoEngagement loads the viewer's state, counters, and current coin balance.
func (r *videoRepo) FindVideoEngagement(ctx context.Context, userID uint64, videoID biz.VideoID) (*biz.VideoEngagement, error) {
	result, err := loadVideoEngagement(r.data.db.WithContext(ctx), userID, uint64(videoID))
	if err != nil {
		return nil, mapVideoInteractionError(err)
	}
	return result, nil
}

// SetVideoLike atomically applies an idempotent desired state, including undo.
func (r *videoRepo) SetVideoLike(ctx context.Context, userID uint64, videoID biz.VideoID, liked bool) (*biz.VideoLike, error) {
	result := &biz.VideoLike{VideoID: videoID, Liked: liked}
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		var like videoLikePO
		err = tx.Where("user_id = ? AND video_id = ?", userID, video.ID).First(&like).Error
		changed := false
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound) && liked:
			if err := tx.Create(&videoLikePO{UserID: userID, VideoID: video.ID, Active: true}).Error; err != nil {
				return err
			}
			changed = true
		case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		case err == nil && like.Active != liked:
			if err := tx.Model(&like).Update("active", liked).Error; err != nil {
				return err
			}
			changed = true
		}
		if changed {
			delta := -1
			if liked {
				delta = 1
			}
			if err := tx.Model(video).UpdateColumn("like_count", gorm.Expr("GREATEST(like_count + ?, 0)", delta)).Error; err != nil {
				return err
			}
			video.LikeCount += int64(delta)
			if video.LikeCount < 0 {
				video.LikeCount = 0
			}
		}
		result.LikeCount = video.LikeCount
		return nil
	})
	if err != nil {
		return nil, mapVideoInteractionError(err)
	}
	return result, nil
}

// SetVideoFavorite atomically applies an idempotent favorite state.
func (r *videoRepo) SetVideoFavorite(ctx context.Context, userID uint64, videoID biz.VideoID, favorited bool) (*biz.VideoEngagement, error) {
	var result *biz.VideoEngagement
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		var favorite videoFavoritePO
		err = tx.Where("user_id = ? AND video_id = ?", userID, video.ID).First(&favorite).Error
		changed := false
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound) && favorited:
			if err := tx.Create(&videoFavoritePO{UserID: userID, VideoID: video.ID, Active: true}).Error; err != nil {
				return err
			}
			changed = true
		case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		case err == nil && favorite.Active != favorited:
			if err := tx.Model(&favorite).Update("active", favorited).Error; err != nil {
				return err
			}
			changed = true
		}
		if changed {
			delta := -1
			if favorited {
				delta = 1
			}
			if err := tx.Model(video).UpdateColumn("favorite_count", gorm.Expr("GREATEST(favorite_count + ?, 0)", delta)).Error; err != nil {
				return err
			}
		}
		result, err = loadVideoEngagement(tx, userID, video.ID)
		return err
	})
	if err != nil {
		return nil, mapVideoInteractionError(err)
	}
	return result, nil
}

// SetVideoCoinAmount irreversibly raises a viewer's cumulative contribution to one or two coins.
func (r *videoRepo) SetVideoCoinAmount(ctx context.Context, userID uint64, videoID biz.VideoID, targetAmount, experiencePerCoin, dailyExperienceLimit int32) (*biz.VideoEngagement, error) {
	var result *biz.VideoEngagement
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		var coin videoCoinPO
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND video_id = ?", userID, video.ID).
			First(&coin).Error
		currentAmount := int32(0)
		if err == nil {
			currentAmount = coin.Amount
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if targetAmount < currentAmount {
			return biz.ErrVideoCoinLimit
		}

		var user userPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		delta := targetAmount - currentAmount
		if delta > 0 {
			if user.CoinBalance < int64(delta) {
				return biz.ErrVideoInsufficientCoins
			}
			if err := tx.Model(&user).UpdateColumn("coin_balance", gorm.Expr("coin_balance - ?", delta)).Error; err != nil {
				return err
			}
			user.CoinBalance -= int64(delta)
			if currentAmount == 0 {
				if err := tx.Create(&videoCoinPO{UserID: userID, VideoID: video.ID, Amount: targetAmount}).Error; err != nil {
					return err
				}
			} else if err := tx.Model(&coin).Update("amount", targetAmount).Error; err != nil {
				return err
			}
			if err := tx.Model(video).UpdateColumn("coin_count", gorm.Expr("coin_count + ?", delta)).Error; err != nil {
				return err
			}
			if _, err := grantDailyExperience(tx, userID, biz.ExperienceSourceCoin, delta*experiencePerCoin, dailyExperienceLimit, time.Now()); err != nil {
				return err
			}
		}
		result, err = loadVideoEngagement(tx, userID, video.ID)
		return err
	})
	if err != nil {
		return nil, mapVideoInteractionError(err)
	}
	return result, nil
}

// CreateVideoShare records one share event and ignores retries with the same request ID.
func (r *videoRepo) CreateVideoShare(ctx context.Context, userID uint64, videoID biz.VideoID, requestID string, dailyExperience int32) (*biz.VideoShare, error) {
	result := &biz.VideoShare{VideoID: videoID}
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		share := videoSharePO{UserID: userID, VideoID: video.ID, RequestID: requestID}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&share)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected > 0 {
			if err := tx.Model(video).UpdateColumn("share_count", gorm.Expr("share_count + 1")).Error; err != nil {
				return err
			}
			if _, err := grantDailyExperience(tx, userID, biz.ExperienceSourceShare, dailyExperience, dailyExperience, time.Now()); err != nil {
				return err
			}
			video.ShareCount++
		}
		result.ShareCount = video.ShareCount
		return nil
	})
	if err != nil {
		return nil, mapVideoInteractionError(err)
	}
	return result, nil
}

// loadVideoEngagement uses one SQL snapshot instead of several sequential point reads.
func loadVideoEngagement(db *gorm.DB, userID, videoID uint64) (*biz.VideoEngagement, error) {
	var row videoEngagementRow
	result := db.Raw(`
		SELECT v.id AS video_id,
		       v.like_count,
		       v.favorite_count,
		       v.coin_count,
		       v.share_count,
		       u.coin_balance,
		       COALESCE(l.active, FALSE) AS liked,
		       COALESCE(f.active, FALSE) AS favorited,
		       COALESCE(c.amount, 0) AS my_coin_amount
		FROM videos v
		JOIN users u ON u.id = ?
		LEFT JOIN user_video_likes l
		  ON l.video_id = v.id AND l.user_id = u.id
		LEFT JOIN user_video_favorites f
		  ON f.video_id = v.id AND f.user_id = u.id
		LEFT JOIN user_video_coins c
		  ON c.video_id = v.id AND c.user_id = u.id
		WHERE v.id = ? AND v.status = ? AND v.deleted_at IS NULL
	`, userID, videoID, string(biz.VideoStatusPublished)).Scan(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, biz.ErrVideoNotFound
	}
	return &biz.VideoEngagement{
		VideoID: biz.VideoID(row.VideoID), Liked: row.Liked, Favorited: row.Favorited,
		MyCoinAmount: row.MyCoinAmount, LikeCount: row.LikeCount,
		FavoriteCount: row.FavoriteCount, CoinCount: row.CoinCount,
		ShareCount: row.ShareCount, CoinBalance: row.CoinBalance,
	}, nil
}

func mapVideoInteractionError(err error) error {
	switch {
	case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoForbidden),
		errors.Is(err, biz.ErrVideoInsufficientCoins), errors.Is(err, biz.ErrVideoCoinLimit):
		return err
	default:
		return biz.ErrVideoStorage
	}
}
