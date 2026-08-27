package data

import (
	"context"
	"errors"
	"time"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type userRepo struct {
	data *Data
}

// NewUserRepo creates a PostgreSQL-backed UserRepo.
func NewUserRepo(data *Data) biz.UserRepo {
	return &userRepo{data: data}
}

// CreateUser atomically inserts one ordinary account without racing the unique username constraint.
func (r *userRepo) CreateUser(ctx context.Context, input biz.UserRegistration) (*biz.User, error) {
	record := userPO{
		Username: input.Username, PasswordHash: input.PasswordHash, DisplayName: input.DisplayName,
		CoinBalance: 1000, Role: "user",
	}
	result := r.data.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "username"}}, DoNothing: true,
	}).Create(&record)
	if result.Error != nil {
		return nil, biz.ErrUserStorage
	}
	if result.RowsAffected != 1 {
		return nil, biz.ErrUserAlreadyExists
	}
	return &biz.User{
		ID: record.ID, Username: record.Username, DisplayName: record.DisplayName,
		CoinBalance: record.CoinBalance,
	}, nil
}

// FindCredentialByUsername loads the password hash and public profile needed by the login usecase.
func (r *userRepo) FindCredentialByUsername(ctx context.Context, username string) (*biz.UserCredential, error) {
	var user userPO
	err := r.data.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrInvalidCredentials
	}
	if err != nil {
		return nil, biz.ErrUserStorage
	}
	return &biz.UserCredential{
		User: biz.User{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			AvatarURL:   user.AvatarURL,
			Bio:         user.Bio,
			CoinBalance: user.CoinBalance,
			Experience:  user.Experience,
			IsAdmin:     user.Role == "admin",
		},
		PasswordHash: user.PasswordHash,
	}, nil
}

// FindUserByID reloads the current public profile while refreshing an authenticated session.
func (r *userRepo) FindUserByID(ctx context.Context, id uint64) (*biz.User, error) {
	var user userPO
	err := r.data.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrUserNotFound
	}
	if err != nil {
		return nil, biz.ErrUserStorage
	}
	return &biz.User{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		AvatarURL: user.AvatarURL, Bio: user.Bio, CoinBalance: user.CoinBalance,
		Experience: user.Experience,
		IsAdmin:    user.Role == "admin",
	}, nil
}

// UpdateUserProfile persists the caller's complete editable profile in one update.
func (r *userRepo) UpdateUserProfile(ctx context.Context, id uint64, update biz.UserProfileUpdate) (*biz.User, error) {
	var user userPO
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&user, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&user).Updates(map[string]any{
			"display_name": update.DisplayName,
			"bio":          update.Bio,
		}).Error; err != nil {
			return err
		}
		return tx.First(&user, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrUserNotFound
	}
	if err != nil {
		return nil, biz.ErrUserStorage
	}
	return &biz.User{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		AvatarURL: user.AvatarURL, Bio: user.Bio, CoinBalance: user.CoinBalance,
		Experience: user.Experience,
		IsAdmin:    user.Role == "admin",
	}, nil
}

// UpdateUserAvatar serializes avatar replacement and returns the previous URL for file cleanup.
func (r *userRepo) UpdateUserAvatar(ctx context.Context, id uint64, avatarURL string) (*biz.User, string, error) {
	var user userPO
	var previous string
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, id).Error; err != nil {
			return err
		}
		previous = user.AvatarURL
		if err := tx.Model(&user).Update("avatar_url", avatarURL).Error; err != nil {
			return err
		}
		user.AvatarURL = avatarURL
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", biz.ErrUserNotFound
	}
	if err != nil {
		return nil, "", biz.ErrUserStorage
	}
	return &biz.User{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		AvatarURL: user.AvatarURL, Bio: user.Bio, CoinBalance: user.CoinBalance,
		Experience: user.Experience,
		IsAdmin:    user.Role == "admin",
	}, previous, nil
}

// GrantDailyExperience awards one capped experience source using the Shanghai calendar day.
func (r *userRepo) GrantDailyExperience(ctx context.Context, userID uint64, source string, amount, dailyLimit int32) (int64, error) {
	var experience int64
	err := r.data.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		experience, err = grantDailyExperience(tx, userID, source, amount, dailyLimit, time.Now())
		return err
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, biz.ErrUserNotFound
	}
	if err != nil {
		return 0, biz.ErrUserStorage
	}
	return experience, nil
}
