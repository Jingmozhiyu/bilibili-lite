package biz

import (
	"context"
	"strings"
	"time"

	v1 "bilibili-lite/api/video/v1"

	"github.com/go-kratos/kratos/v3/errors"
)

var (
	// ErrVideoNotFound is returned when a video does not exist.
	ErrVideoNotFound = errors.NotFound(v1.ErrorReason_VIDEO_NOT_FOUND.String(), "video not found")
	// ErrVideoInvalidArgument is returned when a video request is invalid.
	ErrVideoInvalidArgument = errors.BadRequest(v1.ErrorReason_VIDEO_INVALID_ARGUMENT.String(), "invalid video argument")
	// ErrVideoStorage is returned when video persistence is unavailable.
	ErrVideoStorage = errors.InternalServer(v1.ErrorReason_VIDEO_UNSPECIFIED.String(), "video storage unavailable")
)

// Video is the domain model for a video detail page.
type Video struct {
	BVID            string
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
	BVID    string
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

// VideoRepo is a video repo.
type VideoRepo interface {
	FindVideoByBVID(context.Context, string) (*Video, error)
	FindVideoPlayByBVID(context.Context, string) (*VideoPlay, error)
}

// VideoUsecase is a Video usecase.
type VideoUsecase struct {
	repo VideoRepo
}

// NewVideoUsecase new a Video usecase.
func NewVideoUsecase(repo VideoRepo) *VideoUsecase {
	return &VideoUsecase{repo: repo}
}

// GetVideo returns a video by BVID.
func (uc *VideoUsecase) GetVideo(ctx context.Context, bvid string) (*Video, error) {
	bvid = strings.TrimSpace(bvid)
	if bvid == "" {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.FindVideoByBVID(ctx, bvid)
}

// GetVideoPlay returns playback metadata for a video.
func (uc *VideoUsecase) GetVideoPlay(ctx context.Context, bvid string) (*VideoPlay, error) {
	bvid = strings.TrimSpace(bvid)
	if bvid == "" {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.FindVideoPlayByBVID(ctx, bvid)
}
