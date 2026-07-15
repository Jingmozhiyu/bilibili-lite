package data

import (
	"context"
	"errors"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
)

type videoRepo struct {
	data *Data
}

// NewVideoRepo creates a PostgreSQL-backed VideoRepo.
func NewVideoRepo(data *Data) biz.VideoRepo {
	return &videoRepo{data: data}
}

func (r *videoRepo) FindVideoByBVID(ctx context.Context, bvid string) (*biz.Video, error) {
	var record videoPO
	err := r.data.db.WithContext(ctx).
		Preload("Owner").
		Where("bvid = ?", bvid).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	}
	if err != nil {
		return nil, biz.ErrVideoStorage
	}
	return record.toBizVideo(), nil
}

func (r *videoRepo) FindVideoPlayByBVID(ctx context.Context, bvid string) (*biz.VideoPlay, error) {
	var record videoPO
	if err := r.data.db.WithContext(ctx).Where("bvid = ?", bvid).First(&record).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrVideoNotFound
	} else if err != nil {
		return nil, biz.ErrVideoStorage
	}

	var streams []videoStreamPO
	if err := r.data.db.WithContext(ctx).Where("video_bvid = ?", record.BVID).Order("id ASC").Find(&streams).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}
	var danmakus []danmakuPO
	if err := r.data.db.WithContext(ctx).Where("video_bvid = ?", record.BVID).Order("time_seconds ASC").Find(&danmakus).Error; err != nil {
		return nil, biz.ErrVideoStorage
	}

	return toBizVideoPlay(record, streams, danmakus), nil
}

func (v videoPO) toBizVideo() *biz.Video {
	return &biz.Video{
		BVID:            v.BVID,
		Title:           v.Title,
		Description:     v.Description,
		OwnerName:       v.Owner.DisplayName,
		OwnerAvatarURL:  v.Owner.AvatarURL,
		CoverURL:        v.CoverURL,
		DurationSeconds: v.DurationSeconds,
		ViewCount:       v.ViewCount,
		DanmakuCount:    v.DanmakuCount,
		LikeCount:       v.LikeCount,
		CoinCount:       v.CoinCount,
		FavoriteCount:   v.FavoriteCount,
		ShareCount:      v.ShareCount,
		PublishTime:     v.PublishTime,
		Tags:            append([]string(nil), v.Tags...),
	}
}

func toBizVideoPlay(video videoPO, streams []videoStreamPO, danmakus []danmakuPO) *biz.VideoPlay {
	out := &biz.VideoPlay{
		BVID:    video.BVID,
		Streams: make([]biz.VideoStream, 0, len(streams)),
		Danmaku: biz.DanmakuConfig{
			Enabled: true,
			Format:  "inline",
			Items:   make([]biz.DanmakuItem, 0, len(danmakus)),
		},
	}
	for _, stream := range streams {
		out.Streams = append(out.Streams, biz.VideoStream{
			ID:            stream.StreamKey,
			Label:         stream.Label,
			Codec:         stream.Codec,
			MimeType:      stream.MimeType,
			URL:           stream.URL,
			Width:         stream.Width,
			Height:        stream.Height,
			Bandwidth:     stream.Bandwidth,
			DefaultStream: stream.DefaultStream,
		})
	}
	for _, danmaku := range danmakus {
		out.Danmaku.Items = append(out.Danmaku.Items, biz.DanmakuItem{
			TimeSeconds: danmaku.TimeSeconds,
			Text:        danmaku.Text,
			Color:       danmaku.Color,
		})
	}
	return out
}
