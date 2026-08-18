package data

import (
	"fmt"
	"strings"

	"bilibili-lite/internal/conf"

	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewVideoRepo, NewUserRepo)

// Data owns the shared PostgreSQL connection used by repository implementations.
type Data struct {
	db              *gorm.DB
	videoSearch     videoSearchIndex
	redis           *redis.Client
	videoRankingKey string
}

// NewData opens PostgreSQL, migrates tables, and optionally seeds configured demo accounts.
func NewData(dataConfig *conf.Data) (*Data, func(), error) {
	if dataConfig == nil || dataConfig.Database == nil {
		return nil, nil, fmt.Errorf("database configuration is required")
	}
	if dataConfig.Database.Driver != "postgres" && dataConfig.Database.Driver != "postgresql" {
		return nil, nil, fmt.Errorf("unsupported database driver %q", dataConfig.Database.Driver)
	}
	db, err := gorm.Open(postgres.Open(dataConfig.Database.Source), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	if err := migratePostgresSchema(db); err != nil {
		return nil, nil, fmt.Errorf("migrate PostgreSQL: %w", err)
	}
	if seedConfig := dataConfig.GetSeed(); seedConfig.GetEnabled() {
		if err := seedInitialUsers(db, seedConfig.GetPassword()); err != nil {
			return nil, nil, fmt.Errorf("seed PostgreSQL accounts: %w", err)
		}
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get PostgreSQL connection: %w", err)
	}
	data := &Data{db: db, videoSearch: newVideoSearchIndex(dataConfig.GetSearch())}
	if redisConfig := dataConfig.GetRedis(); redisConfig != nil && strings.TrimSpace(redisConfig.GetAddress()) != "" {
		data.redis = redis.NewClient(&redis.Options{
			Addr: redisConfig.GetAddress(), Password: redisConfig.GetPassword(), DB: int(redisConfig.GetDatabase()),
		})
		data.videoRankingKey = redisConfig.GetVideoRankingKey()
	}

	cleanup := func() {
		log.Info("closing the data resources")
		if data.redis != nil {
			if err := data.redis.Close(); err != nil {
				log.Error("close Redis: ", err)
			}
		}
		if err := sqlDB.Close(); err != nil {
			log.Error("close PostgreSQL: ", err)
		}
	}
	return data, cleanup, nil
}
