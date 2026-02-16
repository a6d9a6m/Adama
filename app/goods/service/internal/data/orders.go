package data

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	rr "github.com/go-resty/resty/v2"
	"github.com/littleSand/adama/app/goods/service/internal/biz"
	"github.com/littleSand/adama/pkg/envutil"
	"github.com/littleSand/adama/pkg/seckill"
	"github.com/yedf/dtmcli"
)

var _ biz.OrdersRepo = (*ordersRepo)(nil)

type TransReq struct {
	Sn string `json:"sn"`
}

type TransReq1 struct {
	Gid    int64 `json:"gid"`
	Amount int64 `json:"amount"`
}

type ordersRepo struct {
	data *Data
	log  *log.Helper
}

func NewOrdersRepo(data *Data, logger log.Logger) biz.OrdersRepo {
	return &ordersRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/orders")),
	}
}

func (m ordersRepo) GetOrders(ctx context.Context, id int64) (*biz.Orders, error) {
	result, err := m.data.db.Order.Get(ctx, id)
	if err != nil {
		m.log.Error(err)
		return nil, err
	}

	return &biz.Orders{
		Sn: result.Sn,
	}, nil
}

func (m ordersRepo) ListOrders(ctx context.Context) (*biz.Orders, error) {
	return &biz.Orders{}, nil
}

func (m ordersRepo) CreateOrders(ctx context.Context, orders biz.Orders) error {
	var dtmServer = envutil.Get("DTM_SERVER_URL", "http://127.0.0.1:36789/api/dtmsvr")
	var busi = envutil.Get("GOODS_SERVICE_URL", "http://127.0.0.1:8003")
	var orderSvc = envutil.Get("ORDER_SERVICE_URL", "http://127.0.0.1:8001")

	gid := dtmcli.MustGenGid(dtmServer)
	err := dtmcli.TccGlobalTransaction(dtmServer, gid, func(tcc *dtmcli.Tcc) (*rr.Response, error) {
		resp, err := tcc.CallBranch(&TransReq{Sn: "sss123123"}, busi+"/ordersTry", busi+"/ordersConfirm", busi+"/ordersCancel")
		if err != nil {
			return resp, err
		}

		return tcc.CallBranch(&TransReq1{Gid: 111, Amount: 99}, orderSvc+"/adama/orderTry", orderSvc+"/adama/orderConfirm", orderSvc+"/adama/orderCancel")
	})
	if err != nil {
		m.log.Error(err)
		return err
	}

	return nil
}

func (m ordersRepo) PrepareStockReservation(ctx context.Context, sn string) error {
	token, err := seckill.ParseStockToken(sn)
	if err != nil {
		return err
	}

	tx, err := m.data.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var status string
	row := tx.QueryRowContext(ctx, "SELECT status FROM adama_stock_reservations WHERE order_id = ?", token.OrderID)
	switch scanErr := row.Scan(&status); scanErr {
	case nil:
		if status == seckill.StockStatusReserved {
			return tx.Commit()
		}
		if status == seckill.StockStatusReleased {
			return errors.New(500, "STOCK_RESERVATION_RELEASED", "stock reservation already released")
		}
	case sql.ErrNoRows:
	default:
		return scanErr
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE adama_goods
		SET stock_count = stock_count - ?
		WHERE goods_id = ? AND stock_count >= ?`,
		token.Amount, token.GoodsID, token.Amount,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New(500, "GOODS_STOCK_EMPTY", "goods stock empty")
	}

	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO adama_stock_reservations(order_id, goods_id, amount, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			updated_at = VALUES(updated_at)`,
		token.OrderID, token.GoodsID, token.Amount, seckill.StockStatusReserved, now, now,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (m ordersRepo) ConfirmStockReservation(ctx context.Context, sn string) error {
	token, err := seckill.ParseStockToken(sn)
	if err != nil {
		return err
	}
	_, err = m.data.sql.ExecContext(ctx, `
		UPDATE adama_stock_reservations
		SET status = ?, updated_at = ?
		WHERE order_id = ?`,
		seckill.StockStatusReserved, time.Now(), token.OrderID,
	)
	return err
}

func (m ordersRepo) CancelStockReservation(ctx context.Context, sn string) error {
	token, err := seckill.ParseStockToken(sn)
	if err != nil {
		return err
	}

	tx, err := m.data.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var status string
	row := tx.QueryRowContext(ctx, "SELECT status FROM adama_stock_reservations WHERE order_id = ?", token.OrderID)
	switch scanErr := row.Scan(&status); scanErr {
	case sql.ErrNoRows:
		return nil
	case nil:
	default:
		return scanErr
	}
	if status == seckill.StockStatusReleased {
		return tx.Commit()
	}

	if _, err = tx.ExecContext(ctx, `
		UPDATE adama_goods
		SET stock_count = stock_count + ?
		WHERE goods_id = ?`,
		token.Amount, token.GoodsID,
	); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE adama_stock_reservations
		SET status = ?, updated_at = ?
		WHERE order_id = ?`,
		seckill.StockStatusReleased, time.Now(), token.OrderID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (m ordersRepo) CreateOrdersConfirm() (interface{}, error) {
	res := dtmcli.OrString("ok", "happy", "SUCCESS")
	type M = map[string]interface{}

	return M{"dtm_result": res}, nil
}

func (m ordersRepo) UpdateOrders(ctx context.Context, orders biz.Orders) error {
	return nil
}
