package service

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
	pb "github.com/littleSand/adama/api/order/service/v1"
	"github.com/littleSand/adama/app/order/service/internal/biz"
	"github.com/littleSand/adama/pkg/seckill"
	"github.com/yedf/dtmcli"
	rr "github.com/go-resty/resty/v2"
)

func (s *OrderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderReply, error) {
	s.log.Infof("CreateOrder request: gid=%d amount=%d", req.Gid, req.Amount)

	order := &biz.Order{
		Gid:    req.Gid,
		Amount: req.Amount,
	}
	if err := s.uc.Create(ctx, order); err != nil {
		return nil, err
	}

	return &pb.CreateOrderReply{}, nil
}

func (s *OrderService) CreateAdamaOrder(ctx context.Context, req *pb.CreateAdamaOrderRequest) (*pb.CreateAdamaOrderReply, error) {
	if s.goods.GetAdamaGoodsOver(ctx, req.Gid) {
		return nil, errors.New(500, "GOODS_STOCK_EMPTY", "goods stock empty")
	}

	amount := req.Amount
	if amount <= 0 {
		amount = 1
	}

	order := &biz.AdamaOrder{
		UserId:  88,
		GoodsId: req.Gid,
		Amount:  amount,
	}
	if err := s.so.CreateAdamaOrder(ctx, order); err != nil {
		s.log.Error(err)
		return nil, err
	}

	order.ExpireAt = time.Now().Add(seckill.DefaultPaymentTTL)
	order.StockToken = seckill.StockToken{
		OrderID: order.OrderId,
		GoodsID: order.GoodsId,
		Amount:  order.Amount,
	}.Encode()

	if err := s.runAdamaTCC(ctx, order); err != nil {
		s.log.Error(err)
		return nil, err
	}

	dispatchErr := s.so.SendKafka(ctx, order)
	if markErr := s.so.MarkSyncResult(ctx, order.OrderId, dispatchErr); markErr != nil {
		s.log.Error(markErr)
	}
	if dispatchErr != nil {
		s.log.Warnf("adama order queued for async repair: order=%d err=%v", order.OrderId, dispatchErr)
	}

	return &pb.CreateAdamaOrderReply{DtmResult: "SUCCESS"}, nil
}

func (s *OrderService) CreateAdamaOrderTry(ctx context.Context, req *pb.CreateAdamaOrderRequest) (*pb.CreateAdamaOrderReply, error) {
	return &pb.CreateAdamaOrderReply{DtmResult: "SUCCESS"}, nil
}

func (s *OrderService) CreateAdamaOrderConfirm(ctx context.Context, req *pb.CreateAdamaOrderRequest) (*pb.CreateAdamaOrderReply, error) {
	return &pb.CreateAdamaOrderReply{DtmResult: "SUCCESS"}, nil
}

func (s *OrderService) CreateAdamaOrderCancel(ctx context.Context, req *pb.CreateAdamaOrderRequest) (*pb.CreateAdamaOrderReply, error) {
	return &pb.CreateAdamaOrderReply{DtmResult: "SUCCESS"}, nil
}

type AdamaOrderTCCRequest struct {
	OrderID    int64     `json:"order_id"`
	UserID     int64     `json:"user_id"`
	GoodsID    int64     `json:"goods_id"`
	Amount     int64     `json:"amount"`
	StockToken string    `json:"stock_token"`
	ExpireAt   time.Time `json:"expire_at"`
}

func (s *OrderService) HandleAdamaOrderTry(ctx context.Context, req *AdamaOrderTCCRequest) error {
	return s.so.Prepare(ctx, &biz.AdamaOrder{
		UserId:     req.UserID,
		OrderId:    req.OrderID,
		GoodsId:    req.GoodsID,
		Amount:     req.Amount,
		StockToken: req.StockToken,
		ExpireAt:   req.ExpireAt,
	})
}

func (s *OrderService) HandleAdamaOrderConfirm(ctx context.Context, req *AdamaOrderTCCRequest) error {
	return s.so.Confirm(ctx, &biz.AdamaOrder{
		UserId:     req.UserID,
		OrderId:    req.OrderID,
		GoodsId:    req.GoodsID,
		Amount:     req.Amount,
		StockToken: req.StockToken,
		ExpireAt:   req.ExpireAt,
	})
}

func (s *OrderService) HandleAdamaOrderCancel(ctx context.Context, req *AdamaOrderTCCRequest) error {
	return s.so.Cancel(ctx, &biz.AdamaOrder{
		UserId:     req.UserID,
		OrderId:    req.OrderID,
		GoodsId:    req.GoodsID,
		Amount:     req.Amount,
		StockToken: req.StockToken,
		ExpireAt:   req.ExpireAt,
	})
}

func (s *OrderService) runAdamaTCC(ctx context.Context, order *biz.AdamaOrder) error {
	const (
		dtmServer   = "http://127.0.0.1:8080/api/dtmsvr"
		goodsSvcURL = "http://127.0.0.1:8003"
		orderSvcURL = "http://127.0.0.1:8001"
	)

	gid := dtmcli.MustGenGid(dtmServer)
	req := &AdamaOrderTCCRequest{
		OrderID:    order.OrderId,
		UserID:     order.UserId,
		GoodsID:    order.GoodsId,
		Amount:     order.Amount,
		StockToken: order.StockToken,
		ExpireAt:   order.ExpireAt,
	}

	return dtmcli.TccGlobalTransaction(dtmServer, gid, func(tcc *dtmcli.Tcc) (*rr.Response, error) {
		if _, err := tcc.CallBranch(map[string]string{"sn": order.StockToken}, goodsSvcURL+"/ordersTry", goodsSvcURL+"/ordersConfirm", goodsSvcURL+"/ordersCancel"); err != nil {
			return nil, err
		}
		return tcc.CallBranch(req, orderSvcURL+"/adama/tcc/order/try", orderSvcURL+"/adama/tcc/order/confirm", orderSvcURL+"/adama/tcc/order/cancel")
	})
}
