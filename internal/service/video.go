package service

import (
	"context"
	"strings"
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

// SearchVideos returns published videos ranked by Meilisearch and hydrated from PostgreSQL.
func (s *VideoService) SearchVideos(ctx context.Context, req *v1.SearchVideosRequest) (*v1.SearchVideosReply, error) {
	result, err := s.videoUsecase.SearchVideos(
		ctx,
		req.GetQuery(),
		convertVideoSearchOrder(req.GetOrder()),
		req.GetOwnerId(),
		req.GetPageSize(),
		req.GetPageToken(),
	)
	if err != nil {
		return nil, err
	}
	out := &v1.SearchVideosReply{
		Videos:        make([]*v1.Video, 0, len(result.Videos)),
		NextPageToken: result.NextPageToken, TotalHits: result.TotalHits,
		ProcessingTimeMs: result.ProcessingTimeMs,
	}
	for index := range result.Videos {
		out.Videos = append(out.Videos, convertVideoReply(&result.Videos[index]))
	}
	return out, nil
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

// ListMyWatchHistory returns distinct videos ordered by the caller's latest playback start.
func (s *VideoService) ListMyWatchHistory(ctx context.Context, req *v1.ListVideoHistoryRequest) (*v1.ListVideoHistoryReply, error) {
	return s.listVideoHistory(ctx, req, biz.VideoHistoryWatched)
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
	viewerID, _ := appMiddleware.UserID(ctx)
	comments, err := s.videoUsecase.ListVideoComments(ctx, viewerID, videoID, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return convertVideoCommentList(comments), nil
}

// ListVideoCommentReplies returns one public page of replies under a root comment.
func (s *VideoService) ListVideoCommentReplies(ctx context.Context, req *v1.ListVideoCommentRepliesRequest) (*v1.ListVideoCommentsReply, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	viewerID, _ := appMiddleware.UserID(ctx)
	comments, err := s.videoUsecase.ListVideoCommentReplies(ctx, viewerID, videoID, req.GetRootCommentId(), req.GetPageSize(), req.GetPageToken())
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
	comment, err := s.videoUsecase.CreateVideoComment(ctx, userID, videoID, req.GetParentCommentId(), req.GetContent())
	if err != nil {
		return nil, err
	}
	return convertVideoComment(comment), nil
}

// LikeVideoComment idempotently sets the caller's comment like state.
func (s *VideoService) LikeVideoComment(ctx context.Context, req *v1.LikeVideoCommentRequest) (*v1.VideoCommentInteraction, error) {
	return s.setVideoCommentLike(ctx, req.GetBvid(), req.GetCommentId(), true)
}

// UnlikeVideoComment idempotently clears the caller's comment like state.
func (s *VideoService) UnlikeVideoComment(ctx context.Context, req *v1.UnlikeVideoCommentRequest) (*v1.VideoCommentInteraction, error) {
	return s.setVideoCommentLike(ctx, req.GetBvid(), req.GetCommentId(), false)
}

// ListMyVideoComments returns the caller's authored comment history.
func (s *VideoService) ListMyVideoComments(ctx context.Context, req *v1.ListVideoHistoryRequest) (*v1.ListVideoCommentHistoryReply, error) {
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	history, err := s.videoUsecase.ListVideoCommentHistory(ctx, userID, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := &v1.ListVideoCommentHistoryReply{Items: make([]*v1.VideoCommentHistoryItem, 0, len(history.Items)), NextPageToken: history.NextPageToken}
	for index := range history.Items {
		item := &history.Items[index]
		out.Items = append(out.Items, &v1.VideoCommentHistoryItem{
			Video: convertVideoReply(&item.Video), Comment: convertVideoComment(&item.Comment),
		})
	}
	return out, nil
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

// SubmitVideoForReview attaches final metadata and enters the moderation queue.
func (s *VideoService) SubmitVideoForReview(ctx context.Context, req *v1.SubmitVideoForReviewRequest) (*v1.Video, error) {
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	video, err := s.videoUsecase.SubmitVideoForReview(ctx, &biz.VideoReviewSubmission{
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

// ListAdminVideos returns one administrator-selected lifecycle state.
func (s *VideoService) ListAdminVideos(ctx context.Context, req *v1.ListAdminVideosRequest) (*v1.ListVideosReply, error) {
	if _, err := appMiddleware.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	status, err := parseAdminVideoStatus(req.GetStatus())
	if err != nil {
		return nil, err
	}
	list, err := s.videoUsecase.ListAdminVideos(ctx, status, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return convertVideoListReply(list), nil
}

// ListPendingReviewVideos returns the administrator's oldest-first moderation queue.
func (s *VideoService) ListPendingReviewVideos(ctx context.Context, req *v1.ListPendingReviewVideosRequest) (*v1.ListVideosReply, error) {
	if _, err := appMiddleware.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	list, err := s.videoUsecase.ListPendingReviewVideos(ctx, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	return convertVideoListReply(list), nil
}

// GetAdminVideo returns non-public video detail to an administrator.
func (s *VideoService) GetAdminVideo(ctx context.Context, req *v1.GetAdminVideoRequest) (*v1.Video, error) {
	if _, err := appMiddleware.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	video, err := s.videoUsecase.GetAdminVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return convertVideoReply(video), nil
}

// GetReviewVideoPlay returns unpublished DASH metadata to an administrator.
func (s *VideoService) GetReviewVideoPlay(ctx context.Context, req *v1.GetReviewVideoPlayRequest) (*v1.VideoPlay, error) {
	if _, err := appMiddleware.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	videoID, err := biz.ParseBVID(req.GetBvid())
	if err != nil {
		return nil, err
	}
	play, err := s.videoUsecase.GetReviewVideoPlay(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return convertVideoPlayReply(play), nil
}

// ApproveVideo publishes one pending submission.
func (s *VideoService) ApproveVideo(ctx context.Context, req *v1.ApproveVideoRequest) (*v1.Video, error) {
	videoID, adminID, err := administratorVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	video, err := s.videoUsecase.ApproveVideo(ctx, adminID, videoID)
	if err != nil {
		return nil, err
	}
	return convertVideoReply(video), nil
}

// RejectVideo returns one pending submission with a visible owner-facing reason.
func (s *VideoService) RejectVideo(ctx context.Context, req *v1.RejectVideoRequest) (*v1.Video, error) {
	videoID, adminID, err := administratorVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	video, err := s.videoUsecase.RejectVideo(ctx, adminID, videoID, req.GetReason())
	if err != nil {
		return nil, err
	}
	return convertVideoReply(video), nil
}

// TakeDownVideo removes one published video from all public discovery and playback paths.
func (s *VideoService) TakeDownVideo(ctx context.Context, req *v1.TakeDownVideoRequest) (*v1.Video, error) {
	videoID, adminID, err := administratorVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	video, err := s.videoUsecase.TakeDownVideo(ctx, adminID, videoID, req.GetReason())
	if err != nil {
		return nil, err
	}
	return convertVideoReply(video), nil
}

// DeleteAdminVideo permanently removes media and records an administrator deletion decision.
func (s *VideoService) DeleteAdminVideo(ctx context.Context, req *v1.DeleteAdminVideoRequest) (*emptypb.Empty, error) {
	videoID, adminID, err := administratorVideoRequest(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	if err := s.videoUsecase.DeleteAdminVideo(ctx, adminID, videoID, req.GetReason()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
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

func (s *VideoService) setVideoCommentLike(ctx context.Context, bvid string, commentID uint64, liked bool) (*v1.VideoCommentInteraction, error) {
	videoID, userID, err := authenticatedVideoRequest(ctx, bvid)
	if err != nil {
		return nil, err
	}
	interaction, err := s.videoUsecase.SetVideoCommentLike(ctx, userID, videoID, commentID, liked)
	if err != nil {
		return nil, err
	}
	return &v1.VideoCommentInteraction{CommentId: interaction.CommentID, Liked: interaction.Liked, LikeCount: interaction.LikeCount}, nil
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
		ReviewReason:    in.ReviewReason,
		SubmittedAt:     timestampOrNil(in.SubmittedAt),
		ReviewedAt:      timestampOrNil(in.ReviewedAt),
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
	case biz.VideoStatusPendingReview:
		return v1.VideoStatus_VIDEO_STATUS_PENDING_REVIEW
	case biz.VideoStatusPublished:
		return v1.VideoStatus_VIDEO_STATUS_PUBLISHED
	case biz.VideoStatusRejected:
		return v1.VideoStatus_VIDEO_STATUS_REJECTED
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

func administratorVideoRequest(ctx context.Context, bvid string) (biz.VideoID, uint64, error) {
	videoID, err := biz.ParseBVID(bvid)
	if err != nil {
		return 0, 0, err
	}
	adminID, err := appMiddleware.RequireAdmin(ctx)
	if err != nil {
		return 0, 0, err
	}
	return videoID, adminID, nil
}

func parseAdminVideoStatus(value string) (biz.VideoStatus, error) {
	status := biz.VideoStatus(strings.TrimSpace(value))
	switch status {
	case biz.VideoStatusProcessing, biz.VideoStatusReady, biz.VideoStatusPendingReview,
		biz.VideoStatusPublished, biz.VideoStatusRejected, biz.VideoStatusFailed, biz.VideoStatusDeleted:
		return status, nil
	default:
		return "", biz.ErrVideoInvalidArgument
	}
}

func convertVideoSearchOrder(order v1.VideoSearchOrder) biz.VideoSearchOrder {
	switch order {
	case v1.VideoSearchOrder_VIDEO_SEARCH_ORDER_MOST_VIEWED:
		return biz.VideoSearchMostViewed
	case v1.VideoSearchOrder_VIDEO_SEARCH_ORDER_LATEST:
		return biz.VideoSearchLatest
	case v1.VideoSearchOrder_VIDEO_SEARCH_ORDER_MOST_DANMAKU:
		return biz.VideoSearchMostDanmaku
	case v1.VideoSearchOrder_VIDEO_SEARCH_ORDER_MOST_FAVORITED:
		return biz.VideoSearchMostFavorited
	default:
		return biz.VideoSearchRelevance
	}
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
		RootId: in.RootID, ParentId: in.ParentID,
		ReplyToUserId: in.ReplyToUserID, ReplyToUserName: in.ReplyToUserName,
		LikeCount: in.LikeCount, Liked: in.Liked, ReplyCount: in.ReplyCount, Deleted: in.Deleted,
	}
}

func convertVideoCommentList(in *biz.VideoCommentList) *v1.ListVideoCommentsReply {
	out := &v1.ListVideoCommentsReply{Comments: make([]*v1.VideoComment, 0, len(in.Comments)), NextPageToken: in.NextPageToken}
	for index := range in.Comments {
		out.Comments = append(out.Comments, convertVideoComment(&in.Comments[index]))
	}
	return out
}
