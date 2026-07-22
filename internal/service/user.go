package service

import (
	"context"

	v1 "bilibili-lite/api/user/v1"
	"bilibili-lite/internal/biz"
	appMiddleware "bilibili-lite/internal/middleware"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserService exposes the minimal authentication API.
type UserService struct {
	v1.UnimplementedUserServiceServer

	userUsecase *biz.UserUsecase
}

// NewUserService creates the transport adapter for authentication operations.
func NewUserService(userUsecase *biz.UserUsecase) *UserService {
	return &UserService{userUsecase: userUsecase}
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
