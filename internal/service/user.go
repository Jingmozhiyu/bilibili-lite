package service

import (
	"context"

	v1 "bilibili-lite/api/user/v1"
	"bilibili-lite/internal/biz"
	appMiddleware "bilibili-lite/internal/middleware"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserService exposes authentication and user profile APIs.
type UserService struct {
	v1.UnimplementedUserServiceServer

	userUsecase *biz.UserUsecase
}

// NewUserService creates the transport adapter for authentication operations.
func NewUserService(userUsecase *biz.UserUsecase) *UserService {
	return &UserService{userUsecase: userUsecase}
}

// GetUser returns one public profile without private account fields.
func (s *UserService) GetUser(ctx context.Context, req *v1.GetUserRequest) (*v1.User, error) {
	user, err := s.userUsecase.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return convertUserReply(user), nil
}

// GetMe returns the authenticated caller's complete profile.
func (s *UserService) GetMe(ctx context.Context, _ *v1.GetMeRequest) (*v1.User, error) {
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.userUsecase.GetMe(ctx, userID)
	if err != nil {
		return nil, err
	}
	return convertUserReply(user), nil
}

// UpdateMe replaces the authenticated caller's editable public profile fields.
func (s *UserService) UpdateMe(ctx context.Context, req *v1.UpdateMeRequest) (*v1.User, error) {
	userID, err := appMiddleware.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.userUsecase.UpdateMe(ctx, userID, biz.UserProfileUpdate{
		DisplayName: req.GetDisplayName(), AvatarURL: req.GetAvatarUrl(), Bio: req.GetBio(),
	})
	if err != nil {
		return nil, err
	}
	return convertUserReply(user), nil
}

// Login converts login credentials from the API request and returns a JWT-backed session reply.
func (s *UserService) Login(ctx context.Context, req *v1.LoginRequest) (*v1.LoginReply, error) {
	session, err := s.userUsecase.Login(ctx, req.GetUsername(), req.GetPassword())
	if err != nil {
		return nil, err
	}
	return convertSessionReply(session), nil
}

// Logout validates the supplied access token; clients complete stateless logout by discarding tokens.
func (s *UserService) Logout(ctx context.Context, req *v1.LogoutRequest) (*emptypb.Empty, error) {
	if _, ok := appMiddleware.UserID(ctx); !ok {
		if err := s.userUsecase.Logout(req.GetAccessToken()); err != nil {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
}

// Refresh exchanges a valid refresh JWT for a newly rotated token pair.
func (s *UserService) Refresh(ctx context.Context, req *v1.RefreshRequest) (*v1.LoginReply, error) {
	session, err := s.userUsecase.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, err
	}
	return convertSessionReply(session), nil
}

// convertSessionReply maps the authentication domain session to its API representation.
func convertSessionReply(session *biz.UserSession) *v1.LoginReply {
	return &v1.LoginReply{
		AccessToken: session.AccessToken, RefreshToken: session.RefreshToken,
		ExpiresAt:        timestamppb.New(session.ExpiresAt),
		RefreshExpiresAt: timestamppb.New(session.RefreshExpiresAt),
		User:             convertUserReply(&session.User),
	}
}

// convertUserReply maps the public user domain object to its API representation.
func convertUserReply(in *biz.User) *v1.User {
	if in == nil {
		return nil
	}
	return &v1.User{
		Id:          in.ID,
		Username:    in.Username,
		DisplayName: in.DisplayName,
		AvatarUrl:   in.AvatarURL,
		Bio:         in.Bio,
		CoinBalance: in.CoinBalance,
	}
}
