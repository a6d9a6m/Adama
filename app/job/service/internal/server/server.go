package server

import (
	"github.com/google/wire"
	"github.com/littleSand/adama/app/job/service/job"
)

// ProviderSet is server providers.
var ProviderSet = wire.NewSet(NewHTTPServer, NewGRPCServer, job.NewJOBServer)

