package tpl

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// getRequest 从 map ctx 取出 .request 子 map（map-based 形态下的固定路径）。
func getRequest(t *testing.T, ctx map[string]any) map[string]any {
	t.Helper()
	req, ok := ctx["request"].(map[string]any)
	if !ok {
		t.Fatalf("ctx.request not a map: %T", ctx["request"])
	}
	return req
}

func TestBuildRenderCtx_BasicFields(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com:8080/api/x?a=1&b=2", strings.NewReader(""))
	req.Header.Set("X-Custom", "yes")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"

	ctx := BuildRenderCtx(req, map[string]string{"id": "42"}, map[string]any{"apiVersion": "v1"})
	r := getRequest(t, ctx)
	if r["method"] != "POST" {
		t.Errorf("method: got %q", r["method"])
	}
	if r["path"] != "/api/x" {
		t.Errorf("path: got %q", r["path"])
	}
	params := r["params"].(map[string]string)
	if params["id"] != "42" {
		t.Errorf("params.id: got %q", params["id"])
	}
	query := r["query"].(map[string]any)
	if query["a"] != "1" {
		t.Errorf("query.a: got %v", query["a"])
	}
	headers := r["headers"].(map[string]string)
	if headers["X-Custom"] != "yes" {
		t.Errorf("headers.X-Custom: got %q", headers["X-Custom"])
	}
	cfg := ctx["config"].(map[string]any)
	if cfg["apiVersion"] != "v1" {
		t.Errorf("config.apiVersion: got %v", cfg["apiVersion"])
	}
	// now / env / osenv 三个槽位也应存在
	if _, ok := ctx["now"]; !ok {
		t.Errorf("ctx.now missing")
	}
	if _, ok := ctx["env"]; !ok {
		t.Errorf("ctx.env missing")
	}
	if _, ok := ctx["osenv"]; !ok {
		t.Errorf("ctx.osenv missing")
	}
}

func TestBuildRenderCtx_JSONBodyParsed(t *testing.T) {
	body := `{"name":"alice","age":30}`
	req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	parsed, ok := r["body"].(map[string]any)
	if !ok {
		t.Fatalf("body should be parsed as map; got %T = %v", r["body"], r["body"])
	}
	if parsed["name"] != "alice" {
		t.Errorf("name: %v", parsed["name"])
	}
	if r["bodyRaw"] != body {
		t.Errorf("bodyRaw: %q", r["bodyRaw"])
	}
}

func TestBuildRenderCtx_TextBodyAsString(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader("hello world"))
	req.Header.Set("Content-Type", "text/plain")

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	if s, ok := r["body"].(string); !ok || s != "hello world" {
		t.Errorf("body: got %v (%T)", r["body"], r["body"])
	}
}

func TestBuildRenderCtx_FormBody(t *testing.T) {
	body := "name=alice&age=30"
	req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	parsed, ok := r["body"].(map[string]any)
	if !ok {
		t.Fatalf("form body should be parsed as map; got %T", r["body"])
	}
	if parsed["name"] != "alice" {
		t.Errorf("name: %v", parsed["name"])
	}
}

// TestBuildRenderCtx_JSONBodyFallbackOnParseError: 坏 JSON 应回落为 string，
// 而不是返回 nil。
func TestBuildRenderCtx_JSONBodyFallbackOnParseError(t *testing.T) {
	raw := `{this is not json`
	req := httptest.NewRequest("POST", "/x", strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	if s, ok := r["body"].(string); !ok || s != raw {
		t.Errorf("bad JSON should fall back to string; got %T = %v", r["body"], r["body"])
	}
}

// TestBuildRenderCtx_JSONPlusSuffixContentType 验证 +json suffix（如 application/vnd.api+json）
// 也走 JSON 解析路径。
func TestBuildRenderCtx_JSONPlusSuffixContentType(t *testing.T) {
	body := `{"v":1}`
	req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	parsed, ok := r["body"].(map[string]any)
	if !ok {
		t.Fatalf("+json suffix should be parsed; got %T", r["body"])
	}
	if parsed["v"].(float64) != 1 {
		t.Errorf("v: got %v", parsed["v"])
	}
}

// TestBuildRenderCtx_UnknownContentTypeAsString: 未识别 CT 视作原始字符串。
func TestBuildRenderCtx_UnknownContentTypeAsString(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader("binary-like"))
	req.Header.Set("Content-Type", "application/octet-stream")

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	if s, ok := r["body"].(string); !ok || s != "binary-like" {
		t.Errorf("body: got %v (%T)", r["body"], r["body"])
	}
}

// TestBuildRenderCtx_MultiValueQueryFlattened: 同 key 多值 query 应展开为 []string。
func TestBuildRenderCtx_MultiValueQueryFlattened(t *testing.T) {
	req := httptest.NewRequest("GET", "/x?tag=a&tag=b&single=1", nil)
	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	q := r["query"].(map[string]any)
	tags, ok := q["tag"].([]string)
	if !ok {
		t.Fatalf("multi-value query should be []string; got %T", q["tag"])
	}
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags: got %v", tags)
	}
	if q["single"] != "1" {
		t.Errorf("single-value query: got %v", q["single"])
	}
}

// TestBuildRenderCtx_ClientIPFromXForwardedFor 验证 X-Forwarded-For 优先取首段。
func TestBuildRenderCtx_ClientIPFromXForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1")
	req.RemoteAddr = "127.0.0.1:9999"

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	if r["ip"] != "203.0.113.1" {
		t.Errorf("ip: got %v", r["ip"])
	}
}

// TestBuildRenderCtx_ClientIPFromXRealIP 验证缺 X-Forwarded-For 时退 X-Real-Ip。
func TestBuildRenderCtx_ClientIPFromXRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Real-Ip", "198.51.100.5")
	req.RemoteAddr = "127.0.0.1:9999"

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	if r["ip"] != "198.51.100.5" {
		t.Errorf("ip: got %v", r["ip"])
	}
}

// TestBuildRenderCtx_ClientIPFromRemoteAddr 两个头都缺时退 RemoteAddr。
func TestBuildRenderCtx_ClientIPFromRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "192.0.2.1:54321"

	ctx := BuildRenderCtx(req, nil, nil)
	r := getRequest(t, ctx)
	if r["ip"] != "192.0.2.1" {
		t.Errorf("ip: got %v", r["ip"])
	}
}
