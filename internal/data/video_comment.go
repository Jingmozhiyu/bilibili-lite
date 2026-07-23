package data

import (
	"context"
	"errors"
	"time"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
)

type videoCommentRow struct {
	ID              uint64
	VideoID         uint64
	UserID          uint64
	UserName        string
	UserAvatarURL   string
	RootID          *uint64
	ParentID        *uint64
	ReplyToUserID   *uint64
	ReplyToUserName string
	Content         string
	LikeCount       int64
	Liked           bool
	ReplyCount      int64
	CreatedAt       time.Time
	DeletedAt       *time.Time
}

// ListVideoComments returns root comments newest first and keeps deleted roots visible while replies remain.
func (r *videoRepo) ListVideoComments(ctx context.Context, viewerID uint64, videoID biz.VideoID, pageSize int, pageToken string) (*biz.VideoCommentList, error) {
	cursor, err := decodeVideoPageToken(pageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	if _, err := findPublishedVideoPO(r.data.db.WithContext(ctx), uint64(videoID), false); err != nil {
		return nil, mapVideoCommentError(err)
	}
	query := `
		SELECT c.id, c.video_id, c.user_id,
		       u.display_name AS user_name, u.avatar_url AS user_avatar_url,
		       c.root_id, c.parent_id, c.reply_to_user_id,
		       COALESCE(reply_user.display_name, '') AS reply_to_user_name,
		       c.content, c.like_count, c.created_at, c.deleted_at,
		       COALESCE(viewer_like.active, FALSE) AS liked,
		       (SELECT COUNT(*) FROM video_comments reply
		          WHERE reply.root_id = c.id AND reply.deleted_at IS NULL) AS reply_count
		FROM video_comments c
		JOIN users u ON u.id = c.user_id
		LEFT JOIN users reply_user ON reply_user.id = c.reply_to_user_id
		LEFT JOIN user_video_comment_likes viewer_like
		  ON viewer_like.comment_id = c.id AND viewer_like.user_id = ?
		WHERE c.video_id = ? AND c.parent_id IS NULL
		  AND (c.deleted_at IS NULL OR EXISTS (
		      SELECT 1 FROM video_comments reply
		      WHERE reply.root_id = c.id AND reply.deleted_at IS NULL
		  ))
	`
	args := []any{viewerID, uint64(videoID)}
	if cursor != 0 {
		query += " AND c.id < ?"
		args = append(args, cursor)
	}
	query += " ORDER BY c.id DESC LIMIT ?"
	args = append(args, pageSize+1)
	return scanVideoCommentPage(r.data.db.WithContext(ctx), query, args, pageSize)
}

// ListVideoCommentReplies returns chronological replies grouped under one root comment.
func (r *videoRepo) ListVideoCommentReplies(ctx context.Context, viewerID uint64, videoID biz.VideoID, rootCommentID uint64, pageSize int, pageToken string) (*biz.VideoCommentList, error) {
	cursor, err := decodeVideoPageToken(pageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	db := r.data.db.WithContext(ctx)
	if _, err := findPublishedVideoPO(db, uint64(videoID), false); err != nil {
		return nil, mapVideoCommentError(err)
	}
	var root videoCommentPO
	if err := db.Where("id = ? AND video_id = ? AND parent_id IS NULL", rootCommentID, uint64(videoID)).First(&root).Error; err != nil {
		return nil, mapVideoCommentError(err)
	}
	query := `
		SELECT c.id, c.video_id, c.user_id,
		       u.display_name AS user_name, u.avatar_url AS user_avatar_url,
		       c.root_id, c.parent_id, c.reply_to_user_id,
		       COALESCE(reply_user.display_name, '') AS reply_to_user_name,
		       c.content, c.like_count, c.created_at, c.deleted_at,
		       COALESCE(viewer_like.active, FALSE) AS liked,
		       0 AS reply_count
		FROM video_comments c
		JOIN users u ON u.id = c.user_id
		LEFT JOIN users reply_user ON reply_user.id = c.reply_to_user_id
		LEFT JOIN user_video_comment_likes viewer_like
		  ON viewer_like.comment_id = c.id AND viewer_like.user_id = ?
		WHERE c.video_id = ? AND c.root_id = ? AND c.deleted_at IS NULL
	`
	args := []any{viewerID, uint64(videoID), rootCommentID}
	if cursor != 0 {
		query += " AND c.id > ?"
		args = append(args, cursor)
	}
	query += " ORDER BY c.id ASC LIMIT ?"
	args = append(args, pageSize+1)
	return scanVideoCommentPage(db, query, args, pageSize)
}

// CreateVideoComment publishes a root comment or a reply while preserving both root and direct-parent relationships.
func (r *videoRepo) CreateVideoComment(ctx context.Context, userID uint64, videoID biz.VideoID, parentCommentID uint64, content string) (*biz.VideoComment, error) {
	var result *biz.VideoComment
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		video, err := findPublishedVideoPO(tx, uint64(videoID), true)
		if err != nil {
			return err
		}
		record := videoCommentPO{VideoID: video.ID, UserID: userID, Content: content}
		if parentCommentID != 0 {
			var parent videoCommentPO
			if err := tx.Where("id = ? AND video_id = ? AND deleted_at IS NULL", parentCommentID, video.ID).First(&parent).Error; err != nil {
				return err
			}
			record.ParentID = &parent.ID
			if parent.ParentID == nil {
				record.RootID = &parent.ID
			} else {
				if parent.RootID == nil {
					return biz.ErrVideoCommentNotFound
				}
				record.RootID = parent.RootID
				record.ReplyToUserID = &parent.UserID
			}
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Model(video).UpdateColumn("comment_count", gorm.Expr("comment_count + 1")).Error; err != nil {
			return err
		}
		if err := tx.Preload("User").Preload("ReplyToUser").First(&record, record.ID).Error; err != nil {
			return err
		}
		result = toBizVideoComment(record, 0)
		return nil
	})
	if err != nil {
		return nil, mapVideoCommentError(err)
	}
	return result, nil
}

// DeleteVideoComment soft-deletes a comment for its author or the video owner without orphaning visible replies.
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

// SetVideoCommentLike atomically applies an idempotent desired state and updates the denormalized count.
func (r *videoRepo) SetVideoCommentLike(ctx context.Context, userID uint64, videoID biz.VideoID, commentID uint64, liked bool) (*biz.VideoCommentInteraction, error) {
	result := &biz.VideoCommentInteraction{CommentID: commentID, Liked: liked}
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := findPublishedVideoPO(tx, uint64(videoID), false); err != nil {
			return err
		}
		var comment videoCommentPO
		if err := tx.Where("id = ? AND video_id = ? AND deleted_at IS NULL", commentID, uint64(videoID)).First(&comment).Error; err != nil {
			return err
		}
		var like videoCommentLikePO
		err := tx.Where("user_id = ? AND comment_id = ?", userID, commentID).First(&like).Error
		changed := false
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound) && liked:
			if err := tx.Create(&videoCommentLikePO{UserID: userID, CommentID: commentID, Active: true}).Error; err != nil {
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
			if err := tx.Model(&comment).UpdateColumn("like_count", gorm.Expr("GREATEST(like_count + ?, 0)", delta)).Error; err != nil {
				return err
			}
			comment.LikeCount = max(comment.LikeCount+int64(delta), 0)
		}
		result.LikeCount = comment.LikeCount
		return nil
	})
	if err != nil {
		return nil, mapVideoCommentError(err)
	}
	return result, nil
}

