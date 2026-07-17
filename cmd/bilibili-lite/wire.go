//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"log/slog"

	"bilibili-lite/internal/auth"
	"bilibili-lite/internal/biz"
	"bilibili-lite/internal/conf"
	"bilibili-lite/internal/data"
	"bilibili-lite/internal/server"
	"bilibili-lite/internal/service"

	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
)

// wireApp assembles the application and its cleanup function.
func wireApp(*conf.Server, *conf.Data, *conf.Auth, *slog.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(server.ProviderSet, data.ProviderSet, biz.ProviderSet, auth.ProviderSet, service.ProviderSet, newApp))
}
