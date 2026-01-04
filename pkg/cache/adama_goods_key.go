package cache

import "fmt"

func AdamaGoodStockKey(id int64) string {
	return fmt.Sprintf("ADAMA:GOODS:%d:STOCK", id)
}

func AdamaGoodOverKey(id int64) string {
	return fmt.Sprintf("ADAMA:GOODS:%d:OVER", id)
}
