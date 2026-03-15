package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	stdhttp "net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"golang.org/x/time/rate"
)

type RateLimitConfig struct {
	TokenRPS            int
	TokenBurst          int
	OrderRPS            int
	OrderBurst          int
	UserGoodsOrderRPS   int
	UserGoodsOrderBurst int
}

type rateLimitHandler struct {
	next         stdhttp.Handler
	log          *log.Helper
	tokenLimiter *rate.Limiter
	orderLimiter *rate.Limiter

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
	if cfg.TokenRPS > 0 && cfg.TokenBurst > 0 {
		handler.tokenLimiter = rate.NewLimiter(rate.Limit(cfg.TokenRPS), cfg.TokenBurst)
	}
	if cfg.OrderRPS > 0 && cfg.OrderBurst > 0 {
		handler.orderLimiter = rate.NewLimiter(rate.Limit(cfg.OrderRPS), cfg.OrderBurst)
	}
	return handler
}

func (h *rateLimitHandler) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if requestScope := requestScope(request); requestScope == "token" && h.tokenLimiter != nil && !h.tokenLimiter.Allow() {
		h.log.Warnf("gateway token rate limited: method=%s path=%s", request.Method, request.URL.Path)
		writeJSONError(writer, stdhttp.StatusServiceUnavailable, "gateway_token_rate_limited", "gateway token rate limit exceeded")
		return
	} else if requestScope == "order" && h.orderLimiter != nil && !h.orderLimiter.Allow() {
		h.log.Warnf("gateway order rate limited: method=%s path=%s", request.Method, request.URL.Path)
		writeJSONError(writer, stdhttp.StatusServiceUnavailable, "gateway_order_rate_limited", "gateway order rate limit exceeded")
		return
	}

	if goodsKey := userGoodsOrderKey(request); goodsKey != "" {
		if !h.allowGoods(goodsKey) {
			h.log.Warnf("gateway user goods order rate limited: method=%s path=%s key=%s", request.Method, request.URL.Path, goodsKey)
			writeJSONError(writer, stdhttp.StatusServiceUnavailable, "user_goods_order_rate_limited", "user goods order rate limit exceeded")
			return
		}
	}

	h.next.ServeHTTP(writer, request)
}

func requestScope(request *stdhttp.Request) string {
	path := request.URL.Path
	switch {
	case request.Method == stdhttp.MethodGet && strings.HasPrefix(path, gatewayPrefix+"/adama/goods/"):
		return "token"
	case request.Method == stdhttp.MethodPost && (path == gatewayPrefix+"/adama/order" || path == gatewayPrefix+"/adama/order/"):
		return "order"
	default:
		return ""
	}
}

func (h *rateLimitHandler) allowGoods(goodsKey string) bool {
	if h.cfg.UserGoodsOrderRPS <= 0 || h.cfg.UserGoodsOrderBurst <= 0 {
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
			limiter: rate.NewLimiter(rate.Limit(h.cfg.UserGoodsOrderRPS), h.cfg.UserGoodsOrderBurst),
		}
		h.goodsLimiter[goodsKey] = entry
	}
	entry.lastSeen = now
	return entry.limiter.Allow()
}

func userGoodsOrderKey(request *stdhttp.Request) string {
	path := request.URL.Path
	if request.Method != stdhttp.MethodPost || (path != gatewayPrefix+"/adama/order" && path != gatewayPrefix+"/adama/order/") {
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
	subject := request.Header.Get("X-User-Id")
	if strings.TrimSpace(subject) == "" {
		subject = request.Header.Get("X-Forwarded-For")
	}
	if strings.TrimSpace(subject) == "" {
		host, _, splitErr := net.SplitHostPort(request.RemoteAddr)
		if splitErr == nil {
			subject = host
		} else {
			subject = request.RemoteAddr
		}
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	if idx := strings.Index(subject, ","); idx >= 0 {
		subject = strings.TrimSpace(subject[:idx])
	}
	return "order:" + subject + ":goods:" + strconv.FormatInt(payload.GID, 10)
}
