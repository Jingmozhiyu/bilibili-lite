package biz

import (
	"context"
	"testing"
)

type userRepoStub struct {
	user   *User
	update UserProfileUpdate
}

func (r *userRepoStub) FindCredentialByUsername(context.Context, string) (*UserCredential, error) {
	return nil, ErrInvalidCredentials
}

func (r *userRepoStub) FindUserByID(context.Context, uint64) (*User, error) {
	copy := *r.user
	return &copy, nil
}

func (r *userRepoStub) UpdateUserProfile(_ context.Context, _ uint64, update UserProfileUpdate) (*User, error) {
	r.update = update
	copy := *r.user
	copy.DisplayName = update.DisplayName
	copy.AvatarURL = update.AvatarURL
	copy.Bio = update.Bio
	return &copy, nil
}

func TestGetUserHidesCoinBalance(t *testing.T) {
	t.Parallel()
	repo := &userRepoStub{user: &User{ID: 1, Username: "demo", CoinBalance: 1000}}
	user, err := NewUserUsecase(repo, nil).GetUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if user.CoinBalance != 0 {
		t.Fatalf("GetUser() CoinBalance = %d, want 0", user.CoinBalance)
	}
	if repo.user.CoinBalance != 1000 {
		t.Fatal("GetUser() mutated the repository model")
	}
}

func TestUpdateMeNormalizesProfile(t *testing.T) {
	t.Parallel()
	repo := &userRepoStub{user: &User{ID: 1, Username: "demo", CoinBalance: 1000}}
	user, err := NewUserUsecase(repo, nil).UpdateMe(context.Background(), 1, UserProfileUpdate{
		DisplayName: "  新昵称  ", AvatarURL: "  https://example.com/avatar.png  ", Bio: "  简介  ",
	})
	if err != nil {
		t.Fatalf("UpdateMe() error = %v", err)
	}
	if user.DisplayName != "新昵称" || repo.update.AvatarURL != "https://example.com/avatar.png" || repo.update.Bio != "简介" {
		t.Fatalf("UpdateMe() normalized update = %+v", repo.update)
	}
}

func TestUpdateMeRejectsEmptyDisplayName(t *testing.T) {
	t.Parallel()
	repo := &userRepoStub{user: &User{ID: 1}}
	if _, err := NewUserUsecase(repo, nil).UpdateMe(context.Background(), 1, UserProfileUpdate{DisplayName: "   "}); err == nil {
		t.Fatal("UpdateMe() unexpectedly accepted an empty display name")
	}
}
