package data

import (
	"context"
	"errors"
	"time"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
)

// ListVideoComments returns published, non-deleted top-level comments newest first.
func (r *videoRepo) ListVideoComments(ctx context.Context, videoID biz.VideoID, pageSize int, pageToken string) (*biz.VideoCommentList, error) {
	cursor, err := decodeVideoPageToken(pageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	if _, err := findPublishedVideoPO(r.data.db.WithContext(ctx), uint64(videoID), false); err != nil {
		return nil, mapVideoCommentError(err)
	}
	query := r.data.db.WithContext(ctx).
		Preload("User").
		Where("video_id = ? AND deleted_at IS NULL", uint64(videoID))
	if cursor != 0 {
		query = query.Where("id < ?", cursor)
	}
	var records []videoCommentPO
	if err := query.Order("id DESC").Limit(pageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	result := &biz.VideoCommentList{Comments: make([]biz.VideoComment, 0, min(len(records), pageSize))}
	if len(records) > pageSize {
		records = records[:pageSize]
		result.NextPageToken = encodeVideoPageToken(records[len(records)-1].ID)
	}
	for _, record := range records {
		result.Comments = append(result.Comments, *toBizVideoComment(record))
	}
	return result, nil
}

// CreateVideoComment publishes one top-level comment and increments its video counter.
func (r *videoRepo) CreateVideoComment(ctx context.Context, userID uint64, videoID biz.VideoID, content string) (*biz.VideoComment, error) {
	var result *biz.VideoComment
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		now := time.Now()
		record := videoCommentPO{VideoID: video.ID, UserID: userID, Content: content, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Model(video).UpdateColumn("comment_count", gorm.Expr("comment_count + 1")).Error; err != nil {
			return err
		}
		if err := tx.Preload("User").First(&record, record.ID).Error; err != nil {
			return err
		}
		result = toBizVideoComment(record)
		return nil
	})
	if err != nil {
		return nil, mapVideoCommentError(err)
	}
	return result, nil
}

// DeleteVideoComment soft-deletes a comment for its author or the video owner.
func (r *videoRepo) DeleteVideoComment(ctx context.Context, userID uint64, videoID biz.VideoID, commentID uint64) error {
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		var record videoCommentPO
		err = tx.Where("id = ? AND video_id = ?", commentID, video.ID).First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrVideoCommentNotFound
		}
		if err != nil {
			return err
		}
		if record.UserID != userID && video.OwnerID != userID {
			return biz.ErrVideoForbidden
		}
		if record.DeletedAt != nil {
			return nil
		}
		now := time.Now()
		if err := tx.Model(&record).Updates(map[string]any{"deleted_at": &now, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(video).UpdateColumn("comment_count", gorm.Expr("GREATEST(comment_count - 1, 0)")).Error
	})
	return mapVideoCommentError(err)
}

func toBizVideoComment(record videoCommentPO) *biz.VideoComment {
	return &biz.VideoComment{
		ID: record.ID, VideoID: biz.VideoID(record.VideoID), UserID: record.UserID,
		UserName: record.User.DisplayName, UserAvatarURL: record.User.AvatarURL,
		Content: record.Content, CreatedAt: record.CreatedAt,
	}
}

func mapVideoCommentError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoCommentNotFound), errors.Is(err, biz.ErrVideoForbidden):
		return err
	default:
		return biz.ErrVideoStorage
	}
}
