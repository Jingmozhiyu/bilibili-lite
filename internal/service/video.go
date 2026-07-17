package service

import (
	"context"

	v1 "bilibili-lite/api/video/v1"
	"bilibili-lite/internal/biz"

	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// VideoService adapts video API requests to domain usecases.
type VideoService struct {
	v1.UnimplementedVideoServiceServer

	videoUsecase *biz.VideoUsecase
	userUsecase  *biz.UserUsecase
}

// NewVideoService injects the video and authentication usecases.
func NewVideoService(videoUsecase *biz.VideoUsecase, userUsecase *biz.UserUsecase) *VideoService {
	return &VideoService{videoUsecase: videoUsecase, userUsecase: userUsecase}
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
	userID, err := s.authenticatedUserID(ctx)
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

// setVideoLike translates authenticated transport state into the shared like usecase operation.
func (s *VideoService) setVideoLike(ctx context.Context, bvid string, liked bool) (*v1.VideoLike, error) {
	videoID, err := biz.ParseBVID(bvid)
	if err != nil {
		return nil, err
	}
	userID, err := s.authenticatedUserID(ctx)
	if err != nil {
		return nil, err
	}
	like, err := s.videoUsecase.SetVideoLike(ctx, userID, videoID, liked)
	if err != nil {
		return nil, err
	}
	return convertVideoLike(like), nil
}

// authenticatedUserID extracts and validates the access JWT from the current transport request.
func (s *VideoService) authenticatedUserID(ctx context.Context) (uint64, error) {
	if tr, ok := transport.FromServerContext(ctx); ok {
		return s.userUsecase.AuthenticateAccess(parseBearerToken(tr.RequestHeader().Get("Authorization")))
	}
	return 0, biz.ErrSessionInvalid
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
		PublishTime:     timestamppb.New(in.PublishTime),
		Tags:            append([]string(nil), in.Tags...),
	}
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
		out.Danmaku.Items = append(out.Danmaku.Items, &v1.DanmakuItem{
			TimeSeconds: item.TimeSeconds,
			Text:        item.Text,
			Color:       item.Color,
		})
	}
	return out
}

// convertVideoLike maps the like domain object to its public API reply.
func convertVideoLike(in *biz.VideoLike) *v1.VideoLike {
	return &v1.VideoLike{Bvid: in.VideoID.BVID(), Liked: in.Liked, LikeCount: in.LikeCount}
}
