package middleware

import (
	"context"
	"testing"

	"bilibili-lite/internal/biz"
)

func TestUserIDContext(t *testing.T) {
	ctx := WithUserID(context.Background(), 42)
	userID, err := RequireUserID(ctx)
	if err != nil || userID != 42 {
		t.Fatalf("RequireUserID() = %d, %v, want 42", userID, err)
	}
	if _, err := RequireUserID(context.Background()); err != biz.ErrSessionInvalid {
		t.Fatalf("missing identity error = %v, want ErrSessionInvalid", err)
	}
}

func TestAdminIdentityContext(t *testing.T) {
	ctx := WithIdentity(context.Background(), Identity{UserID: 7, IsAdmin: true})
	userID, err := RequireAdmin(ctx)
	if err != nil || userID != 7 {
		t.Fatalf("RequireAdmin() = %d, %v, want 7", userID, err)
	}
	if _, err := RequireAdmin(WithUserID(context.Background(), 7)); err != biz.ErrUserForbidden {
		t.Fatalf("non-admin error = %v, want ErrUserForbidden", err)
	}
}
