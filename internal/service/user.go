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

	uc *biz.UserUsecase
}

func NewUserService(uc *biz.UserUsecase) *UserService {
	return &UserService{uc: uc}
}

func (s *UserService) Login(ctx context.Context, req *v1.LoginRequest) (*v1.LoginReply, error) {
	session, err := s.uc.Login(ctx, req.GetUsername(), req.GetPassword())
	if err != nil {
		return nil, err
	}
	return &v1.LoginReply{
		AccessToken: session.AccessToken,
		ExpiresAt:   timestamppb.New(session.ExpiresAt),
		User:        convertUserReply(&session.User),
	}, nil
}

func (s *UserService) Logout(ctx context.Context, req *v1.LogoutRequest) (*emptypb.Empty, error) {
	accessToken := req.GetAccessToken()
	if tr, ok := transport.FromServerContext(ctx); ok {
		if bearer := parseBearerToken(tr.RequestHeader().Get("Authorization")); bearer != "" {
			accessToken = bearer
		}
	}
	if err := s.uc.Logout(ctx, accessToken); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

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

func parseBearerToken(value string) string {
	const prefix = "Bearer "
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(value[len(prefix):])
}
