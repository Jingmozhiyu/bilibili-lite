package middleware

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"bilibili-lite/internal/biz"

	"github.com/go-kratos/kratos/v3/log"
	kratosMiddleware "github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	kratosHTTP "github.com/go-kratos/kratos/v3/transport/http"
	"google.golang.org/grpc/peer"
)

type userRateRule struct {
	bucket string
	limit  int64
	window time.Duration
}

// UserRateLimiterMiddleware bounds authentication and profile traffic through Redis.
type UserRateLimiterMiddleware struct {
	limiter biz.UserRequestLimiter
}

// NewUserRateLimiterMiddleware creates the shared HTTP/gRPC user API limiter.
func NewUserRateLimiterMiddleware(limiter biz.UserRequestLimiter) *UserRateLimiterMiddleware {
	return &UserRateLimiterMiddleware{limiter: limiter}
}

// Server applies per-client auth budgets and per-user profile budgets.
func (m *UserRateLimiterMiddleware) Server() kratosMiddleware.Middleware {
	return func(next kratosMiddleware.Handler) kratosMiddleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return next(ctx, req)
			}
			rule, limited := userRateRuleForOperation(tr.Operation())
			if !limited {
				return next(ctx, req)
			}
			subject := requestClientAddress(ctx)
			if identity, authenticated := RequestIdentity(ctx); authenticated && rule.bucket == "profile" {
				subject = "user:" + strconv.FormatUint(identity.UserID, 10)
			}
			allowed, err := m.limiter.Allow(ctx, rule.bucket, subject, rule.limit, rule.window)
			if err != nil {
				log.Warn("user API rate limiter degraded", "operation", tr.Operation(), "error", err)
				return next(ctx, req)
			}
			if !allowed {
				return nil, biz.ErrUserRateLimited
			}
			return next(ctx, req)
		}
	}
}

func userRateRuleForOperation(operation string) (userRateRule, bool) {
	switch operation {
	case "/user.v1.UserService/Register":
		return userRateRule{bucket: "register", limit: 5, window: time.Hour}, true
	case "/user.v1.UserService/Login":
		return userRateRule{bucket: "login", limit: 10, window: time.Minute}, true
	case "/user.v1.UserService/Refresh":
		return userRateRule{bucket: "refresh", limit: 30, window: time.Minute}, true
	default:
		if strings.HasPrefix(operation, "/user.v1.UserService/") {
			return userRateRule{bucket: "profile", limit: 120, window: time.Minute}, true
		}
		return userRateRule{}, false
	}
}

func requestClientAddress(ctx context.Context) string {
	if request, ok := kratosHTTP.RequestFromServerContext(ctx); ok {
		for _, candidate := range []string{
			request.Header.Get("CF-Connecting-IP"),
			firstForwardedAddress(request.Header.Get("X-Forwarded-For")),
			hostOnly(request.RemoteAddr),
		} {
			if parsed := net.ParseIP(strings.TrimSpace(candidate)); parsed != nil {
				return parsed.String()
			}
		}
	}
	if remote, ok := peer.FromContext(ctx); ok && remote.Addr != nil {
		if address := hostOnly(remote.Addr.String()); address != "" {
			return address
		}
	}
	return "unknown"
}

func firstForwardedAddress(value string) string {
	address, _, _ := strings.Cut(value, ",")
	return strings.TrimSpace(address)
}

func hostOnly(address string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err == nil {
		return host
	}
	return strings.Trim(strings.TrimSpace(address), "[]")
}
