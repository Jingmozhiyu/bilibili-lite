package middleware

import (
	"fmt"
	"strconv"
	"time"

	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/conf"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenType  = "access"
	refreshTokenType = "refresh"
)

type claims struct {
	TokenType string `json:"token_type"`
	IsAdmin   bool   `json:"is_admin,omitempty"`
	jwt.RegisteredClaims
}

type jwtManager struct {
	issuer     string
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewJWTManager creates the configured HS256 token manager.
func NewJWTManager(authConfig *conf.Auth) (biz.TokenManager, error) {
	if authConfig == nil || len(authConfig.Secret) < 32 {
		return nil, fmt.Errorf("auth JWT secret must contain at least 32 characters")
	}
	accessTTL := authConfig.AccessTtl.AsDuration()
	refreshTTL := authConfig.RefreshTtl.AsDuration()
	if accessTTL <= 0 || refreshTTL <= accessTTL {
		return nil, fmt.Errorf("auth JWT TTLs are invalid")
	}
	return &jwtManager{
		issuer: authConfig.Issuer, secret: []byte(authConfig.Secret),
		accessTTL: accessTTL, refreshTTL: refreshTTL,
	}, nil
}

// Issue signs a short-lived access JWT and a long-lived refresh JWT for one user.
func (m *jwtManager) Issue(userID uint64, isAdmin bool) (*biz.TokenPair, error) {
	now := time.Now()
	accessExpiresAt := now.Add(m.accessTTL)
	refreshExpiresAt := now.Add(m.refreshTTL)
	access, err := m.sign(userID, isAdmin, accessTokenType, now, accessExpiresAt)
	if err != nil {
		return nil, err
	}
	refresh, err := m.sign(userID, isAdmin, refreshTokenType, now, refreshExpiresAt)
	if err != nil {
		return nil, err
	}
	return &biz.TokenPair{
		AccessToken: access, RefreshToken: refresh,
		AccessExpiresAt: accessExpiresAt, RefreshExpiresAt: refreshExpiresAt,
	}, nil
}

// ParseAccess validates an access JWT and returns its domain claims.
func (m *jwtManager) ParseAccess(token string) (*biz.TokenClaims, error) {
	return m.parse(token, accessTokenType)
}

// ParseRefresh validates a refresh JWT and returns its domain claims.
func (m *jwtManager) ParseRefresh(token string) (*biz.TokenClaims, error) {
	return m.parse(token, refreshTokenType)
}

func (m *jwtManager) sign(userID uint64, isAdmin bool, tokenType string, issuedAt, expiresAt time.Time) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		TokenType: tokenType,
		IsAdmin:   isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: m.issuer, Subject: strconv.FormatUint(userID, 10),
			IssuedAt: jwt.NewNumericDate(issuedAt), ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})
	return token.SignedString(m.secret)
}

func (m *jwtManager) parse(raw, expectedType string) (*biz.TokenClaims, error) {
	parsed, err := jwt.ParseWithClaims(raw, &claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !parsed.Valid {
		return nil, biz.ErrSessionInvalid
	}
	tokenClaims, ok := parsed.Claims.(*claims)
	if !ok || tokenClaims.TokenType != expectedType {
		return nil, biz.ErrSessionInvalid
	}
	userID, err := strconv.ParseUint(tokenClaims.Subject, 10, 64)
	if err != nil || userID == 0 {
		return nil, biz.ErrSessionInvalid
	}
	return &biz.TokenClaims{UserID: userID, TokenType: tokenClaims.TokenType, IsAdmin: tokenClaims.IsAdmin}, nil
}
