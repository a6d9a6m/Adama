package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	gwmiddleware "github.com/littleSand/adama/app/gateway/internal/middleware"
)

const gatewayPrefix = "/api/v1"

type Upstream struct {
	name     string
	baseURL  *url.URL
	proxy    *httputil.ReverseProxy
	basePath string
}

type Route struct {
	Prefix   string
	Upstream *Upstream
}

type DispatcherConfig struct {
	Logger          log.Logger
	Routes          []Route
	BlockedExact    []string
	BlockedPrefixes []string
}

type Dispatcher struct {
	log             *log.Helper
	routes          []Route
	blockedExact    map[string]struct{}
	blockedPrefixes []string
}

func NewUpstream(name, rawURL string, timeout time.Duration, logger log.Logger) (*Upstream, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("upstream %s base_url is required", name)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream %s: %w", name, err)
	}

	reverseProxy := httputil.NewSingleHostReverseProxy(parsedURL)
	baseDirector := reverseProxy.Director

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if timeout > 0 {
		transport.ResponseHeaderTimeout = timeout
	}
	reverseProxy.Transport = transport
	reverseProxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, proxyErr error) {
		log.NewHelper(logger).Errorf("gateway proxy %s failed: %v", name, proxyErr)
		writeJSONError(writer, http.StatusBadGateway, "upstream_unavailable", "upstream request failed")
	}
	reverseProxy.Director = func(req *http.Request) {
		baseDirector(req)
		req.Host = parsedURL.Host
	}

	return &Upstream{
		name:     name,
		baseURL:  parsedURL,
		proxy:    reverseProxy,
		basePath: strings.TrimRight(parsedURL.Path, "/"),
	}, nil
}

func NewDispatcher(cfg DispatcherConfig) *Dispatcher {
	blockedExact := make(map[string]struct{}, len(cfg.BlockedExact))
	for _, item := range cfg.BlockedExact {
		blockedExact[item] = struct{}{}
	}

	return &Dispatcher{
		log:             log.NewHelper(cfg.Logger),
		routes:          cfg.Routes,
		blockedExact:    blockedExact,
		blockedPrefixes: cfg.BlockedPrefixes,
	}
}

func (d *Dispatcher) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	path, err := trimGatewayPrefix(request.URL.Path)
	if err != nil {
		writeJSONError(writer, http.StatusNotFound, "route_not_found", "route not found")
		return
	}

	if d.isBlocked(path) {
		writeJSONError(writer, http.StatusForbidden, "route_forbidden", "route is not exposed by gateway")
		return
	}

	target := d.match(path)
	if target == nil {
		writeJSONError(writer, http.StatusNotFound, "route_not_found", "route not found")
		return
	}

	forwardRequest := cloneRequest(request, target, path)
	requestID, traceID := gwmiddleware.EnsureForwardHeaders(forwardRequest)

	writer.Header().Set(gwmiddleware.HeaderRequestID, requestID)
	writer.Header().Set(gwmiddleware.HeaderTraceID, traceID)

	d.log.Infof("gateway proxy target=%s method=%s path=%s trace_id=%s", target.name, request.Method, path, traceID)
	target.proxy.ServeHTTP(writer, forwardRequest)
}

func (d *Dispatcher) isBlocked(path string) bool {
	if _, ok := d.blockedExact[path]; ok {
		return true
	}
	for _, prefix := range d.blockedPrefixes {
		if hasPathPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (d *Dispatcher) match(path string) *Upstream {
	for _, route := range d.routes {
		if hasPathPrefix(path, route.Prefix) {
			return route.Upstream
		}
	}
	return nil
}

func trimGatewayPrefix(path string) (string, error) {
	if path == gatewayPrefix || path == gatewayPrefix+"/" {
		return "", errors.New("missing downstream path")
	}
	if !hasPathPrefix(path, gatewayPrefix) {
		return "", fmt.Errorf("path %q is outside gateway prefix", path)
	}

	trimmed := strings.TrimPrefix(path, gatewayPrefix)
	if trimmed == "" {
		return "", errors.New("missing downstream path")
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return trimmed, nil
}

func cloneRequest(request *http.Request, upstream *Upstream, path string) *http.Request {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	cloned.URL.Path = joinURLPath(upstream.basePath, path)
	cloned.URL.RawPath = cloned.URL.Path
	cloned.Host = upstream.baseURL.Host
	cloned.RequestURI = ""
	return cloned
}

func joinURLPath(basePath, requestPath string) string {
	switch {
	case basePath == "":
		return requestPath
	case requestPath == "":
		return basePath
	case strings.HasSuffix(basePath, "/") && strings.HasPrefix(requestPath, "/"):
		return basePath + strings.TrimPrefix(requestPath, "/")
	case !strings.HasSuffix(basePath, "/") && !strings.HasPrefix(requestPath, "/"):
		return basePath + "/" + requestPath
	default:
		return basePath + requestPath
	}
}

func hasPathPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func writeJSONError(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]interface{}{
		"code":    code,
		"message": message,
	})
}
