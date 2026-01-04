package seckill

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	OrderStatusPreparing     = "preparing"
	OrderStatusPendingPay    = "pending_payment"
	OrderStatusCancelled     = "cancelled"
	OrderStatusTimeoutClosed = "timeout_closed"
	OrderStatusPaid          = "paid"

	StockStatusReserving = "reserving"
	StockStatusReserved  = "reserved"
	StockStatusReleased  = "released"

	CacheStatusReserved = "reserved"
	CacheStatusReleased = "released"

	SyncStatusPending   = "pending"
	SyncStatusSynced    = "synced"
	SyncStatusCancelled = "cancelled"
)

const DefaultPaymentTTL = 15 * time.Minute

type StockToken struct {
	OrderID int64
	GoodsID int64
	Amount  int64
}

func (t StockToken) Encode() string {
	return fmt.Sprintf("%d:%d:%d", t.OrderID, t.GoodsID, t.Amount)
}

func ParseStockToken(raw string) (StockToken, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return StockToken{}, fmt.Errorf("invalid stock token: %q", raw)
	}

	orderID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return StockToken{}, fmt.Errorf("parse order id: %w", err)
	}
	goodsID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return StockToken{}, fmt.Errorf("parse goods id: %w", err)
	}
	amount, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return StockToken{}, fmt.Errorf("parse amount: %w", err)
	}
	if amount <= 0 {
		return StockToken{}, fmt.Errorf("invalid amount: %d", amount)
	}

	return StockToken{
		OrderID: orderID,
		GoodsID: goodsID,
		Amount:  amount,
	}, nil
}
