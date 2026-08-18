package data

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/conf"

	"github.com/meilisearch/meilisearch-go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const searchTaskPollInterval = 50 * time.Millisecond

// videoSearchDocument is the denormalized published-video projection stored by a search engine.
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

// videoSearchPage carries engine-neutral hit IDs and pagination metadata back to the repository.
type videoSearchPage struct {
	IDs              []uint64
	TotalHits        int64
	ProcessingTimeMs int64
	HasNext          bool
}

// videoSearchIndex is the data-layer seam for replaceable search engines.
type videoSearchIndex interface {
	Ensure(context.Context) error
	Search(context.Context, biz.VideoSearchOptions, int64) (*videoSearchPage, error)
	Upsert(context.Context, videoSearchDocument) error
	Delete(context.Context, biz.VideoID) error
}

// meiliVideoSearchIndex adapts the Meilisearch client to videoSearchIndex.
type meiliVideoSearchIndex struct {
	client   meilisearch.ServiceManager
	indexUID string
}

// newVideoSearchIndex constructs the optional search adapter when its endpoint and index are configured.
func newVideoSearchIndex(config *conf.Data_Search) videoSearchIndex {
	if config == nil || strings.TrimSpace(config.GetAddress()) == "" || strings.TrimSpace(config.GetVideoIndex()) == "" {
		return nil
	}
	return &meiliVideoSearchIndex{
		client:   meilisearch.New(strings.TrimRight(config.GetAddress(), "/"), meilisearch.WithAPIKey(config.GetApiKey())),
		indexUID: config.GetVideoIndex(),
	}
}

// SearchVideos uses Meilisearch when healthy and degrades to PostgreSQL otherwise.
func (r *videoRepo) SearchVideos(ctx context.Context, options biz.VideoSearchOptions) (*biz.VideoSearchResult, error) {
	offset, err := decodeSearchPageToken(options.PageToken)
	if err != nil {
		return nil, biz.ErrVideoInvalidArgument
	}
	if r.data.videoSearch == nil {
		return r.searchVideosInPostgres(ctx, options, offset)
	}
	page, err := r.data.videoSearch.Search(ctx, options, offset)
	if err != nil {
		return r.searchVideosInPostgres(ctx, options, offset)
	}
	videos, err := r.findPublishedVideosInOrder(ctx, page.IDs)
	if err != nil {
		return nil, err
	}
	result := &biz.VideoSearchResult{Videos: videos, TotalHits: page.TotalHits, ProcessingTimeMs: page.ProcessingTimeMs}
	if page.HasNext {
		result.NextPageToken = encodeSearchPageToken(offset + int64(options.PageSize))
	}
	return result, nil
}

// PrepareVideoSearch configures the optional index and seeds the durable outbox.
func (r *videoRepo) PrepareVideoSearch(ctx context.Context) error {
	if r.data.videoSearch == nil {
		return fmt.Errorf("video search is not configured")
	}
	if err := r.data.videoSearch.Ensure(ctx); err != nil {
		return err
	}
	now := time.Now()
	return r.data.db.WithContext(ctx).Exec(`
		INSERT INTO video_search_outbox (video_id, available_at, created_at, updated_at)
		SELECT id, ?, ?, ? FROM videos
		ON CONFLICT (video_id) DO UPDATE SET available_at = EXCLUDED.available_at, locked_at = NULL, updated_at = EXCLUDED.updated_at
	`, now, now, now).Error
}

// ProcessNextVideoSearchUpdate retries one database-backed index mutation.
func (r *videoRepo) ProcessNextVideoSearchUpdate(ctx context.Context) (bool, error) {
	if r.data.videoSearch == nil {
		return false, fmt.Errorf("video search is not configured")
	}
	outbox, err := r.claimSearchOutbox(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var record videoPO
	err = r.data.db.WithContext(ctx).Preload("Owner").
		Where("id = ? AND status = ? AND deleted_at IS NULL", outbox.VideoID, string(biz.VideoStatusPublished)).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = r.data.videoSearch.Delete(ctx, biz.VideoID(outbox.VideoID))
	} else if err == nil {
		err = r.data.videoSearch.Upsert(ctx, newVideoSearchDocument(record))
	}
	if err != nil {
		r.retrySearchOutbox(outbox, err)
		return true, err
	}
	deleteQuery := r.data.db.WithContext(ctx).Where("video_id = ?", outbox.VideoID)
	if outbox.LockedAt != nil {
		deleteQuery = deleteQuery.Where("locked_at = ?", *outbox.LockedAt)
	}
	if err := deleteQuery.Delete(&videoSearchOutboxPO{}).Error; err != nil {
		return true, err
	}
	return true, nil
}

