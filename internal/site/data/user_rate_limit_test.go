package data

import (
	"context"
	"testing"
	"time"
)

func TestUserRequestLimiterDegradesWithoutRedis(t *testing.T) {
	allowed, err := (&userRequestLimiter{}).Allow(context.Background(), "login", "127.0.0.1", 1, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("Allow() = %t, %v", allowed, err)
	}
}
