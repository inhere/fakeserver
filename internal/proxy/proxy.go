package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/sizeparse"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Mount registers every proxy route in cfg onto r. Routes without a proxy
// block are skipped (mock.Mount handles them).
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		route := &cfg.Routes[i]
		if route.Proxy == nil {
			continue
		}
		handler, err := Build(route, i, renderer, cfg.Env)
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

// Build constructs the rux handler for one proxy route. Compilation
// (rewrite rules / timeout / bodyLimit / target URL) happens once here;
// the returned handler is allocation-light on the request path.
//
// design §9.2 modeling boundaries: only proxy.headers and
// proxy.responseHeaders value strings go through the template renderer.
// All other fields (target, rewrite, stripPathPrefix, timeout, etc.) are
// literal. The rewrite regex's $1, $2... are Go regexp capture groups,
// NOT template variables.
func Build(route *config.Route, routeIndex int, renderer tpl.Renderer, envMap map[string]any) (rux.HandlerFunc, error) {
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

	rules, err := compileRewrites(p.Rewrite)
	if err != nil {
		return nil, err
	}

	timeout := 30 * time.Second
	if p.Timeout != "" {
		d, terr := time.ParseDuration(p.Timeout)
		if terr != nil {
			return nil, fmt.Errorf("proxy.timeout %q: %w", p.Timeout, terr)
		}
		timeout = d
	}

	var bodyLimit int64 = -1 // sentinel: no per-route limit
	if p.BodyLimit != "" {
		n, berr := ParseByteSize(p.BodyLimit)
		if berr != nil {
			return nil, fmt.Errorf("proxy.bodyLimit %q: %w", p.BodyLimit, berr)
		}
		bodyLimit = n
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: p.InsecureSkipVerify}, //nolint:gosec
		// ResponseHeaderTimeout caps "first response byte arrival". We
		// pair it with ctx.WithTimeout below for total request lifetime.
		ResponseHeaderTimeout: timeout,
	}

	rp := &httputil.ReverseProxy{
		Transport: transport,
		Director: func(req *http.Request) {
			// 1. URL scheme + host
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			// 2. Host header: preserve client's Host iff configured
			if !p.PreserveHost {
				req.Host = targetURL.Host
			}
			// 3. Path: stripPathPrefix → rewrite
			path := req.URL.Path
			if p.StripPathPrefix != "" && strings.HasPrefix(path, p.StripPathPrefix) {
				path = path[len(p.StripPathPrefix):]
				if path == "" {
					path = "/"
				}
			}
			if newPath, ok := applyRewrites(rules, path); ok {
				path = newPath
			}
			req.URL.Path = path
			// 4. Inject request headers (template-rendered values).
			// NOTE: We build a lightweight render ctx manually here to avoid
			// consuming req.Body (BuildRenderCtx reads+closes the body, which
			// would break body forwarding to upstream).
			if len(p.Headers) > 0 {
				ctx := buildProxyRenderCtx(req, envMap)
				for k, v := range p.Headers {
					rendered, rerr := renderer.Render(v, ctx)
					if rerr != nil {
						req.Header.Set("X-Fakeserver-Proxy-Render-Error", fmt.Sprintf("request header %s: %v", k, rerr))
						return
					}
					req.Header.Set(k, rendered)
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			if len(p.ResponseHeaders) == 0 {
				return nil
			}
			ctx := buildProxyRenderCtx(resp.Request, envMap)
			for k, v := range p.ResponseHeaders {
				rendered, rerr := renderer.Render(v, ctx)
				if rerr != nil {
					return fmt.Errorf("response header %s: %w", k, rerr)
				}
				resp.Header.Set(k, rendered)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, perr error) {
			if strings.HasPrefix(perr.Error(), "response header ") {
				writeProxyError(w, http.StatusBadGateway, "proxy header render failed", perr.Error(), route, p.Target)
				return
			}
			// Distinguish timeout (504) from other transport errors (502).
			if errors.Is(perr, context.DeadlineExceeded) ||
				errors.Is(perr, context.Canceled) ||
				strings.Contains(perr.Error(), "deadline") ||
				strings.Contains(perr.Error(), "timeout") {
				writeProxyError(w, http.StatusGatewayTimeout, "upstream timeout", perr.Error(), route, p.Target)
				return
			}
			writeProxyError(w, http.StatusBadGateway, "upstream dial failed", perr.Error(), route, p.Target)
		},
	}

	return func(c *rux.Context) {
		req := c.Req
		// Render request headers before forwarding so failures fail closed.
		for k, v := range p.Headers {
			ctx := buildProxyRenderCtx(req, envMap)
			rendered, rerr := renderer.Render(v, ctx)
			if rerr != nil {
				writeProxyError(c.Resp, http.StatusBadGateway, "proxy header render failed", fmt.Sprintf("route=%s %s header=%s: %v", strings.Join(route.Method, ","), route.Path, k, rerr), route, p.Target)
				return
			}
			req.Header.Set(k, rendered)
		}
		recorder.SetRouteMatch(req.Context(), recorder.RequestTrace{
			RouteIndex:  &routeIndex,
			RouteMode:   "proxy",
			RouteSource: route.SourceFile,
			ProxyTarget: p.Target,
		})

		// Apply bodyLimit on request body — too-large requests never reach upstream.
		// We use readUpTo (not http.MaxBytesReader) because MaxBytesReader's error
		// surfaces during ReverseProxy's body copy, past the ErrorHandler boundary,
		// so the 413 status can't be reliably returned.
		if bodyLimit >= 0 && req.Body != nil {
			buf := make([]byte, bodyLimit+1)
			n, rerr := readUpTo(req.Body, buf)
			if rerr != nil {
				writeProxyError(c.Resp, http.StatusRequestEntityTooLarge, "request body exceeds proxy.bodyLimit", fmt.Sprintf("limit=%d bytes", bodyLimit), route, p.Target)
				return
			}
			// Reinstall body for ReverseProxy
			req.Body = io.NopCloser(bytes.NewReader(buf[:n]))
			req.ContentLength = int64(n)
		}

		// Wrap with per-request timeout
		if timeout > 0 {
			ctx, cancel := context.WithTimeout(req.Context(), timeout)
			defer cancel()
			req = req.WithContext(ctx)
		}

		rp.ServeHTTP(c.Resp, req)
	}, nil
}

// buildProxyRenderCtx constructs a minimal template render context for
// proxy header injection. Unlike tpl.BuildRenderCtx, this does NOT drain
// req.Body (which would corrupt forwarding).
//
// Deliberately omitted keys (vs production BuildRenderCtx):
//   - body / bodyRaw: would require draining req.Body
//   - params:        rux path params aren't accessible at Director time
//     (Director runs after rux handed off the request)
//
// Templates referencing those keys will get nil → empty string. Users who
// need request body in header injection should switch to a mock route.
func buildProxyRenderCtx(req *http.Request, envMap map[string]any) map[string]any {
	headers := make(map[string]string, len(req.Header))
	for k, vs := range req.Header {
		if len(vs) > 0 {
			headers[k] = vs[0]
		}
	}
	query := make(map[string]any)
	for k, vs := range req.URL.Query() {
		if len(vs) == 1 {
			query[k] = vs[0]
		} else {
			query[k] = vs
		}
	}
	if envMap == nil {
		envMap = map[string]any{}
	}
	return map[string]any{
		"request": map[string]any{
			"method":  req.Method,
			"path":    req.URL.Path,
			"proto":   req.Proto,
			"host":    req.Host,
			"ip":      proxyClientIP(req),
			"query":   query,
			"headers": headers,
		},
		"now":    time.Now(),
		"env":    envMap,
		"osenv":  map[string]string{},
		"config": map[string]any{},
	}
}

// proxyClientIP extracts the client IP from the request, using the same
// heuristic as tpl.clientIP but inlined to avoid coupling internal packages.
// Priority: X-Forwarded-For (first value) → X-Real-Ip → RemoteAddr (port stripped).
func proxyClientIP(req *http.Request) string {
	if v := req.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.Index(v, ","); i >= 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := req.Header.Get("X-Real-Ip"); v != "" {
		return v
	}
	host := req.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host
}

// readUpTo reads from r into buf. Returns (n, nil) when r ends within
// len(buf) bytes; returns (n, err) when body >= len(buf) — signaling
// the caller "body exceeded the limit".
//
// buf must be sized max+1 by the caller so that filling buf exactly means
// "body is at least max+1 bytes" — over the limit.
func readUpTo(r io.ReadCloser, buf []byte) (int, error) {
	// Close the original body once. The caller will reinstall a new
	// io.NopCloser(bytes.NewReader(...)) on the request before passing
	// to ReverseProxy, so double-close is not a concern — the original
	// is owned by us at this point.
	defer r.Close()
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err == io.EOF {
			if total >= len(buf) {
				// buf is max+1; filling it exactly means body == max+1 — over limit.
				return total, errors.New("body exceeds limit")
			}
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
	// Loop exited because total == len(buf) == max+1 — buffer filled, over limit.
	return total, errors.New("body exceeds limit")
}

// ParseByteSize keeps the proxy package API stable while sharing the parser
// with config validation.
func ParseByteSize(s string) (int64, error) {
	return sizeparse.ParseByteSize(s)
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
