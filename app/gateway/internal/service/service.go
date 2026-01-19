package service

import (
	stdhttp "net/http"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/gateway/internal/biz"
)

type GatewayService struct {
	handler stdhttp.Handler
	log     *log.Helper
}

func NewGatewayService(uc *biz.GatewayUsecase, logger log.Logger) *GatewayService {
	return &GatewayService{
		handler: uc.Handler(),
		log:     log.NewHelper(logger),
	}
}

func (s *GatewayService) Handler() stdhttp.Handler {
	return s.handler
}
