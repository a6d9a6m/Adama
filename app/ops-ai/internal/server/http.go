package server

import (
	"net/http"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/metrics"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/littleSand/adama/app/ops-ai/internal/conf"
	"github.com/littleSand/adama/app/ops-ai/internal/service"
)

func NewHTTPServer(cfg *conf.Server, svc *service.OpsAIService, logger log.Logger) *khttp.Server {
	var opts = []khttp.ServerOption{
		khttp.Middleware(
			recovery.Recovery(),
			tracing.Server(),
			logging.Server(logger),
			metrics.Server(),
		),
	}

	if cfg.HTTP.Network != "" {
		opts = append(opts, khttp.Network(cfg.HTTP.Network))
	}
	if cfg.HTTP.Addr != "" {
		opts = append(opts, khttp.Address(cfg.HTTP.Addr))
	}
	if cfg.HTTP.Timeout > 0 {
		opts = append(opts, khttp.Timeout(cfg.HTTP.Timeout))
	}

	srv := khttp.NewServer(opts...)
	srv.Handle("/healthz", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok"))
	}))

	router := srv.Route("/")
	router.POST("/ops-ai/ask", svc.Ask)
	return srv
}
