package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	dtmcli "github.com/dtm-labs/client/dtmcli"
	"github.com/go-kratos/kratos/v2/log"
	rr "github.com/go-resty/resty/v2"
	"github.com/littleSand/adama/app/job/service/internal/biz"
	"github.com/littleSand/adama/pkg/cache"
	"github.com/littleSand/adama/pkg/envutil"
	"github.com/littleSand/adama/pkg/poolutil"
	"github.com/littleSand/adama/pkg/seckill"
	"gorm.io/gorm/clause"
)

var taskUpstreamHTTPClient = poolutil.NewHTTPClient(
	"TASK_UPSTREAM",
	envutil.Duration("TASK_UPSTREAM_HTTP_TIMEOUT", 3*time.Second),
)

type orderRepo struct {
	data *Data
	log  *log.Helper
}

type AdamaOrder struct {
	ID      int64 `gorm:"column:id;primaryKey;autoIncrement"`
	UserId  int64 `gorm:"column:user_id"`
	OrderId int64 `gorm:"column:order_id;uniqueIndex"`
	GoodsId int64 `gorm:"column:goods_id"`
}

func (AdamaOrder) TableName() string {
	return "adama_orders"
}

type AdamaOrderWorkflow struct {
	OrderID       int64      `gorm:"column:order_id;primaryKey"`
	UserID        int64      `gorm:"column:user_id"`
	GoodsID       int64      `gorm:"column:goods_id;index:idx_status_expire,priority:1"`
	Amount        int64      `gorm:"column:amount"`
	StockToken    string     `gorm:"column:stock_token"`
	Status        string     `gorm:"column:status;index:idx_status_expire,priority:1"`
	StockStatus   string     `gorm:"column:stock_status"`
	CacheStatus   string     `gorm:"column:cache_status"`
	SyncStatus    string     `gorm:"column:sync_status;index:idx_sync_status,priority:1"`
	KafkaAttempts int        `gorm:"column:kafka_attempts"`
	LastError     string     `gorm:"column:last_error"`
	ExpireAt      time.Time  `gorm:"column:expire_at;index:idx_status_expire,priority:2"`
	PaidAt        *time.Time `gorm:"column:paid_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;index:idx_sync_status,priority:2"`
}

func (AdamaOrderWorkflow) TableName() string {
	return "adama_order_workflows"
}

func (o *orderRepo) CreateOrder(ctx context.Context, oo *biz.AdamaOrder) (*biz.AdamaOrder, error) {
	workflow, err := o.loadWorkflow(ctx, oo.OrderId)
	if err != nil {
		return nil, err
	}
	if workflow.Status == seckill.OrderStatusCancelled || workflow.Status == seckill.OrderStatusTimeoutClosed {
		return oo, nil
	}
	if workflow.SyncStatus == seckill.SyncStatusSynced {
		return oo, nil
	}
	if workflow.Status != seckill.OrderStatusPendingPay || workflow.StockStatus != seckill.StockStatusReserved {
		if err := o.runQueuedAdamaTCC(ctx, workflow); err != nil {
			_ = o.data.db.WithContext(ctx).
				Model(&AdamaOrderWorkflow{}).
				Where("order_id = ?", oo.OrderId).
				Updates(map[string]interface{}{
					"last_error": err.Error(),
					"updated_at": time.Now(),
				}).Error
			return nil, err
		}
	}

	order := AdamaOrder{
		UserId:  oo.UserId,
		OrderId: oo.OrderId,
		GoodsId: oo.GoodsId,
	}

	if err := o.data.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "order_id"}},
			DoNothing: true,
		}).
		Create(&order).Error; err != nil {
		return nil, err
	}

	if err := o.data.db.WithContext(ctx).
		Model(&AdamaOrderWorkflow{}).
		Where("order_id = ?", oo.OrderId).
		Updates(map[string]interface{}{
			"status":       seckill.OrderStatusPendingPay,
			"stock_status": seckill.StockStatusReserved,
			"cache_status": seckill.CacheStatusReserved,
			"sync_status":  seckill.SyncStatusSynced,
			"last_error":   "",
			"updated_at":   time.Now(),
		}).Error; err != nil {
		return nil, err
	}

	return &biz.AdamaOrder{
		OrderId: oo.OrderId,
		UserId:  oo.UserId,
		GoodsId: oo.GoodsId,
		Amount:  oo.Amount,
	}, nil
}

