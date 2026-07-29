package server

import (
	userV1 "bilibili-lite/api/user/v1"
	videoV1 "bilibili-lite/api/video/v1"
	"bilibili-lite/internal/conf"
	appMiddleware "bilibili-lite/internal/middleware"
	"bilibili-lite/internal/service"

	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/middleware/selector"
	"github.com/go-kratos/kratos/v3/transport/grpc"
)

// NewGRPCServer creates and registers the gRPC transport.
func NewGRPCServer(serverConfig *conf.Server, authenticator *appMiddleware.Authenticator, videoService *service.VideoService, userService *service.UserService) *grpc.Server {
	opts := []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			authenticator.Server(),
			selector.Server(authenticator.Admin()).Match(adminOperation).Build(),
		),
	}
	if serverConfig.Grpc.Network != "" {
		opts = append(opts, grpc.Network(serverConfig.Grpc.Network))
	}
	if serverConfig.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(serverConfig.Grpc.Addr))
	}
	if serverConfig.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(serverConfig.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	videoV1.RegisterVideoServiceServer(srv, videoService)
	userV1.RegisterUserServiceServer(srv, userService)
	return srv
}
