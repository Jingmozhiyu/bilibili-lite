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
	maxDanmakuLength     = 100
	maxCommentLength     = 2000
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
	ErrVideoInsufficientCoins = errors.BadRequest(v1.ErrorReason_VIDEO_INSUFFICIENT_COINS.String(), "coin balance is insufficient")
	ErrVideoCoinLimit         = errors.Conflict(v1.ErrorReason_VIDEO_COIN_LIMIT_REACHED.String(), "video coin amount cannot be reduced or exceed two")
	ErrVideoDanmakuNotFound   = errors.NotFound(v1.ErrorReason_VIDEO_DANMAKU_NOT_FOUND.String(), "danmaku not found")
	ErrVideoCommentNotFound   = errors.NotFound(v1.ErrorReason_VIDEO_COMMENT_NOT_FOUND.String(), "video comment not found")
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
	CommentCount    int64
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
	ID          uint64
	UserID      uint64
	UserName    string
	TimeSeconds float64
	Text        string
	Color       string
	CreatedAt   time.Time
}

// VideoLike is a user's current like state and the video's authoritative count.
type VideoLike struct {
	VideoID   VideoID
	Liked     bool
	LikeCount int64
}

// VideoEngagement is the authenticated viewer's state and authoritative video counters.
type VideoEngagement struct {
	VideoID       VideoID
	Liked         bool
	Favorited     bool
	MyCoinAmount  int32
	LikeCount     int64
	FavoriteCount int64
	CoinCount     int64
	ShareCount    int64
	CoinBalance   int64
}

// VideoShare reports the count after recording one idempotent share event.
type VideoShare struct {
	VideoID    VideoID
	ShareCount int64
}

// VideoHistoryKind selects one authenticated interaction history.
type VideoHistoryKind string

const (
	VideoHistoryLiked     VideoHistoryKind = "liked"
	VideoHistoryFavorited VideoHistoryKind = "favorited"
	VideoHistoryCoined    VideoHistoryKind = "coined"
)

// VideoHistoryItem combines a published video with one viewer interaction.
type VideoHistoryItem struct {
	Video        Video
	InteractedAt time.Time
	CoinAmount   int32
}

// VideoHistoryList is one keyset-paginated interaction history page.
type VideoHistoryList struct {
	Items         []VideoHistoryItem
	NextPageToken string
}

// VideoComment is one top-level comment with its author profile.
type VideoComment struct {
	ID              uint64
	VideoID         VideoID
	UserID          uint64
	UserName        string
	UserAvatarURL   string
	Content         string
	CreatedAt       time.Time
	RootID          uint64
	ParentID        uint64
	ReplyToUserID   uint64
	ReplyToUserName string
	LikeCount       int64
	Liked           bool
	ReplyCount      int64
	Deleted         bool
}

// VideoCommentList is one keyset-paginated top-level comment page.
type VideoCommentList struct {
	Comments      []VideoComment
	NextPageToken string
}

// VideoCommentInteraction is the viewer's desired state and authoritative count.
type VideoCommentInteraction struct {
	CommentID uint64
	Liked     bool
	LikeCount int64
}

// VideoCommentHistoryItem combines one authored comment with its published video.
type VideoCommentHistoryItem struct {
	Video   Video
	Comment VideoComment
}

