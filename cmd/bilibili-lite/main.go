package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"bilibili-lite/internal/conf"
	"bilibili-lite/internal/worker"

	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/config"
	"github.com/go-kratos/kratos/v3/config/env"
	"github.com/go-kratos/kratos/v3/config/file"
	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"

	_ "go.uber.org/automaxprocs"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string
	// Version is the version of the compiled software.
	Version string
	// flagconf is the config flag.
	flagconf string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(logger *slog.Logger, gs *grpc.Server, hs *http.Server, uploadJanitor *worker.UploadJanitor, videoTranscoder *worker.VideoTranscoder, searchIndexer *worker.SearchIndexer, recommendationRefresher *worker.RecommendationRefresher) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			gs,
			hs,
			uploadJanitor,
			videoTranscoder,
			searchIndexer,
			recommendationRefresher,
		),
	)
}

func loadBootstrap(path string) (*conf.Bootstrap, error) {
	c := config.New(
		config.WithResolveActualTypes(true),
		config.WithSource(
			file.NewSource(path),
			env.NewSource("BILI_"),
		),
	)
	defer c.Close()
	if err := c.Load(); err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	var bootstrap conf.Bootstrap
	if err := c.Scan(&bootstrap); err != nil {
		return nil, fmt.Errorf("decode configuration: %w", err)
	}
	return &bootstrap, nil
}

func main() {
	flag.Parse()
	logger := log.NewLogger(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelInfo,
		}),
		log.WithExtractor(tracing.TraceAttrs),
	).With(
		slog.String("service.id", id),
		slog.String("service.name", Name),
		slog.String("service.version", Version),
	)
	log.SetDefault(logger)
	bc, err := loadBootstrap(flagconf)
	if err != nil {
		panic(err)
	}

	app, cleanup, err := wireApp(bc.Server, bc.Data, bc.Auth, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// start and wait for stop signal
	if err := app.Run(); err != nil {
		panic(err)
	}
}
