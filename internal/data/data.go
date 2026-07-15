package data

import (
	"fmt"

	"bilibili-lite/internal/conf"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewVideoRepo, NewUserRepo)

// Data .
type Data struct {
	db *gorm.DB
}

// NewData .
func NewData(c *conf.Data) (*Data, func(), error) {
	if c == nil || c.Database == nil {
		return nil, nil, fmt.Errorf("database configuration is required")
	}
	if c.Database.Driver != "postgres" && c.Database.Driver != "postgresql" {
		return nil, nil, fmt.Errorf("unsupported database driver %q", c.Database.Driver)
	}

	db, err := gorm.Open(postgres.Open(c.Database.Source), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, nil, fmt.Errorf("migrate PostgreSQL: %w", err)
	}
	if err := seedInitialData(db); err != nil {
		return nil, nil, fmt.Errorf("seed PostgreSQL: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get PostgreSQL connection: %w", err)
	}
	cleanup := func() {
		log.Info("closing the data resources")
		if err := sqlDB.Close(); err != nil {
			log.Error("close PostgreSQL: ", err)
		}
	}
	return &Data{db: db}, cleanup, nil
}

func migrate(db *gorm.DB) error {
	if err := db.Exec("CREATE SEQUENCE IF NOT EXISTS video_bvid_seq START WITH 1").Error; err != nil {
		return err
	}
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
	if err := db.Exec("DROP TABLE IF EXISTS user_sessions").Error; err != nil {
		return err
	}
	return nil
}
