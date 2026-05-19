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
