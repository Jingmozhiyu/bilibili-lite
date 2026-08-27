package data

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"bilibili-lite/internal/biz"

	"github.com/redis/go-redis/v9"
)

var incrementRateLimit = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return count
`)

type userRequestLimiter struct {
	client *redis.Client
	prefix string
}

// NewUserRequestLimiter creates the shared Redis-backed user API limiter.
func NewUserRequestLimiter(data *Data) biz.UserRequestLimiter {
	return &userRequestLimiter{client: data.redis, prefix: strings.TrimSuffix(strings.TrimSpace(data.userRateKey), ":")}
}

// Allow atomically consumes one fixed-window request budget.
func (l *userRequestLimiter) Allow(ctx context.Context, bucket, subject string, limit int64, window time.Duration) (bool, error) {
	if l.client == nil || l.prefix == "" {
		return true, nil
	}
	if bucket == "" || subject == "" || limit <= 0 || window <= 0 {
		return false, fmt.Errorf("invalid user rate limit")
	}
	digest := sha256.Sum256([]byte(subject))
	key := fmt.Sprintf("%s:%s:%x", l.prefix, bucket, digest[:12])
	count, err := incrementRateLimit.Run(ctx, l.client, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return count <= limit, nil
}
