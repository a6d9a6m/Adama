package server

import (
	"log"

	"github.com/go-kratos/etcd/registry"
	"github.com/google/wire"
	"github.com/littleSand/adama/pkg/envutil"
	etcd "go.etcd.io/etcd/client/v3"
)

// ProviderSet is server providers.
var ProviderSet = wire.NewSet(NewHTTPServer, NewGRPCServer, NewRegistrar)

// NewRegistrar etcd-service
func NewRegistrar() *registry.Registry {

	//sc := []constant.ServerConfig{
	//	*constant.NewServerConfig("192.168.2.27", 8848),
	//}
	//
	//cc := &constant.ClientConfig{
	//	NamespaceId:         "public", //namespace id
	//	TimeoutMs:           5000,
	//	NotLoadCacheAtStart: true,
	//	LogDir:              "/tmp/nacos/log",
	//	CacheDir:            "/tmp/nacos/cache",
	//	RotateTime:          "1h",
	//	MaxAge:              3,
	//	LogLevel:            "debug",
	//}
	//
	//// a more graceful way to create naming client
	//client, err := clients.NewNamingClient(
	//	vo.NacosClientParam{
	//		ClientConfig:  cc,
	//		ServerConfigs: sc,
	//	},
	//)
	//if err != nil {
	//	panic(err)
	//}
	//
	//return registry.New(client)

	client, err := etcd.New(etcd.Config{
		Endpoints: envutil.CSV("ETCD_ENDPOINTS", []string{"192.168.0.111:2379"}),
	})
	if err != nil {
		log.Fatal(err)
	}

	return registry.New(client)
}
