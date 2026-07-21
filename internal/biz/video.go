package biz

import (
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	v1 "bilibili-lite/api/video/v1"

	"github.com/go-kratos/kratos/v3/errors"
)

const (
	bvidPrefix           = "BV"
	defaultVideoPageSize = 20
	maxVideoPageSize     = 50
)

var (
	ErrVideoNotFound          = errors.NotFound(v1.ErrorReason_VIDEO_NOT_FOUND.String(), "video not found")
	ErrVideoInvalidArgument   = errors.BadRequest(v1.ErrorReason_VIDEO_INVALID_ARGUMENT.String(), "invalid video argument")
	ErrVideoStorage           = errors.InternalServer(v1.ErrorReason_VIDEO_UNSPECIFIED.String(), "video storage unavailable")
	ErrVideoProcessing        = errors.InternalServer(v1.ErrorReason_VIDEO_PROCESSING_FAILED.String(), "video processing failed")
	ErrVideoUploadInterrupted = errors.New(408, v1.ErrorReason_VIDEO_UPLOAD_INTERRUPTED.String(), "video upload interrupted")
	ErrVideoUploadTooLarge    = errors.BadRequest(v1.ErrorReason_VIDEO_UPLOAD_TOO_LARGE.String(), "video upload is too large")
	ErrVideoForbidden         = errors.Forbidden(v1.ErrorReason_VIDEO_FORBIDDEN.String(), "video operation is not allowed")
	ErrVideoInvalidState      = errors.Conflict(v1.ErrorReason_VIDEO_INVALID_STATE.String(), "video is not in the required state")
	ErrVideoViewTooEarly      = errors.BadRequest(v1.ErrorReason_VIDEO_VIEW_TOO_EARLY.String(), "video must be watched for at least five seconds")
)

// VideoStatus is the lifecycle state persisted for every allocated BV identifier.
type VideoStatus string

const (
	VideoStatusProcessing VideoStatus = "processing"
	VideoStatusReady      VideoStatus = "ready"
	VideoStatusPublished  VideoStatus = "published"
	VideoStatusFailed     VideoStatus = "failed"
	VideoStatusDeleted    VideoStatus = "deleted"
)

// VideoID is the internal numeric identifier shared by the video domain and persistence layers.
type VideoID uint64

// ParseBVID converts a public identifier such as BV12 into its numeric video ID.
func ParseBVID(value string) (VideoID, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, bvidPrefix) {
		return 0, ErrVideoInvalidArgument
	}
	numericID, err := strconv.ParseUint(strings.TrimPrefix(value, bvidPrefix), 10, 64)
	videoID := VideoID(numericID)
	if err != nil || videoID == 0 || videoID.BVID() != value {
		return 0, ErrVideoInvalidArgument
	}
	return videoID, nil
}

// BVID formats a numeric video ID for use in public API paths and responses.
func (id VideoID) BVID() string {
	if id == 0 {
		return ""
	}
	return bvidPrefix + strconv.FormatUint(uint64(id), 10)
}

// Video is the domain model shared by video detail and list responses.
type Video struct {
	ID              VideoID
	OwnerID         uint64
	Title           string
	Description     string
	OwnerName       string
	OwnerAvatarURL  string
	CoverURL        string
	Status          VideoStatus
	DurationSeconds int64
	ViewCount       int64
	DanmakuCount    int64
	LikeCount       int64
	CoinCount       int64
	FavoriteCount   int64
	ShareCount      int64
	PublishTime     time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Tags            []string
}

// VideoListOptions describes one keyset-paginated published video query.
type VideoListOptions struct {
	OwnerID   uint64
	PageSize  int
	PageToken string
}

// VideoList is one page of videos and its continuation token.
type VideoList struct {
	Videos        []Video
	NextPageToken string
}

// VideoPlay describes playable media and timed danmaku metadata.
type VideoPlay struct {
	VideoID VideoID
	Streams []VideoStream
	Danmaku DanmakuConfig
}

// VideoStream is a playable media variant.
type VideoStream struct {
	ID            string
	Label         string
	Codec         string
	MimeType      string
	URL           string
	Width         int32
	Height        int32
	Bandwidth     int32
	DefaultStream bool
}

// DanmakuConfig describes initial danmaku state for the player.
type DanmakuConfig struct {
	Enabled bool
	Format  string
	Items   []DanmakuItem
}

// DanmakuItem is a single timed comment.
type DanmakuItem struct {
	TimeSeconds float64
	Text        string
	Color       string
}

// VideoLike is a user's current like state and the video's authoritative count.
type VideoLike struct {
	VideoID   VideoID
	Liked     bool
	LikeCount int64
}

// VideoUploadInput carries one MP4 upload and an optional custom cover stream.
type VideoUploadInput struct {
	OwnerID uint64
	Content io.Reader
	Cover   io.Reader
}

// VideoUploadResult identifies media that has finished processing and is ready to publish.
type VideoUploadResult struct {
	VideoID     VideoID
	Status      VideoStatus
	ManifestURL string
	CoverURL    string
}

// VideoUploadStatus reports owner-only processing and publication state.
type VideoUploadStatus struct {
	VideoID       VideoID
	Status        VideoStatus
	FailureReason string
	ManifestURL   string
	CoverURL      string
}

// VideoPublishInput supplies metadata after media processing has completed.
type VideoPublishInput struct {
	OwnerID     uint64
	VideoID     VideoID
	Title       string
	Description string
	Tags        []string
}

// VideoViewSession starts the server-side minimum-watch timer.
type VideoViewSession struct {
	ID        string
	StartedAt time.Time
}

