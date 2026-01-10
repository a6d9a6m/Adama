package biz

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
	"time"
)

type AdamaOrder struct {
	UserId    int64
	OrderId   int64
	GoodsId   int64
	Amount    int64
	StockToken string
	ExpireAt  time.Time
}

type AdamaOrderRepo interface {
	CreateAdamaOrder(ctx context.Context, adamaOrder *AdamaOrder) error
	PostAdamaOrder(ctx context.Context, adamaOrder *AdamaOrder) error
	GetAdamaOrder(ctx context.Context, id int64) (*AdamaOrder, error)
	SendKafkaOrder(ctx context.Context, order *AdamaOrder) error
	PrepareAdamaOrder(ctx context.Context, order *AdamaOrder) error
	ConfirmAdamaOrder(ctx context.Context, order *AdamaOrder) error
	CancelAdamaOrder(ctx context.Context, order *AdamaOrder) error
	MarkOrderSyncResult(ctx context.Context, orderID int64, dispatchErr error) error
}

type AdamaOrderUsecase struct {
	repo AdamaOrderRepo
	log  *log.Helper
}

func NewAdamaOrderUsecase(repo AdamaOrderRepo, logger log.Logger) *AdamaOrderUsecase {
	return &AdamaOrderUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

func (uc *AdamaOrderUsecase) CreateAdamaOrder(ctx context.Context, o *AdamaOrder) error {

	return uc.repo.CreateAdamaOrder(ctx, o)
}

func (uc *AdamaOrderUsecase) PostOrder(ctx context.Context, o *AdamaOrder) error {
	return uc.repo.PostAdamaOrder(ctx, o)
}

func (uc *AdamaOrderUsecase) GetOrder(ctx context.Context, id int64) (*AdamaOrder, error) {
	return uc.repo.GetAdamaOrder(ctx, id)
}

func (uc *AdamaOrderUsecase) SendKafka(ctx context.Context, order *AdamaOrder) error {
	return uc.repo.SendKafkaOrder(ctx, order)
}

func (uc *AdamaOrderUsecase) Prepare(ctx context.Context, order *AdamaOrder) error {
	return uc.repo.PrepareAdamaOrder(ctx, order)
}

func (uc *AdamaOrderUsecase) Confirm(ctx context.Context, order *AdamaOrder) error {
	return uc.repo.ConfirmAdamaOrder(ctx, order)
}

func (uc *AdamaOrderUsecase) Cancel(ctx context.Context, order *AdamaOrder) error {
	return uc.repo.CancelAdamaOrder(ctx, order)
}

func (uc *AdamaOrderUsecase) MarkSyncResult(ctx context.Context, orderID int64, dispatchErr error) error {
	return uc.repo.MarkOrderSyncResult(ctx, orderID, dispatchErr)
}
