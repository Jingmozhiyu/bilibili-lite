package data

import (
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const seedPassword = "demo123456"

func seedInitialData(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended('bilibili-lite-seed', 0))").Error; err != nil {
			return fmt.Errorf("lock seed initialization: %w", err)
		}
		users, err := seedUsers(tx)
		if err != nil {
			return err
		}
		var videoCount int64
		if err := tx.Model(&videoPO{}).Count(&videoCount).Error; err != nil || videoCount > 0 {
			return err
		}
		video, err := seedVideo(tx, users["up-one"], users["viewer"])
		if err != nil {
			return err
		}
		if err := seedEngagementHistory(tx, users, video.BVID); err != nil {
			return err
		}
		return refreshEngagementCounts(tx, video.BVID)
	})
}

func seedUsers(db *gorm.DB) (map[string]userPO, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(seedPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash seed password: %w", err)
	}
	seeds := []userPO{
		{Username: "demo", PasswordHash: string(passwordHash), DisplayName: "演示用户", Bio: "bilibili-lite 本地演示账号"},
		{Username: "up-one", PasswordHash: string(passwordHash), DisplayName: "轻量放映室", Bio: "分享本地视频和开发记录"},
		{Username: "viewer", PasswordHash: string(passwordHash), DisplayName: "普通观众", Bio: "正在体验视频详情页"},
	}
	users := make(map[string]userPO, len(seeds))
	for _, seed := range seeds {
		candidate := seed
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "username"}},
			DoNothing: true,
		}).Create(&candidate).Error; err != nil {
			return nil, fmt.Errorf("seed user %s: %w", seed.Username, err)
		}
		var user userPO
		if err := db.Where("username = ?", seed.Username).First(&user).Error; err != nil {
			return nil, fmt.Errorf("load seed user %s: %w", seed.Username, err)
		}
		users[user.Username] = user
	}
	return users, nil
}

func seedVideo(db *gorm.DB, owner, danmakuUser userPO) (videoPO, error) {
	video := videoPO{
		OwnerID:         owner.ID,
		Title:           "第一支本地视频：播放器链路跑通",
		Description:     "从视频详情接口到媒体 Range 请求的完整演示。",
		DurationSeconds: 187,
		ViewCount:       126,
		PublishTime:     time.Now(),
		Tags:            []string{"本地视频", "播放器", "Kratos"},
	}
	if err := db.Create(&video).Error; err != nil {
		return video, fmt.Errorf("seed video: %w", err)
	}
	return video, ensureSeedMedia(db, video.BVID, danmakuUser.ID)
}

func ensureSeedMedia(db *gorm.DB, bvid string, userID uint64) error {
	stream := videoStreamPO{
		VideoBVID:     bvid,
		StreamKey:     "local-1080p-avc",
		Label:         "1080P",
		Codec:         "avc1",
		MimeType:      "video/mp4",
		URL:           "/media/videos/test.MP4",
		Width:         1920,
		Height:        1080,
		Bandwidth:     4200000,
		DefaultStream: true,
	}
	if err := db.Where("video_bvid = ? AND stream_key = ?", bvid, stream.StreamKey).FirstOrCreate(&videoStreamPO{}, stream).Error; err != nil {
		return fmt.Errorf("seed video stream: %w", err)
	}

	var count int64
	if err := db.Model(&danmakuPO{}).Where("video_bvid = ?", bvid).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	return db.Create([]danmakuPO{
		{VideoBVID: bvid, UserID: &userID, TimeSeconds: 1.2, Text: "来了来了", Color: "#ffffff"},
		{VideoBVID: bvid, UserID: &userID, TimeSeconds: 3.8, Text: "数据库版本也跑起来了", Color: "#00aeec"},
		{VideoBVID: bvid, UserID: &userID, TimeSeconds: 7.4, Text: "播放器初始化成功", Color: "#ffffff"},
	}).Error
}

func seedEngagementHistory(db *gorm.DB, users map[string]userPO, bvid string) error {
	var count int64
	if err := db.Model(&videoLikePO{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	demoID := users["demo"].ID
	viewerID := users["viewer"].ID
	if err := db.Create([]videoLikePO{
		{UserID: demoID, VideoBVID: bvid, Active: true},
		{UserID: viewerID, VideoBVID: bvid, Active: true},
	}).Error; err != nil {
		return err
	}
	if err := db.Create(&videoFavoritePO{UserID: demoID, VideoBVID: bvid, Active: true}).Error; err != nil {
		return err
	}
	if err := db.Create(&videoCoinPO{UserID: demoID, VideoBVID: bvid, Amount: 2}).Error; err != nil {
		return err
	}
	return db.Create(&videoSharePO{UserID: viewerID, VideoBVID: bvid}).Error
}

func refreshEngagementCounts(db *gorm.DB, bvid string) error {
	var likeCount, favoriteCount, shareCount, danmakuCount int64
	var coinCount int64
	if err := db.Model(&videoLikePO{}).Where("video_bvid = ? AND active = ?", bvid, true).Count(&likeCount).Error; err != nil {
		return err
	}
	if err := db.Model(&videoFavoritePO{}).Where("video_bvid = ? AND active = ?", bvid, true).Count(&favoriteCount).Error; err != nil {
		return err
	}
	if err := db.Model(&videoSharePO{}).Where("video_bvid = ?", bvid).Count(&shareCount).Error; err != nil {
		return err
	}
	if err := db.Model(&danmakuPO{}).Where("video_bvid = ?", bvid).Count(&danmakuCount).Error; err != nil {
		return err
	}
	if err := db.Model(&videoCoinPO{}).Where("video_bvid = ?", bvid).Select("COALESCE(SUM(amount), 0)").Scan(&coinCount).Error; err != nil {
		return err
	}
	return db.Model(&videoPO{}).Where("bvid = ?", bvid).Updates(map[string]any{
		"like_count":     likeCount,
		"coin_count":     coinCount,
		"favorite_count": favoriteCount,
		"share_count":    shareCount,
		"danmaku_count":  danmakuCount,
	}).Error
}
