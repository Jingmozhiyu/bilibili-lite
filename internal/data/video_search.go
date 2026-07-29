package data

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"bilibili-lite/internal/biz"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/meilisearch/meilisearch-go"
	"gorm.io/gorm"
)

const searchTaskPollInterval = 50 * time.Millisecond

type videoSearchDocument struct {
	ID               uint64   `json:"id"`
	BVID             string   `json:"bvid"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Tags             []string `json:"tags"`
	OwnerID          uint64   `json:"owner_id"`
	OwnerName        string   `json:"owner_name"`
	ViewCount        int64    `json:"view_count"`
	DanmakuCount     int64    `json:"danmaku_count"`
	FavoriteCount    int64    `json:"favorite_count"`
	PublishTimestamp int64    `json:"publish_timestamp"`
}

// SearchVideos asks Meilisearch for ranked IDs and hydrates authoritative video data from PostgreSQL.
func (r *videoRepo) SearchVideos(ctx context.Context, options biz.VideoSearchOptions) (*biz.VideoSearchResult, error) {
	if r.data.search == nil {
		return nil, biz.ErrVideoStorage
	}
	offset, err := decodeSearchPageToken(options.PageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	request := &meilisearch.SearchRequest{
		Offset:               offset,
		Limit:                int64(options.PageSize + 1),
		AttributesToRetrieve: []string{"id"},
		Sort:                 searchSort(options.Order),
	}
	if options.OwnerID != 0 {
		request.Filter = fmt.Sprintf("owner_id = %d", options.OwnerID)
	}
	response, err := r.data.search.Index(r.data.videoSearchIndex).SearchWithContext(ctx, options.Query, request)
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	var hits []struct {
		ID uint64 `json:"id"`
	}
	if err := response.Hits.DecodeInto(&hits); err != nil {
		return nil, biz.ErrVideoStorage
	}
	hasNext := len(hits) > options.PageSize
	if hasNext {
		hits = hits[:options.PageSize]
	}
	ids := make([]uint64, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	videos, err := r.findPublishedVideosInOrder(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := &biz.VideoSearchResult{
		Videos: videos, TotalHits: response.EstimatedTotalHits,
		ProcessingTimeMs: response.ProcessingTimeMs,
	}
	if hasNext {
		result.NextPageToken = encodeSearchPageToken(offset + int64(options.PageSize))
	}
	return result, nil
}

func initializeVideoSearch(ctx context.Context, db *gorm.DB, client meilisearch.ServiceManager, indexUID string) error {
	health, err := client.HealthWithContext(ctx)
	if err != nil || health.Status != "available" {
		return fmt.Errorf("Meilisearch is unavailable: %w", err)
	}
	if _, err := client.GetIndexWithContext(ctx, indexUID); err != nil {
		var meiliError *meilisearch.Error
		if !errors.As(err, &meiliError) || meiliError.StatusCode != http.StatusNotFound {
			return err
		}
		task, err := client.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{Uid: indexUID, PrimaryKey: "id"})
		if err != nil {
			return err
		}
		if err := waitSearchTask(ctx, client, task.TaskUID); err != nil {
			return err
		}
	}
	index := client.Index(indexUID)
	settingsTask, err := index.UpdateSettingsWithContext(ctx, &meilisearch.Settings{
		SearchableAttributes: []string{"title", "tags", "owner_name", "description", "bvid"},
		DisplayedAttributes:  []string{"id"},
		FilterableAttributes: []string{"owner_id"},
		SortableAttributes:   []string{"view_count", "publish_timestamp", "danmaku_count", "favorite_count"},
	})
	if err != nil {
		return err
	}
	if err := waitSearchTask(ctx, client, settingsTask.TaskUID); err != nil {
		return err
	}
	deleteTask, err := index.DeleteAllDocumentsWithContext(ctx, nil)
	if err != nil {
		return err
	}
	if err := waitSearchTask(ctx, client, deleteTask.TaskUID); err != nil {
		return err
	}
	var records []videoPO
	if err := db.WithContext(ctx).Preload("Owner").
		Where("status = ? AND deleted_at IS NULL", string(biz.VideoStatusPublished)).
		Find(&records).Error; err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	documents := make([]videoSearchDocument, 0, len(records))
	for _, record := range records {
		documents = append(documents, newVideoSearchDocument(record))
	}
	addTask, err := index.AddDocumentsWithContext(ctx, documents, &meilisearch.DocumentOptions{PrimaryKey: meilisearch.StringPtr("id")})
	if err != nil {
		return err
	}
	return waitSearchTask(ctx, client, addTask.TaskUID)
}

func (r *videoRepo) findPublishedVideosInOrder(ctx context.Context, ids []uint64) ([]biz.Video, error) {
	if len(ids) == 0 {
		return []biz.Video{}, nil
	}
	var records []videoPO
	if err := r.data.db.WithContext(ctx).Preload("Owner").
		Where("id IN ? AND status = ? AND deleted_at IS NULL", ids, string(biz.VideoStatusPublished)).
		Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	recordsByID := make(map[uint64]videoPO, len(records))
	for _, record := range records {
		recordsByID[record.ID] = record
	}
	videos := make([]biz.Video, 0, len(records))
	for _, id := range ids {
		if record, ok := recordsByID[id]; ok {
			videos = append(videos, *toBizVideo(record))
		}
	}
	return videos, nil
}

func (r *videoRepo) syncPublishedVideoToSearch(ctx context.Context, videoID biz.VideoID) {
	if r.data.search == nil {
		return
	}
	syncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	var record videoPO
	err := r.data.db.WithContext(syncCtx).Preload("Owner").
		Where("id = ? AND status = ? AND deleted_at IS NULL", uint64(videoID), string(biz.VideoStatusPublished)).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		r.removeVideoFromSearch(syncCtx, videoID)
		return
	}
	if err != nil {
		log.Error("load video for search sync", "bvid", videoID.BVID(), "error", err)
		return
	}
	task, err := r.data.search.Index(r.data.videoSearchIndex).AddDocumentsWithContext(
		syncCtx,
		[]videoSearchDocument{newVideoSearchDocument(record)},
		&meilisearch.DocumentOptions{PrimaryKey: meilisearch.StringPtr("id")},
	)
	if err == nil {
		err = waitSearchTask(syncCtx, r.data.search, task.TaskUID)
	}
	if err != nil {
		log.Error("sync video to Meilisearch", "bvid", videoID.BVID(), "error", err)
	}
}

func (r *videoRepo) removeVideoFromSearch(ctx context.Context, videoID biz.VideoID) {
	if r.data.search == nil {
		return
	}
	task, err := r.data.search.Index(r.data.videoSearchIndex).
		DeleteDocumentWithContext(ctx, strconv.FormatUint(uint64(videoID), 10), nil)
	if err == nil {
		err = waitSearchTask(ctx, r.data.search, task.TaskUID)
	}
	if err != nil {
		log.Error("remove video from Meilisearch", "bvid", videoID.BVID(), "error", err)
	}
}

func newVideoSearchDocument(record videoPO) videoSearchDocument {
	var publishedAt int64
	if record.PublishTime != nil {
		publishedAt = record.PublishTime.Unix()
	}
	return videoSearchDocument{
		ID: record.ID, BVID: biz.VideoID(record.ID).BVID(),
		Title: record.Title, Description: record.Description, Tags: append([]string(nil), record.Tags...),
		OwnerID: record.OwnerID, OwnerName: record.Owner.DisplayName,
		ViewCount: record.ViewCount, DanmakuCount: record.DanmakuCount,
		FavoriteCount: record.FavoriteCount, PublishTimestamp: publishedAt,
	}
}

func waitSearchTask(ctx context.Context, client meilisearch.ServiceManager, taskUID int64) error {
	task, err := client.WaitForTaskWithContext(ctx, taskUID, searchTaskPollInterval)
	if err != nil {
		return err
	}
	if task.Status != meilisearch.TaskStatusSucceeded {
		return fmt.Errorf("Meilisearch task %d ended with status %s", taskUID, task.Status)
	}
	return nil
}

func searchSort(order biz.VideoSearchOrder) []string {
	switch order {
	case biz.VideoSearchMostViewed:
		return []string{"view_count:desc", "publish_timestamp:desc"}
	case biz.VideoSearchLatest:
		return []string{"publish_timestamp:desc"}
	case biz.VideoSearchMostDanmaku:
		return []string{"danmaku_count:desc", "publish_timestamp:desc"}
	case biz.VideoSearchMostFavorited:
		return []string{"favorite_count:desc", "publish_timestamp:desc"}
	default:
		return nil
	}
}

func encodeSearchPageToken(offset int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(offset, 10)))
}

func decodeSearchPageToken(token string) (int64, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, err
	}
	offset, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid search page token")
	}
	return offset, nil
}
