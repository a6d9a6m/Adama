package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

type Goods struct {
	Id    int64
	Title string
	Intro string
}

type GoodsListQuery struct {
	Page     int
	PageSize int
	Keyword  string
}

type GoodsListResult struct {
	Items []Goods
	Total int64
	Page  int
	Size  int
}

type GoodsUsecase struct {
	repo GoodsRepo
	log  *log.Helper
}

func NewGoodsUsecase(repo GoodsRepo, logger log.Logger) *GoodsUsecase {
	return &GoodsUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

type GoodsRepo interface {
	GetGoods(ctx context.Context, id int64) (*Goods, error)
	ListGoods(ctx context.Context, query GoodsListQuery) (*GoodsListResult, error)
}

func (uc *GoodsUsecase) Get(ctx context.Context, id int64) (*Goods, error) {
	return uc.repo.GetGoods(ctx, id)
}

func (uc *GoodsUsecase) List(ctx context.Context, query GoodsListQuery) (*GoodsListResult, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 10
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	return uc.repo.ListGoods(ctx, query)
}