// ListVideoCommentHistory returns the caller's non-deleted comments with their published videos.
func (r *videoRepo) ListVideoCommentHistory(ctx context.Context, userID uint64, pageSize int, pageToken string) (*biz.VideoCommentHistoryList, error) {
	cursor, err := decodeVideoPageToken(pageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	query := r.data.db.WithContext(ctx).
		Preload("User").Preload("ReplyToUser").Preload("Video.Owner").
		Where("video_comments.user_id = ? AND video_comments.deleted_at IS NULL", userID).
		Where("EXISTS (SELECT 1 FROM videos WHERE videos.id = video_comments.video_id AND videos.status = ? AND videos.deleted_at IS NULL)", string(biz.VideoStatusPublished))
	if cursor != 0 {
		query = query.Where("video_comments.id < ?", cursor)
	}
	var records []videoCommentPO
	if err := query.Order("video_comments.id DESC").Limit(pageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	hasNext := len(records) > pageSize
	if hasNext {
		records = records[:pageSize]
	}
	items := make([]biz.VideoCommentHistoryItem, 0, len(records))
	for index := range records {
		items = append(items, biz.VideoCommentHistoryItem{
			Video:   *toBizVideo(records[index].Video),
			Comment: *toBizVideoComment(records[index], 0),
		})
	}
	result := &biz.VideoCommentHistoryList{Items: items}
	if hasNext && len(records) > 0 {
		result.NextPageToken = encodeVideoPageToken(records[len(records)-1].ID)
	}
	return result, nil
}

func scanVideoCommentPage(db *gorm.DB, query string, args []any, pageSize int) (*biz.VideoCommentList, error) {
	var rows []videoCommentRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	hasNext := len(rows) > pageSize
	if hasNext {
		rows = rows[:pageSize]
	}
	result := &biz.VideoCommentList{Comments: make([]biz.VideoComment, 0, len(rows))}
	for index := range rows {
		result.Comments = append(result.Comments, *rowToBizVideoComment(rows[index]))
	}
	if hasNext && len(rows) > 0 {
		result.NextPageToken = encodeVideoPageToken(rows[len(rows)-1].ID)
	}
	return result, nil
}

func rowToBizVideoComment(row videoCommentRow) *biz.VideoComment {
	comment := &biz.VideoComment{
		ID: row.ID, VideoID: biz.VideoID(row.VideoID), UserID: row.UserID,
		UserName: row.UserName, UserAvatarURL: row.UserAvatarURL,
		Content: row.Content, LikeCount: row.LikeCount, Liked: row.Liked,
		ReplyCount: row.ReplyCount, CreatedAt: row.CreatedAt, Deleted: row.DeletedAt != nil,
	}
	if row.RootID != nil {
		comment.RootID = *row.RootID
	}
	if row.ParentID != nil {
		comment.ParentID = *row.ParentID
	}
	if row.ReplyToUserID != nil {
		comment.ReplyToUserID = *row.ReplyToUserID
		comment.ReplyToUserName = row.ReplyToUserName
	}
	if comment.Deleted {
		comment.Content = ""
	}
	return comment
}

func toBizVideoComment(record videoCommentPO, replyCount int64) *biz.VideoComment {
	row := videoCommentRow{
		ID: record.ID, VideoID: record.VideoID, UserID: record.UserID,
		UserName: record.User.DisplayName, UserAvatarURL: record.User.AvatarURL,
		RootID: record.RootID, ParentID: record.ParentID, ReplyToUserID: record.ReplyToUserID,
		Content: record.Content, LikeCount: record.LikeCount, ReplyCount: replyCount,
		CreatedAt: record.CreatedAt, DeletedAt: record.DeletedAt,
	}
	if record.ReplyToUser != nil {
		row.ReplyToUserName = record.ReplyToUser.DisplayName
	}
	return rowToBizVideoComment(row)
}

func mapVideoCommentError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, biz.ErrVideoNotFound), errors.Is(err, biz.ErrVideoCommentNotFound), errors.Is(err, biz.ErrVideoForbidden):
		return err
	case errors.Is(err, gorm.ErrRecordNotFound):
		return biz.ErrVideoCommentNotFound
	default:
		return biz.ErrVideoStorage
	}
}
