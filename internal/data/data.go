package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bilibili-lite/internal/conf"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewVideoRepo, NewUserRepo)

// Data owns the shared database connection and local media storage configuration.
type Data struct {
	db                *gorm.DB
	mediaRoot         string
	uploadIdleTimeout time.Duration
	transcodeTimeout  time.Duration
	maxUploadBytes    int64
	janitorCancel     context.CancelFunc
	janitorWG         sync.WaitGroup
}

// NewData opens PostgreSQL, prepares DASH storage, migrates tables, seeds users, and starts upload cleanup.
func NewData(dataConfig *conf.Data) (*Data, func(), error) {
	if dataConfig == nil || dataConfig.Database == nil {
		return nil, nil, fmt.Errorf("database configuration is required")
	}
	if dataConfig.Database.Driver != "postgres" && dataConfig.Database.Driver != "postgresql" {
		return nil, nil, fmt.Errorf("unsupported database driver %q", dataConfig.Database.Driver)
	}
	if dataConfig.Media == nil || dataConfig.Media.StorageDir == "" {
		return nil, nil, fmt.Errorf("media storage configuration is required")
	}
	uploadIdleTimeout := dataConfig.Media.UploadIdleTimeout.AsDuration()
	transcodeTimeout := dataConfig.Media.TranscodeTimeout.AsDuration()
	if uploadIdleTimeout <= 0 || transcodeTimeout <= 0 || dataConfig.Media.MaxUploadBytes <= 0 {
		return nil, nil, fmt.Errorf("media timeout and size limits must be positive")
	}
	mediaRoot, err := filepath.Abs(dataConfig.Media.StorageDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve media storage path: %w", err)
	}
	for _, dir := range []string{mediaRoot, filepath.Join(mediaRoot, ".uploads"), filepath.Join(mediaRoot, "dash")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("create media storage %s: %w", dir, err)
		}
	}

	db, err := gorm.Open(postgres.Open(dataConfig.Database.Source), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	if err := migratePostgresSchema(db); err != nil {
		return nil, nil, fmt.Errorf("migrate PostgreSQL: %w", err)
	}
	if err := seedInitialUsers(db); err != nil {
		return nil, nil, fmt.Errorf("seed PostgreSQL users: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get PostgreSQL connection: %w", err)
	}
	janitorCtx, janitorCancel := context.WithCancel(context.Background())
	data := &Data{
		db: db, mediaRoot: mediaRoot,
		uploadIdleTimeout: uploadIdleTimeout,
		transcodeTimeout:  transcodeTimeout,
		maxUploadBytes:    dataConfig.Media.MaxUploadBytes,
		janitorCancel:     janitorCancel,
	}
	data.startUploadJanitor(janitorCtx)

	cleanup := func() {
		log.Info("closing the data resources")
		data.janitorCancel()
		data.janitorWG.Wait()
		if err := sqlDB.Close(); err != nil {
			log.Error("close PostgreSQL: ", err)
		}
	}
	return data, cleanup, nil
}

// migratePostgresSchema creates the PostgreSQL tables required by the current models.
func migratePostgresSchema(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&userPO{},
		&videoPO{},
		&videoStreamPO{},
		&danmakuPO{},
		&videoLikePO{},
		&videoFavoritePO{},
		&videoCoinPO{},
		&videoSharePO{},
	); err != nil {
		return err
	}
	return nil
}