func (o *orderRepo) RepairPendingOrders(ctx context.Context, limit int) (int, error) {
	var workflows []AdamaOrderWorkflow
	if err := o.data.db.WithContext(ctx).
		Where("sync_status <> ? AND status IN ?", seckill.SyncStatusSynced, []string{seckill.OrderStatusPreparing, seckill.OrderStatusPendingPay}).
		Order("updated_at ASC").
		Limit(limit).
		Find(&workflows).Error; err != nil {
		return 0, err
	}

	repaired := 0
	for _, workflow := range workflows {
		if _, err := o.CreateOrder(ctx, &biz.AdamaOrder{
			UserId:     workflow.UserID,
			OrderId:    workflow.OrderID,
			GoodsId:    workflow.GoodsID,
			Amount:     workflow.Amount,
			StockToken: workflow.StockToken,
		}); err != nil {
			_ = o.data.db.WithContext(ctx).Model(&AdamaOrderWorkflow{}).
				Where("order_id = ?", workflow.OrderID).
				Updates(map[string]interface{}{
					"last_error": err.Error(),
					"updated_at": time.Now(),
				}).Error
			continue
		}
		repaired++
	}
	return repaired, nil
}

func (o *orderRepo) CloseExpiredOrders(ctx context.Context, now time.Time, limit int) (int, error) {
	var workflows []AdamaOrderWorkflow
	if err := o.data.db.WithContext(ctx).
		Where("(status = ? AND expire_at <= ?) OR (status = ? AND stock_status <> ?)", seckill.OrderStatusPendingPay, now, seckill.OrderStatusTimeoutClosed, seckill.StockStatusReleased).
		Order("expire_at ASC").
		Limit(limit).
		Find(&workflows).Error; err != nil {
		return 0, err
	}

	closed := 0
	for _, workflow := range workflows {
		if err := o.closeOneExpired(ctx, &workflow, now); err != nil {
			o.log.Errorf("close expired order failed: order=%d err=%v", workflow.OrderID, err)
			continue
		}
		closed++
	}
	return closed, nil
}

func (o *orderRepo) CheckStockConsistency(ctx context.Context, limit int) (int, error) {
	var workflows []AdamaOrderWorkflow
	if err := o.data.db.WithContext(ctx).
		Where("status = ? AND (stock_status <> ? OR cache_status <> ?)", seckill.OrderStatusPendingPay, seckill.StockStatusReserved, seckill.CacheStatusReserved).
		Order("updated_at ASC").
		Limit(limit).
		Find(&workflows).Error; err != nil {
		return 0, err
	}
	return len(workflows), nil
}

