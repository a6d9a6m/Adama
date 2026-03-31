package proxy

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTrimGatewayPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "normal path", path: "/api/v1/goods/list", want: "/goods/list"},
		{name: "missing slash after prefix", path: "/api/v1goods/list", wantErr: true},
		{name: "prefix only", path: "/api/v1", wantErr: true},
		{name: "prefix with slash", path: "/api/v1/", wantErr: true},
		{name: "outside prefix", path: "/goods/list", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := trimGatewayPrefix(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for path %q", tc.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("trimGatewayPrefix(%q) error = %v", tc.path, err)
			}
			if got != tc.want {
				t.Fatalf("trimGatewayPrefix(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestJoinURLPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		basePath    string
		requestPath string
		want        string
	}{
		{basePath: "", requestPath: "/goods/list", want: "/goods/list"},
		{basePath: "/api", requestPath: "/goods/list", want: "/api/goods/list"},
		{basePath: "/api/", requestPath: "/goods/list", want: "/api/goods/list"},
		{basePath: "/api", requestPath: "goods/list", want: "/api/goods/list"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.basePath+"_"+tc.requestPath, func(t *testing.T) {
			t.Parallel()
			got := joinURLPath(tc.basePath, tc.requestPath)
			if got != tc.want {
				t.Fatalf("joinURLPath(%q, %q) = %q, want %q", tc.basePath, tc.requestPath, got, tc.want)
			}
		})
	}
}

func TestUserGoodsOrderKey(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPost, gatewayPrefix+"/adama/order", io.NopCloser(strings.NewReader(`{"gid":12}`)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-User-Id", "42")
	req.RemoteAddr = "127.0.0.1:10086"

	key := userGoodsOrderKey(req)
	if key != "order:42:goods:12" {
		t.Fatalf("userGoodsOrderKey = %q", key)
	}

	body, _ := io.ReadAll(req.Body)
	if got := string(body); got != `{"gid":12}` {
		t.Fatalf("request body not restored, got %q", got)
	}
}

func TestUserGoodsOrderKeyFallbackToForwardedFor(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPost, gatewayPrefix+"/adama/order", io.NopCloser(strings.NewReader(`{"gid":99}`)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")

	key := userGoodsOrderKey(req)
	if key != "order:10.0.0.1:goods:99" {
		t.Fatalf("userGoodsOrderKey fallback = %q", key)
	}
}

func TestAllowGoodsCleansExpiredEntries(t *testing.T) {
	t.Parallel()

	handler := &rateLimitHandler{
		goodsLimiter: make(map[string]*goodsLimiterEntry),
		cfg: RateLimitConfig{
			UserGoodsOrderRPS:   1,
			UserGoodsOrderBurst: 1,
		},
	}

	expiredAt := time.Now().Add(-11 * time.Minute)
	for i := 0; i < 1025; i++ {
		handler.goodsLimiter[strings.Repeat("k", 4)+"-"+time.Duration(i).String()] = &goodsLimiterEntry{
			lastSeen: expiredAt,
		}
	}

	if !handler.allowGoods("order:user:goods:1") {
		t.Fatal("expected first request to pass limiter")
	}
	if len(handler.goodsLimiter) >= 1025 {
		t.Fatalf("expected expired limiter entries to be cleaned, got %d entries", len(handler.goodsLimiter))
	}
}
