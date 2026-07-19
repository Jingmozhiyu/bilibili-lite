package data

import "gorm.io/gorm"

// migratePostgresSchema creates the PostgreSQL tables required by the current persistence models.
func migratePostgresSchema(db *gorm.DB) error {
	return db.AutoMigrate(
		&userPO{},
		&videoPO{},
		&videoStreamPO{},
		&danmakuPO{},
		&videoLikePO{},
		&videoFavoritePO{},
		&videoCoinPO{},
		&videoSharePO{},
	)
}
