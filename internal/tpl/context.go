package tpl

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RenderCtx is the root data object passed to text/html template execution.
// Field structure mirrors design §4.1.
type RenderCtx struct {
	Request RequestCtx     // .request
	Now     time.Time      // .now
	Env     map[string]any // .env  (Phase 3: empty; v0.2 populates from env file)
	OSEnv   map[string]string
	Config  map[string]any
}

// RequestCtx exposes the inbound HTTP request to templates.
type RequestCtx struct {
	Method  string
	Path    string
	Proto   string
	Host    string
	IP      string
	Params  map[string]string
	Query   map[string]any
	Headers map[string]string
	Body    any
	BodyRaw string
}

// BuildRenderCtx constructs a *RenderCtx for a single request. params come
// from the router (rux path params); globals is cfg.Globals (set once at
// load time). Body is parsed lazily based on Content-Type:
//   application/json (or */+json) → map or slice
//   application/x-www-form-urlencoded → map[string]any (multi-value → []string)
//   text/* → string
//   other / empty → original bytes as string
// On parse failure for json/form, body falls back to string.
func BuildRenderCtx(req *http.Request, params map[string]string, globals map[string]any) *RenderCtx {
	bodyBytes, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	bodyRaw := string(bodyBytes)
	body := parseBodyByCT(req.Header.Get("Content-Type"), bodyBytes, bodyRaw)

	if params == nil {
		params = map[string]string{}
	}

	return &RenderCtx{
		Request: RequestCtx{
			Method:  req.Method,
			Path:    req.URL.Path,
			Proto:   req.Proto,
			Host:    req.Host,
			IP:      clientIP(req),
			Params:  params,
			Query:   flattenQuery(req.URL.Query()),
			Headers: flattenHeaders(req.Header),
			Body:    body,
			BodyRaw: bodyRaw,
		},
		Now:    time.Now(),
		Env:    map[string]any{},
		OSEnv:  map[string]string{},
		Config: globals,
	}
}

func parseBodyByCT(ct string, raw []byte, fallback string) any {
	mediaType, _, _ := mime.ParseMediaType(ct)
	switch {
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return fallback
		}
		return out
	case mediaType == "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(raw))
		if err != nil {
			return fallback
		}
		return flattenQuery(values)
	case strings.HasPrefix(mediaType, "text/"):
		return fallback
	default:
		return fallback
	}
}

func flattenQuery(v url.Values) map[string]any {
	out := make(map[string]any, len(v))
	for k, vs := range v {
		if len(vs) == 1 {
			out[k] = vs[0]
		} else {
			out[k] = vs
		}
	}
	return out
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

func clientIP(req *http.Request) string {
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
