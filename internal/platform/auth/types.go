package auth

import (
	"time"

	"github.com/go-kratos/kratos/v3/errors"
)

var ErrSessionInvalid = errors.Unauthorized("USER_SESSION_INVALID", "invalid session")
var ErrUserForbidden = errors.Forbidden("USER_FORBIDDEN", "administrator access is required")

type Config struct {
	Issuer, Secret        string
	AccessTTL, RefreshTTL time.Duration
}

// TokenClaims identifies an authenticated user and token kind.
type TokenClaims struct {
	UserID    uint64
	TokenType string
	IsAdmin   bool
}

// TokenPair contains the two JWTs issued for a login or refresh.
type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

// TokenManager signs and validates access and refresh JWTs.
type TokenManager interface {
	Issue(uint64, bool) (*TokenPair, error)
	ParseAccess(string) (*TokenClaims, error)
	ParseRefresh(string) (*TokenClaims, error)
}
