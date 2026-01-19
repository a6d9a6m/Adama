package data

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/gateway/internal/conf"
	"github.com/littleSand/adama/app/gateway/internal/proxy"
)

type Data struct {
	User  *proxy.Upstream
	Goods *proxy.Upstream
	Order *proxy.Upstream
}

func NewData(cfg *conf.Upstreams, logger log.Logger) (*Data, error) {
	user, err := proxy.NewUpstream("user", cfg.User.BaseURL, cfg.User.Timeout, logger)
	if err != nil {
		return nil, err
	}
	goods, err := proxy.NewUpstream("goods", cfg.Goods.BaseURL, cfg.Goods.Timeout, logger)
	if err != nil {
		return nil, err
	}
	order, err := proxy.NewUpstream("order", cfg.Order.BaseURL, cfg.Order.Timeout, logger)
	if err != nil {
		return nil, err
	}

	return &Data{
		User:  user,
		Goods: goods,
		Order: order,
	}, nil
}
