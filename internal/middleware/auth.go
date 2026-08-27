package middleware

import (
	"context"
	"net/http"
	"strings"

	"bilibili-lite/internal/biz"

	kratosMiddleware "github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	kratosHTTP "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/google/wire"
)

// ProviderSet provides JWT infrastructure and request authentication middleware.
var ProviderSet = wire.NewSet(NewJWTManager, NewAuthenticator, NewUserRateLimiterMiddleware)

type identityContextKey struct{}

// Identity is the authenticated principal propagated to transport handlers.
type Identity struct {
	UserID  uint64
	IsAdmin bool
}

// Authenticator validates optional transport JWTs and injects their user ID into request context.
type Authenticator struct {
	tokens biz.TokenManager
}

// NewAuthenticator creates the shared HTTP/gRPC access-token middleware.
func NewAuthenticator(tokens biz.TokenManager) *Authenticator {
	return &Authenticator{tokens: tokens}
}

// Server authenticates an optional Bearer token for generated HTTP and gRPC operations.
func (a *Authenticator) Server() kratosMiddleware.Middleware {
	return func(next kratosMiddleware.Handler) kratosMiddleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return next(ctx, req)
			}
			raw := parseBearerToken(tr.RequestHeader().Get("Authorization"))
			if raw == "" {
				return next(ctx, req)
			}
			claims, err := a.tokens.ParseAccess(raw)
			if err != nil {
				return nil, biz.ErrSessionInvalid
			}
			return next(WithIdentity(ctx, Identity{UserID: claims.UserID, IsAdmin: claims.IsAdmin}), req)
		}
	}
}

// Admin rejects selected transport operations unless authentication established an administrator identity.
func (a *Authenticator) Admin() kratosMiddleware.Middleware {
	return func(next kratosMiddleware.Handler) kratosMiddleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if _, err := RequireAdmin(ctx); err != nil {
				return nil, err
			}
			return next(ctx, req)
		}
	}
}

// RequireHTTP protects a raw net/http handler with the same JWT validation used by Kratos operations.
func (a *Authenticator) RequireHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := parseBearerToken(r.Header.Get("Authorization"))
		claims, err := a.tokens.ParseAccess(raw)
		if err != nil {
			kratosHTTP.DefaultErrorEncoder(w, r, biz.ErrSessionInvalid)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), Identity{UserID: claims.UserID, IsAdmin: claims.IsAdmin})))
	})
}

// WithIdentity stores an authenticated principal in request context.
func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}

// WithUserID stores an authenticated user ID in request context.
func WithUserID(ctx context.Context, userID uint64) context.Context {
	return WithIdentity(ctx, Identity{UserID: userID})
}

// RequestIdentity returns the authenticated principal supplied by middleware.
func RequestIdentity(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	return identity, ok && identity.UserID != 0
}

// UserID returns the authenticated user ID when one was supplied by middleware.
func UserID(ctx context.Context) (uint64, bool) {
	identity, ok := RequestIdentity(ctx)
	return identity.UserID, ok
}

// RequireUserID returns an authentication error when middleware did not establish an identity.
func RequireUserID(ctx context.Context) (uint64, error) {
	if userID, ok := UserID(ctx); ok {
		return userID, nil
	}
	return 0, biz.ErrSessionInvalid
}

// RequireAdmin returns the authenticated administrator or a forbidden error.
func RequireAdmin(ctx context.Context) (uint64, error) {
	identity, ok := RequestIdentity(ctx)
	if !ok {
		return 0, biz.ErrSessionInvalid
	}
	if !identity.IsAdmin {
		return 0, biz.ErrUserForbidden
	}
	return identity.UserID, nil
}

func parseBearerToken(value string) string {
	const prefix = "Bearer "
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(value[len(prefix):])
}
