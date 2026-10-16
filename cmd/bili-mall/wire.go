//go:build wireinject

package main

import (
	"bilibili-lite/internal/mall/admission"
	"bilibili-lite/internal/mall/biz"
	"bilibili-lite/internal/mall/conf"
	"bilibili-lite/internal/mall/data"
	"bilibili-lite/internal/mall/observability"
	"bilibili-lite/internal/mall/payment"
	"bilibili-lite/internal/mall/server"
	"bilibili-lite/internal/mall/service"
	"bilibili-lite/internal/mall/worker"
	"bilibili-lite/internal/platform/auth"

	"github.com/go-kratos/kratos/v3"
	"github.com/google/wire"
)

func wireApp(config *conf.Config) (*kratos.App, func(), error) {
	panic(wire.Build(
		observability.New,
		data.ProviderSet,
		data.NewSimulatedStore,
		payment.NewSimulated,
		biz.ProviderSet,
		wire.Bind(new(biz.PurchaseGuard), new(*admission.Protection)),
		wire.Bind(new(service.MallUsecase), new(*biz.MallUsecase)),
		service.NewMallService,
		server.NewHTTPServer,
		newAuth,
		auth.NewAuthenticator,
		worker.NewMaintenance,
		newApp,
	))
}
