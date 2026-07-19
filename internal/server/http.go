package server

import (
	"net/http"
	"path/filepath"
	"strings"

	userV1 "bilibili-lite/api/user/v1"
	videoV1 "bilibili-lite/api/video/v1"
	"bilibili-lite/internal/conf"
	"bilibili-lite/internal/media"
	"bilibili-lite/internal/service"

	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/middleware/validate"
	kratosHTTP "github.com/go-kratos/kratos/v3/transport/http"
	"go.einride.tech/aip/fieldbehavior"
	"google.golang.org/protobuf/proto"
)

// NewHTTPServer creates and registers the HTTP transport.
func NewHTTPServer(serverConfig *conf.Server, mediaManager *media.Manager, videoService *service.VideoService, videoUploadHandler *service.VideoUploadHTTPHandler, userService *service.UserService) *kratosHTTP.Server {
	opts := []kratosHTTP.ServerOption{
		kratosHTTP.Middleware(
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
	if serverConfig.Http.Network != "" {
		opts = append(opts, kratosHTTP.Network(serverConfig.Http.Network))
	}
	if serverConfig.Http.Addr != "" {
		opts = append(opts, kratosHTTP.Address(serverConfig.Http.Addr))
	}
	if serverConfig.Http.Timeout != nil {
		opts = append(opts, kratosHTTP.Timeout(serverConfig.Http.Timeout.AsDuration()))
	}
	srv := kratosHTTP.NewServer(opts...)
	videoV1.RegisterVideoServiceHTTPServer(srv, videoService)
	userV1.RegisterUserServiceHTTPServer(srv, userService)
	srv.Handle("/api/v1/videos/upload", videoUploadHandler)
	srv.HandlePrefix("/media/dash/", http.StripPrefix("/media/dash/", dashFileServer(mediaManager.DASHRoot())))
	return srv
}

// dashFileServer serves MPD and media segments with content types understood by DASH clients.
func dashFileServer(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".mpd"):
			w.Header().Set("Content-Type", "application/dash+xml")
		case strings.Contains(filepath.Base(r.URL.Path), "stream1"):
			w.Header().Set("Content-Type", "audio/iso.segment")
		case strings.HasSuffix(r.URL.Path, ".m4s"):
			w.Header().Set("Content-Type", "video/iso.segment")
		}
		files.ServeHTTP(w, r)
	})
}
