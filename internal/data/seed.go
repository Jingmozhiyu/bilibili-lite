package data

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	seedAdminUsername = "admin"
)

// seedInitialUsers keeps configured demo authentication usable and initializes the administrator account.
func seedInitialUsers(db *gorm.DB, password string) error {
	if len(password) < 10 || len(password) > 72 {
		return fmt.Errorf("seed password must contain between 10 and 72 bytes")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended('bilibili-lite-user-seed', 0))").Error; err != nil {
			return fmt.Errorf("lock user seed initialization: %w", err)
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash seed password: %w", err)
		}
		users := []userPO{
			{Username: "demo", PasswordHash: string(passwordHash), DisplayName: "演示用户", Bio: "bilibili-lite 本地演示账号", CoinBalance: 1000},
			{Username: "up-one", PasswordHash: string(passwordHash), DisplayName: "轻量放映室", Bio: "分享本地视频和开发记录", CoinBalance: 1000},
			{Username: "viewer", PasswordHash: string(passwordHash), DisplayName: "普通观众", Bio: "正在体验视频详情页", CoinBalance: 1000},
		}
		for index := range users {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "username"}},
				DoNothing: true,
			}).Create(&users[index]).Error; err != nil {
				return fmt.Errorf("seed user %s: %w", users[index].Username, err)
			}
		}
		return seedAdministrator(tx, string(passwordHash))
	})
}

// seedAdministrator creates the local administrator and repairs an existing same-name account's role.
func seedAdministrator(tx *gorm.DB, passwordHash string) error {
	admin := userPO{
		Username:     seedAdminUsername,
		PasswordHash: passwordHash,
		DisplayName:  "内容管理员",
		Bio:          "负责本地演示内容审核",
		CoinBalance:  1000,
		Role:         "admin",
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}},
		DoUpdates: clause.Assignments(map[string]any{"role": "admin"}),
	}).Create(&admin).Error; err != nil {
		return fmt.Errorf("seed administrator %s: %w", admin.Username, err)
	}
	return nil
}
