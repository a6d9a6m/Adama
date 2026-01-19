package middleware

import (
	"net"
	stdhttp "net/http"

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

func EnsureForwardHeaders(req *stdhttp.Request) (string, string) {
	requestID := firstNonEmpty(req.Header.Get(HeaderRequestID), uuid.NewString())
	traceID := firstNonEmpty(req.Header.Get(HeaderTraceID), requestID)

	req.Header.Set(HeaderRequestID, requestID)
	req.Header.Set(HeaderTraceID, traceID)
	req.Header.Set(HeaderForwardedProto, scheme(req))
	req.Header.Set(HeaderForwardedHost, req.Host)
	req.Header.Set(HeaderForwardedURI, req.URL.RequestURI())
	req.Header.Set(HeaderForwardedPrefix, "/api/v1")

	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil && host != "" {
		appendForwardedFor(req.Header, host)
	}

	return requestID, traceID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func scheme(req *stdhttp.Request) string {
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

func appendForwardedFor(header stdhttp.Header, host string) {
	current := header.Get(HeaderForwardedFor)
	if current == "" {
		header.Set(HeaderForwardedFor, host)
		return
	}
	header.Set(HeaderForwardedFor, current+", "+host)
}
