package middleware

import (
	"bilibili-lite/internal/platform/auth"
	"bilibili-lite/internal/site/biz"
	"bilibili-lite/internal/site/conf"
)

func NewJWTManager(c *conf.Auth) (biz.TokenManager, error) {
	if c == nil {
		return auth.NewJWTManager(nil)
	}
	return auth.NewJWTManager(&auth.Config{Issuer: c.Issuer, Secret: c.Secret, AccessTTL: c.AccessTtl.AsDuration(), RefreshTTL: c.RefreshTtl.AsDuration()})
}
