package tpl

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildRenderCtx_BasicFields(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com:8080/api/x?a=1&b=2", strings.NewReader(""))
	req.Header.Set("X-Custom", "yes")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"

	ctx := BuildRenderCtx(req, map[string]string{"id": "42"}, map[string]any{"apiVersion": "v1"})
	if ctx.Request.Method != "POST" {
		t.Errorf("method: got %q", ctx.Request.Method)
	}
	if ctx.Request.Path != "/api/x" {
		t.Errorf("path: got %q", ctx.Request.Path)
	}
	if ctx.Request.Params["id"] != "42" {
		t.Errorf("params.id: got %q", ctx.Request.Params["id"])
	}
	if ctx.Request.Query["a"] != "1" {
		t.Errorf("query.a: got %v", ctx.Request.Query["a"])
	}
	if ctx.Request.Headers["X-Custom"] != "yes" {
		t.Errorf("headers.X-Custom: got %q", ctx.Request.Headers["X-Custom"])
	}
	if ctx.Config["apiVersion"] != "v1" {
		t.Errorf("config.apiVersion: got %v", ctx.Config["apiVersion"])
	}
}

func TestBuildRenderCtx_JSONBodyParsed(t *testing.T) {
	body := `{"name":"alice","age":30}`
	req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	ctx := BuildRenderCtx(req, nil, nil)
	parsed, ok := ctx.Request.Body.(map[string]any)
	if !ok {
		t.Fatalf("body should be parsed as map; got %T = %v", ctx.Request.Body, ctx.Request.Body)
	}
	if parsed["name"] != "alice" {
		t.Errorf("name: %v", parsed["name"])
	}
	if ctx.Request.BodyRaw != body {
		t.Errorf("BodyRaw: %q", ctx.Request.BodyRaw)
	}
}

func TestBuildRenderCtx_TextBodyAsString(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader("hello world"))
	req.Header.Set("Content-Type", "text/plain")

	ctx := BuildRenderCtx(req, nil, nil)
	if s, ok := ctx.Request.Body.(string); !ok || s != "hello world" {
		t.Errorf("body: got %v (%T)", ctx.Request.Body, ctx.Request.Body)
	}
}

func TestBuildRenderCtx_FormBody(t *testing.T) {
	body := "name=alice&age=30"
	req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx := BuildRenderCtx(req, nil, nil)
	parsed, ok := ctx.Request.Body.(map[string]any)
	if !ok {
		t.Fatalf("form body should be parsed as map; got %T", ctx.Request.Body)
	}
	if parsed["name"] != "alice" {
		t.Errorf("name: %v", parsed["name"])
	}
}
