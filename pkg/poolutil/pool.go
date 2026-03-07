package poolutil

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-resty/resty/v2"
	"github.com/littleSand/adama/pkg/envutil"
)

func ConfigureSQLDB(db *sql.DB, prefix string) {
	if db == nil {
		return
	}
	db.SetMaxOpenConns(envutil.Int(prefix+"_DB_MAX_OPEN_CONNS", 32))
	db.SetMaxIdleConns(envutil.Int(prefix+"_DB_MAX_IDLE_CONNS", 8))
	db.SetConnMaxLifetime(envutil.Duration(prefix+"_DB_CONN_MAX_LIFETIME", 30*time.Minute))
	db.SetConnMaxIdleTime(envutil.Duration(prefix+"_DB_CONN_MAX_IDLE_TIME", 5*time.Minute))
}

func ConfigureRedisOptions(opts *redis.Options, prefix string) {
	if opts == nil {
		return
	}
	opts.PoolSize = envutil.Int(prefix+"_REDIS_POOL_SIZE", 64)
	opts.MinIdleConns = envutil.Int(prefix+"_REDIS_MIN_IDLE_CONNS", 8)
	opts.PoolTimeout = envutil.Duration(prefix+"_REDIS_POOL_TIMEOUT", 4*time.Second)
	opts.IdleTimeout = envutil.Duration(prefix+"_REDIS_IDLE_TIMEOUT", 5*time.Minute)
	opts.IdleCheckFrequency = envutil.Duration(prefix+"_REDIS_IDLE_CHECK_FREQUENCY", time.Minute)
}

func NewHTTPTransport(prefix string) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = envutil.Int(prefix+"_HTTP_MAX_IDLE_CONNS", 256)
	transport.MaxIdleConnsPerHost = envutil.Int(prefix+"_HTTP_MAX_IDLE_CONNS_PER_HOST", 64)
	transport.MaxConnsPerHost = envutil.Int(prefix+"_HTTP_MAX_CONNS_PER_HOST", 128)
	transport.IdleConnTimeout = envutil.Duration(prefix+"_HTTP_IDLE_CONN_TIMEOUT", 90*time.Second)
	transport.ResponseHeaderTimeout = envutil.Duration(prefix+"_HTTP_RESPONSE_HEADER_TIMEOUT", 3*time.Second)
	return transport
}

func NewHTTPClient(prefix string, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewHTTPTransport(prefix),
		Timeout:   timeout,
	}
}

func ConfigureRestyClient(client *resty.Client, prefix string, timeout time.Duration) {
	if client == nil {
		return
	}
	client.SetTransport(NewHTTPTransport(prefix))
	client.SetTimeout(timeout)
}
