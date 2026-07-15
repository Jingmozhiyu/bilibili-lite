package service

import (
	"context"

	v1 "bilibili-lite/api/video/v1"
	"bilibili-lite/internal/biz"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// VideoService is a video service.
type VideoService struct {
	v1.UnimplementedVideoServiceServer

	uc *biz.VideoUsecase
}

// NewVideoService new a video service.
func NewVideoService(uc *biz.VideoUsecase) *VideoService {
	return &VideoService{uc: uc}
}

// GetVideo returns video detail by BVID.
func (s *VideoService) GetVideo(ctx context.Context, req *v1.GetVideoRequest) (*v1.Video, error) {
	video, err := s.uc.GetVideo(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	return convertVideoReply(video), nil
}

// GetVideoPlay returns playback metadata by BVID.
func (s *VideoService) GetVideoPlay(ctx context.Context, req *v1.GetVideoPlayRequest) (*v1.VideoPlay, error) {
	play, err := s.uc.GetVideoPlay(ctx, req.GetBvid())
	if err != nil {
		return nil, err
	}
	return convertVideoPlayReply(play), nil
}

func convertVideoReply(in *biz.Video) *v1.Video {
	if in == nil {
		return nil
	}
	return &v1.Video{
		Bvid:            in.BVID,
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

func convertVideoPlayReply(in *biz.VideoPlay) *v1.VideoPlay {
	if in == nil {
		return nil
	}
	out := &v1.VideoPlay{
		Bvid:    in.BVID,
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
