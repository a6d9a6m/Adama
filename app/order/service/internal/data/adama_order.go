package data

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/bwmarrin/snowflake"
	dtmcli "github.com/dtm-labs/client/dtmcli"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
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
	node, err := snowflake.NewNode(1)
	if err != nil {
		s.log.Error("snowflake generate error")
		return err
	}

	order.OrderId = node.Generate().Int64()
	return nil
}

func (s adamaOrderRepo) ReserveSeckillOrder(ctx context.Context, order *biz.AdamaOrder, token string, userMarkerTTL time.Duration) error {
	if order.Amount <= 0 {
		order.Amount = 1
	}
	node, err := snowflake.NewNode(1)
	if err != nil {
		s.log.Error("snowflake generate error")
		return err
	}

	order.OrderId = node.Generate().Int64()
	tokenKey := cache.AdamaOrderTokenKey(order.UserId, order.GoodsId, token)
	keys := []string{
		cache.AdamaGoodStockKey(order.GoodsId),
		cache.AdamaGoodOverKey(order.GoodsId),
		tokenKey,
		cache.AdamaOrderUserSetKey(order.GoodsId),
	}
	args := []interface{}{
		strconv.FormatInt(order.UserId, 10),
		strconv.FormatInt(order.Amount, 10),
		strconv.FormatInt(int64(userMarkerTTL/time.Second), 10),
	}
	res, err := s.data.rdb.Eval(ctx, reserveSeckillOrderScript, keys, args...).Result()
	if err != nil {
		return err
	}
	code, remain, err := parseReserveResult(res)
	if err != nil {
		return err
	}
	switch code {
	case 0:
		if remain <= 0 {
			_ = s.data.rdb.Set(ctx, cache.AdamaGoodOverKey(order.GoodsId), true, 0).Err()
		}
		return nil
	case 1:
		return errors.New(400, "SECKILL_TOKEN_INVALID", "seckill token invalid or already used")
	case 2:
		return errors.New(429, "SECKILL_DUPLICATE_REQUEST", "duplicate seckill request")
	case 3:
		return errors.New(500, "GOODS_STOCK_EMPTY", "goods stock empty")
	default:
		return errors.New(500, "SECKILL_RESERVE_FAILED", "reserve seckill order failed")
	}
}

func (s adamaOrderRepo) PrepareAdamaOrder(ctx context.Context, order *biz.AdamaOrder) error {
	tx, err := s.data.msql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = s.PrepareAdamaOrderTx(ctx, tx, order); err != nil {
		return err
	}
	return tx.Commit()
}

func (s adamaOrderRepo) PrepareAdamaOrderTx(ctx context.Context, tx *sql.Tx, order *biz.AdamaOrder) error {
	now := time.Now()
	if order.Amount <= 0 {
		order.Amount = 1
	}
	_, err := tx.ExecContext(ctx, `
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

func (s adamaOrderRepo) PrepareAdamaOrderBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *biz.AdamaOrder) error {
	return barrier.CallWithDB(s.data.msql, func(tx *sql.Tx) error {
		return s.PrepareAdamaOrderTx(ctx, tx, order)
	})
}

func (s adamaOrderRepo) ConfirmAdamaOrder(ctx context.Context, order *biz.AdamaOrder) error {
	tx, err := s.data.msql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = s.ConfirmAdamaOrderTx(ctx, tx, order); err != nil {
		return err
	}
	return tx.Commit()
}

func (s adamaOrderRepo) ConfirmAdamaOrderTx(ctx context.Context, tx *sql.Tx, order *biz.AdamaOrder) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE adama_order_workflows
		SET status = ?, stock_status = ?, cache_status = ?, sync_status = ?, updated_at = ?
		WHERE order_id = ?`,
		seckill.OrderStatusPendingPay, seckill.StockStatusReserved, seckill.CacheStatusReserved, seckill.SyncStatusPending,
		time.Now(), order.OrderId,
	)
	return err
}

func (s adamaOrderRepo) ConfirmAdamaOrderBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *biz.AdamaOrder) error {
	return barrier.CallWithDB(s.data.msql, func(tx *sql.Tx) error {
		return s.ConfirmAdamaOrderTx(ctx, tx, order)
	})
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
	if err = s.CancelAdamaOrderTx(ctx, tx, order); err != nil {
		return err
	}
	return tx.Commit()
}

