package requestctx

import (
	"context"
	"net"
	"net/http"
	"strconv"

	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/uuid"
)

const (
	HeaderRequestID       = "X-Request-Id"
	HeaderTraceID         = "X-Trace-Id"
	HeaderUserID          = "X-User-Id"
	HeaderForwardedFor    = "X-Forwarded-For"
	HeaderForwardedProto  = "X-Forwarded-Proto"
	HeaderForwardedHost   = "X-Forwarded-Host"
	HeaderForwardedURI    = "X-Forwarded-Uri"
	HeaderForwardedPrefix = "X-Forwarded-Prefix"
)

func EnsureHeaders(req *http.Request, forwardedPrefix string) (string, string) {
	requestID := firstNonEmpty(req.Header.Get(HeaderRequestID), uuid.NewString())
	traceID := firstNonEmpty(req.Header.Get(HeaderTraceID), requestID)

	req.Header.Set(HeaderRequestID, requestID)
	req.Header.Set(HeaderTraceID, traceID)
	req.Header.Set(HeaderForwardedProto, scheme(req))
	req.Header.Set(HeaderForwardedHost, req.Host)
	req.Header.Set(HeaderForwardedURI, req.URL.RequestURI())
	if forwardedPrefix != "" {
		req.Header.Set(HeaderForwardedPrefix, forwardedPrefix)
	}

	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil && host != "" {
		appendForwardedFor(req.Header, host)
	}

	return requestID, traceID
}

func UserID(ctx context.Context) (int64, bool) {
	value := HeaderValue(ctx, HeaderUserID)
	if value == "" {
		return 0, false
	}
	userID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return userID, true
}

func RequestID(ctx context.Context) string {
	return HeaderValue(ctx, HeaderRequestID)
}

func TraceID(ctx context.Context) string {
	return HeaderValue(ctx, HeaderTraceID)
}

func HeaderValue(ctx context.Context, key string) string {
	if tr, ok := transport.FromServerContext(ctx); ok {
		if value := tr.RequestHeader().Get(key); value != "" {
			return value
		}
		if ht, ok := tr.(*khttp.Transport); ok {
			return ht.Request().Header.Get(key)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func scheme(req *http.Request) string {
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

func appendForwardedFor(header http.Header, host string) {
	current := header.Get(HeaderForwardedFor)
	if current == "" {
		header.Set(HeaderForwardedFor, host)
		return
	}
	header.Set(HeaderForwardedFor, current+", "+host)
}