func (o *orderRepo) CollectWorkflowStats(ctx context.Context) (map[string]int64, error) {
	type statusCount struct {
		Status string
		Count  int64
	}
	var rows []statusCount
	if err := o.data.db.WithContext(ctx).
		Model(&AdamaOrderWorkflow{}).
		Select("status, count(*) as count").
		Group("status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	stats := make(map[string]int64, len(rows))
	for _, row := range rows {
		stats[row.Status] = row.Count
	}
	return stats, nil
}

func (o *orderRepo) closeOneExpired(ctx context.Context, workflow *AdamaOrderWorkflow, now time.Time) error {
	updates := map[string]interface{}{
		"status":     seckill.OrderStatusTimeoutClosed,
		"updated_at": now,
	}
	if err := o.data.db.WithContext(ctx).
		Model(&AdamaOrderWorkflow{}).
		Where("order_id = ?", workflow.OrderID).
		Updates(updates).Error; err != nil {
		return err
	}

	if workflow.CacheStatus != seckill.CacheStatusReleased {
		if err := o.data.rdb.IncrBy(ctx, cache.AdamaGoodStockKey(workflow.GoodsID), workflow.Amount).Err(); err != nil {
			return err
		}
		_ = o.data.rdb.Del(ctx, cache.AdamaGoodOverKey(workflow.GoodsID)).Err()
		_ = o.data.rdb.SRem(ctx, cache.AdamaOrderUserSetKey(workflow.GoodsID), fmt.Sprintf("%d", workflow.UserID)).Err()
		_ = o.data.rdb.Del(ctx, cache.AdamaOrderIdempotencyKey(workflow.UserID, workflow.GoodsID)).Err()
		if err := o.data.db.WithContext(ctx).
			Model(&AdamaOrderWorkflow{}).
			Where("order_id = ?", workflow.OrderID).
			Updates(map[string]interface{}{
				"cache_status": seckill.CacheStatusReleased,
				"updated_at":   time.Now(),
			}).Error; err != nil {
			return err
		}
	}

	if workflow.StockStatus != seckill.StockStatusReleased {
		if err := callGoodsCancel(ctx, workflow.StockToken); err != nil {
			_ = o.data.db.WithContext(ctx).
				Model(&AdamaOrderWorkflow{}).
				Where("order_id = ?", workflow.OrderID).
				Updates(map[string]interface{}{
					"last_error":   err.Error(),
					"updated_at":   time.Now(),
					"stock_status": workflow.StockStatus,
				}).Error
			return err
		}
		if err := o.data.db.WithContext(ctx).
			Model(&AdamaOrderWorkflow{}).
			Where("order_id = ?", workflow.OrderID).
			Updates(map[string]interface{}{
				"stock_status": seckill.StockStatusReleased,
				"last_error":   "",
				"updated_at":   time.Now(),
			}).Error; err != nil {
			return err
		}
	}

	return nil
}

func NewOrderRepo(data *Data, logger log.Logger) biz.OrderQueueRepo {
	return &orderRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (o *orderRepo) loadWorkflow(ctx context.Context, orderID int64) (*AdamaOrderWorkflow, error) {
	var workflow AdamaOrderWorkflow
	if err := o.data.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		First(&workflow).Error; err != nil {
		return nil, err
	}
	return &workflow, nil
}

type queuedOrderTCCRequest struct {
	OrderID    int64     `json:"order_id"`
	UserID     int64     `json:"user_id"`
	GoodsID    int64     `json:"goods_id"`
	Amount     int64     `json:"amount"`
	StockToken string    `json:"stock_token"`
	ExpireAt   time.Time `json:"expire_at"`
}

func (o *orderRepo) runQueuedAdamaTCC(ctx context.Context, workflow *AdamaOrderWorkflow) error {
	dtmServer := envutil.Get("DTM_SERVER_URL", "http://127.0.0.1:36789/api/dtmsvr")
	goodsSvcURL := envutil.Get("GOODS_SERVICE_URL", "http://127.0.0.1:8003")
	orderSvcURL := envutil.Get("ORDER_SERVICE_URL", "http://127.0.0.1:8001")

	gid, err := generateDTMGID(dtmServer)
	if err != nil {
		return err
	}

	req := &queuedOrderTCCRequest{
		OrderID:    workflow.OrderID,
		UserID:     workflow.UserID,
		GoodsID:    workflow.GoodsID,
		Amount:     workflow.Amount,
		StockToken: workflow.StockToken,
		ExpireAt:   workflow.ExpireAt,
	}

	return dtmcli.TccGlobalTransaction(dtmServer, gid, func(tcc *dtmcli.Tcc) (*rr.Response, error) {
		if _, err := tcc.CallBranch(map[string]string{"sn": workflow.StockToken}, goodsSvcURL+"/ordersTry", goodsSvcURL+"/ordersConfirm", goodsSvcURL+"/ordersCancel"); err != nil {
			return nil, err
		}
		return tcc.CallBranch(req, orderSvcURL+"/adama/tcc/order/try", orderSvcURL+"/adama/tcc/order/confirm", orderSvcURL+"/adama/tcc/order/cancel")
	})
}

func generateDTMGID(server string) (string, error) {
	res := map[string]string{}
	resp, err := dtmcli.GetRestyClient().R().SetResult(&res).Get(server + "/newGid")
	if err != nil || res["gid"] == "" {
		return "", fmt.Errorf("generate dtm gid failed: %v, resp: %v", err, resp)
	}
	return res["gid"], nil
}

func callGoodsCancel(ctx context.Context, stockToken string) error {
	payload, _ := json.Marshal(map[string]string{"sn": stockToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, envutil.Get("GOODS_CANCEL_URL", "http://127.0.0.1:8003/internal/orders/cancel"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := taskUpstreamHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("goods cancel returned status %d", resp.StatusCode)
	}
	return nil
}
