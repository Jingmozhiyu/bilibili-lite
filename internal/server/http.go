package server

import (
	"net/http"
	"os"
	"path/filepath"

	userV1 "bilibili-lite/api/user/v1"
	videoV1 "bilibili-lite/api/video/v1"
	"bilibili-lite/internal/conf"
	"bilibili-lite/internal/service"

	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/middleware/validate"
	kratosHttp "github.com/go-kratos/kratos/v3/transport/http"
	"go.einride.tech/aip/fieldbehavior"
	"google.golang.org/protobuf/proto"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, video *service.VideoService, user *service.UserService) *kratosHttp.Server {
	var opts = []kratosHttp.ServerOption{
		kratosHttp.Middleware(
			recovery.Recovery(),
			validate.Validator(func(req any) error {
				if msg, ok := req.(proto.Message); ok {
					if err := fieldbehavior.ValidateRequiredFields(msg); err != nil {
						return err
					}
				}
				return nil
			}),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, kratosHttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, kratosHttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, kratosHttp.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := kratosHttp.NewServer(opts...)
	videoV1.RegisterVideoServiceHTTPServer(srv, video)
	userV1.RegisterUserServiceHTTPServer(srv, user)
	srv.HandlePrefix("/media/videos/", http.StripPrefix("/media/videos/", http.FileServer(http.Dir(videoMediaDir()))))
	return srv
}

func videoMediaDir() string {
	for _, dir := range []string{
		"storage/videos",
		filepath.Join("..", "storage", "videos"),
		filepath.Join("..", "..", "storage", "videos"),
	} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return "storage/videos"
}
