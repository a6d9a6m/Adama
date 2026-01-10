package biz

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
	"time"
)

// AdamaGoods 定义对应数据库实体
type AdamaGoods struct {
	GoodsId    int64
	AdamaPrice float64
	StockCount int64
	StartDate  time.Time
	EndDate    time.Time
}

// AdamaGoodsRepo 定义方法
type AdamaGoodsRepo interface {
	GetGoods(ctx context.Context, id int64) (*AdamaGoods, error)
	DecrGoodsStock(ctx context.Context, id int64) error
	GetGoodsOver(ctx context.Context, GoodsId int64) bool
	SetGoodsOver(ctx context.Context, GoodsId int64) error
}

type AdamaGoodsUsecase struct {
	repo AdamaGoodsRepo
	log  *log.Helper
}

func NewAdamaGoodsUsecase(repo AdamaGoodsRepo, logger log.Logger) *AdamaGoodsUsecase {
	return &AdamaGoodsUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

func (g AdamaGoodsUsecase) GetAdamaGoods(ctx context.Context, id int64) (*AdamaGoods, error) {
	goods, err := g.repo.GetGoods(ctx, id)
	if err != nil {
		return nil, err
	}
	return goods, nil
}

func (g AdamaGoodsUsecase) DecrGoodsStock(ctx context.Context, id int64) error {
	err := g.repo.DecrGoodsStock(ctx, id)
	if err != nil {
		return err
	}
	return nil
}

func (g AdamaGoodsUsecase) GetAdamaGoodsOver(ctx context.Context, id int64) bool {
	return g.repo.GetGoodsOver(ctx, id)
}

func (g AdamaGoodsUsecase) SetAdamaGoodsOver(ctx context.Context, id int64) error {
	return g.repo.SetGoodsOver(ctx, id)
}
