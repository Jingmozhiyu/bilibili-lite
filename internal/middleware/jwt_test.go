package middleware

import (
	"testing"
	"time"

	"bilibili-lite/internal/conf"

	"google.golang.org/protobuf/types/known/durationpb"
)

func TestJWTManagerIssuesTypedTokens(t *testing.T) {
	manager, err := NewJWTManager(&conf.Auth{
		Issuer: "test", Secret: "test-secret-long-enough-for-hs256",
		AccessTtl: durationpb.New(2 * time.Hour), RefreshTtl: durationpb.New(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := manager.Issue(42, true)
	if err != nil {
		t.Fatal(err)
	}
	access, err := manager.ParseAccess(pair.AccessToken)
	if err != nil || access.UserID != 42 || access.TokenType != accessTokenType || !access.IsAdmin {
		t.Fatalf("unexpected access claims: %#v, %v", access, err)
	}
	refresh, err := manager.ParseRefresh(pair.RefreshToken)
	if err != nil || refresh.UserID != 42 || refresh.TokenType != refreshTokenType {
		t.Fatalf("unexpected refresh claims: %#v, %v", refresh, err)
	}
	if _, err := manager.ParseAccess(pair.RefreshToken); err == nil {
		t.Fatal("refresh JWT must not authenticate as an access JWT")
	}
	if got := pair.AccessExpiresAt.Sub(pair.RefreshExpiresAt.Add(-30 * 24 * time.Hour)); got < 119*time.Minute || got > 121*time.Minute {
		t.Fatalf("unexpected access TTL: %v", got)
	}
}

func TestJWTManagerRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewJWTManager(&conf.Auth{}); err == nil {
		t.Fatal("expected empty secret to fail")
	}
}
