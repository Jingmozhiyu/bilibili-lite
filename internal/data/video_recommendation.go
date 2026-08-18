package data

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"bilibili-lite/internal/biz"

	"github.com/redis/go-redis/v9"
)

// ListRecommendedVideos reads the Redis hot ranking and falls back to PostgreSQL scoring.
func (r *videoRepo) ListRecommendedVideos(ctx context.Context, pageSize int, pageToken string) (*biz.VideoList, error) {
	offset, err := decodeSearchPageToken(pageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	if r.data.redis == nil || r.data.videoRankingKey == "" {
		return r.listRecommendedVideosInPostgres(ctx, pageSize, offset)
	}
	values, err := r.data.redis.ZRevRange(ctx, r.data.videoRankingKey, offset, offset+int64(pageSize)).Result()
	if err != nil || len(values) == 0 {
		return r.listRecommendedVideosInPostgres(ctx, pageSize, offset)
	}
	hasNext := len(values) > pageSize
	if hasNext {
		values = values[:pageSize]
	}
	ids := make([]uint64, 0, len(values))
	for _, value := range values {
		id, parseErr := strconv.ParseUint(value, 10, 64)
		if parseErr == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	videos, err := r.findPublishedVideosInOrder(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := &biz.VideoList{Videos: videos}
	if hasNext {
		result.NextPageToken = encodeSearchPageToken(offset + int64(pageSize))
	}
	return result, nil
}

// RefreshVideoRanking atomically replaces the Redis sorted set with fresh time-decayed scores.
func (r *videoRepo) RefreshVideoRanking(ctx context.Context) error {
	if r.data.redis == nil || r.data.videoRankingKey == "" {
		return fmt.Errorf("Redis video ranking is not configured")
	}
	var records []videoPO
	if err := r.data.db.WithContext(ctx).
		Where("status = ? AND deleted_at IS NULL", string(biz.VideoStatusPublished)).
		Find(&records).Error; err != nil {
		return biz.ErrVideoStorage
	}
	now := time.Now()
	members := make([]redis.Z, 0, len(records))
	for _, record := range records {
		members = append(members, redis.Z{Score: videoHotScore(record, now), Member: strconv.FormatUint(record.ID, 10)})
	}
	pipeline := r.data.redis.TxPipeline()
	pipeline.Del(ctx, r.data.videoRankingKey)
	if len(members) > 0 {
		pipeline.ZAdd(ctx, r.data.videoRankingKey, members...)
	}
	_, err := pipeline.Exec(ctx)
	return err
}

func (r *videoRepo) listRecommendedVideosInPostgres(ctx context.Context, pageSize int, offset int64) (*biz.VideoList, error) {
	var records []videoPO
	if err := r.data.db.WithContext(ctx).Preload("Owner").
		Where("status = ? AND deleted_at IS NULL", string(biz.VideoStatusPublished)).
		Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	now := time.Now()
	sort.SliceStable(records, func(left, right int) bool {
		leftScore := videoHotScore(records[left], now)
		rightScore := videoHotScore(records[right], now)
		if leftScore == rightScore {
			return records[left].ID > records[right].ID
		}
		return leftScore > rightScore
	})
	start := min(int(offset), len(records))
	end := min(start+pageSize, len(records))
	result := &biz.VideoList{Videos: make([]biz.Video, 0, end-start)}
	for _, record := range records[start:end] {
		result.Videos = append(result.Videos, *toBizVideo(record))
	}
	if end < len(records) {
		result.NextPageToken = encodeSearchPageToken(int64(end))
	}
	return result, nil
}

func videoHotScore(video videoPO, now time.Time) float64 {
	publishedAt := video.CreatedAt
	if video.PublishTime != nil {
		publishedAt = *video.PublishTime
	}
	ageHours := max(now.Sub(publishedAt).Hours(), 0)
	engagement := math.Log1p(float64(video.ViewCount)) +
		1.8*math.Log1p(float64(video.LikeCount)) +
		2.2*math.Log1p(float64(video.FavoriteCount)) +
		1.4*math.Log1p(float64(video.DanmakuCount)) +
		1.2*math.Log1p(float64(video.CommentCount)) +
		0.8*math.Log1p(float64(video.CoinCount))
	freshness := 12 * math.Exp(-ageHours/(24*7))
	return engagement + freshness
}
