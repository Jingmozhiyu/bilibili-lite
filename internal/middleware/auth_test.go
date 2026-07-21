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
