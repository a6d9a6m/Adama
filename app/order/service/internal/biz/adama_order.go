package biz

import (
	"context"
	"database/sql"
	dtmcli "github.com/dtm-labs/client/dtmcli"
	"github.com/go-kratos/kratos/v2/log"
	"time"
)

type AdamaOrder struct {
	UserId     int64
	OrderId    int64
	GoodsId    int64
	Amount     int64
	StockToken string
	ExpireAt   time.Time
}

type AdamaOrderRepo interface {
	CreateAdamaOrder(ctx context.Context, adamaOrder *AdamaOrder) error
	PostAdamaOrder(ctx context.Context, adamaOrder *AdamaOrder) error
	GetAdamaOrder(ctx context.Context, id int64) (*AdamaOrder, error)
	SendKafkaOrder(ctx context.Context, order *AdamaOrder) error
	PrepareAdamaOrder(ctx context.Context, order *AdamaOrder) error
	ConfirmAdamaOrder(ctx context.Context, order *AdamaOrder) error
	CancelAdamaOrder(ctx context.Context, order *AdamaOrder) error
	PrepareAdamaOrderTx(ctx context.Context, tx *sql.Tx, order *AdamaOrder) error
	ConfirmAdamaOrderTx(ctx context.Context, tx *sql.Tx, order *AdamaOrder) error
	CancelAdamaOrderTx(ctx context.Context, tx *sql.Tx, order *AdamaOrder) error
	PrepareAdamaOrderBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *AdamaOrder) error
	ConfirmAdamaOrderBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *AdamaOrder) error
	CancelAdamaOrderBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *AdamaOrder) error
	MarkOrderSyncResult(ctx context.Context, orderID int64, dispatchErr error) error
	IssueSeckillToken(ctx context.Context, userID int64, goodsID int64, expireAt time.Time) (string, error)
	ConsumeSeckillToken(ctx context.Context, userID int64, goodsID int64, token string) error
	AcquireUserOrderLimit(ctx context.Context, userID int64, goodsID int64, ttl time.Duration) error
	ReserveSeckillOrder(ctx context.Context, order *AdamaOrder, token string, userMarkerTTL time.Duration) error
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

func (uc *AdamaOrderUsecase) PrepareWithBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *AdamaOrder) error {
	return uc.repo.PrepareAdamaOrderBarrier(ctx, barrier, order)
}

func (uc *AdamaOrderUsecase) ConfirmWithBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *AdamaOrder) error {
	return uc.repo.ConfirmAdamaOrderBarrier(ctx, barrier, order)
}

func (uc *AdamaOrderUsecase) CancelWithBarrier(ctx context.Context, barrier *dtmcli.BranchBarrier, order *AdamaOrder) error {
	return uc.repo.CancelAdamaOrderBarrier(ctx, barrier, order)
}

func (uc *AdamaOrderUsecase) MarkSyncResult(ctx context.Context, orderID int64, dispatchErr error) error {
	return uc.repo.MarkOrderSyncResult(ctx, orderID, dispatchErr)
}

func (uc *AdamaOrderUsecase) IssueToken(ctx context.Context, userID int64, goodsID int64, expireAt time.Time) (string, error) {
	return uc.repo.IssueSeckillToken(ctx, userID, goodsID, expireAt)
}

func (uc *AdamaOrderUsecase) ConsumeToken(ctx context.Context, userID int64, goodsID int64, token string) error {
	return uc.repo.ConsumeSeckillToken(ctx, userID, goodsID, token)
}

func (uc *AdamaOrderUsecase) AcquireUserLimit(ctx context.Context, userID int64, goodsID int64, ttl time.Duration) error {
	return uc.repo.AcquireUserOrderLimit(ctx, userID, goodsID, ttl)
}

func (uc *AdamaOrderUsecase) Reserve(ctx context.Context, order *AdamaOrder, token string, userMarkerTTL time.Duration) error {
	return uc.repo.ReserveSeckillOrder(ctx, order, token, userMarkerTTL)
}
