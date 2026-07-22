package service

import (
	"context"
	"time"

	v1 "bilibili-lite/api/video/v1"
	"bilibili-lite/internal/biz"
	appMiddleware "bilibili-lite/internal/middleware"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// VideoService adapts video API requests to domain usecases.
type VideoService struct {
	v1.UnimplementedVideoServiceServer

	videoUsecase *biz.VideoUsecase
}

// NewVideoService injects the video usecase.
func NewVideoService(videoUsecase *biz.VideoUsecase) *VideoService {
	return &VideoService{videoUsecase: videoUsecase}
}

// ListVideos returns one page of published videos for the homepage.
func (s *VideoService) ListVideos(ctx context.Context, req *v1.ListVideosRequest) (*v1.ListVideosReply, error) {
	list, err := s.videoUsecase.ListVideos(ctx, 0, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return convertVideoListReply(list), nil
}

// ListUserVideos returns one page of published submissions owned by a user.
func (s *VideoService) ListUserVideos(ctx context.Context, req *v1.ListUserVideosRequest) (*v1.ListVideosReply, error) {
	if req.GetUserId() == 0 {
		return nil, biz.ErrVideoInvalidArgument
	}
	list, err := s.videoUsecase.ListVideos(ctx, req.GetUserId(), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return convertVideoListReply(list), nil
}

// GetVideo returns video detail by BVID.
func (s *VideoService) GetVideo(ctx context.Context, req *v1.GetVideoRequest) (*v1.Video, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	video, err := s.videoUsecase.GetVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return convertVideoReply(video), nil
}

// GetVideoPlay returns playback metadata by BVID.
func (s *VideoService) GetVideoPlay(ctx context.Context, req *v1.GetVideoPlayRequest) (*v1.VideoPlay, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	play, err := s.videoUsecase.GetVideoPlay(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return convertVideoPlayReply(play), nil
}

// GetVideoLike authenticates the caller and returns their current like state for a video.
func (s *VideoService) GetVideoLike(ctx context.Context, req *v1.GetVideoLikeRequest) (*v1.VideoLike, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	like, err := s.videoUsecase.GetVideoLike(ctx, userID, videoID)
	if err != nil {
		return nil, err
	}
	return convertVideoLike(like), nil
}

// LikeVideo idempotently sets the authenticated caller's like state to active.
func (s *VideoService) LikeVideo(ctx context.Context, req *v1.LikeVideoRequest) (*v1.VideoLike, error) {
	return s.setVideoLike(ctx, req.GetBvid(), true)
}

// UnlikeVideo idempotently sets the authenticated caller's like state to inactive.
func (s *VideoService) UnlikeVideo(ctx context.Context, req *v1.UnlikeVideoRequest) (*v1.VideoLike, error) {
	return s.setVideoLike(ctx, req.GetBvid(), false)
}

// GetVideoEngagement returns the authenticated viewer's complete interaction state.
func (s *VideoService) GetVideoEngagement(ctx context.Context, req *v1.GetVideoEngagementRequest) (*v1.VideoEngagement, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	engagement, err := s.videoUsecase.GetVideoEngagement(ctx, userID, videoID)
	if err != nil {
		return nil, err
	}
	return convertVideoEngagement(engagement), nil
}

// FavoriteVideo idempotently sets the authenticated viewer's favorite state.
func (s *VideoService) FavoriteVideo(ctx context.Context, req *v1.FavoriteVideoRequest) (*v1.VideoEngagement, error) {
	return s.setVideoFavorite(ctx, req.GetBvid(), true)
}

// UnfavoriteVideo idempotently clears the authenticated viewer's favorite state.
func (s *VideoService) UnfavoriteVideo(ctx context.Context, req *v1.UnfavoriteVideoRequest) (*v1.VideoEngagement, error) {
	return s.setVideoFavorite(ctx, req.GetBvid(), false)
}

// CoinVideo irreversibly raises the viewer's cumulative contribution to the requested target.
func (s *VideoService) CoinVideo(ctx context.Context, req *v1.CoinVideoRequest) (*v1.VideoEngagement, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	engagement, err := s.videoUsecase.SetVideoCoinAmount(ctx, userID, videoID, req.GetTargetAmount())
	if err != nil {
		return nil, err
	}
	return convertVideoEngagement(engagement), nil
}

// ShareVideo records one idempotent share event.
func (s *VideoService) ShareVideo(ctx context.Context, req *v1.ShareVideoRequest) (*v1.VideoShare, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	share, err := s.videoUsecase.ShareVideo(ctx, userID, videoID, req.GetRequestId())
	if err != nil {
		return nil, err
	}
	return &v1.VideoShare{Bvid: share.VideoID.BVID(), ShareCount: share.ShareCount}, nil
}

// ListMyLikedVideos returns the caller's active like history.
func (s *VideoService) ListMyLikedVideos(ctx context.Context, req *v1.ListVideoHistoryRequest) (*v1.ListVideoHistoryReply, error) {
	return s.listVideoHistory(ctx, req, biz.VideoHistoryLiked)
}

// ListMyFavoriteVideos returns the caller's active favorite history.
func (s *VideoService) ListMyFavoriteVideos(ctx context.Context, req *v1.ListVideoHistoryRequest) (*v1.ListVideoHistoryReply, error) {
	return s.listVideoHistory(ctx, req, biz.VideoHistoryFavorited)
}

// ListMyCoinedVideos returns the caller's irreversible coin history.
func (s *VideoService) ListMyCoinedVideos(ctx context.Context, req *v1.ListVideoHistoryRequest) (*v1.ListVideoHistoryReply, error) {
	return s.listVideoHistory(ctx, req, biz.VideoHistoryCoined)
}

// CreateDanmaku publishes a timed comment at the supplied playback position.
func (s *VideoService) CreateDanmaku(ctx context.Context, req *v1.CreateDanmakuRequest) (*v1.DanmakuItem, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	item, err := s.videoUsecase.CreateDanmaku(ctx, userID, videoID, req.GetTimeSeconds(), req.GetText(), req.GetColor())
	if err != nil {
		return nil, err
	}
	return convertDanmakuItem(item), nil
}

// DeleteDanmaku removes a timed comment for its author or the video owner.
func (s *VideoService) DeleteDanmaku(ctx context.Context, req *v1.DeleteDanmakuRequest) (*emptypb.Empty, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	if err := s.videoUsecase.DeleteDanmaku(ctx, userID, videoID, req.GetDanmakuId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ListVideoComments returns one public page of top-level comments.
func (s *VideoService) ListVideoComments(ctx context.Context, req *v1.ListVideoCommentsRequest) (*v1.ListVideoCommentsReply, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	comments, err := s.videoUsecase.ListVideoComments(ctx, videoID, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return convertVideoCommentList(comments), nil
}

// CreateVideoComment publishes one authenticated top-level comment.
func (s *VideoService) CreateVideoComment(ctx context.Context, req *v1.CreateVideoCommentRequest) (*v1.VideoComment, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	comment, err := s.videoUsecase.CreateVideoComment(ctx, userID, videoID, req.GetContent())
	if err != nil {
		return nil, err
	}
	return convertVideoComment(comment), nil
}

// DeleteVideoComment removes a comment for its author or the video owner.
func (s *VideoService) DeleteVideoComment(ctx context.Context, req *v1.DeleteVideoCommentRequest) (*emptypb.Empty, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	if err := s.videoUsecase.DeleteVideoComment(ctx, userID, videoID, req.GetCommentId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// GetVideoUploadStatus returns owner-only processing state for one allocated BV identifier.
func (s *VideoService) GetVideoUploadStatus(ctx context.Context, req *v1.GetVideoUploadStatusRequest) (*v1.VideoUploadStatus, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	status, err := s.videoUsecase.GetVideoUploadStatus(ctx, userID, videoID)
	if err != nil {
		return nil, err
	}
	return &v1.VideoUploadStatus{
		Bvid: status.VideoID.BVID(), Status: convertVideoStatus(status.Status),
		FailureReason: status.FailureReason, ManifestUrl: status.ManifestURL, CoverUrl: status.CoverURL,
	}, nil
}

// PublishVideo attaches final metadata to media that has reached the ready state.
func (s *VideoService) PublishVideo(ctx context.Context, req *v1.PublishVideoRequest) (*v1.Video, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	video, err := s.videoUsecase.PublishVideo(ctx, &biz.VideoPublishInput{
		OwnerID: userID, VideoID: videoID,
		Title: req.GetTitle(), Description: req.GetDescription(), Tags: append([]string(nil), req.GetTags()...),
	})
	if err != nil {
		return nil, err
	}
	return convertVideoReply(video), nil
}

// DeleteVideo marks an owned video deleted while preserving its consumed BV identifier.
func (s *VideoService) DeleteVideo(ctx context.Context, req *v1.DeleteVideoRequest) (*emptypb.Empty, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.videoUsecase.DeleteVideo(ctx, userID, videoID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// StartVideoView starts the server-side five-second qualification timer.
func (s *VideoService) StartVideoView(ctx context.Context, req *v1.StartVideoViewRequest) (*v1.VideoViewSession, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	session, err := s.videoUsecase.StartVideoView(ctx, userID, videoID)
	if err != nil {
		return nil, err
	}
	return &v1.VideoViewSession{SessionId: session.ID, StartedAt: timestamppb.New(session.StartedAt)}, nil
}

// CompleteVideoView records a qualified view subject to hourly and daily limits.
func (s *VideoService) CompleteVideoView(ctx context.Context, req *v1.CompleteVideoViewRequest) (*v1.VideoViewResult, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.videoUsecase.CompleteVideoView(ctx, userID, videoID, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	return &v1.VideoViewResult{
		Counted: result.Counted, ViewCount: result.ViewCount,
		RemainingToday: result.RemainingToday, NextEligibleAt: timestamppb.New(result.NextEligibleAt),
	}, nil
}

// setVideoLike translates authenticated transport state into the shared like usecase operation.
func (s *VideoService) setVideoLike(ctx context.Context, bvid string, liked bool) (*v1.VideoLike, error) {
	videoID, err := biz.ParseBVID(bvid)
	if err != nil {
		return nil, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	like, err := s.videoUsecase.SetVideoLike(ctx, userID, videoID, liked)
	if err != nil {
		return nil, err
	}
	return convertVideoLike(like), nil
}

// convertVideoReply maps the video detail domain object to its public API reply.
func convertVideoReply(in *biz.Video) *v1.Video {
	if in == nil {
		return nil
	}
	return &v1.Video{
		Bvid:            in.ID.BVID(),
		Title:           in.Title,
		Description:     in.Description,
		OwnerName:       in.OwnerName,
		OwnerAvatarUrl:  in.OwnerAvatarURL,
		CoverUrl:        in.CoverURL,
		DurationSeconds: in.DurationSeconds,
		ViewCount:       in.ViewCount,
		DanmakuCount:    in.DanmakuCount,
		LikeCount:       in.LikeCount,
		CoinCount:       in.CoinCount,
		FavoriteCount:   in.FavoriteCount,
		ShareCount:      in.ShareCount,
		CommentCount:    in.CommentCount,
		PublishTime:     timestampOrNil(in.PublishTime),
		Tags:            append([]string(nil), in.Tags...),
		OwnerId:         in.OwnerID,
		Status:          convertVideoStatus(in.Status),
		CreatedAt:       timestampOrNil(in.CreatedAt),
		UpdatedAt:       timestampOrNil(in.UpdatedAt),
	}
}

func convertVideoListReply(in *biz.VideoList) *v1.ListVideosReply {
	out := &v1.ListVideosReply{Videos: make([]*v1.Video, 0, len(in.Videos)), NextPageToken: in.NextPageToken}
	for i := range in.Videos {
		out.Videos = append(out.Videos, convertVideoReply(&in.Videos[i]))
	}
	return out
}

func convertVideoStatus(status biz.VideoStatus) v1.VideoStatus {
	switch status {
	case biz.VideoStatusProcessing:
		return v1.VideoStatus_VIDEO_STATUS_PROCESSING
	case biz.VideoStatusReady:
		return v1.VideoStatus_VIDEO_STATUS_READY
	case biz.VideoStatusPublished:
		return v1.VideoStatus_VIDEO_STATUS_PUBLISHED
	case biz.VideoStatusFailed:
		return v1.VideoStatus_VIDEO_STATUS_FAILED
	case biz.VideoStatusDeleted:
		return v1.VideoStatus_VIDEO_STATUS_DELETED
	default:
		return v1.VideoStatus_VIDEO_STATUS_UNSPECIFIED
	}
}

func timestampOrNil(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value)
}

// convertVideoPlayReply maps stream and danmaku domain objects to playback DTOs.
func convertVideoPlayReply(in *biz.VideoPlay) *v1.VideoPlay {
	if in == nil {
		return nil
	}
	out := &v1.VideoPlay{
		Bvid:    in.VideoID.BVID(),
		Streams: make([]*v1.VideoStream, 0, len(in.Streams)),
		Danmaku: &v1.DanmakuConfig{
			Enabled: in.Danmaku.Enabled,
			Format:  in.Danmaku.Format,
			Items:   make([]*v1.DanmakuItem, 0, len(in.Danmaku.Items)),
		},
	}
	for _, stream := range in.Streams {
		out.Streams = append(out.Streams, &v1.VideoStream{
			Id:            stream.ID,
			Label:         stream.Label,
			Codec:         stream.Codec,
			MimeType:      stream.MimeType,
			Url:           stream.URL,
			Width:         stream.Width,
			Height:        stream.Height,
			Bandwidth:     stream.Bandwidth,
			DefaultStream: stream.DefaultStream,
		})
	}
	for _, item := range in.Danmaku.Items {
		out.Danmaku.Items = append(out.Danmaku.Items, convertDanmakuItem(&item))
	}
	return out
}

// convertVideoLike maps the like domain object to its public API reply.
func convertVideoLike(in *biz.VideoLike) *v1.VideoLike {
	return &v1.VideoLike{Bvid: in.VideoID.BVID(), Liked: in.Liked, LikeCount: in.LikeCount}
}

func (s *VideoService) setVideoFavorite(ctx context.Context, bvid string, favorited bool) (*v1.VideoEngagement, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, bvid)
	if err != nil {
		return nil, err
	}
	engagement, err := s.videoUsecase.SetVideoFavorite(ctx, userID, videoID, favorited)
	if err != nil {
		return nil, err
	}
	return convertVideoEngagement(engagement), nil
}

func (s *VideoService) listVideoHistory(ctx context.Context, req *v1.ListVideoHistoryRequest, kind biz.VideoHistoryKind) (*v1.ListVideoHistoryReply, error) {
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	history, err := s.videoUsecase.ListVideoHistory(ctx, userID, kind, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := &v1.ListVideoHistoryReply{Items: make([]*v1.VideoHistoryItem, 0, len(history.Items)), NextPageToken: history.NextPageToken}
	for index := range history.Items {
		item := &history.Items[index]
		out.Items = append(out.Items, &v1.VideoHistoryItem{
			Video: convertVideoReply(&item.Video), InteractedAt: timestampOrNil(item.InteractedAt), CoinAmount: item.CoinAmount,
		})
	}
	return out, nil
}

func authenticatedVideoRequest(ctx context.Context, bvid string) (biz.VideoID, uint64, error) {
	videoID, err := biz.ParseBVID(bvid)
	if err != nil {
		return 0, 0, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return 0, 0, err
	}
	return videoID, userID, nil
}

func convertVideoEngagement(in *biz.VideoEngagement) *v1.VideoEngagement {
	return &v1.VideoEngagement{
		Bvid: in.VideoID.BVID(), Liked: in.Liked, Favorited: in.Favorited,
		MyCoinAmount: in.MyCoinAmount, LikeCount: in.LikeCount,
		FavoriteCount: in.FavoriteCount, CoinCount: in.CoinCount,
		ShareCount: in.ShareCount, CoinBalance: in.CoinBalance,
	}
}

func convertDanmakuItem(in *biz.DanmakuItem) *v1.DanmakuItem {
	return &v1.DanmakuItem{
		Id: in.ID, UserId: in.UserID, UserName: in.UserName,
		TimeSeconds: in.TimeSeconds, Text: in.Text, Color: in.Color,
		CreatedAt: timestampOrNil(in.CreatedAt),
	}
}

func convertVideoComment(in *biz.VideoComment) *v1.VideoComment {
	return &v1.VideoComment{
		Id: in.ID, Bvid: in.VideoID.BVID(), UserId: in.UserID,
		UserName: in.UserName, UserAvatarUrl: in.UserAvatarURL,
		Content: in.Content, CreatedAt: timestampOrNil(in.CreatedAt),
	}
}

func convertVideoCommentList(in *biz.VideoCommentList) *v1.ListVideoCommentsReply {
	out := &v1.ListVideoCommentsReply{Comments: make([]*v1.VideoComment, 0, len(in.Comments)), NextPageToken: in.NextPageToken}
	for index := range in.Comments {
		out.Comments = append(out.Comments, convertVideoComment(&in.Comments[index]))
	}
	return out
}
