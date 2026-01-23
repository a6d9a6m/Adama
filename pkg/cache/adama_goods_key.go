package cache

import "fmt"

func AdamaGoodStockKey(id int64) string {
	return fmt.Sprintf("ADAMA:GOODS:%d:STOCK", id)
}

func AdamaGoodOverKey(id int64) string {
	return fmt.Sprintf("ADAMA:GOODS:%d:OVER", id)
}

func AdamaOrderTokenKey(userID, goodsID int64, token string) string {
	return fmt.Sprintf("ADAMA:ORDER:TOKEN:%d:%d:%s", userID, goodsID, token)
}

func AdamaOrderIdempotencyKey(userID, goodsID int64) string {
	return fmt.Sprintf("ADAMA:ORDER:IDEMPOTENT:%d:%d", userID, goodsID)
}
