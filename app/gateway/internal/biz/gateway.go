package biz

import (
	stdhttp "net/http"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/gateway/internal/data"
	"github.com/littleSand/adama/app/gateway/internal/proxy"
	"github.com/littleSand/adama/pkg/envutil"
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

	handler := proxy.NewRateLimitHandler(dispatcher, proxy.RateLimitConfig{
		TokenRPS:            envutil.Int("GATEWAY_TOKEN_RPS", 2000),
		TokenBurst:          envutil.Int("GATEWAY_TOKEN_BURST", 4000),
		OrderRPS:            envutil.Int("GATEWAY_ORDER_RPS", 1200),
		OrderBurst:          envutil.Int("GATEWAY_ORDER_BURST", 2400),
		UserGoodsOrderRPS:   envutil.Int("GATEWAY_USER_GOODS_ORDER_RPS", 5),
		UserGoodsOrderBurst: envutil.Int("GATEWAY_USER_GOODS_ORDER_BURST", 10),
	}, logger)

	return &GatewayUsecase{handler: handler}
}

func (uc *GatewayUsecase) Handler() stdhttp.Handler {
	return uc.handler
}
