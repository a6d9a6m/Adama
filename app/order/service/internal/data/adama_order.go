package data

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/order/service/internal/biz"
	"github.com/littleSand/adama/app/order/service/internal/data/kafka"
	"github.com/littleSand/adama/pkg/cache"
	"github.com/littleSand/adama/pkg/seckill"
)

var _ biz.AdamaOrderRepo = (*adamaOrderRepo)(nil)

type adamaOrderRepo struct {
	data *Data
	log  *log.Helper
}

func (s adamaOrderRepo) SendKafkaOrder(ctx context.Context, order *biz.AdamaOrder) error {
	msg := kafka.NewMessage("adama-order", []byte("order"), map[string]string{
		"uid":         strconv.FormatInt(order.UserId, 10),
		"goods_id":    strconv.FormatInt(order.GoodsId, 10),
		"order_id":    strconv.FormatInt(order.OrderId, 10),
		"amount":      strconv.FormatInt(order.Amount, 10),
		"expire_at":   order.ExpireAt.Format(time.RFC3339),
		"stock_token": order.StockToken,
	})
	return s.data.kafka.Send(ctx, msg)
}

func (s adamaOrderRepo) GetAdamaOrder(ctx context.Context, id int64) (*biz.AdamaOrder, error) {
	entityOrder, err := s.data.db.AdamaOrder.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &biz.AdamaOrder{
		GoodsId: entityOrder.GoodsID,
		OrderId: entityOrder.OrderID,
		UserId:  entityOrder.UserID,
		Amount:  1,
	}, nil
}

func (s adamaOrderRepo) PostAdamaOrder(ctx context.Context, order *biz.AdamaOrder) error {
	entityOrder, err := s.data.db.AdamaOrder.Create().
		SetOrderID(order.OrderId).
		SetGoodsID(order.GoodsId).
		SetUserID(order.UserId).
		Save(ctx)
	s.log.Debug(entityOrder, err)
	return err
}

func (s adamaOrderRepo) CreateAdamaOrder(ctx context.Context, order *biz.AdamaOrder) error {
	if order.Amount <= 0 {
		order.Amount = 1
	}
	stock := s.data.rdb.Decr(ctx, cache.AdamaGoodStockKey(order.GoodsId)).Val()
	if stock < 0 {
		_ = s.data.rdb.Incr(ctx, cache.AdamaGoodStockKey(order.GoodsId)).Err()
		_ = s.data.rdb.Set(ctx, cache.AdamaGoodOverKey(order.GoodsId), true, 0).Err()
		return errors.New(500, "GOODS_STOCK_EMPTY", "goods stock empty")
	}

	node, err := snowflake.NewNode(1)
	if err != nil {
		s.log.Error("snowflake generate error")
		return err
	}

	order.OrderId = node.Generate().Int64()
	return nil
}

func (s adamaOrderRepo) PrepareAdamaOrder(ctx context.Context, order *biz.AdamaOrder) error {
	now := time.Now()
	if order.Amount <= 0 {
		order.Amount = 1
	}
	_, err := s.data.msql.ExecContext(ctx, `
		INSERT INTO adama_order_workflows
			(order_id, user_id, goods_id, amount, stock_token, status, stock_status, cache_status, sync_status, expire_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			user_id = VALUES(user_id),
			goods_id = VALUES(goods_id),
			amount = VALUES(amount),
			stock_token = VALUES(stock_token),
			expire_at = VALUES(expire_at),
			updated_at = VALUES(updated_at)`,
		order.OrderId, order.UserId, order.GoodsId, order.Amount, order.StockToken,
		seckill.OrderStatusPreparing, seckill.StockStatusReserving, seckill.CacheStatusReserved, seckill.SyncStatusPending,
		order.ExpireAt, now, now,
	)
	return err
}

func (s adamaOrderRepo) ConfirmAdamaOrder(ctx context.Context, order *biz.AdamaOrder) error {
	_, err := s.data.msql.ExecContext(ctx, `
		UPDATE adama_order_workflows
		SET status = ?, stock_status = ?, cache_status = ?, sync_status = ?, updated_at = ?
		WHERE order_id = ?`,
		seckill.OrderStatusPendingPay, seckill.StockStatusReserved, seckill.CacheStatusReserved, seckill.SyncStatusPending,
		time.Now(), order.OrderId,
	)
	return err
}

func (s adamaOrderRepo) CancelAdamaOrder(ctx context.Context, order *biz.AdamaOrder) error {
	tx, err := s.data.msql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	workflow, err := loadWorkflow(ctx, tx, order.OrderId)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if workflow.CacheStatus != seckill.CacheStatusReleased {
		if redisErr := s.data.rdb.IncrBy(ctx, cache.AdamaGoodStockKey(workflow.GoodsID), workflow.Amount).Err(); redisErr != nil {
			return redisErr
		}
		_ = s.data.rdb.Del(ctx, cache.AdamaGoodOverKey(workflow.GoodsID)).Err()
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE adama_order_workflows
		SET status = ?, stock_status = ?, cache_status = ?, sync_status = ?, updated_at = ?, last_error = ''
		WHERE order_id = ?`,
		seckill.OrderStatusCancelled, seckill.StockStatusReleased, seckill.CacheStatusReleased, seckill.SyncStatusCancelled,
		time.Now(), order.OrderId,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s adamaOrderRepo) MarkOrderSyncResult(ctx context.Context, orderID int64, dispatchErr error) error {
	lastError := ""
	if dispatchErr != nil {
		lastError = dispatchErr.Error()
	}
	_, err := s.data.msql.ExecContext(ctx, `
		UPDATE adama_order_workflows
		SET kafka_attempts = kafka_attempts + 1, last_error = ?, updated_at = ?
		WHERE order_id = ?`,
		lastError, time.Now(), orderID,
	)
	return err
}

func NewAdamaOrderRepo(data *Data, logger log.Logger) biz.AdamaOrderRepo {
	return &adamaOrderRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/server-service")),
	}
}

type workflowRecord struct {
	OrderID     int64
	UserID      int64
	GoodsID     int64
	Amount      int64
	StockToken  string
	Status      string
	StockStatus string
	CacheStatus string
	SyncStatus  string
}

func loadWorkflow(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, orderID int64) (*workflowRecord, error) {
	row := querier.QueryRowContext(ctx, `
		SELECT order_id, user_id, goods_id, amount, stock_token, status, stock_status, cache_status, sync_status
		FROM adama_order_workflows
		WHERE order_id = ?`, orderID)

	record := &workflowRecord{}
	err := row.Scan(
		&record.OrderID,
		&record.UserID,
		&record.GoodsID,
		&record.Amount,
		&record.StockToken,
		&record.Status,
		&record.StockStatus,
		&record.CacheStatus,
		&record.SyncStatus,
	)
	if err != nil {
		return nil, err
	}
	return record, nil
}
