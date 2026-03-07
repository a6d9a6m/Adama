package data

import (
	"context"
	"database/sql"
	"fmt"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/littleSand/adama/app/goods/service/internal/conf"
	"github.com/littleSand/adama/app/goods/service/internal/data/ent"
	"github.com/littleSand/adama/pkg/envutil"
	"github.com/littleSand/adama/pkg/poolutil"

	_ "github.com/go-sql-driver/mysql"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewGoodsRepo, NewOrdersRepo)

// Data holds database clients.
type Data struct {
	db  *ent.Client
	sql *sql.DB
}

// NewData creates the data layer.
func NewData(conf *conf.Data, logger log.Logger) (*Data, func(), error) {
	log := log.NewHelper(log.With(logger, "module", "server-service/data"))
	databaseSource := envutil.Get("MYSQL_DSN", conf.Database.Source)

	sqlDB, err := sql.Open(conf.Database.Driver, databaseSource)
	if err != nil {
		log.Errorf("failed opening connection to mysql: %v", err)
		return nil, nil, err
	}
	poolutil.ConfigureSQLDB(sqlDB, "GOODS")
	client := ent.NewClient(ent.Driver(entsql.OpenDB(conf.Database.Driver, sqlDB)))
	if err := client.Schema.Create(context.Background()); err != nil {
		log.Errorf("failed creating schema resources: %v", err)
		return nil, nil, err
	}

	d := &Data{
		db:  client,
		sql: sqlDB,
	}
	if err := ensureStockReservationSchema(d.sql); err != nil {
		return nil, nil, err
	}
	if err := ensureDTMBarrierSchema(d.sql); err != nil {
		return nil, nil, err
	}
	return d, func() {
		if err := d.db.Close(); err != nil {
			log.Error(err)
		}
	}, nil
}

func ensureStockReservationSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS adama_stock_reservations (
		order_id BIGINT PRIMARY KEY,
		goods_id BIGINT NOT NULL,
		amount BIGINT NOT NULL DEFAULT 1,
		status VARCHAR(32) NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		INDEX idx_goods_status(goods_id, status)
	)`)
	if err != nil {
		return fmt.Errorf("ensure stock reservation schema: %w", err)
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
