package data

import (
	"context"
	"errors"

	"bilibili-lite/internal/biz"

	"gorm.io/gorm"
)

type userRepo struct {
	data *Data
}

func NewUserRepo(data *Data) biz.UserRepo {
	return &userRepo{data: data}
}

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
		},
		PasswordHash: user.PasswordHash,
	}, nil
}

func (r *userRepo) FindUserByID(ctx context.Context, id uint64) (*biz.User, error) {
	var user userPO
	err := r.data.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrSessionInvalid
	}
	if err != nil {
		return nil, biz.ErrUserStorage
	}
	return &biz.User{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		AvatarURL: user.AvatarURL, Bio: user.Bio,
	}, nil
}
