package biz

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
	"time"
)

type OrderQueueRepo interface {
	CreateOrder(ctx context.Context, o *AdamaOrder) (*AdamaOrder, error)
	RepairPendingOrders(ctx context.Context, limit int) (int, error)
	CloseExpiredOrders(ctx context.Context, now time.Time, limit int) (int, error)
	CheckStockConsistency(ctx context.Context, limit int) (int, error)
	CollectWorkflowStats(ctx context.Context) (map[string]int64, error)
}

type AdamaOrder struct {
	UserId     int64
	OrderId    int64
	GoodsId    int64
	Amount     int64
	StockToken string
}

type OrderQueueUsecase struct {
	repo OrderQueueRepo
	log  *log.Helper
}

func NewOrderQueueUsecase(repo OrderQueueRepo, logger log.Logger) *OrderQueueUsecase {
	return &OrderQueueUsecase{repo: repo, log: log.NewHelper(logger)}
}

func (uc *OrderQueueUsecase) Create(ctx context.Context, o *AdamaOrder) (*AdamaOrder, error) {
	return uc.repo.CreateOrder(ctx, o)
}

func (uc *OrderQueueUsecase) RepairPending(ctx context.Context, limit int) (int, error) {
	return uc.repo.RepairPendingOrders(ctx, limit)
}

func (uc *OrderQueueUsecase) CloseExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	return uc.repo.CloseExpiredOrders(ctx, now, limit)
}

func (uc *OrderQueueUsecase) CheckStockConsistency(ctx context.Context, limit int) (int, error) {
	return uc.repo.CheckStockConsistency(ctx, limit)
}

func (uc *OrderQueueUsecase) CollectWorkflowStats(ctx context.Context) (map[string]int64, error) {
	return uc.repo.CollectWorkflowStats(ctx)
}
