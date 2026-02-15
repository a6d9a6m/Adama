package data

import (
	"fmt"
	"time"

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
	userTimeout, err := parseTimeout(cfg.User.Timeout)
	if err != nil {
		return nil, fmt.Errorf("user upstream timeout: %w", err)
	}
	user, err := proxy.NewUpstream("user", cfg.User.BaseURL, userTimeout, logger)
	if err != nil {
		return nil, err
	}
	goodsTimeout, err := parseTimeout(cfg.Goods.Timeout)
	if err != nil {
		return nil, fmt.Errorf("goods upstream timeout: %w", err)
	}
	goods, err := proxy.NewUpstream("goods", cfg.Goods.BaseURL, goodsTimeout, logger)
	if err != nil {
		return nil, err
	}
	orderTimeout, err := parseTimeout(cfg.Order.Timeout)
	if err != nil {
		return nil, fmt.Errorf("order upstream timeout: %w", err)
	}
	order, err := proxy.NewUpstream("order", cfg.Order.BaseURL, orderTimeout, logger)
	if err != nil {
		return nil, err
	}

	return &Data{
		User:  user,
		Goods: goods,
		Order: order,
	}, nil
}

func parseTimeout(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	return time.ParseDuration(raw)
}
