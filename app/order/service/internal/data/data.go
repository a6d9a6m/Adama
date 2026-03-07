package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	dtmcli "github.com/dtm-labs/client/dtmcli"
	"github.com/go-kratos/etcd/registry"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-redis/redis/extra/redisotel"
	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/wire"
	"github.com/littleSand/adama/app/order/service/internal/conf"
	"github.com/littleSand/adama/app/order/service/internal/data/ent"
	"github.com/littleSand/adama/app/order/service/internal/data/kafka"
	"github.com/littleSand/adama/app/order/service/internal/data/kafka/event"
	"github.com/littleSand/adama/pkg/envutil"
	"github.com/littleSand/adama/pkg/poolutil"
	ggrpc "google.golang.org/grpc"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewOrderRepo, NewAdamaOrderRepo, NewAdamaGoodsRepo)

// Data .
type Data struct {
	// TODO warpped database client
	db *ent.Client

	msql *sql.DB

	// Gconn *grpc.ClientConn
	userRpc *ggrpc.ClientConn

	rdb *redis.Client

	kafka event.Sender
}

// NewData .
func NewData(conf *conf.Data, logger log.Logger, rr *registry.Registry) (*Data, func(), error) {

	//orm ent
	log := log.NewHelper(log.With(logger, "module", "server-service/data"))
	databaseSource := envutil.Get("MYSQL_DSN", conf.Database.Source)

	msql, err := sql.Open("mysql", databaseSource)
	if err != nil {
		log.Errorf("failed opening connection to mysql: %v", err)
		return nil, nil, err
	}
	poolutil.ConfigureSQLDB(msql, "ORDER")
	client := ent.NewClient(ent.Driver(entsql.OpenDB(conf.Database.Driver, msql)))
	// Run the auto migration tool.
	if err := client.Schema.Create(context.Background()); err != nil {
		log.Errorf("failed creating schema resources: %v", err)
		return nil, nil, err
	}

	// redis
	redisOptions := &redis.Options{
		Addr:         conf.Redis.Addr,
		Password:     conf.Redis.Password,
		DB:           int(conf.Redis.Db),
		WriteTimeout: conf.Redis.WriteTimeout.AsDuration(),
		ReadTimeout:  conf.Redis.ReadTimeout.AsDuration(),
	}
	poolutil.ConfigureRedisOptions(redisOptions, "ORDER")
	rdb := redis.NewClient(redisOptions)
	rdb.AddHook(redisotel.TracingHook{})
	poolutil.ConfigureRestyClient(dtmcli.GetRestyClient(), "ORDER_DTM", envutil.Duration("ORDER_DTM_HTTP_TIMEOUT", 3*time.Second))

	//gRpc
	userEndpoint := envutil.Get("USER_GRPC_ENDPOINT", "")
	var userRpc *ggrpc.ClientConn
	if userEndpoint != "" {
		userRpc, err = grpc.DialInsecure(
			context.Background(),
			grpc.WithEndpoint(userEndpoint),
		)
	} else {
		userRpc, err = grpc.DialInsecure(
			context.Background(),
			grpc.WithEndpoint("discovery:///user.grpc"),
			grpc.WithDiscovery(rr),
		)
	}

	if err != nil {
		panic("grpc-error")
	}

	//kafka-sender
	senderClient, err1 := kafka.NewKafkaSender(strings.Split(conf.Kafka.Addr, ","), "order")

	if err1 != nil {
		panic("kafka-error")
	}

	d := &Data{
		db:      client,
		msql:    msql,
		userRpc: userRpc,
		rdb:     rdb,
		kafka:   senderClient,
	}
	if err := ensureOrderWorkflowSchema(d.msql); err != nil {
		return nil, nil, err
	}
	if err := ensureDTMBarrierSchema(d.msql); err != nil {
		return nil, nil, err
	}
	return d, func() {
		if err := d.db.Close(); err != nil {
			log.Error(err)
		}
	}, nil
}

func ensureOrderWorkflowSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS adama_order_workflows (
			order_id BIGINT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			goods_id BIGINT NOT NULL,
			amount BIGINT NOT NULL DEFAULT 1,
			stock_token VARCHAR(128) NOT NULL DEFAULT '',
			status VARCHAR(32) NOT NULL,
			stock_status VARCHAR(32) NOT NULL,
			cache_status VARCHAR(32) NOT NULL,
			sync_status VARCHAR(32) NOT NULL,
			kafka_attempts INT NOT NULL DEFAULT 0,
			last_error VARCHAR(255) NOT NULL DEFAULT '',
			expire_at DATETIME NOT NULL,
			paid_at DATETIME NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			INDEX idx_status_expire(status, expire_at),
			INDEX idx_sync_status(sync_status, updated_at)
		)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("ensure order workflow schema: %w", err)
		}
	}
	return nil
}

func ensureDTMBarrierSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS dtm_barrier.barrier (
		id BIGINT(22) NOT NULL AUTO_INCREMENT,
		trans_type VARCHAR(45) NOT NULL,
		gid VARCHAR(128) NOT NULL,
		branch_id VARCHAR(128) NOT NULL,
		op VARCHAR(45) NOT NULL,
		barrier_id VARCHAR(45) NOT NULL,
		reason VARCHAR(45) NOT NULL,
		create_time DATETIME DEFAULT NULL,
		update_time DATETIME DEFAULT NULL,
		PRIMARY KEY (id),
		UNIQUE KEY uniq_barrier (trans_type, gid, branch_id, op, barrier_id)
	)`)
	if err != nil {
		return fmt.Errorf("ensure dtm barrier schema: %w", err)
	}
	return nil
}
