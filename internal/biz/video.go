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

const bvidPrefix = "BV"

var (
	// ErrVideoNotFound is returned when a video does not exist.
	ErrVideoNotFound = errors.NotFound(v1.ErrorReason_VIDEO_NOT_FOUND.String(), "video not found")
	// ErrVideoInvalidArgument is returned when a video request is invalid.
	ErrVideoInvalidArgument = errors.BadRequest(v1.ErrorReason_VIDEO_INVALID_ARGUMENT.String(), "invalid video argument")
	// ErrVideoStorage is returned when video persistence is unavailable.
	ErrVideoStorage = errors.InternalServer(v1.ErrorReason_VIDEO_UNSPECIFIED.String(), "video storage unavailable")
	// ErrVideoProcessing is returned when uploaded media cannot be inspected or converted.
	ErrVideoProcessing = errors.InternalServer(v1.ErrorReason_VIDEO_PROCESSING_FAILED.String(), "video processing failed")
	// ErrVideoUploadInterrupted is returned when the upload stream is disconnected.
	ErrVideoUploadInterrupted = errors.New(408, v1.ErrorReason_VIDEO_UPLOAD_INTERRUPTED.String(), "video upload interrupted")
	// ErrVideoUploadTooLarge is returned when an upload exceeds the configured limit.
	ErrVideoUploadTooLarge = errors.BadRequest(v1.ErrorReason_VIDEO_UPLOAD_TOO_LARGE.String(), "video upload is too large")
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

// Video is the domain model for a video detail page.
type Video struct {
	ID              VideoID
	Title           string
	Description     string
	OwnerName       string
	OwnerAvatarURL  string
	CoverURL        string
	DurationSeconds int64
	ViewCount       int64
	DanmakuCount    int64
	LikeCount       int64
	CoinCount       int64
	FavoriteCount   int64
	ShareCount      int64
	PublishTime     time.Time
	Tags            []string
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

// VideoUploadInput carries one MP4 upload and its user-editable metadata.
type VideoUploadInput struct {
	OwnerID     uint64
	Title       string
	Description string
	Tags        []string
	Content     io.Reader
}

// VideoUploadResult points clients to the newly published video and DASH manifest.
type VideoUploadResult struct {
	VideoID     VideoID
	ManifestURL string
}

// VideoRepo is a video repo.
type VideoRepo interface {
	FindVideoByID(context.Context, VideoID) (*Video, error)
	FindVideoPlayByID(context.Context, VideoID) (*VideoPlay, error)
	FindVideoLike(context.Context, uint64, VideoID) (*VideoLike, error)
	SetVideoLike(context.Context, uint64, VideoID, bool) (*VideoLike, error)
	PublishVideoFromMP4(context.Context, *VideoUploadInput) (*VideoUploadResult, error)
}

// VideoUsecase coordinates video domain operations through VideoRepo.
type VideoUsecase struct {
	repo VideoRepo
}

// NewVideoUsecase injects video persistence into the usecase.
func NewVideoUsecase(repo VideoRepo) *VideoUsecase {
	return &VideoUsecase{repo: repo}
}

// GetVideo returns a video by its numeric ID.
func (uc *VideoUsecase) GetVideo(ctx context.Context, videoID VideoID) (*Video, error) {
	if videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.FindVideoByID(ctx, videoID)
}

// GetVideoPlay returns playback metadata for a numeric video ID.
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

// SetVideoLike idempotently applies the requested like state.
func (uc *VideoUsecase) SetVideoLike(ctx context.Context, userID uint64, videoID VideoID, liked bool) (*VideoLike, error) {
	if userID == 0 || videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.SetVideoLike(ctx, userID, videoID, liked)
}

// UploadVideo validates metadata before publishing the MP4 stream as DASH media.
func (uc *VideoUsecase) UploadVideo(ctx context.Context, input *VideoUploadInput) (*VideoUploadResult, error) {
	if input == nil {
		return nil, ErrVideoInvalidArgument
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.OwnerID == 0 || input.Title == "" || len(input.Title) > 200 || input.Content == nil {
		return nil, ErrVideoInvalidArgument
	}
	if len(input.Description) > 10000 || len(input.Tags) > 12 {
		return nil, ErrVideoInvalidArgument
	}
	cleanTags := make([]string, 0, len(input.Tags))
	seen := make(map[string]struct{}, len(input.Tags))
	for _, tag := range input.Tags {
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
	input.Tags = cleanTags
	return uc.repo.PublishVideoFromMP4(ctx, input)
}
