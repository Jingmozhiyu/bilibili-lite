package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"bilibili-lite/internal/biz"

	"github.com/go-kratos/kratos/v3/transport"
)

type rateLimiterStub struct {
	allowed bool
	calls   int
}

func (l *rateLimiterStub) Allow(context.Context, string, string, int64, time.Duration) (bool, error) {
	l.calls++
	return l.allowed, nil
}

type rateTransportStub struct{ operation string }

func (t rateTransportStub) Kind() transport.Kind            { return transport.KindHTTP }
func (t rateTransportStub) Endpoint() string                { return "" }
func (t rateTransportStub) Operation() string               { return t.operation }
func (t rateTransportStub) RequestHeader() transport.Header { return nil }
func (t rateTransportStub) ReplyHeader() transport.Header   { return nil }

func TestUserRateLimiterRejectsExhaustedLoginBudget(t *testing.T) {
	limiter := &rateLimiterStub{allowed: false}
	handler := NewUserRateLimiterMiddleware(limiter).Server()(func(context.Context, any) (any, error) {
		t.Fatal("limited handler was called")
		return nil, nil
	})
	ctx := transport.NewServerContext(context.Background(), rateTransportStub{operation: "/user.v1.UserService/Login"})
	if _, err := handler(ctx, nil); !errors.Is(err, biz.ErrUserRateLimited) {
		t.Fatalf("handler error = %v", err)
	}
}

func TestUserRateLimiterSkipsVideoOperations(t *testing.T) {
	limiter := &rateLimiterStub{allowed: false}
	called := false
	handler := NewUserRateLimiterMiddleware(limiter).Server()(func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	ctx := transport.NewServerContext(context.Background(), rateTransportStub{operation: "/video.v1.VideoService/ListVideos"})
	if _, err := handler(ctx, nil); err != nil || !called || limiter.calls != 0 {
		t.Fatalf("video handler called=%t limiter calls=%d error=%v", called, limiter.calls, err)
	}
}
