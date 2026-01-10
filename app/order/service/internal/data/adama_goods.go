package data

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/order/service/internal/biz"
	"github.com/littleSand/adama/pkg/cache"
)

var _ biz.AdamaGoodsRepo = (*adamaGoodsRepo)(nil)

type adamaGoodsRepo struct {
	data *Data
	log  *log.Helper
}

func (s adamaGoodsRepo) GetGoodsOver(ctx context.Context, goodsID int64) bool {
	exist := s.data.rdb.Exists(ctx, cache.AdamaGoodOverKey(goodsID)).Val()
	return exist == 1
}

func (s adamaGoodsRepo) SetGoodsOver(ctx context.Context, goodsID int64) error {
	return s.data.rdb.Set(ctx, cache.AdamaGoodOverKey(goodsID), true, 0).Err()
}

func (s adamaGoodsRepo) GetGoods(ctx context.Context, id int64) (*biz.AdamaGoods, error) {
	s.log.Infof("GetGoods param: %v", id)
	g, err := s.data.db.AdamaGoods.Get(ctx, id)
	s.log.Infof("GetGoods result: %v %v", g, err)
	if err != nil {
		return nil, err
	}

	return &biz.AdamaGoods{
		GoodsId:    g.GoodsID,
		AdamaPrice: g.AdamaPrice,
		StockCount: g.StockCount,
		StartDate:  g.StartDate,
		EndDate:    g.EndDate,
	}, nil
}

func (s adamaGoodsRepo) DecrGoodsStock(ctx context.Context, id int64) error {
	_, err := s.data.db.AdamaGoods.UpdateOneID(id).AddStockCount(-1).Save(ctx)
	return err
}

func NewAdamaGoodsRepo(data *Data, logger log.Logger) biz.AdamaGoodsRepo {
	return &adamaGoodsRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/server-service")),
	}
}
