package service

import (
	"context"
	"strings"

	v1 "bilibili-lite/api/user/v1"
	"bilibili-lite/internal/biz"

	"github.com/go-kratos/kratos/v3/transport"
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
	accessToken := req.GetAccessToken()
	if tr, ok := transport.FromServerContext(ctx); ok {
		if bearer := parseBearerToken(tr.RequestHeader().Get("Authorization")); bearer != "" {
			accessToken = bearer
		}
	}
	if err := s.userUsecase.Logout(accessToken); err != nil {
		return nil, err
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
	}
}

// parseBearerToken extracts a token from a case-insensitive Authorization Bearer header.
func parseBearerToken(value string) string {
	const prefix = "Bearer "
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(value[len(prefix):])
}