// VideoCommentHistoryList is one keyset-paginated authored comment page.
type VideoCommentHistoryList struct {
	Items         []VideoCommentHistoryItem
	NextPageToken string
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
	FindVideoEngagement(context.Context, uint64, VideoID) (*VideoEngagement, error)
	FindVideoUploadStatus(context.Context, uint64, VideoID) (*VideoUploadStatus, error)
	SetVideoLike(context.Context, uint64, VideoID, bool) (*VideoLike, error)
	SetVideoFavorite(context.Context, uint64, VideoID, bool) (*VideoEngagement, error)
	SetVideoCoinAmount(context.Context, uint64, VideoID, int32) (*VideoEngagement, error)
	CreateVideoShare(context.Context, uint64, VideoID, string) (*VideoShare, error)
	ListVideoHistory(context.Context, uint64, VideoHistoryKind, int, string) (*VideoHistoryList, error)
	CreateDanmaku(context.Context, uint64, VideoID, float64, string, string) (*DanmakuItem, error)
	DeleteDanmaku(context.Context, uint64, VideoID, uint64) error
	ListVideoComments(context.Context, uint64, VideoID, int, string) (*VideoCommentList, error)
	ListVideoCommentReplies(context.Context, uint64, VideoID, uint64, int, string) (*VideoCommentList, error)
	CreateVideoComment(context.Context, uint64, VideoID, uint64, string) (*VideoComment, error)
	DeleteVideoComment(context.Context, uint64, VideoID, uint64) error
	SetVideoCommentLike(context.Context, uint64, VideoID, uint64, bool) (*VideoCommentInteraction, error)
	ListVideoCommentHistory(context.Context, uint64, int, string) (*VideoCommentHistoryList, error)
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

// GetVideoEngagement returns all viewer-specific interaction state in one read.
func (uc *VideoUsecase) GetVideoEngagement(ctx context.Context, userID uint64, videoID VideoID) (*VideoEngagement, error) {
	if userID == 0 || videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.FindVideoEngagement(ctx, userID, videoID)
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

// SetVideoFavorite idempotently applies the requested favorite state.
func (uc *VideoUsecase) SetVideoFavorite(ctx context.Context, userID uint64, videoID VideoID, favorited bool) (*VideoEngagement, error) {
	if userID == 0 || videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.SetVideoFavorite(ctx, userID, videoID, favorited)
}

// SetVideoCoinAmount irreversibly raises the viewer's cumulative amount to one or two.
func (uc *VideoUsecase) SetVideoCoinAmount(ctx context.Context, userID uint64, videoID VideoID, targetAmount int32) (*VideoEngagement, error) {
	if userID == 0 || videoID == 0 || targetAmount < 1 || targetAmount > 2 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.SetVideoCoinAmount(ctx, userID, videoID, targetAmount)
}

// ShareVideo records one share event, deduplicated by a client-generated request ID.
func (uc *VideoUsecase) ShareVideo(ctx context.Context, userID uint64, videoID VideoID, requestID string) (*VideoShare, error) {
	requestID = strings.TrimSpace(requestID)
	if userID == 0 || videoID == 0 || requestID == "" || len(requestID) > 64 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.CreateVideoShare(ctx, userID, videoID, requestID)
}

// ListVideoHistory returns one authenticated interaction history page.
func (uc *VideoUsecase) ListVideoHistory(ctx context.Context, userID uint64, kind VideoHistoryKind, pageSize int32, pageToken string) (*VideoHistoryList, error) {
	if userID == 0 || (kind != VideoHistoryLiked && kind != VideoHistoryFavorited && kind != VideoHistoryCoined) {
		return nil, ErrVideoInvalidArgument
	}
	pageSize, err := normalizeVideoPageSize(pageSize)
	if err != nil {
		return nil, err
	}
	return uc.repo.ListVideoHistory(ctx, userID, kind, int(pageSize), strings.TrimSpace(pageToken))
}

// CreateDanmaku validates and publishes a timed comment on a video.
func (uc *VideoUsecase) CreateDanmaku(ctx context.Context, userID uint64, videoID VideoID, timeSeconds float64, text, color string) (*DanmakuItem, error) {
	text = strings.TrimSpace(text)
	color = strings.TrimSpace(color)
	if color == "" {
		color = "#ffffff"
	}
	if userID == 0 || videoID == 0 || timeSeconds < 0 || text == "" || len([]rune(text)) > maxDanmakuLength || !validDanmakuColor(color) {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.CreateDanmaku(ctx, userID, videoID, timeSeconds, text, color)
}

// DeleteDanmaku removes a timed comment when called by its author or the video owner.
func (uc *VideoUsecase) DeleteDanmaku(ctx context.Context, userID uint64, videoID VideoID, danmakuID uint64) error {
	if userID == 0 || videoID == 0 || danmakuID == 0 {
		return ErrVideoInvalidArgument
	}
	return uc.repo.DeleteDanmaku(ctx, userID, videoID, danmakuID)
}

// ListVideoComments returns one page of top-level comments.
func (uc *VideoUsecase) ListVideoComments(ctx context.Context, viewerID uint64, videoID VideoID, pageSize int32, pageToken string) (*VideoCommentList, error) {
	if videoID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	pageSize, err := normalizeVideoPageSize(pageSize)
	if err != nil {
		return nil, err
	}
	return uc.repo.ListVideoComments(ctx, viewerID, videoID, int(pageSize), strings.TrimSpace(pageToken))
}

// ListVideoCommentReplies returns one page of replies flattened under a root comment.
func (uc *VideoUsecase) ListVideoCommentReplies(ctx context.Context, viewerID uint64, videoID VideoID, rootCommentID uint64, pageSize int32, pageToken string) (*VideoCommentList, error) {
	if videoID == 0 || rootCommentID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	pageSize, err := normalizeVideoPageSize(pageSize)
	if err != nil {
		return nil, err
	}
	return uc.repo.ListVideoCommentReplies(ctx, viewerID, videoID, rootCommentID, int(pageSize), strings.TrimSpace(pageToken))
}

// CreateVideoComment publishes a root comment or a reply to an existing comment.
func (uc *VideoUsecase) CreateVideoComment(ctx context.Context, userID uint64, videoID VideoID, parentCommentID uint64, content string) (*VideoComment, error) {
	content = strings.TrimSpace(content)
	if userID == 0 || videoID == 0 || content == "" || len([]rune(content)) > maxCommentLength {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.CreateVideoComment(ctx, userID, videoID, parentCommentID, content)
}

// DeleteVideoComment removes a comment when called by its author or the video owner.
func (uc *VideoUsecase) DeleteVideoComment(ctx context.Context, userID uint64, videoID VideoID, commentID uint64) error {
	if userID == 0 || videoID == 0 || commentID == 0 {
		return ErrVideoInvalidArgument
	}
	return uc.repo.DeleteVideoComment(ctx, userID, videoID, commentID)
}

// SetVideoCommentLike idempotently applies the requested comment like state.
func (uc *VideoUsecase) SetVideoCommentLike(ctx context.Context, userID uint64, videoID VideoID, commentID uint64, liked bool) (*VideoCommentInteraction, error) {
	if userID == 0 || videoID == 0 || commentID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	return uc.repo.SetVideoCommentLike(ctx, userID, videoID, commentID, liked)
}

// ListVideoCommentHistory returns comments authored by the authenticated caller.
func (uc *VideoUsecase) ListVideoCommentHistory(ctx context.Context, userID uint64, pageSize int32, pageToken string) (*VideoCommentHistoryList, error) {
	if userID == 0 {
		return nil, ErrVideoInvalidArgument
	}
	pageSize, err := normalizeVideoPageSize(pageSize)
	if err != nil {
		return nil, err
	}
	return uc.repo.ListVideoCommentHistory(ctx, userID, int(pageSize), strings.TrimSpace(pageToken))
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

func normalizeVideoPageSize(pageSize int32) (int32, error) {
	if pageSize < 0 || pageSize > maxVideoPageSize {
		return 0, ErrVideoInvalidArgument
	}
	if pageSize == 0 {
		return defaultVideoPageSize, nil
	}
	return pageSize, nil
}

func validDanmakuColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(color[1:], 16, 24)
	return err == nil
}
