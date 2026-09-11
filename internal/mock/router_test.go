package mock

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

func mountedServer(t *testing.T, cfg *config.Config) *httptest.Server {
	t.Helper()
	r := rux.New()
	renderer := tpl.NewRenderer(nil, nil, 0)
	if err := Mount(r, cfg, renderer); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	return httptest.NewServer(r)
}

func TestMount_SingleResponseRouteResponds(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/ping", Body: "pong"},
		},
	}
	ts := mountedServer(t, cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("body: %q", body)
	}
}

func TestMount_ParamPathRendersFromTemplate(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method: []string{"GET"},
				Path:   "/u/{id}",
				Body:   map[string]any{"id": "{{ .request.params.id }}"},
			},
		},
	}
	ts := mountedServer(t, cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/u/42")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if got["id"] != "42" {
		t.Errorf("id: got %v", got["id"])
	}
}

func TestMount_SkipsCasesAndProxyRoutes(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/single", Body: "ok"},
			{Method: []string{"GET"}, Path: "/with-cases", Cases: []config.RouteCase{{Body: "case1"}}},
			{Method: []string{"GET"}, Path: "/proxy", Proxy: &config.ProxyConfig{Target: "http://upstream"}},
		},
	}
	r := rux.New()
	renderer := tpl.NewRenderer(nil, nil, 0)
	if err := Mount(r, cfg, renderer); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	ts := httptest.NewServer(r)
	defer ts.Close()

	// /single 应有 mock handler
	resp1, _ := http.Get(ts.URL + "/single")
	resp1.Body.Close()
	if resp1.StatusCode != 200 {
		t.Errorf("/single: status %d", resp1.StatusCode)
	}

	// Phase 4: /with-cases 已注册（cases handler），不再是 404
	resp2, _ := http.Get(ts.URL + "/with-cases")
	resp2.Body.Close()
	if resp2.StatusCode == 404 {
		t.Errorf("/with-cases (Phase 4: cases routes are registered): got 404, want non-404")
	}

	// /proxy 仍由 proxy.Mount 处理，mock.Mount 不注册，返回 404
	resp3, _ := http.Get(ts.URL + "/proxy")
	resp3.Body.Close()
	if resp3.StatusCode != 404 {
		t.Errorf("/proxy (proxy skipped by mock.Mount): status %d, want 404", resp3.StatusCode)
	}
}

func TestMount_MultipleMethods(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET", "HEAD"}, Path: "/x", Body: "x"},
		},
	}
	ts := mountedServer(t, cfg)
	defer ts.Close()

	resp1, _ := http.Get(ts.URL + "/x")
	resp1.Body.Close()
	if resp1.StatusCode != 200 {
		t.Errorf("GET /x: %d", resp1.StatusCode)
	}
	resp2, _ := http.Head(ts.URL + "/x")
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("HEAD /x: %d", resp2.StatusCode)
	}
}

func TestMount_RegistersCasesRoute(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x", Strategy: "first-match",
			Cases: []config.RouteCase{
				{When: `request.query.fail == "1"`, Status: 500, Body: "boom"},
				{Status: 200, Body: "ok"},
			},
		}},
	}
	r := rux.New()
	rdr := tpl.NewRenderer(nil, nil, 1)
	if err := Mount(r, cfg, rdr); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/x?fail=1")
	if resp.StatusCode != 500 {
		t.Errorf("cases first-match got %d want 500", resp.StatusCode)
	}
	resp, _ = http.Get(srv.URL + "/x")
	if resp.StatusCode != 200 {
		t.Errorf("cases fallback got %d want 200", resp.StatusCode)
	}
}

func TestMount_StillSkipsProxyRoute(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy: &config.ProxyConfig{Target: "http://upstream:8080"},
		}},
	}
	r := rux.New()
	rdr := tpl.NewRenderer(nil, nil, 1)
	if err := Mount(r, cfg, rdr); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	srv := httptest.NewServer(r)
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/api/users/1")
	if resp.StatusCode != 404 {
		t.Errorf("proxy route should not be mounted by mock.Mount; got status %d", resp.StatusCode)
	}
}

func TestMount_CasesCompileError(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Cases: []config.RouteCase{
				{When: `bad syntax ==`, Status: 200, Body: "a"},
			},
		}},
	}
	r := rux.New()
	rdr := tpl.NewRenderer(nil, nil, 1)
	err := Mount(r, cfg, rdr)
	if err == nil {
		t.Fatal("Mount should error on bad when (Validate normally catches this; defense-in-depth)")
	}
	if !strings.Contains(err.Error(), "routes[0]") {
		t.Errorf("error should include route position; got %v", err)
	}
	if !strings.Contains(err.Error(), "when") {
		t.Errorf("error should mention 'when'; got %v", err)
	}
}