func (s adamaOrderRepo) CancelAdamaOrderTx(ctx context.Context, tx *sql.Tx, order *biz.AdamaOrder) error {
	workflow, err := loadWorkflowForUpdate(ctx, tx, order.OrderId)
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
		_ = s.data.rdb.SRem(ctx, cache.AdamaOrderUserSetKey(workflow.GoodsID), strconv.FormatInt(workflow.UserID, 10)).Err()
		_ = s.data.rdb.Del(ctx, cache.AdamaOrderIdempotencyKey(workflow.UserID, workflow.GoodsID)).Err()
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
	return nil
}

func (s adamaOrderRepo) CancelAdamaOrderBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *biz.AdamaOrder) error {
	return barrier.CallWithDB(s.data.msql, func(tx *sql.Tx) error {
		return s.CancelAdamaOrderTx(ctx, tx, order)
	})
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

func (s adamaOrderRepo) IssueSeckillToken(ctx context.Context, userID int64, goodsID int64, expireAt time.Time) (string, error) {
	token := uuid.NewString()
	ttl := time.Until(expireAt)
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if err := s.data.rdb.Set(ctx, cache.AdamaOrderTokenKey(userID, goodsID, token), "1", ttl).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s adamaOrderRepo) ConsumeSeckillToken(ctx context.Context, userID int64, goodsID int64, token string) error {
	key := cache.AdamaOrderTokenKey(userID, goodsID, token)
	deleted, err := s.data.rdb.Del(ctx, key).Result()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return errors.New(400, "SECKILL_TOKEN_INVALID", "seckill token invalid or already used")
	}
	return nil
}

func (s adamaOrderRepo) AcquireUserOrderLimit(ctx context.Context, userID int64, goodsID int64, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = seckill.DefaultPaymentTTL
	}
	ok, err := s.data.rdb.SetNX(ctx, cache.AdamaOrderIdempotencyKey(userID, goodsID), "1", ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New(429, "SECKILL_DUPLICATE_REQUEST", "duplicate seckill request")
	}
	return nil
}

func NewAdamaOrderRepo(data *Data, logger log.Logger) biz.AdamaOrderRepo {
	return &adamaOrderRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/server-service")),
	}
}

const reserveSeckillOrderScript = `
local stock = tonumber(redis.call("GET", KEYS[1]) or "-1")
local amount = tonumber(ARGV[2])
local marker_ttl = tonumber(ARGV[3]) or 0

if redis.call("EXISTS", KEYS[3]) == 0 then
	return {1, stock}
end

if redis.call("SISMEMBER", KEYS[4], ARGV[1]) == 1 then
	return {2, stock}
end

if stock < amount then
	if stock <= 0 then
		redis.call("SET", KEYS[2], "1")
	end
	return {3, stock}
end

local remain = redis.call("DECRBY", KEYS[1], amount)
redis.call("DEL", KEYS[3])
redis.call("SADD", KEYS[4], ARGV[1])
if marker_ttl > 0 then
	redis.call("EXPIRE", KEYS[4], marker_ttl)
end
if remain <= 0 then
	redis.call("SET", KEYS[2], "1")
else
	redis.call("DEL", KEYS[2])
end
return {0, remain}
`

func parseReserveResult(raw interface{}) (int64, int64, error) {
	values, ok := raw.([]interface{})
	if !ok || len(values) != 2 {
		return 0, 0, fmt.Errorf("unexpected reserve result: %T", raw)
	}
	code, ok := values[0].(int64)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected reserve code type: %T", values[0])
	}
	remain, ok := values[1].(int64)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected reserve remain type: %T", values[1])
	}
	return code, remain, nil
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

func loadWorkflowForUpdate(ctx context.Context, tx *sql.Tx, orderID int64) (*workflowRecord, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT order_id, user_id, goods_id, amount, stock_token, status, stock_status, cache_status, sync_status
		FROM adama_order_workflows
		WHERE order_id = ? FOR UPDATE`, orderID)

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
