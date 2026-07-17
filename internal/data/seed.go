package data

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const seedPassword = "demo123456"

// seedInitialUsers keeps local authentication usable without creating any video records.
func seedInitialUsers(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended('bilibili-lite-user-seed', 0))").Error; err != nil {
			return fmt.Errorf("lock user seed initialization: %w", err)
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(seedPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash seed password: %w", err)
		}
		users := []userPO{
			{Username: "demo", PasswordHash: string(passwordHash), DisplayName: "演示用户", Bio: "bilibili-lite 本地演示账号"},
			{Username: "up-one", PasswordHash: string(passwordHash), DisplayName: "轻量放映室", Bio: "分享本地视频和开发记录"},
			{Username: "viewer", PasswordHash: string(passwordHash), DisplayName: "普通观众", Bio: "正在体验视频详情页"},
		}
		for index := range users {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "username"}},
				DoNothing: true,
			}).Create(&users[index]).Error; err != nil {
				return fmt.Errorf("seed user %s: %w", users[index].Username, err)
			}
		}
		return nil
	})
}
