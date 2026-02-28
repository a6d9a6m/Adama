package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"golang.org/x/time/rate"
)

type RateLimitConfig struct {
	GlobalRPS         int
	GlobalBurst       int
	GoodsHotspotRPS   int
	GoodsHotspotBurst int
}

type rateLimitHandler struct {
	next          stdhttp.Handler
	log           *log.Helper
	globalLimiter *rate.Limiter

	mu           sync.Mutex
	goodsLimiter map[string]*goodsLimiterEntry
	cfg          RateLimitConfig
}

type goodsLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimitHandler(next stdhttp.Handler, cfg RateLimitConfig, logger log.Logger) stdhttp.Handler {
	handler := &rateLimitHandler{
		next:         next,
		log:          log.NewHelper(logger),
		goodsLimiter: make(map[string]*goodsLimiterEntry),
		cfg:          cfg,
	}
	if cfg.GlobalRPS > 0 && cfg.GlobalBurst > 0 {
		handler.globalLimiter = rate.NewLimiter(rate.Limit(cfg.GlobalRPS), cfg.GlobalBurst)
	}
	return handler
}

func (h *rateLimitHandler) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if h.globalLimiter != nil && !h.globalLimiter.Allow() {
		h.log.Warnf("gateway global rate limited: method=%s path=%s", request.Method, request.URL.Path)
		writeJSONError(writer, stdhttp.StatusServiceUnavailable, "gateway_rate_limited", "gateway global rate limit exceeded")
		return
	}

	if goodsKey := goodsHotspotKey(request); goodsKey != "" {
		if !h.allowGoods(goodsKey) {
			h.log.Warnf("gateway goods hotspot rate limited: method=%s path=%s key=%s", request.Method, request.URL.Path, goodsKey)
			writeJSONError(writer, stdhttp.StatusServiceUnavailable, "goods_hotspot_rate_limited", "goods hotspot rate limit exceeded")
			return
		}
	}

	h.next.ServeHTTP(writer, request)
}

func (h *rateLimitHandler) allowGoods(goodsKey string) bool {
	if h.cfg.GoodsHotspotRPS <= 0 || h.cfg.GoodsHotspotBurst <= 0 {
		return true
	}

	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.goodsLimiter) > 1024 {
		for key, entry := range h.goodsLimiter {
			if now.Sub(entry.lastSeen) > 10*time.Minute {
				delete(h.goodsLimiter, key)
			}
		}
	}

	entry := h.goodsLimiter[goodsKey]
	if entry == nil {
		entry = &goodsLimiterEntry{
			limiter: rate.NewLimiter(rate.Limit(h.cfg.GoodsHotspotRPS), h.cfg.GoodsHotspotBurst),
		}
		h.goodsLimiter[goodsKey] = entry
	}
	entry.lastSeen = now
	return entry.limiter.Allow()
}

func goodsHotspotKey(request *stdhttp.Request) string {
	path := request.URL.Path
	switch {
	case strings.HasPrefix(path, gatewayPrefix+"/adama/goods/"):
		return "goods:" + strings.TrimPrefix(path, gatewayPrefix+"/adama/goods/")
	case path == gatewayPrefix+"/adama/order" || path == gatewayPrefix+"/adama/order/":
		if request.Method != stdhttp.MethodPost {
			return ""
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return ""
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		var payload struct {
			GID int64 `json:"gid"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.GID <= 0 {
			return ""
		}
		return "goods:" + strconv.FormatInt(payload.GID, 10)
	default:
		return ""
	}
}
