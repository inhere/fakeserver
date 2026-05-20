package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Mount registers every proxy route in cfg onto r. Routes without a proxy
// block are skipped (mock.Mount handles them).
//
// Each proxy route gets its own *httputil.ReverseProxy instance so target
// hostnames / TLS settings / timeouts don't have to be re-computed on
// every request.
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		route := &cfg.Routes[i]
		if route.Proxy == nil {
			continue
		}
		handler, err := Build(route, renderer)
		if err != nil {
			return fmt.Errorf("routes[%d] (%s %s): %w", i, strings.Join(route.Method, ","), route.Path, err)
		}
		for _, m := range route.Method {
			method := strings.ToUpper(m)
			if method == "*" {
				r.Any(route.Path, handler)
			} else {
				r.Add(route.Path, handler, method)
			}
		}
	}
	return nil
}

// Build constructs the rux handler for one proxy route. The returned
// handler is goroutine-safe (ReverseProxy is concurrency-safe; rewrite
// rules are read-only after compilation).
//
// Task 7 implements the basic forwarding + 502-on-dial-failure path.
// Task 8 layers on stripPathPrefix, rewrite, headers, responseHeaders,
// bodyLimit, timeout, preserveHost, insecureSkipVerify.
func Build(route *config.Route, renderer tpl.Renderer) (rux.HandlerFunc, error) {
	p := route.Proxy
	if p == nil {
		return nil, fmt.Errorf("Build called with nil Proxy")
	}
	targetURL, err := url.Parse(p.Target)
	if err != nil {
		return nil, fmt.Errorf("proxy.target %q: %w", p.Target, err)
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		return nil, fmt.Errorf("proxy.target scheme must be http/https, got %q", targetURL.Scheme)
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.Host = targetURL.Host
			// Path: keep client's path as-is in Task 7; Task 8 layers
			// stripPathPrefix + rewrite here.
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, perr error) {
			writeProxyError(w, http.StatusBadGateway, "upstream dial failed", perr.Error(), route, p.Target)
		},
	}
	return func(c *rux.Context) {
		rp.ServeHTTP(c.Resp, c.Req)
	}, nil
}

// writeProxyError emits the §9.3/§6 error body. The `target` field is the
// distinguishing feature of proxy errors vs mock errors.
func writeProxyError(w http.ResponseWriter, status int, short, detail string, route *config.Route, target string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  short,
		"detail": detail,
		"route":  fmt.Sprintf("%s %s", strings.Join(route.Method, ","), route.Path),
		"target": target,
	})
}
