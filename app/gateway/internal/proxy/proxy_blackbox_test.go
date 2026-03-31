package proxy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/gateway/internal/proxy"
	"github.com/littleSand/adama/pkg/requestctx"
)

func TestDispatcherProxiesRequestAndInjectsHeaders(t *testing.T) {
	t.Parallel()

	type observedRequest struct {
		Path       string `json:"path"`
		RequestID  string `json:"request_id"`
		TraceID    string `json:"trace_id"`
		ForwardURI string `json:"forward_uri"`
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(observedRequest{
			Path:       r.URL.Path,
			RequestID:  r.Header.Get(requestctx.HeaderRequestID),
			TraceID:    r.Header.Get(requestctx.HeaderTraceID),
			ForwardURI: r.Header.Get(requestctx.HeaderForwardedURI),
		})
	}))
	defer upstream.Close()

	target, err := proxy.NewUpstream("goods", upstream.URL, time.Second, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatalf("new upstream: %v", err)
	}

	dispatcher := proxy.NewDispatcher(proxy.DispatcherConfig{
		Logger: log.NewStdLogger(io.Discard),
		Routes: []proxy.Route{
			{Prefix: "/goods", Upstream: target},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/goods/list?page=1", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	dispatcher.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got observedRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Path != "/goods/list" {
		t.Fatalf("upstream path = %q", got.Path)
	}
	if got.RequestID == "" || got.TraceID == "" {
		t.Fatalf("expected request and trace IDs, got %+v", got)
	}
	if got.ForwardURI != "/goods/list?page=1" {
		t.Fatalf("forward uri = %q", got.ForwardURI)
	}
	if rec.Header().Get(requestctx.HeaderRequestID) == "" || rec.Header().Get(requestctx.HeaderTraceID) == "" {
		t.Fatal("expected response headers to include request and trace IDs")
	}
}

func TestDispatcherRejectsBlockedRoute(t *testing.T) {
	t.Parallel()

	dispatcher := proxy.NewDispatcher(proxy.DispatcherConfig{
		Logger:       log.NewStdLogger(io.Discard),
		BlockedExact: []string{"/order/orderTry"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/order/orderTry", nil)
	rec := httptest.NewRecorder()

	dispatcher.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRateLimitHandlerLimitsOrderTraffic(t *testing.T) {
	t.Parallel()

	hit := 0
	handler := proxy.NewRateLimitHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.WriteHeader(http.StatusOK)
	}), proxy.RateLimitConfig{
		OrderRPS:   1,
		OrderBurst: 1,
	}, log.NewStdLogger(io.Discard))

	first := httptest.NewRequest(http.MethodPost, "/api/v1/adama/order", io.NopCloser(strings.NewReader(`{"gid":1}`)))
	first.Header.Set("X-User-Id", "1")
	first.RemoteAddr = "127.0.0.1:10000"
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)

	second := httptest.NewRequest(http.MethodPost, "/api/v1/adama/order", io.NopCloser(strings.NewReader(`{"gid":1}`)))
	second.Header.Set("X-User-Id", "1")
	second.RemoteAddr = "127.0.0.1:10000"
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, second)

	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status = %d", firstRec.Code)
	}
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("second status = %d, want %d", secondRec.Code, http.StatusServiceUnavailable)
	}
	if hit != 1 {
		t.Fatalf("handler hit count = %d, want 1", hit)
	}
}
