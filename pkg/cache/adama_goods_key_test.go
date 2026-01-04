package cache

import (
	"fmt"
	"testing"
)

func TestAdamaGoodStockKey(t *testing.T) {
	key := AdamaGoodStockKey(99)
	fmt.Println(key)
}
