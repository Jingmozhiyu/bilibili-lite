package data

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
)

type videoHistoryCursor struct {
	UpdatedAt time.Time
	ID        uint64
}

// ListVideoHistory returns active likes, active favorites, or irreversible coin records newest first.
func (r *videoRepo) ListVideoHistory(ctx context.Context, userID uint64, kind biz.VideoHistoryKind, pageSize int, pageToken string) (*biz.VideoHistoryList, error) {
	cursor, err := decodeVideoHistoryToken(pageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	switch kind {
	case biz.VideoHistoryLiked:
		return r.listLikedVideoHistory(ctx, userID, pageSize, cursor)
	case biz.VideoHistoryFavorited:
		return r.listFavoriteVideoHistory(ctx, userID, pageSize, cursor)
	case biz.VideoHistoryCoined:
		return r.listCoinedVideoHistory(ctx, userID, pageSize, cursor)
	case biz.VideoHistoryWatched:
		return r.listWatchedVideoHistory(ctx, userID, pageSize, cursor)
	default:
		return nil, biz.ErrVideoInvalidArgument
	}
}

func (r *videoRepo) listWatchedVideoHistory(ctx context.Context, userID uint64, pageSize int, cursor videoHistoryCursor) (*biz.VideoHistoryList, error) {
	var records []videoWatchHistoryPO
	query := r.data.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("EXISTS (SELECT 1 FROM videos WHERE videos.id = user_video_watch_history.video_id AND videos.status = ? AND videos.deleted_at IS NULL)", string(biz.VideoStatusPublished)).
		Preload("Video.Owner").
		Order("watched_at DESC, id DESC")
	if cursor.ID != 0 {
		query = query.Where("(watched_at < ?) OR (watched_at = ? AND id < ?)", cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
	}
	if err := query.Limit(pageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	items := make([]biz.VideoHistoryItem, 0, min(len(records), pageSize))
	for index := 0; index < len(records) && index < pageSize; index++ {
		items = append(items, biz.VideoHistoryItem{Video: *toBizVideo(records[index].Video), InteractedAt: records[index].WatchedAt})
	}
	return buildVideoHistoryList(items, len(records) > pageSize, historyCursorFromWatched(records, pageSize)), nil
}

func (r *videoRepo) listLikedVideoHistory(ctx context.Context, userID uint64, pageSize int, cursor videoHistoryCursor) (*biz.VideoHistoryList, error) {
	var records []videoLikePO
	query := interactionHistoryQuery(r.data.db.WithContext(ctx), "user_video_likes", userID, cursor).
		Where("user_video_likes.active = ?", true).
		Preload("Video.Owner")
	if err := query.Limit(pageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	items := make([]biz.VideoHistoryItem, 0, min(len(records), pageSize))
	for index := 0; index < len(records) && index < pageSize; index++ {
		items = append(items, biz.VideoHistoryItem{Video: *toBizVideo(records[index].Video), InteractedAt: records[index].UpdatedAt})
	}
	return buildVideoHistoryList(items, len(records) > pageSize, historyCursorFromLikes(records, pageSize)), nil
}

func (r *videoRepo) listFavoriteVideoHistory(ctx context.Context, userID uint64, pageSize int, cursor videoHistoryCursor) (*biz.VideoHistoryList, error) {
	var records []videoFavoritePO
	query := interactionHistoryQuery(r.data.db.WithContext(ctx), "user_video_favorites", userID, cursor).
		Where("user_video_favorites.active = ?", true).
		Preload("Video.Owner")
	if err := query.Limit(pageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	items := make([]biz.VideoHistoryItem, 0, min(len(records), pageSize))
	for index := 0; index < len(records) && index < pageSize; index++ {
		items = append(items, biz.VideoHistoryItem{Video: *toBizVideo(records[index].Video), InteractedAt: records[index].UpdatedAt})
	}
	return buildVideoHistoryList(items, len(records) > pageSize, historyCursorFromFavorites(records, pageSize)), nil
}

func (r *videoRepo) listCoinedVideoHistory(ctx context.Context, userID uint64, pageSize int, cursor videoHistoryCursor) (*biz.VideoHistoryList, error) {
	var records []videoCoinPO
	query := interactionHistoryQuery(r.data.db.WithContext(ctx), "user_video_coins", userID, cursor).
		Preload("Video.Owner")
	if err := query.Limit(pageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	items := make([]biz.VideoHistoryItem, 0, min(len(records), pageSize))
	for index := 0; index < len(records) && index < pageSize; index++ {
		items = append(items, biz.VideoHistoryItem{
			Video: *toBizVideo(records[index].Video), InteractedAt: records[index].UpdatedAt, CoinAmount: records[index].Amount,
		})
	}
	return buildVideoHistoryList(items, len(records) > pageSize, historyCursorFromCoins(records, pageSize)), nil
}

func interactionHistoryQuery(db *gorm.DB, table string, userID uint64, cursor videoHistoryCursor) *gorm.DB {
	query := db.Table(table).
		Where(table+".user_id = ?", userID).
		Where("EXISTS (SELECT 1 FROM videos WHERE videos.id = "+table+".video_id AND videos.status = ? AND videos.deleted_at IS NULL)", string(biz.VideoStatusPublished)).
		Order(table + ".updated_at DESC, " + table + ".id DESC")
	if cursor.ID != 0 {
		query = query.Where("("+table+".updated_at < ?) OR ("+table+".updated_at = ? AND "+table+".id < ?)", cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
	}
	return query
}

func buildVideoHistoryList(items []biz.VideoHistoryItem, hasNext bool, cursor videoHistoryCursor) *biz.VideoHistoryList {
	result := &biz.VideoHistoryList{Items: items}
	if hasNext && cursor.ID != 0 {
		result.NextPageToken = encodeVideoHistoryToken(cursor)
	}
	return result
}

func historyCursorFromLikes(records []videoLikePO, pageSize int) videoHistoryCursor {
	if len(records) <= pageSize || pageSize == 0 {
		return videoHistoryCursor{}
	}
	record := records[pageSize-1]
	return videoHistoryCursor{UpdatedAt: record.UpdatedAt, ID: record.ID}
}

func historyCursorFromFavorites(records []videoFavoritePO, pageSize int) videoHistoryCursor {
	if len(records) <= pageSize || pageSize == 0 {
		return videoHistoryCursor{}
	}
	record := records[pageSize-1]
	return videoHistoryCursor{UpdatedAt: record.UpdatedAt, ID: record.ID}
}

func historyCursorFromCoins(records []videoCoinPO, pageSize int) videoHistoryCursor {
	if len(records) <= pageSize || pageSize == 0 {
		return videoHistoryCursor{}
	}
	record := records[pageSize-1]
	return videoHistoryCursor{UpdatedAt: record.UpdatedAt, ID: record.ID}
}

func historyCursorFromWatched(records []videoWatchHistoryPO, pageSize int) videoHistoryCursor {
	if len(records) <= pageSize || pageSize == 0 {
		return videoHistoryCursor{}
	}
	record := records[pageSize-1]
	return videoHistoryCursor{UpdatedAt: record.WatchedAt, ID: record.ID}
}

func encodeVideoHistoryToken(cursor videoHistoryCursor) string {
	raw := fmt.Sprintf("%d:%d", cursor.UpdatedAt.UnixNano(), cursor.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeVideoHistoryToken(token string) (videoHistoryCursor, error) {
	if token == "" {
		return videoHistoryCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return videoHistoryCursor{}, err
	}
	timestamp, idText, ok := strings.Cut(string(raw), ":")
	if !ok {
		return videoHistoryCursor{}, fmt.Errorf("invalid history token")
	}
	nanoseconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return videoHistoryCursor{}, err
	}
	id, err := strconv.ParseUint(idText, 10, 64)
	if err != nil || id == 0 {
		return videoHistoryCursor{}, fmt.Errorf("invalid history id")
	}
	return videoHistoryCursor{UpdatedAt: time.Unix(0, nanoseconds), ID: id}, nil
}
