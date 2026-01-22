package data

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/order/service/internal/biz"
	"github.com/littleSand/adama/app/order/service/internal/data/ent"
	entorder "github.com/littleSand/adama/app/order/service/internal/data/ent/order"
)

var _ biz.OrderRepo = (*orderRepo)(nil)

type orderRepo struct {
	data *Data
	log  *log.Helper
}

func NewOrderRepo(data *Data, logger log.Logger) biz.OrderRepo {
	return &orderRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/server-service")),
	}
}

func (o orderRepo) CreateOrder(ctx context.Context, order *biz.Order) error {

	res, err := o.data.db.Order.Create().SetGid(order.Gid).SetSn("2222").SetUID(order.Uid).Save(ctx)

	if err != nil {
		return err
	}

	o.log.Infof("order-create-result: %v", res)
	return nil
}

func (o orderRepo) ListOrders(ctx context.Context, userID int64, page int, pageSize int) ([]biz.Order, int64, error) {
	builder := o.data.db.Order.Query()
	if userID > 0 {
		builder = builder.Where(entorder.UIDEQ(userID))
	}

	total, err := builder.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := builder.
		Order(ent.Asc(entorder.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	items := make([]biz.Order, 0, len(rows))
	for _, row := range rows {
		items = append(items, biz.Order{
			Id:  row.ID,
			Uid: row.UID,
			Gid: row.Gid,
			Sn:  row.Sn,
		})
	}

	return items, int64(total), nil
}
