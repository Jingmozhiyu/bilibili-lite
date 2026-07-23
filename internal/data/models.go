package data

import "time"

type userPO struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"size:64;not null;uniqueIndex"`
	PasswordHash string `gorm:"size:100;not null"`
	DisplayName  string `gorm:"size:100;not null"`
	AvatarURL    string `gorm:"size:500"`
	Bio          string `gorm:"size:500"`
	CoinBalance  int64  `gorm:"not null;default:1000"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (userPO) TableName() string { return "users" }

type videoPO struct {
	ID              uint64 `gorm:"primaryKey;autoIncrement"`
	OwnerID         uint64 `gorm:"not null;index"`
	Owner           userPO `gorm:"foreignKey:OwnerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Title           string `gorm:"size:200;not null"`
	Description     string `gorm:"type:text"`
	CoverURL        string `gorm:"size:500"`
	Status          string `gorm:"size:32;not null;default:published;index"`
	FailureReason   string `gorm:"type:text;not null;default:''"`
	DurationSeconds int64  `gorm:"not null;default:0"`
	ViewCount       int64  `gorm:"not null;default:0"`
	DanmakuCount    int64  `gorm:"not null;default:0"`
	LikeCount       int64  `gorm:"not null;default:0"`
	CoinCount       int64  `gorm:"not null;default:0"`
	FavoriteCount   int64  `gorm:"not null;default:0"`
	ShareCount      int64  `gorm:"not null;default:0"`
	CommentCount    int64  `gorm:"not null;default:0"`
	ReadyAt         *time.Time
	PublishTime     *time.Time
	DeletedAt       *time.Time `gorm:"index"`
	Tags            []string   `gorm:"serializer:json;type:jsonb"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (videoPO) TableName() string { return "videos" }

type videoStreamPO struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement"`
	VideoID       uint64  `gorm:"not null;index;uniqueIndex:idx_video_stream"`
	Video         videoPO `gorm:"foreignKey:VideoID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	StreamKey     string  `gorm:"size:64;not null;uniqueIndex:idx_video_stream"`
	Label         string  `gorm:"size:32;not null"`
	Codec         string  `gorm:"size:64;not null"`
	MimeType      string  `gorm:"size:100;not null"`
	URL           string  `gorm:"size:1000;not null"`
	Width         int32
	Height        int32
	Bandwidth     int32
	DefaultStream bool `gorm:"not null;default:false"`
	CreatedAt     time.Time
}

func (videoStreamPO) TableName() string { return "video_streams" }

type danmakuPO struct {
	ID          uint64  `gorm:"primaryKey;autoIncrement"`
	VideoID     uint64  `gorm:"not null;index"`
	Video       videoPO `gorm:"foreignKey:VideoID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UserID      *uint64 `gorm:"index"`
	User        *userPO `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
	TimeSeconds float64 `gorm:"not null"`
	Text        string  `gorm:"size:500;not null"`
	Color       string  `gorm:"size:16;not null;default:#ffffff"`
	CreatedAt   time.Time
}

func (danmakuPO) TableName() string { return "danmakus" }

type videoLikePO struct {
	ID        uint64  `gorm:"primaryKey;autoIncrement"`
	UserID    uint64  `gorm:"not null;uniqueIndex:idx_user_video_like"`
	User      userPO  `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	VideoID   uint64  `gorm:"not null;uniqueIndex:idx_user_video_like;index"`
	Video     videoPO `gorm:"foreignKey:VideoID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Active    bool    `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (videoLikePO) TableName() string { return "user_video_likes" }

type videoFavoritePO struct {
	ID        uint64  `gorm:"primaryKey;autoIncrement"`
	UserID    uint64  `gorm:"not null;uniqueIndex:idx_user_video_favorite"`
	User      userPO  `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	VideoID   uint64  `gorm:"not null;uniqueIndex:idx_user_video_favorite;index"`
	Video     videoPO `gorm:"foreignKey:VideoID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Active    bool    `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (videoFavoritePO) TableName() string { return "user_video_favorites" }

type videoCoinPO struct {
	ID        uint64  `gorm:"primaryKey;autoIncrement"`
	UserID    uint64  `gorm:"not null;index;uniqueIndex:idx_user_video_coin"`
	User      userPO  `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	VideoID   uint64  `gorm:"not null;index;uniqueIndex:idx_user_video_coin"`
	Video     videoPO `gorm:"foreignKey:VideoID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Amount    int32   `gorm:"not null;check:amount >= 1 AND amount <= 2"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (videoCoinPO) TableName() string { return "user_video_coins" }

type videoSharePO struct {
	ID        uint64  `gorm:"primaryKey;autoIncrement"`
	UserID    uint64  `gorm:"not null;index;uniqueIndex:idx_user_video_share_request"`
	User      userPO  `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	VideoID   uint64  `gorm:"not null;index;uniqueIndex:idx_user_video_share_request"`
	Video     videoPO `gorm:"foreignKey:VideoID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	RequestID string  `gorm:"size:64;not null;uniqueIndex:idx_user_video_share_request"`
	CreatedAt time.Time
}

func (videoSharePO) TableName() string { return "user_video_shares" }

type videoCommentPO struct {
	ID            uint64          `gorm:"primaryKey;autoIncrement"`
	VideoID       uint64          `gorm:"not null;index"`
	Video         videoPO         `gorm:"foreignKey:VideoID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UserID        uint64          `gorm:"not null;index"`
	User          userPO          `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	RootID        *uint64         `gorm:"index"`
	Root          *videoCommentPO `gorm:"foreignKey:RootID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ParentID      *uint64         `gorm:"index"`
	Parent        *videoCommentPO `gorm:"foreignKey:ParentID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ReplyToUserID *uint64         `gorm:"index"`
	ReplyToUser   *userPO         `gorm:"foreignKey:ReplyToUserID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
	Content       string          `gorm:"size:2000;not null"`
	LikeCount     int64           `gorm:"not null;default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time `gorm:"index"`
}

func (videoCommentPO) TableName() string { return "video_comments" }

type videoCommentLikePO struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	UserID    uint64         `gorm:"not null;uniqueIndex:idx_user_comment_like"`
	User      userPO         `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	CommentID uint64         `gorm:"not null;uniqueIndex:idx_user_comment_like;index"`
	Comment   videoCommentPO `gorm:"foreignKey:CommentID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Active    bool           `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (videoCommentLikePO) TableName() string { return "user_video_comment_likes" }

type videoViewSessionPO struct {
	ID          string  `gorm:"size:64;primaryKey"`
	VideoID     uint64  `gorm:"not null;index:idx_view_session_video_user"`
	Video       videoPO `gorm:"foreignKey:VideoID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	UserID      uint64  `gorm:"not null;index:idx_view_session_video_user"`
	User        userPO  `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	StartedAt   time.Time
	CompletedAt *time.Time `gorm:"index"`
	Counted     bool       `gorm:"not null;default:false"`
	CreatedAt   time.Time
}

func (videoViewSessionPO) TableName() string { return "video_view_sessions" }
