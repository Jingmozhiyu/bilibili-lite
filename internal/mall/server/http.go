package server

import (
	"time"

	v1 "bilibili-lite/api/mall/v1"
	"bilibili-lite/internal/mall/admission"
	"bilibili-lite/internal/mall/conf"
	"bilibili-lite/internal/mall/observability"
	"bilibili-lite/internal/mall/service"
	"bilibili-lite/internal/platform/auth"

	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/transport/http"
)

func NewHTTPServer(config *conf.Config, authenticator *auth.Authenticator, service *service.MallService, protection *admission.Protection, metrics *observability.Metrics) *http.Server {
	opts := []http.ServerOption{
		http.Address(config.Address),
		http.RequestDecoder(decodeJSON),
		http.ResponseEncoder(encodeJSON),
		http.Timeout(5 * time.Second),
		http.Middleware(recovery.Recovery(), authenticator.Server()),
		http.ErrorEncoder(admission.ErrorEncoder),
	}
	filters := []http.FilterFunc{metrics.Filter}
	if protection != nil {
		filters = append(filters, protection.Filter)
	}
	// Kratos replaces, rather than appends, filters on repeated options.
	opts = append(opts, http.Filter(filters...))
	server := http.NewServer(opts...)
	if protection != nil {
		metrics.Registry.MustRegister(protection)
	}
	server.Handle("/metrics", metrics.Handler())
	v1.RegisterMallServiceHTTPServer(server, service)
	return server
}
