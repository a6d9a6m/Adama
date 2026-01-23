package service

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	pb "github.com/littleSand/adama/api/order/service/v1"
	"github.com/littleSand/adama/app/order/service/internal/biz"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(NewOrderService)

type OrderService struct {
	pb.UnimplementedOrderServer

	uc    *biz.OrderUsecase
	goods *biz.AdamaGoodsUsecase
	so    *biz.AdamaOrderUsecase
	log   *log.Helper
}

func NewOrderService(uc *biz.OrderUsecase, so *biz.AdamaOrderUsecase, goods *biz.AdamaGoodsUsecase, logger log.Logger) *OrderService {
	return &OrderService{
		uc:    uc,
		so:    so,
		goods: goods,
		log:   log.NewHelper(logger),
	}
}
