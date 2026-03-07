package data

import (
	"time"

	dtmcli "github.com/dtm-labs/client/dtmcli"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-redis/redis/v8"
	"github.com/google/wire"
	"github.com/littleSand/adama/app/job/service/internal/conf"
	"github.com/littleSand/adama/pkg/envutil"
	"github.com/littleSand/adama/pkg/poolutil"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	// init mysql driver
	_ "github.com/go-sql-driver/mysql"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewDB, NewGreeterRepo, NewOrderRepo)

// Data .
type Data struct {
	// TODO warpped database client
	db  *gorm.DB
	rdb *redis.Client
	log *log.Helper
}

func NewDB(conf *conf.Data, logger log.Logger) *gorm.DB {
	log := log.NewHelper(log.With(logger, "module", "order-service/data/gorm"))
	databaseSource := envutil.Get("MYSQL_DSN", conf.Database.Source)

	db, err := gorm.Open(mysql.Open(databaseSource), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed opening connection to mysql: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed getting sql db: %v", err)
	}
	poolutil.ConfigureSQLDB(sqlDB, "TASK")

	if err := db.AutoMigrate(&AdamaOrder{}); err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(&AdamaOrderWorkflow{}); err != nil {
		log.Fatal(err)
	}
	return db
}

// NewData .
func NewData(conf *conf.Data, db *gorm.DB, logger log.Logger) (*Data, func(), error) {

	log1 := log.NewHelper(log.With(logger, "module", "order-service/data"))
	redisOptions := &redis.Options{
		Addr:         conf.Redis.Addr,
		WriteTimeout: conf.Redis.WriteTimeout.AsDuration(),
		ReadTimeout:  conf.Redis.ReadTimeout.AsDuration(),
	}
	poolutil.ConfigureRedisOptions(redisOptions, "TASK")
	rdb := redis.NewClient(redisOptions)
	poolutil.ConfigureRestyClient(dtmcli.GetRestyClient(), "TASK_DTM", envutil.Duration("TASK_DTM_HTTP_TIMEOUT", 3*time.Second))

	cleanup := func() {
		logger.Log(log.LevelInfo, "closing the data resources")
	}
	return &Data{
		db:  db,
		rdb: rdb,
		log: log1,
	}, cleanup, nil
}
