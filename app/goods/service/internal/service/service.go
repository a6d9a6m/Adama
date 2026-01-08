package service

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	pb "github.com/littleSand/adama/api/goods/service/v1"
	"github.com/littleSand/adama/app/goods/service/internal/biz"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(NewGoodsService)

type GoodsService struct {
	pb.UnimplementedGoodsServer

	uc  *biz.GoodsUsecase
	oc  *biz.OrdersUsecase
	log *log.Helper
}

func NewGoodsService(uc *biz.GoodsUsecase, oc *biz.OrdersUsecase, logger log.Logger) *GoodsService {
	return &GoodsService{
		uc:  uc,
		oc:  oc,
		log: log.NewHelper(logger),
	}
}

