package middleware

import (
	"context"
	"testing"

	"bilibili-lite/internal/biz"
)

func TestRequireAdminEnforcesAuthenticatedRole(t *testing.T) {
	tests := []struct {
		name   string
		ctx    context.Context
		wantID uint64
		want   error
	}{
		{name: "administrator", ctx: WithIdentity(context.Background(), Identity{UserID: 7, IsAdmin: true}), wantID: 7},
		{name: "ordinary user", ctx: WithUserID(context.Background(), 7), want: biz.ErrUserForbidden},
		{name: "missing identity", ctx: context.Background(), want: biz.ErrSessionInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			userID, err := RequireAdmin(test.ctx)
			if err != test.want || userID != test.wantID {
				t.Fatalf("RequireAdmin() = %d, %v, want %d, %v", userID, err, test.wantID, test.want)
			}
		})
	}
}
