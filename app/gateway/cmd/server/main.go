package main

import (
	"flag"
	"os"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/littleSand/adama/app/gateway/internal/biz"
	"github.com/littleSand/adama/app/gateway/internal/conf"
	"github.com/littleSand/adama/app/gateway/internal/data"
	"github.com/littleSand/adama/app/gateway/internal/server"
	"github.com/littleSand/adama/app/gateway/internal/service"
)

var (
	Name     = "gateway"
	Version  string
	flagconf string
	id, _    = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(logger log.Logger, hs *khttp.Server) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(hs),
	)
}

func main() {
	flag.Parse()

	logger := log.With(
		log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", id,
		"service.name", Name,
		"service.version", Version,
		"trace_id", log.TraceID(),
		"span_id", log.SpanID(),
	)

	c := config.New(config.WithSource(file.NewSource(flagconf)))
	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	gatewayData, err := data.NewData(&bc.Upstreams, logger)
	if err != nil {
		panic(err)
	}

	gatewayUsecase := biz.NewGatewayUsecase(gatewayData, logger)
	gatewayService := service.NewGatewayService(gatewayUsecase, logger)
	httpServer := server.NewHTTPServer(&bc.Server, gatewayService, logger)
	app := newApp(logger, httpServer)

	if err := app.Run(); err != nil {
		panic(err)
	}
}
