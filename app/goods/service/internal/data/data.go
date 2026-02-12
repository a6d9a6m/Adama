package data

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/littleSand/adama/app/goods/service/internal/conf"
	"github.com/littleSand/adama/app/goods/service/internal/data/ent"
	"github.com/littleSand/adama/pkg/envutil"
	"github.com/yedf/dtmcli"

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

	client, err := ent.Open(
		conf.Database.Driver,
		databaseSource,
	)
	if err != nil {
		log.Errorf("failed opening connection to sqlite: %v", err)
		return nil, nil, err
	}
	if err := client.Schema.Create(context.Background()); err != nil {
		log.Errorf("failed creating schema resources: %v", err)
		return nil, nil, err
	}

	var dbSQLDTMConf = map[string]string{
		"driver":   conf.Database.Driver,
		"host":     envutil.Get("DTM_DB_HOST", "192.168.0.111"),
		"user":     "root",
		"password": "Root@123456",
		"port":     envutil.Get("DTM_DB_PORT", "3307"),
	}
	dbSQLDTM, err := dtmcli.SdbGet(dbSQLDTMConf)
	dtmcli.FatalIfError(err)

	d := &Data{
		db:  client,
		sql: dbSQLDTM,
	}
	if err := ensureStockReservationSchema(d.sql); err != nil {
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

// BeginTx starts a mysql transaction for dtm integration.
func BeginTx(db *sql.DB) *sql.Tx {
	tx, err := db.Begin()
	dtmcli.FatalIfError(err)
	return tx
}
