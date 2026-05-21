package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/admin"
	"github.com/inhere/fakeserver/internal/config"
)

func TestHealthz_Returns200OK(t *testing.T) {
	r := rux.New()
	admin.Mount(r, nil)
	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/__fakeserver/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if parsed["status"] != "ok" {
		t.Errorf(`expected status=="ok", got %q`, parsed["status"])
	}
}

func TestAdmin_RoutesEndpoint(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/ping", Body: "pong"},
			{Method: []string{"GET"}, Path: "/u/{id}", Strategy: "first-match",
				Cases: []config.RouteCase{
					{Status: 200, Body: "a"},
				}},
			{Method: []string{"*"}, Path: "/api/*rest",
				Proxy: &config.ProxyConfig{Target: "http://up:80"}},
		},
	}

	r := rux.New()
	admin.Mount(r, cfg)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/__fakeserver/routes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d want 200", resp.StatusCode)
	}
	var got []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	modes := []string{}
	for _, r := range got {
		modes = append(modes, r["mode"].(string))
	}
	wantModes := map[string]int{"mock": 0, "cases": 0, "proxy": 0}
	for _, m := range modes {
		wantModes[m]++
	}
	if wantModes["mock"] != 1 || wantModes["cases"] != 1 || wantModes["proxy"] != 1 {
		t.Errorf("mode distribution wrong: %v", wantModes)
	}
}

func TestRoutesHandler_IncludesDebugMetadata(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method:     []string{"GET", "HEAD"},
				Path:       "/u/{id}",
				SourceFile: ".fakeserver/routes/users.json5",
				Strategy:   "first-match",
				Cases: []config.RouteCase{
					{When: `request.query.empty == "1"`, Status: 200},
					{Status: 404},
				},
			},
			{
				Method:     []string{"*"},
				Path:       "/proxy/*rest",
				SourceFile: ".fakeserver/routes/proxy.json5",
				Proxy:      &config.ProxyConfig{Target: "https://example.test"},
			},
		},
	}

	r := rux.New()
	admin.Mount(r, cfg)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/__fakeserver/routes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	first := got[0]
	if first["index"].(float64) != 0 || first["source"].(string) != ".fakeserver/routes/users.json5" {
		t.Fatalf("first metadata wrong: %+v", first)
	}
	if first["mode"].(string) != "cases" {
		t.Fatalf("mode=%q want cases", first["mode"])
	}
	if params := first["params"].([]any); len(params) != 1 || params[0].(string) != "id" {
		t.Fatalf("params=%v want [id]", params)
	}
	if cases := first["cases"].([]any); len(cases) != 2 {
		t.Fatalf("cases=%v want len 2", cases)
	}
	proxy := got[2]
	if proxy["proxyTarget"].(string) != "https://example.test" {
		t.Fatalf("proxyTarget=%q", proxy["proxyTarget"])
	}
	if params := proxy["params"].([]any); len(params) != 1 || params[0].(string) != "rest" {
		t.Fatalf("proxy params=%v want [rest]", params)
	}
}

func TestAdmin_RoutesEmptyConfig(t *testing.T) {
	r := rux.New()
	admin.Mount(r, nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/__fakeserver/routes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d", resp.StatusCode)
	}
	var got []any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 0 {
		t.Errorf("nil cfg → empty list; got %v", got)
	}
}