// VideoViewResult reports whether a qualified view passed frequency limits.
type VideoViewResult struct {
	Counted        bool
	ViewCount      int64
	RemainingToday int32
	NextEligibleAt time.Time
}

// VideoRepo owns persistence and media-side implementation details for video operations.
type VideoRepo interface {
	ListVideos(context.Context, VideoListOptions) (*VideoList, error)
	FindVideoByID(context.Context, VideoID) (*Video, error)
	FindVideoPlayByID(context.Context, VideoID) (*VideoPlay, error)
	FindVideoLike(context.Context, uint64, VideoID) (*VideoLike, error)
	FindVideoUploadStatus(context.Context, uint64, VideoID) (*VideoUploadStatus, error)
	SetVideoLike(context.Context, uint64, VideoID, bool) (*VideoLike, error)
	ProcessVideoUpload(context.Context, *VideoUploadInput) (*VideoUploadResult, error)
	PublishVideo(context.Context, *VideoPublishInput) (*Video, error)
	DeleteVideo(context.Context, uint64, VideoID) error
	CreateVideoViewSession(context.Context, uint64, VideoID) (*VideoViewSession, error)
	CompleteVideoViewSession(context.Context, uint64, VideoID, string) (*VideoViewResult, error)
}

// VideoUsecase coordinates video domain operations through VideoRepo.
type VideoUsecase struct {
	repo VideoRepo
}

// NewVideoUsecase injects video persistence into the usecase.
func NewVideoUsecase(repo VideoRepo) *VideoUsecase {
	return &VideoUsecase{repo: repo}
}

// ListVideos returns one page of published videos, optionally limited to an owner.
func (uc *VideoUsecase) ListVideos(ctx context.Context, ownerID uint64, pageSize int32, pageToken string) (*VideoList, error) {
	if pageSize < 0 || pageSize > maxVideoPageSize {
		return nil, ErrVideoInvalidArgument
	}
	if pageSize == 0 {
		pageSize = defaultVideoPageSize
	}
	return uc.repo.ListVideos(ctx, VideoListOptions{
		OwnerID: ownerID, PageSize: int(pageSize), PageToken: strings.TrimSpace(pageToken),
	})
}

// GetVideo returns a published video by its numeric ID.
func (uc *VideoUsecase) GetVideo(ctx context.Context, videoID VideoID) (*Video, error) {
	if videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.FindVideoByID(ctx, videoID)
}

// GetVideoPlay returns playback metadata for a published numeric video ID.
func (uc *VideoUsecase) GetVideoPlay(ctx context.Context, videoID VideoID) (*VideoPlay, error) {
	if videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.FindVideoPlayByID(ctx, videoID)
}

// GetVideoLike returns the authenticated user's current state.
func (uc *VideoUsecase) GetVideoLike(ctx context.Context, userID uint64, videoID VideoID) (*VideoLike, error) {
	if userID == 0 || videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.FindVideoLike(ctx, userID, videoID)
}

// GetVideoUploadStatus returns processing state only to the video's owner.
func (uc *VideoUsecase) GetVideoUploadStatus(ctx context.Context, userID uint64, videoID VideoID) (*VideoUploadStatus, error) {
	if userID == 0 || videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.FindVideoUploadStatus(ctx, userID, videoID)
}

// SetVideoLike idempotently applies the requested like state.
func (uc *VideoUsecase) SetVideoLike(ctx context.Context, userID uint64, videoID VideoID, liked bool) (*VideoLike, error) {
	if userID == 0 || videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.SetVideoLike(ctx, userID, videoID, liked)
}

// UploadVideo allocates a BV identifier immediately and processes uploaded media into a ready draft.
func (uc *VideoUsecase) UploadVideo(ctx context.Context, input *VideoUploadInput) (*VideoUploadResult, error) {
	if input == nil || input.OwnerID == 0 || input.Content == nil {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.ProcessVideoUpload(ctx, input)
}

// PublishVideo validates final metadata and transitions a ready draft to published.
func (uc *VideoUsecase) PublishVideo(ctx context.Context, input *VideoPublishInput) (*Video, error) {
	if input == nil {
		return nil, ErrVideoInvalidArgument
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.OwnerID == 0 || input.VideoID == 0 || input.Title == "" || len(input.Title) > 200 || len(input.Description) > 10000 || len(input.Tags) > 12 {
		return nil, ErrVideoInvalidArgument
	}
	input.Tags = cleanVideoTags(input.Tags)
	return uc.repo.PublishVideo(ctx, input)
}

// DeleteVideo marks a BV identifier deleted and removes its published media.
func (uc *VideoUsecase) DeleteVideo(ctx context.Context, userID uint64, videoID VideoID) error {
	if userID == 0 || videoID == 0 {
		return ErrVideoInvalidArgument
	}
	return uc.repo.DeleteVideo(ctx, userID, videoID)
}

// StartVideoView begins a server-timed playback session for a logged-in viewer.
func (uc *VideoUsecase) StartVideoView(ctx context.Context, userID uint64, videoID VideoID) (*VideoViewSession, error) {
	if userID == 0 || videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.CreateVideoViewSession(ctx, userID, videoID)
}

// CompleteVideoView qualifies a session after five seconds and applies hourly/daily limits atomically.
func (uc *VideoUsecase) CompleteVideoView(ctx context.Context, userID uint64, videoID VideoID, sessionID string) (*VideoViewResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if userID == 0 || videoID == 0 || sessionID == "" || len(sessionID) > 64 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.CompleteVideoViewSession(ctx, userID, videoID, sessionID)
}

func cleanVideoTags(tags []string) []string {
	cleanTags := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len(tag) > 30 {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		cleanTags = append(cleanTags, tag)
	}
	return cleanTags
}
