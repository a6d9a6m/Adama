package data

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-redis/redis/extra/redisotel"
	"github.com/go-redis/redis/v8"
	"github.com/google/wire"
	"github.com/littleSand/adama/app/user/service/internal/conf"
	"github.com/littleSand/adama/app/user/service/internal/data/ent"
	"github.com/littleSand/adama/pkg/envutil"

	_ "github.com/go-sql-driver/mysql"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewUserRepo)

// Data .
type Data struct {
	// TODO warpped database client
	//db *ent.Client
	db  *ent.Client
	sql *sql.DB

	rdb *redis.Client
}

// NewData .
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
	// Run the auto migration tool.
	if err := client.Schema.Create(context.Background()); err != nil {
		log.Errorf("failed creating schema resources: %v", err)
		return nil, nil, err
	}

	sqlDB, err := sql.Open("mysql", databaseSource)
	if err != nil {
		return nil, nil, err
	}

	// redis
	rdb := redis.NewClient(&redis.Options{
		Addr:         conf.Redis.Addr,
		Password:     conf.Redis.Password,
		DB:           int(conf.Redis.Db),
		WriteTimeout: conf.Redis.WriteTimeout.AsDuration(),
		ReadTimeout:  conf.Redis.ReadTimeout.AsDuration(),
	})
	rdb.AddHook(redisotel.TracingHook{})

	d := &Data{
		db:  client,
		sql: sqlDB,
		rdb: rdb,
	}
	if err := ensureUserAddressSchema(d.sql); err != nil {
		return nil, nil, err
	}
	return d, func() {
		if err := d.db.Close(); err != nil {
			log.Error(err)
		}
	}, nil
}

func ensureUserAddressSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS user_addresses (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		user_id BIGINT NOT NULL,
		consignee VARCHAR(64) NOT NULL,
		phone VARCHAR(32) NOT NULL DEFAULT '',
		province VARCHAR(64) NOT NULL DEFAULT '',
		city VARCHAR(64) NOT NULL DEFAULT '',
		detail VARCHAR(255) NOT NULL DEFAULT '',
		is_default TINYINT(1) NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		INDEX idx_user_id(user_id)
	)`)
	if err != nil {
		return fmt.Errorf("ensure user address schema: %w", err)
	}
	return nil
}
