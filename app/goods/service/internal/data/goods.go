package data

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/goods/service/internal/biz"
	"github.com/littleSand/adama/app/goods/service/internal/data/ent"
	entgoods "github.com/littleSand/adama/app/goods/service/internal/data/ent/goods"
)

var _ biz.GoodsRepo = (*goodsRepo)(nil)

type goodsRepo struct {
	data *Data
	log  *log.Helper
}

func NewGoodsRepo(data *Data, logger log.Logger) biz.GoodsRepo {
	return &goodsRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/server-service")),
	}
}

func (g goodsRepo) GetGoods(ctx context.Context, id int64) (*biz.Goods, error) {
	vo, err := g.data.db.Goods.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &biz.Goods{
		Title: vo.Title,
		Intro: vo.Intro,
	}, nil
}

func (g goodsRepo) ListGoods(ctx context.Context, query biz.GoodsListQuery) (*biz.GoodsListResult, error) {
	builder := g.data.db.Goods.Query()
	if query.Keyword != "" {
		builder = builder.Where(entgoods.TitleContains(query.Keyword))
	}

	total, err := builder.Count(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := builder.
		Order(ent.Asc(entgoods.FieldID)).
		Offset((query.Page - 1) * query.PageSize).
		Limit(query.PageSize).
		All(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]biz.Goods, 0, len(rows))
	for _, row := range rows {
		items = append(items, biz.Goods{
			Id:    row.ID,
			Title: row.Title,
			Intro: row.Intro,
		})
	}

	return &biz.GoodsListResult{
		Items: items,
		Total: int64(total),
		Page:  query.Page,
		Size:  query.PageSize,
	}, nil
}
