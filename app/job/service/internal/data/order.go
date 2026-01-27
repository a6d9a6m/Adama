package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/job/service/internal/biz"
	"github.com/littleSand/adama/pkg/cache"
	"github.com/littleSand/adama/pkg/seckill"
	"gorm.io/gorm/clause"
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
			"sync_status": seckill.SyncStatusSynced,
			"last_error":  "",
			"updated_at":  time.Now(),
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
		Where("status = ? AND sync_status <> ?", seckill.OrderStatusPendingPay, seckill.SyncStatusSynced).
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

func callGoodsCancel(ctx context.Context, stockToken string) error {
	payload, _ := json.Marshal(map[string]string{"sn": stockToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:8003/ordersCancel", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("goods cancel returned status %d", resp.StatusCode)
	}
	return nil
}
