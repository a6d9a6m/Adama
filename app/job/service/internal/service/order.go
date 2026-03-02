package service

import (
	"context"
	"time"

	"github.com/littleSand/adama/app/job/service/internal/biz"
)

// OrderService is an order service.
type OrderService struct {
	oc *biz.OrderQueueUsecase
}

func (s *OrderService) Create(ctx context.Context, m *biz.AdamaOrder) error {
	_, err := s.oc.Create(ctx, m)
	return err
}

// NewOrderService creates an order service.
func NewOrderService(oc *biz.OrderQueueUsecase) *OrderService {
	return &OrderService{oc: oc}
}

func (s *OrderService) RepairPending(ctx context.Context, limit int) (int, error) {
	return s.oc.RepairPending(ctx, limit)
}

func (s *OrderService) CloseExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	return s.oc.CloseExpired(ctx, now, limit)
}

func (s *OrderService) CheckStockConsistency(ctx context.Context, limit int) (int, error) {
	return s.oc.CheckStockConsistency(ctx, limit)
}

func (s *OrderService) CollectWorkflowStats(ctx context.Context) (map[string]int64, error) {
	return s.oc.CollectWorkflowStats(ctx)
}
