package dtmutil

import (
	"context"
	"fmt"

	dtmcli "github.com/dtm-labs/client/dtmcli"
	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// BarrierFromHTTPContext extracts DTM branch barrier metadata from an HTTP request context.
func BarrierFromHTTPContext(ctx context.Context) (*dtmcli.BranchBarrier, error) {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return nil, fmt.Errorf("transport context not found")
	}
	ht, ok := tr.(*khttp.Transport)
	if !ok || ht.Request() == nil {
		return nil, fmt.Errorf("http transport context not found")
	}
	barrier, err := dtmcli.BarrierFromQuery(ht.Request().URL.Query())
	if err != nil {
		return nil, err
	}
	barrier.DBType = dtmcli.DBTypeMysql
	return barrier, nil
}
