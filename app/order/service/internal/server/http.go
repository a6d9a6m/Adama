package server

import (
	stdhttp "net/http"

	pb "github.com/littleSand/adama/api/order/service/v1"
	"github.com/littleSand/adama/app/order/service/internal/conf"
	"github.com/littleSand/adama/app/order/service/internal/service"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/metrics"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/middleware/validate"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// NewHTTPServer new a HTTP server.
func NewHTTPServer(c *conf.Server, s *service.OrderService, logger log.Logger) *khttp.Server {
	var opts = []khttp.ServerOption{
		khttp.Middleware(
			recovery.Recovery(),
			tracing.Server(),
			logging.Server(logger),
			metrics.Server(),
			validate.Validator(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, khttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, khttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, khttp.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := khttp.NewServer(opts...)
	pb.RegisterOrderHTTPServer(srv, s)
	registerAdamaTCCRoutes(srv, s)
	return srv
}

func registerAdamaTCCRoutes(srv *khttp.Server, s *service.OrderService) {
	router := srv.Route("/")
	router.POST("/adama/tcc/order/try", func(ctx khttp.Context) error {
		var req service.AdamaOrderTCCRequest
		if err := ctx.Bind(&req); err != nil {
			return err
		}
		if err := s.HandleAdamaOrderTry(ctx, &req); err != nil {
			return err
		}
		return ctx.JSON(stdhttp.StatusOK, map[string]string{"dtm_result": "SUCCESS"})
	})
	router.POST("/adama/tcc/order/confirm", func(ctx khttp.Context) error {
		var req service.AdamaOrderTCCRequest
		if err := ctx.Bind(&req); err != nil {
			return err
		}
		if err := s.HandleAdamaOrderConfirm(ctx, &req); err != nil {
			return err
		}
		return ctx.JSON(stdhttp.StatusOK, map[string]string{"dtm_result": "SUCCESS"})
	})
	router.POST("/adama/tcc/order/cancel", func(ctx khttp.Context) error {
		var req service.AdamaOrderTCCRequest
		if err := ctx.Bind(&req); err != nil {
			return err
		}
		if err := s.HandleAdamaOrderCancel(ctx, &req); err != nil {
			return err
		}
		return ctx.JSON(stdhttp.StatusOK, map[string]string{"dtm_result": "SUCCESS"})
	})
}

