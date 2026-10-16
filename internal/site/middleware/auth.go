package middleware

import (
	"bilibili-lite/internal/platform/auth"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewJWTManager, NewAuthenticator, NewUserRateLimiterMiddleware)

type Identity = auth.Identity
type Authenticator = auth.Authenticator

var NewAuthenticator = auth.NewAuthenticator
var WithIdentity = auth.WithIdentity
var WithUserID = auth.WithUserID
var RequestIdentity = auth.RequestIdentity
var UserID = auth.UserID
var RequireUserID = auth.RequireUserID
var RequireAdmin = auth.RequireAdmin
