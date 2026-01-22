package server

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/metrics"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/middleware/validate"
	"github.com/go-kratos/kratos/v2/transport/http"
	pb "github.com/littleSand/adama/api/user/service/v1"
	"github.com/littleSand/adama/app/user/service/internal/conf"
	"github.com/littleSand/adama/app/user/service/internal/service"
)

// NewHTTPServer new a HTTP server.
func NewHTTPServer(c *conf.Server, s *service.UserService, logger log.Logger) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
			tracing.Server(),
			logging.Server(logger),
			metrics.Server(),
			validate.Validator(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	pb.RegisterUserHTTPServer(srv, s)
	registerUserRoutes(srv, s)
	return srv
}

func registerUserRoutes(srv *http.Server, s *service.UserService) {
	router := srv.Route("/")
	router.GET("/user/address/list", s.ListAddressesHTTP)
	router.POST("/user/address", s.CreateAddressHTTP)
	router.PUT("/user/address/{id}", s.UpdateAddressHTTP)
	router.DELETE("/user/address/{id}", s.DeleteAddressHTTP)
	router.PUT("/user/address/{id}/default", s.SetDefaultAddressHTTP)
}
