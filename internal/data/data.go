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

// Data owns the shared PostgreSQL connection used by repository implementations.
type Data struct {
	db *gorm.DB
}

// NewData opens PostgreSQL, migrates tables, and seeds the initial local user.
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
	if err := seedInitialUsers(db); err != nil {
		return nil, nil, fmt.Errorf("seed PostgreSQL users: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get PostgreSQL connection: %w", err)
	}
	data := &Data{db: db}

	cleanup := func() {
		log.Info("closing the data resources")
		if err := sqlDB.Close(); err != nil {
			log.Error("close PostgreSQL: ", err)
		}
	}
	return data, cleanup, nil
}
