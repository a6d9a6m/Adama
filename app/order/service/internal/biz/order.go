package biz

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
)

type Order struct {
	Id     int64
	Uid    int64
	Gid    int64
	Amount int64
	Sn     string
}

type OrderRepo interface {
	CreateOrder(ctx context.Context, order *Order) error
	ListOrders(ctx context.Context, userID int64, page int, pageSize int) ([]Order, int64, error)
}

type OrderUsecase struct {
	repo OrderRepo
	log  *log.Helper
}

func NewOrderUsecase(repo OrderRepo, logger log.Logger) *OrderUsecase {
	return &OrderUsecase{repo: repo, log: log.NewHelper(logger)}
}

func (uc *OrderUsecase) Create(ctx context.Context, order *Order) error {
	return uc.repo.CreateOrder(ctx, order)
}

func (uc *OrderUsecase) List(ctx context.Context, userID int64, page int, pageSize int) ([]Order, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return uc.repo.ListOrders(ctx, userID, page, pageSize)
}

type OrderQueueRepo interface {
	CreateOrder(ctx context.Context) error
}

type OrderQueueUsecase struct {
	repo OrderQueueRepo
	log  *log.Helper
}

func NewOrderQueueUsecase(repo OrderQueueRepo, logger log.Logger) *OrderQueueUsecase {
	return &OrderQueueUsecase{repo: repo, log: log.NewHelper(logger)}
}

func (uc *OrderQueueUsecase) CreateQueue(ctx context.Context) error {
	uc.log.Info("queue test")
	return uc.repo.CreateOrder(ctx)
}

type OrderQueueReceiverRepo interface {
	CreateOrder(ctx context.Context) error
}

type OrderQueueReceiverUsecase struct {
	repo OrderQueueReceiverRepo
	log  *log.Helper
}

func NewOrderQueueReceiverUsecase(repo OrderQueueReceiverRepo, logger log.Logger) *OrderQueueReceiverUsecase {
	return &OrderQueueReceiverUsecase{repo: repo, log: log.NewHelper(logger)}
}

func (uc *OrderQueueReceiverUsecase) CreateQueue(ctx context.Context) error {
	uc.log.Info("queue test")
	return uc.repo.CreateOrder(ctx)
}
