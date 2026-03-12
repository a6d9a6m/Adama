package service

import (
	"context"
	stdhttp "net/http"
	"strconv"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	pb "github.com/littleSand/adama/api/goods/service/v1"
	"github.com/littleSand/adama/app/goods/service/internal/biz"
	"github.com/littleSand/adama/pkg/dtmutil"
)

func (s *GoodsService) GetGoods(ctx context.Context, req *pb.GetGoodsRequest) (*pb.GetGoodsReply, error) {
	rv, err := s.uc.Get(ctx, req.Id)
	s.log.Info("GetGoods request: ", rv, req)
	return &pb.GetGoodsReply{
		Title: rv.Title,
		Intro: rv.Intro,
	}, err
}

func (s *GoodsService) GetOrders(ctx context.Context, req *pb.GetOrdersRequest) (*pb.GetOrdersReply, error) {
	rv, err := s.oc.GetOrders(ctx, req.Id)

	s.log.Info("request: ", rv, req)
	return &pb.GetOrdersReply{
		Sn: rv.Sn,
	}, err
}

func (s *GoodsService) CreateOrders(ctx context.Context, req *pb.CreateOrdersRequest) (*pb.CreateOrdersReply, error) {
	err := s.oc.CreateOrders(ctx, biz.Orders{
		Sn: req.Sn,
	})

	s.log.Info("request: ", req)

	if err != nil {
		s.log.Error(err)
		return nil, err
	}

	return &pb.CreateOrdersReply{}, nil
}

// tcc-try
func (s *GoodsService) CreateOrdersTccTry(ctx context.Context, req *pb.CreateOrdersRequest) (*pb.CreateOrdersReply, error) {
	barrier, err := dtmutil.BarrierFromHTTPContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.oc.PrepareStockReservationBarrier(ctx, barrier, req.Sn); err != nil {
		return nil, err
	}
	return &pb.CreateOrdersReply{
		DtmResult: "SUCCESS",
	}, nil
}

// tcc-confirm
func (s *GoodsService) CreateOrdersConfirm(ctx context.Context, req *pb.CreateOrdersRequest) (*pb.CreateOrdersReply, error) {
	barrier, err := dtmutil.BarrierFromHTTPContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.oc.ConfirmStockReservationBarrier(ctx, barrier, req.Sn); err != nil {
		return nil, err
	}
	return &pb.CreateOrdersReply{
		DtmResult: "SUCCESS",
	}, nil
}

// tcc-cancel
func (s *GoodsService) CreateOrdersTccCancel(ctx context.Context, req *pb.CreateOrdersRequest) (*pb.CreateOrdersReply, error) {
	barrier, err := dtmutil.BarrierFromHTTPContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.oc.CancelStockReservationBarrier(ctx, barrier, req.Sn); err != nil {
		return nil, err
	}
	return &pb.CreateOrdersReply{
		DtmResult: "SUCCESS",
	}, nil
}

func (s *GoodsService) CancelOrdersInternal(ctx khttp.Context) error {
	var req pb.CreateOrdersRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if err := s.oc.CancelStockReservation(ctx, req.Sn); err != nil {
		return err
	}
	return ctx.JSON(stdhttp.StatusOK, map[string]string{"dtm_result": "SUCCESS"})
}

func (s *GoodsService) ListGoodsHTTP(ctx khttp.Context) error {
	page := goodsIntQuery(ctx.Request(), "page", 1)
	pageSize := goodsIntQuery(ctx.Request(), "page_size", 10)
	keyword := ctx.Request().URL.Query().Get("keyword")

	result, err := s.uc.List(ctx, biz.GoodsListQuery{
		Page:     page,
		PageSize: pageSize,
		Keyword:  keyword,
	})
	if err != nil {
		return err
	}

	return ctx.JSON(stdhttp.StatusOK, map[string]interface{}{
		"items": result.Items,
		"page":  result.Page,
		"size":  result.Size,
		"total": result.Total,
	})
}

func goodsIntQuery(req *stdhttp.Request, key string, defaultValue int) int {
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
