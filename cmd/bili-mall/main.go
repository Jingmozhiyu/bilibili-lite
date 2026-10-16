package main

import (
	"bilibili-lite/internal/mall/conf"
	"bilibili-lite/internal/mall/worker"
	"bilibili-lite/internal/platform/auth"

	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/transport/http"
	_ "go.uber.org/automaxprocs"
)

var Name = "bili-mall"
var Version = "dev"

func newAuth(config *conf.Config) (auth.TokenManager, error) {
	return auth.NewJWTManager(&config.Auth)
}

func newApp(h *http.Server, w *worker.Maintenance) *kratos.App {
	return kratos.New(kratos.Name(Name), kratos.Version(Version), kratos.Server(h, w))
}

func main() {
	config, err := conf.Load()
	if err != nil {
		panic(err)
	}
	app, cleanup, err := wireApp(config)
	if err != nil {
		panic(err)
	}
	defer cleanup()
	if err = app.Run(); err != nil {
		panic(err)
	}
}
