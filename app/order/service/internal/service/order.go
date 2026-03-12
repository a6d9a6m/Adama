package service

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"time"

	dtmcli "github.com/dtm-labs/client/dtmcli"
	"github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	rr "github.com/go-resty/resty/v2"
	pb "github.com/littleSand/adama/api/order/service/v1"
	"github.com/littleSand/adama/app/order/service/internal/biz"
	"github.com/littleSand/adama/pkg/dtmutil"
	"github.com/littleSand/adama/pkg/envutil"
	"github.com/littleSand/adama/pkg/requestctx"
	"github.com/littleSand/adama/pkg/seckill"
)

const headerSeckillToken = "X-Seckill-Token"

func (s *OrderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderReply, error) {
	s.log.Infof("CreateOrder request: gid=%d amount=%d", req.Gid, req.Amount)

	userID, err := requiredUserID(ctx)
	if err != nil {
		return nil, err
	}
	order := &biz.Order{
		Uid:    userID,
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

	userID, err := requiredUserID(ctx)
	if err != nil {
		return nil, err
	}
	token := requestctx.HeaderValue(ctx, headerSeckillToken)
	if token == "" {
		return nil, errors.New(400, "SECKILL_TOKEN_REQUIRED", "seckill token required")
	}

	goods, err := s.goods.GetAdamaGoods(ctx, req.Gid)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if now.Before(goods.StartDate) {
		return nil, errors.New(400, "SECKILL_NOT_STARTED", "seckill not started")
	}
	if now.After(goods.EndDate) {
		return nil, errors.New(400, "SECKILL_ENDED", "seckill ended")
	}

	order := &biz.AdamaOrder{
		UserId:   userID,
		GoodsId:  req.Gid,
		Amount:   amount,
		ExpireAt: now.Add(seckill.DefaultPaymentTTL),
	}

	userMarkerTTL := time.Until(goods.EndDate)
	if userMarkerTTL <= 0 {
		userMarkerTTL = seckill.DefaultPaymentTTL
	}
	if err := s.so.Reserve(ctx, order, token, userMarkerTTL); err != nil {
		s.log.Error(err)
		return nil, err
	}

	order.StockToken = seckill.StockToken{
		OrderID: order.OrderId,
		GoodsID: order.GoodsId,
		Amount:  order.Amount,
	}.Encode()

	if err := s.so.Prepare(ctx, order); err != nil {
		s.log.Error(err)
		_ = s.so.Cancel(ctx, order)
		return nil, err
	}

	dispatchErr := s.so.SendKafka(ctx, order)
	if dispatchErr != nil {
		s.log.Warnf("adama order queue dispatch failed: order=%d err=%v", order.OrderId, dispatchErr)
		if cancelErr := s.so.Cancel(ctx, order); cancelErr != nil {
			s.log.Errorf("cancel queued adama order failed: order=%d err=%v", order.OrderId, cancelErr)
		}
		if markErr := s.so.MarkSyncResult(ctx, order.OrderId, dispatchErr); markErr != nil {
			s.log.Error(markErr)
		}
		return nil, dispatchErr
	}
	if markErr := s.so.MarkSyncResult(ctx, order.OrderId, nil); markErr != nil {
		s.log.Error(markErr)
	}

	return &pb.CreateAdamaOrderReply{DtmResult: "QUEUED"}, nil
}

func (s *OrderService) CreateAdamaOrderTry(ctx context.Context, req *pb.CreateAdamaOrderRequest) (*pb.CreateAdamaOrderReply, error) {
	return nil, errors.New(410, "DEPRECATED_TCC_ENDPOINT", "use /adama/tcc/order/try")
}

func (s *OrderService) CreateAdamaOrderConfirm(ctx context.Context, req *pb.CreateAdamaOrderRequest) (*pb.CreateAdamaOrderReply, error) {
	return nil, errors.New(410, "DEPRECATED_TCC_ENDPOINT", "use /adama/tcc/order/confirm")
}

func (s *OrderService) CreateAdamaOrderCancel(ctx context.Context, req *pb.CreateAdamaOrderRequest) (*pb.CreateAdamaOrderReply, error) {
	return nil, errors.New(410, "DEPRECATED_TCC_ENDPOINT", "use /adama/tcc/order/cancel")
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
	barrier, err := dtmutil.BarrierFromHTTPContext(ctx)
	if err != nil {
		return err
	}
	return s.so.PrepareWithBarrier(ctx, barrier, &biz.AdamaOrder{
		UserId:     req.UserID,
		OrderId:    req.OrderID,
		GoodsId:    req.GoodsID,
		Amount:     req.Amount,
		StockToken: req.StockToken,
		ExpireAt:   req.ExpireAt,
	})
}

func (s *OrderService) HandleAdamaOrderConfirm(ctx context.Context, req *AdamaOrderTCCRequest) error {
	barrier, err := dtmutil.BarrierFromHTTPContext(ctx)
	if err != nil {
		return err
	}
	return s.so.ConfirmWithBarrier(ctx, barrier, &biz.AdamaOrder{
		UserId:     req.UserID,
		OrderId:    req.OrderID,
		GoodsId:    req.GoodsID,
		Amount:     req.Amount,
		StockToken: req.StockToken,
		ExpireAt:   req.ExpireAt,
	})
}

func (s *OrderService) HandleAdamaOrderCancel(ctx context.Context, req *AdamaOrderTCCRequest) error {
	barrier, err := dtmutil.BarrierFromHTTPContext(ctx)
	if err != nil {
		return err
	}
	return s.so.CancelWithBarrier(ctx, barrier, &biz.AdamaOrder{
		UserId:     req.UserID,
		OrderId:    req.OrderID,
		GoodsId:    req.GoodsID,
		Amount:     req.Amount,
		StockToken: req.StockToken,
		ExpireAt:   req.ExpireAt,
	})
}

func (s *OrderService) runAdamaTCC(ctx context.Context, gid string, order *biz.AdamaOrder) error {
	dtmServer := envutil.Get("DTM_SERVER_URL", "http://127.0.0.1:36789/api/dtmsvr")
	goodsSvcURL := envutil.Get("GOODS_SERVICE_URL", "http://127.0.0.1:8003")
	orderSvcURL := envutil.Get("ORDER_SERVICE_URL", "http://127.0.0.1:8001")

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

func generateDTMGID(server string) (string, error) {
	res := map[string]string{}
	resp, err := dtmcli.GetRestyClient().R().SetResult(&res).Get(server + "/newGid")
	if err != nil || res["gid"] == "" {
		return "", errors.New(500, "DTM_UNAVAILABLE", fmt.Sprintf("generate dtm gid failed: %v, resp: %v", err, resp))
	}
	return res["gid"], nil
}

func (s *OrderService) ListOrdersHTTP(ctx khttp.Context) error {
	page := orderIntQuery(ctx.Request(), "page", 1)
	pageSize := orderIntQuery(ctx.Request(), "page_size", 10)

	userID, ok := requestctx.UserID(ctx)
	if !ok || userID <= 0 {
		userID = int64(orderIntQuery(ctx.Request(), "user_id", 0))
	}

	items, total, err := s.uc.List(ctx, userID, page, pageSize)
	if err != nil {
		return err
	}

	return ctx.JSON(stdhttp.StatusOK, map[string]interface{}{
		"items": items,
		"page":  page,
		"size":  pageSize,
		"total": total,
	})
}

func userIDFromContext(ctx context.Context, fallback int64) int64 {
	userID, ok := requestctx.UserID(ctx)
	if !ok || userID <= 0 {
		return fallback
	}
	return userID
}

func requiredUserID(ctx context.Context) (int64, error) {
	userID, ok := requestctx.UserID(ctx)
	if !ok || userID <= 0 {
		return 0, errors.New(401, "USER_CONTEXT_REQUIRED", "user context required")
	}
	return userID, nil
}

func orderIntQuery(req *stdhttp.Request, key string, defaultValue int) int {
	raw := req.URL.Query().Get(key)
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return value
}

func (s *OrderService) GetAdamaGoodsHTTP(ctx khttp.Context) error {
	goodsID, err := strconv.ParseInt(ctx.Vars().Get("id"), 10, 64)
	if err != nil {
		return err
	}

	userID, ok := requestctx.UserID(ctx)
	if !ok || userID <= 0 {
		userID = int64(orderIntQuery(ctx.Request(), "user_id", 0))
	}
	if userID <= 0 {
		return errors.New(401, "USER_CONTEXT_REQUIRED", "user context required")
	}

	goods, err := s.goods.GetAdamaGoods(ctx, goodsID)
	if err != nil {
		return err
	}

	now := time.Now()
	if now.Before(goods.StartDate) {
		return errors.New(400, "SECKILL_NOT_STARTED", "seckill not started")
	}
	if now.After(goods.EndDate) {
		return errors.New(400, "SECKILL_ENDED", "seckill ended")
	}

	token, err := s.so.IssueToken(ctx, userID, goodsID, goods.EndDate)
	if err != nil {
		return err
	}

	return ctx.JSON(stdhttp.StatusOK, map[string]interface{}{
		"goods_id":      goods.GoodsId,
		"adama_price":   goods.AdamaPrice,
		"stock_count":   goods.StockCount,
		"start_date":    goods.StartDate,
		"end_date":      goods.EndDate,
		"seckill_token": token,
		"token_header":  headerSeckillToken,
		"token_usage":   fmt.Sprintf("set %s on POST /adama/order", headerSeckillToken),
	})
}