// Ensure verifies Meilisearch health, creates the index when absent, and applies its searchable and sortable settings.
func (m *meiliVideoSearchIndex) Ensure(ctx context.Context) error {
	health, err := m.client.HealthWithContext(ctx)
	if err != nil || health.Status != "available" {
		return fmt.Errorf("Meilisearch is unavailable: %w", err)
	}
	if _, err := m.client.GetIndexWithContext(ctx, m.indexUID); err != nil {
		var meiliError *meilisearch.Error
		if !errors.As(err, &meiliError) || meiliError.StatusCode != http.StatusNotFound {
			return err
		}
		task, err := m.client.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{Uid: m.indexUID, PrimaryKey: "id"})
		if err != nil {
			return err
		}
		if err := waitSearchTask(ctx, m.client, task.TaskUID); err != nil {
			return err
		}
	}
	task, err := m.client.Index(m.indexUID).UpdateSettingsWithContext(ctx, &meilisearch.Settings{
		SearchableAttributes: []string{"title", "tags", "owner_name", "description", "bvid"},
		DisplayedAttributes:  []string{"id"},
		FilterableAttributes: []string{"owner_id"},
		SortableAttributes:   []string{"view_count", "publish_timestamp", "danmaku_count", "favorite_count"},
		LocalizedAttributes:  []*meilisearch.LocalizedAttributes{{AttributePatterns: []string{"title", "tags", "owner_name", "description"}, Locales: []string{"cmn"}}},
	})
	if err != nil {
		return err
	}
	return waitSearchTask(ctx, m.client, task.TaskUID)
}

