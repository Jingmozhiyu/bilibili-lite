package biz

import (
	"context"
	"testing"
)

type userRepoStub struct {
	user   *User
	update UserProfileUpdate
}

func (r *userRepoStub) GrantDailyExperience(_ context.Context, _ uint64, _ string, amount, _ int32) (int64, error) {
	r.user.Experience += int64(amount)
	return r.user.Experience, nil
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
	copy.Bio = update.Bio
	return &copy, nil
}

func (r *userRepoStub) UpdateUserAvatar(_ context.Context, _ uint64, avatarURL string) (*User, string, error) {
	copy := *r.user
	previous := copy.AvatarURL
	copy.AvatarURL = avatarURL
	return &copy, previous, nil
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
		DisplayName: "  新昵称  ", Bio: "  简介  ",
	})
	if err != nil {
		t.Fatalf("UpdateMe() error = %v", err)
	}
	if user.DisplayName != "新昵称" || repo.update.Bio != "简介" {
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

func TestUpdateAvatarAcceptsOnlyManagedURLs(t *testing.T) {
	t.Parallel()
	repo := &userRepoStub{user: &User{ID: 1, AvatarURL: "/media/avatars/old.jpg"}}
	user, previous, err := NewUserUsecase(repo, nil).UpdateAvatar(context.Background(), 1, "/media/avatars/new.png")
	if err != nil {
		t.Fatalf("UpdateAvatar() error = %v", err)
	}
	if user.AvatarURL != "/media/avatars/new.png" || previous != "/media/avatars/old.jpg" {
		t.Fatalf("UpdateAvatar() user = %+v, previous = %q", user, previous)
	}
	if _, _, err := NewUserUsecase(repo, nil).UpdateAvatar(context.Background(), 1, "https://example.com/avatar.png"); err == nil {
		t.Fatal("UpdateAvatar() unexpectedly accepted an external URL")
	}
}

func TestUserLevelThresholds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		experience int64
		level      int32
	}{{0, 0}, {9, 0}, {10, 1}, {49, 1}, {50, 2}, {149, 2}, {150, 3}, {450, 4}, {1080, 5}, {2880, 6}, {999999, 6}}
	for _, test := range tests {
		if got := UserLevel(test.experience); got != test.level {
			t.Errorf("UserLevel(%d) = %d, want %d", test.experience, got, test.level)
		}
	}
}
