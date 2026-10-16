package data

import (
	"embed"
	"fmt"
	"time"

	"bilibili-lite/internal/mall/admission"
	"bilibili-lite/internal/mall/conf"
	"bilibili-lite/internal/mall/observability"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var ProviderSet = wire.NewSet(NewData, NewMallRepo, NewProtection)

//go:embed migrations/*.sql
var migrations embed.FS

type Data struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewData(config *conf.Config, metrics *observability.Metrics) (*Data, func(), error) {
	db, err := gorm.Open(postgres.Open(config.DSN), &gorm.Config{})
	if err != nil {
		return nil, nil, err
	}
	sql, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	client := redis.NewClient(&redis.Options{
		Addr:                  config.RedisAddress,
		Password:              config.RedisPassword,
		MaxRetries:            -1,
		DialTimeout:           time.Second,
		ReadTimeout:           time.Second,
		WriteTimeout:          time.Second,
		ContextTimeoutEnabled: true,
	})
	cleanup := func() {
		_ = client.Close()
		_ = sql.Close()
	}
	sql.SetMaxOpenConns(20)
	sql.SetMaxIdleConns(5)
	if err = migrate(db); err != nil {
		cleanup()
		return nil, nil, err
	}
	if config.SeedEnabled {
		if err = seedCatalogue(db); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	data := &Data{db: db, redis: client}
	metrics.Registry.MustRegister(newStorageCollector(data, sql))
	return data, cleanup, nil
}

func NewProtection(d *Data, config *conf.Config) *admission.Protection {
	return admission.New(d.redis, config)
}

func migrate(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x4d414c4c)).Error; err != nil {
			return err
		}
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS mall_schema_migrations(version TEXT PRIMARY KEY)").Error; err != nil {
			return err
		}
		entries, err := migrations.ReadDir("migrations")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			var n int64
			if err = tx.Raw("SELECT count(*) FROM mall_schema_migrations WHERE version=?", entry.Name()).Scan(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				continue
			}
			script, err := migrations.ReadFile("migrations/" + entry.Name())
			if err != nil {
				return err
			}
			if err = tx.Exec(string(script)).Error; err != nil {
				return fmt.Errorf("migration %s: %w", entry.Name(), err)
			}
			if err = tx.Exec("INSERT INTO mall_schema_migrations VALUES (?)", entry.Name()).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
