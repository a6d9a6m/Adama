package biz

import (
	stdhttp "net/http"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/gateway/internal/data"
	"github.com/littleSand/adama/app/gateway/internal/proxy"
)

type GatewayUsecase struct {
	handler stdhttp.Handler
}

func NewGatewayUsecase(data *data.Data, logger log.Logger) *GatewayUsecase {
	dispatcher := proxy.NewDispatcher(proxy.DispatcherConfig{
		Logger: logger,
		Routes: []proxy.Route{
			{Prefix: "/user", Upstream: data.User},
			{Prefix: "/goods", Upstream: data.Goods},
			{Prefix: "/order", Upstream: data.Order},
			{Prefix: "/adama/goods", Upstream: data.Order},
			{Prefix: "/adama/order", Upstream: data.Order},
		},
		BlockedExact: []string{
			"/adama/orderTry",
			"/adama/orderConfirm",
			"/adama/orderCancel",
			"/ordersTry",
			"/ordersConfirm",
			"/ordersCancel",
		},
		BlockedPrefixes: []string{
			"/adama/tcc",
			"/internal",
			"/task",
			"/compensate",
		},
	})

	return &GatewayUsecase{handler: dispatcher}
}

func (uc *GatewayUsecase) Handler() stdhttp.Handler {
	return uc.handler
}