// Search executes one Meilisearch query and returns only ordered IDs so PostgreSQL can hydrate authoritative records.
func (m *meiliVideoSearchIndex) Search(ctx context.Context, options biz.VideoSearchOptions, offset int64) (*videoSearchPage, error) {
	request := &meilisearch.SearchRequest{
		Offset: offset, Limit: int64(options.PageSize + 1), AttributesToRetrieve: []string{"id"},
		Sort: searchSort(options.Order), Locales: []string{"cmn"},
	}
	if options.OwnerID != 0 {
		request.Filter = fmt.Sprintf("owner_id = %d", options.OwnerID)
	}
	response, err := m.client.Index(m.indexUID).SearchWithContext(ctx, options.Query, request)
	if err != nil {
		return nil, err
	}
	var hits []struct {
		ID uint64 `json:"id"`
	}
	if err := response.Hits.DecodeInto(&hits); err != nil {
		return nil, err
	}
	hasNext := len(hits) > options.PageSize
	if hasNext {
		hits = hits[:options.PageSize]
	}
	ids := make([]uint64, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	return &videoSearchPage{IDs: ids, TotalHits: response.EstimatedTotalHits, ProcessingTimeMs: response.ProcessingTimeMs, HasNext: hasNext}, nil
}

// Upsert adds or replaces one video document and waits until Meilisearch finishes the asynchronous task.
func (m *meiliVideoSearchIndex) Upsert(ctx context.Context, document videoSearchDocument) error {
	task, err := m.client.Index(m.indexUID).AddDocumentsWithContext(ctx, []videoSearchDocument{document}, &meilisearch.DocumentOptions{PrimaryKey: meilisearch.StringPtr("id")})
	if err != nil {
		return err
	}
	return waitSearchTask(ctx, m.client, task.TaskUID)
}

// Delete removes one video document and waits until Meilisearch finishes the asynchronous task.
func (m *meiliVideoSearchIndex) Delete(ctx context.Context, videoID biz.VideoID) error {
	task, err := m.client.Index(m.indexUID).DeleteDocumentWithContext(ctx, strconv.FormatUint(uint64(videoID), 10), nil)
	if err != nil {
		return err
	}
	return waitSearchTask(ctx, m.client, task.TaskUID)
}

// searchVideosInPostgres provides a title, description, tag, and owner-name fallback when external search is unavailable.
func (r *videoRepo) searchVideosInPostgres(ctx context.Context, options biz.VideoSearchOptions, offset int64) (*biz.VideoSearchResult, error) {
	pattern := "%" + strings.ToLower(options.Query) + "%"
	query := r.data.db.WithContext(ctx).Model(&videoPO{}).Joins("Owner").
		Where("videos.status = ? AND videos.deleted_at IS NULL", string(biz.VideoStatusPublished)).
		Where("LOWER(videos.title) LIKE ? OR LOWER(videos.description) LIKE ? OR LOWER(CAST(videos.tags AS TEXT)) LIKE ? OR LOWER(Owner.display_name) LIKE ?", pattern, pattern, pattern, pattern)
	if options.OwnerID != 0 {
		query = query.Where("videos.owner_id = ?", options.OwnerID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	switch options.Order {
	case biz.VideoSearchMostViewed:
		query = query.Order("videos.view_count DESC, videos.publish_time DESC")
	case biz.VideoSearchMostDanmaku:
		query = query.Order("videos.danmaku_count DESC, videos.publish_time DESC")
	case biz.VideoSearchMostFavorited:
		query = query.Order("videos.favorite_count DESC, videos.publish_time DESC")
	default:
		query = query.Order("videos.publish_time DESC")
	}
	var records []videoPO
	if err := query.Preload("Owner").Offset(int(offset)).Limit(options.PageSize + 1).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	hasNext := len(records) > options.PageSize
	if hasNext {
		records = records[:options.PageSize]
	}
	result := &biz.VideoSearchResult{Videos: make([]biz.Video, 0, len(records)), TotalHits: total}
	for _, record := range records {
		result.Videos = append(result.Videos, *toBizVideo(record))
	}
	if hasNext {
		result.NextPageToken = encodeSearchPageToken(offset + int64(options.PageSize))
	}
	return result, nil
}

// findPublishedVideosInOrder hydrates search hits from PostgreSQL while preserving the engine-provided ranking order.
func (r *videoRepo) findPublishedVideosInOrder(ctx context.Context, ids []uint64) ([]biz.Video, error) {
	if len(ids) == 0 {
		return []biz.Video{}, nil
	}
	var records []videoPO
	if err := r.data.db.WithContext(ctx).Preload("Owner").Where("id IN ? AND status = ? AND deleted_at IS NULL", ids, string(biz.VideoStatusPublished)).Find(&records).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	byID := make(map[uint64]videoPO, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	videos := make([]biz.Video, 0, len(records))
	for _, id := range ids {
		if record, ok := byID[id]; ok {
			videos = append(videos, *toBizVideo(record))
		}
	}
	return videos, nil
}

// claimSearchOutbox exclusively claims the oldest available event and skips rows held by concurrent consumers.
func (r *videoRepo) claimSearchOutbox(ctx context.Context) (*videoSearchOutboxPO, error) {
	var entry videoSearchOutboxPO
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("available_at <= ? AND (locked_at IS NULL OR locked_at < ?)", time.Now(), time.Now().Add(-time.Minute)).
			Order("available_at ASC, video_id ASC").First(&entry).Error; err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&entry).Updates(map[string]any{"locked_at": &now, "updated_at": now}).Error
	})
	return &entry, err
}

// retrySearchOutbox releases a failed claim with capped exponential backoff unless a newer event replaced it.
func (r *videoRepo) retrySearchOutbox(entry *videoSearchOutboxPO, searchErr error) {
	attempts := entry.Attempts + 1
	delay := time.Second * time.Duration(1<<min(attempts, 8))
	query := r.data.db.WithContext(context.Background()).Model(&videoSearchOutboxPO{}).Where("video_id = ?", entry.VideoID)
	if entry.LockedAt != nil {
		query = query.Where("locked_at = ?", *entry.LockedAt)
	}
	_ = query.Updates(map[string]any{
		"attempts": attempts, "available_at": time.Now().Add(delay), "locked_at": nil,
		"last_error": searchErr.Error(), "updated_at": time.Now(),
	}).Error
}

// newVideoSearchDocument converts the authoritative PostgreSQL row into the denormalized search projection.
func newVideoSearchDocument(record videoPO) videoSearchDocument {
	var publishedAt int64
	if record.PublishTime != nil {
		publishedAt = record.PublishTime.Unix()
	}
	return videoSearchDocument{
		ID: record.ID, BVID: biz.VideoID(record.ID).BVID(), Title: record.Title, Description: record.Description,
		Tags: append([]string(nil), record.Tags...), OwnerID: record.OwnerID, OwnerName: record.Owner.DisplayName,
		ViewCount: record.ViewCount, DanmakuCount: record.DanmakuCount, FavoriteCount: record.FavoriteCount, PublishTimestamp: publishedAt,
	}
}

// waitSearchTask waits for a Meilisearch mutation and rejects terminal states other than succeeded.
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

// searchSort translates the domain search order into Meilisearch sort expressions.
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

// encodeSearchPageToken hides a numeric offset in a URL-safe opaque page token.
func encodeSearchPageToken(offset int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(offset, 10)))
}

// decodeSearchPageToken validates and restores the non-negative offset carried by a page token.
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
