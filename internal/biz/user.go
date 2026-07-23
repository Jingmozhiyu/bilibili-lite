package biz

import (
	"context"
	"strings"
	"time"

	v1 "bilibili-lite/api/user/v1"

	"github.com/go-kratos/kratos/v3/errors"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserInvalidArgument = errors.BadRequest(v1.ErrorReason_USER_INVALID_ARGUMENT.String(), "invalid user argument")
	ErrUserNotFound        = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "user not found")
	ErrInvalidCredentials  = errors.Unauthorized(v1.ErrorReason_USER_INVALID_CREDENTIALS.String(), "invalid username or password")
	ErrSessionInvalid      = errors.Unauthorized(v1.ErrorReason_USER_SESSION_INVALID.String(), "invalid session")
	ErrUserStorage         = errors.InternalServer(v1.ErrorReason_USER_UNSPECIFIED.String(), "user storage unavailable")
)

// User is the public user domain model.
type User struct {
	ID          uint64
	Username    string
	DisplayName string
	AvatarURL   string
	Bio         string
	CoinBalance int64
}

// UserProfileUpdate contains the editable public profile fields.
type UserProfileUpdate struct {
	DisplayName string
	AvatarURL   string
	Bio         string
}

// UserCredential contains the stored credential needed during authentication.
type UserCredential struct {
	User
	PasswordHash string
}

// UserSession is returned after a successful login.
type UserSession struct {
	AccessToken      string
	RefreshToken     string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	User             User
}

// TokenClaims identifies an authenticated user and token kind.
type TokenClaims struct {
	UserID    uint64
	TokenType string
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
	Issue(uint64) (*TokenPair, error)
	ParseAccess(string) (*TokenClaims, error)
	ParseRefresh(string) (*TokenClaims, error)
}

// UserRepo reads user identity and credentials.
type UserRepo interface {
	FindCredentialByUsername(context.Context, string) (*UserCredential, error)
	FindUserByID(context.Context, uint64) (*User, error)
	UpdateUserProfile(context.Context, uint64, UserProfileUpdate) (*User, error)
}

// UserUsecase handles authentication.
type UserUsecase struct {
	repo   UserRepo
	tokens TokenManager
}

// NewUserUsecase injects user persistence and JWT capabilities into the authentication usecase.
func NewUserUsecase(repo UserRepo, tokens TokenManager) *UserUsecase {
	return &UserUsecase{repo: repo, tokens: tokens}
}

// GetUser returns one public profile without exposing its private coin balance.
func (uc *UserUsecase) GetUser(ctx context.Context, userID uint64) (*User, error) {
	if userID == 0 {
		return nil, ErrUserInvalidArgument
	}
	user, err := uc.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.CoinBalance = 0
	return user, nil
}

// GetMe returns the authenticated caller's full profile and coin balance.
func (uc *UserUsecase) GetMe(ctx context.Context, userID uint64) (*User, error) {
	if userID == 0 {
		return nil, ErrUserInvalidArgument
	}
	return uc.repo.FindUserByID(ctx, userID)
}

// UpdateMe validates and persists the authenticated caller's editable profile.
func (uc *UserUsecase) UpdateMe(ctx context.Context, userID uint64, update UserProfileUpdate) (*User, error) {
	update.DisplayName = strings.TrimSpace(update.DisplayName)
	update.AvatarURL = strings.TrimSpace(update.AvatarURL)
	update.Bio = strings.TrimSpace(update.Bio)
	if userID == 0 || update.DisplayName == "" || len([]rune(update.DisplayName)) > 100 || len(update.AvatarURL) > 500 || len([]rune(update.Bio)) > 500 {
		return nil, ErrUserInvalidArgument
	}
	return uc.repo.UpdateUserProfile(ctx, userID, update)
}

// Login verifies a username and password before issuing a new access and refresh token pair.
func (uc *UserUsecase) Login(ctx context.Context, username, password string) (*UserSession, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrUserInvalidArgument
	}

	credential, err := uc.repo.FindCredentialByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}

	tokens, err := uc.tokens.Issue(credential.ID)
	if err != nil {
		return nil, errors.InternalServer(v1.ErrorReason_USER_UNSPECIFIED.String(), "failed to create session")
	}

	return &UserSession{
		AccessToken:      tokens.AccessToken,
		RefreshToken:     tokens.RefreshToken,
		ExpiresAt:        tokens.AccessExpiresAt,
		RefreshExpiresAt: tokens.RefreshExpiresAt,
		User:             credential.User,
	}, nil
}

// Refresh validates a refresh JWT and rotates both JWTs.
func (uc *UserUsecase) Refresh(ctx context.Context, refreshToken string) (*UserSession, error) {
	claims, err := uc.tokens.ParseRefresh(strings.TrimSpace(refreshToken))
	if err != nil {
		return nil, ErrSessionInvalid
	}
	user, err := uc.repo.FindUserByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrSessionInvalid
		}
		return nil, err
	}
	tokens, err := uc.tokens.Issue(user.ID)
	if err != nil {
		return nil, errors.InternalServer(v1.ErrorReason_USER_UNSPECIFIED.String(), "failed to refresh session")
	}
	return &UserSession{
		AccessToken:      tokens.AccessToken,
		RefreshToken:     tokens.RefreshToken,
		ExpiresAt:        tokens.AccessExpiresAt,
		RefreshExpiresAt: tokens.RefreshExpiresAt,
		User:             *user,
	}, nil
}

// AuthenticateAccess returns the user ID carried by a valid access JWT.
func (uc *UserUsecase) AuthenticateAccess(accessToken string) (uint64, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return 0, ErrSessionInvalid
	}
	claims, err := uc.tokens.ParseAccess(accessToken)
	if err != nil {
		return 0, ErrSessionInvalid
	}
	return claims.UserID, nil
}

// Logout validates the access JWT. Stateless logout is completed by the client
// discarding both tokens; already-issued access JWTs expire within two hours.
func (uc *UserUsecase) Logout(accessToken string) error {
	_, err := uc.AuthenticateAccess(accessToken)
	return err
}
