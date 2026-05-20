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

// RenderCtx 与 RequestCtx 保留作为 design §4.1 的"类型契约说明"——它们
// 仅供文档参考，**实际运行期使用 map[string]any**（键名为字段的 json
// tag / lowercase），以便 Go text/template 的反射能匹配 design §4.1 全
// 小写字段访问（如 `{{ .request.params.id }}`）。
//
// Phase 3 后续阶段如需类型断言可基于本结构，但 BuildRenderCtx 不会再
// 返回这两个 struct。
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

// BuildRenderCtx constructs the template-execution data object for a single
// request. Returns map[string]any with all keys lowercased so design §4.1
// access patterns like {{ .request.params.id }} / {{ .now }} / {{ .config.x }}
// work with Go text/template's reflection-based field access.
//
// params come from the router (rux path params); globals is cfg.Globals
// (set once at load time); envMap is cfg.Env (resolved env file values,
// design §8.4 / v0.2 Phase 2). A nil envMap is treated as an empty map.
// Body is parsed lazily based on Content-Type:
//
//	application/json (or */+json) → map or slice
//	application/x-www-form-urlencoded → map[string]any (multi-value → []string)
//	text/* → string
//	other / empty → original bytes as string
//
// On parse failure for json/form, body falls back to string.
func BuildRenderCtx(req *http.Request, params map[string]string, globals, envMap map[string]any) map[string]any {
	bodyBytes, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	bodyRaw := string(bodyBytes)
	body := parseBodyByCT(req.Header.Get("Content-Type"), bodyBytes, bodyRaw)

	if params == nil {
		params = map[string]string{}
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
			"ip":      clientIP(req),
			"params":  params,
			"query":   flattenQuery(req.URL.Query()),
			"headers": flattenHeaders(req.Header),
			"body":    body,
			"bodyRaw": bodyRaw,
		},
		"now":    time.Now(),
		"env":    envMap,
		"osenv":  map[string]string{},
		"config": globals,
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
