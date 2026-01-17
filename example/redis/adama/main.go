package main

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/littleSand/adama/pkg/cache"
)

var ctx = context.Background()

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "192.168.2.27:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	goodsId := 33
	stock := 99

	rdb.Set(ctx, cache.AdamaGoodStockKey(int64(goodsId)), stock, 0).Err()
	rdb.Set(ctx, cache.AdamaGoodOverKey(int64(goodsId)), true, 0).Err()

	fmt.Println("set adama stock ok")
}
