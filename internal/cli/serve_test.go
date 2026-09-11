// 此测试在 `package cli`（非 `cli_test`），以便复用包级私有 assembleHandler。
package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/scenario"
	"github.com/inhere/fakeserver/internal/tpl"
)

func TestServe_Healthz(t *testing.T) {
	ts := httptest.NewServer(assembleHandler(nil, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, nil, nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/__fakeserver/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServe_EchoOnAnything(t *testing.T) {
	ts := httptest.NewServer(assembleHandler(nil, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, nil, nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anything/abc?x=1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("expected JSON, got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
}

// TestServe_EchoCatchAllOnUnknownPath: rux v2 的 MountEchoRoutes 注册了
// /*path 兜底，未匹配路径会被回显（fallback 默认 echo）。
func TestServe_EchoCatchAllOnUnknownPath(t *testing.T) {
	ts := httptest.NewServer(assembleHandler(nil, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, nil, nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/totally/unknown/path")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (echo fallback), got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Fakeserver-Fallback"); got != "echo" {
		t.Fatalf("expected echo fallback marker, got %q", got)
	}
}

func TestServe_StatusEndpoint(t *testing.T) {
	ts := httptest.NewServer(assembleHandler(nil, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, nil, nil))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status/503")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestServe_E2E_CasesRoute(t *testing.T) {
	cfg, err := config.Load([]string{"../config/testdata/valid/cases-first-match.json5"}, "", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}
	rdr := tpl.NewRenderer(nil, nil, 1)
	srv := httptest.NewServer(assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, nil, nil))
	defer srv.Close()

	// branch: fail=1 → 500
	resp, _ := http.Get(srv.URL + "/u/42?fail=1")
	if resp.StatusCode != 500 {
		t.Errorf("fail=1: status=%d want 500", resp.StatusCode)
	}
	resp.Body.Close()

	// branch: default → 200 + id=42
	resp, _ = http.Get(srv.URL + "/u/42")
	if resp.StatusCode != 200 {
		t.Errorf("default: status=%d want 200", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["id"] != "42" {
		t.Errorf("body=%v want id=42", got)
	}
}

func TestAssembleHandler_UsesCLIScenario(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method:   []string{"GET"},
			Path:     "/api/users",
			Strategy: "first-match",
			Cases: []config.RouteCase{
				{Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
				{Name: "empty", Status: 200, Body: map[string]any{"state": "empty"}},
			},
		}},
		Scenarios: map[string]config.ScenarioConfig{
			"emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
		},
	}
	enabled := true
	cfg.Server.AdminEnabled = &enabled
	rdr := tpl.NewRenderer(nil, nil, 0)
	store := scenario.NewStore()
	handler := assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true, Scenario: "emptyUsers"}, recorder.New(10), store)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/api/users", nil))
	if !strings.Contains(rr.Body.String(), `"state":"empty"`) {
		t.Fatalf("response = %s", rr.Body.String())
	}
}

func TestServe_E2E_ProxyRouteCoexistsWithMock(t *testing.T) {
	// Spin up a fake upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("UP:" + r.URL.Path))
	}))
	defer upstream.Close()

	// Build config: one precise mock + one wildcard proxy
	cfg := &config.Config{
		Fallback: "echo",
		Routes: []config.Route{
			{
				Method: []string{"GET"}, Path: "/api/users",
				Status: 200, Body: map[string]any{"local": true},
			},
			{
				Method: []string{"*"}, Path: "/api/*rest",
				Proxy: &config.ProxyConfig{Target: upstream.URL, StripPathPrefix: "/api"},
			},
		},
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}

	rdr := tpl.NewRenderer(nil, nil, 1)
	srv := httptest.NewServer(assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, nil, nil))
	defer srv.Close()

	// /api/users → precise mock wins
	resp, _ := http.Get(srv.URL + "/api/users")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"local":true`) {
		t.Errorf("precise mock should win for /api/users; body=%s", string(b))
	}

	// /api/orders/1 → wildcard proxy
	resp, _ = http.Get(srv.URL + "/api/orders/1")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(b), "UP:/orders/1") {
		t.Errorf("proxy should win for /api/orders/1; body=%s", string(b))
	}
}

func TestParseVarOverrides_SingleFlag(t *testing.T) {
	got := parseVarOverrides([]string{"a=1"})
	if got["a"] != "1" {
		t.Errorf("a=%q want 1", got["a"])
	}
}
func TestParseVarOverrides_CommaSeparated(t *testing.T) {
	got := parseVarOverrides([]string{"a=1,b=2"})
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %v", got)
	}
}
func TestParseVarOverrides_MultipleFlag(t *testing.T) {
	got := parseVarOverrides([]string{"a=1", "b=2"})
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %v", got)
	}
}
func TestParseVarOverrides_MixedCSVAndMultiple(t *testing.T) {
	got := parseVarOverrides([]string{"a=1,b=2", "c=3"})
	if got["a"] != "1" || got["b"] != "2" || got["c"] != "3" {
		t.Errorf("got %v", got)
	}
}
func TestParseVarOverrides_MalformedSkipped(t *testing.T) {
	got := parseVarOverrides([]string{"a=1", "no-equals", "b=2"})
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %v", got)
	}
	if _, has := got["no-equals"]; has {
		t.Error("malformed should be skipped")
	}
}

func TestServe_MockRouteRespondsAfterPhase3(t *testing.T) {
	cfg, err := config.Load(
		[]string{"../config/testdata/valid/single-full.json5"},
		"", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	renderer := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	ts := httptest.NewServer(assembleHandler(cfg, renderer, serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, nil, nil))
	defer ts.Close()

	// single-full.json5 的 /ping route 应该被 mock 响应（不再走 echo catch-all）
	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("/ping: status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("/ping body: %q (want pong from mock; if got JSON echo, mock didn't register)", body)
	}

	// fallback: "404" returns a structured not-found response.
	resp2, err2 := http.Get(ts.URL + "/totally/unknown")
	if err2 != nil {
		t.Fatal(err2)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("/totally/unknown: status %d (want 404)", resp2.StatusCode)
	}
}

func TestServe_Fallback404IncludesMarkerAndRequest(t *testing.T) {
	cfg := &config.Config{Fallback: "404"}
	h := assembleHandler(cfg, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true}, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/missing", nil))
	if r.Code != http.StatusNotFound || r.Header().Get("X-Fakeserver-Fallback") != "404" || !strings.Contains(r.Body.String(), `"method":"POST"`) {
		t.Fatalf("fallback response: %d %s %q", r.Code, r.Header().Get("X-Fakeserver-Fallback"), r.Body.String())
	}
}

func TestServe_FallbackCustomRendersTemplate(t *testing.T) {
	cfg := &config.Config{Fallback: map[string]any{"status": float64(418), "headers": map[string]any{"X-Test": "{{ .request.path }}"}, "body": map[string]any{"path": "{{ .request.path }}"}}}
	h := assembleHandler(cfg, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true}, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/custom", nil))
	if r.Code != 418 || r.Header().Get("X-Fakeserver-Fallback") != "custom" || r.Header().Get("X-Test") != "/custom" || !strings.Contains(r.Body.String(), "/custom") {
		t.Fatalf("custom fallback: %d %q %s", r.Code, r.Header().Get("X-Test"), r.Body.String())
	}
}

func TestServe_AdminRemoteBlockedHealthzAllowed(t *testing.T) {
	enabled := true
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: &enabled}}
	h := assembleHandler(cfg, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true}, nil, nil)
	remote := httptest.NewRequest(http.MethodGet, "/__fakeserver/api/config", nil)
	remote.RemoteAddr = "203.0.113.10:1234"
	r := httptest.NewRecorder()
	h.ServeHTTP(r, remote)
	if r.Code != http.StatusForbidden {
		t.Fatalf("remote admin status=%d", r.Code)
	}
	if !strings.Contains(r.Body.String(), "admin endpoints are restricted to loopback clients") {
		t.Fatalf("unexpected admin error body: %s", r.Body.String())
	}
	health := httptest.NewRequest(http.MethodGet, "/__fakeserver/healthz", nil)
	health.RemoteAddr = "203.0.113.10:1234"
	r = httptest.NewRecorder()
	h.ServeHTTP(r, health)
	if r.Code != http.StatusOK {
		t.Fatalf("remote healthz status=%d", r.Code)
	}
}

func TestServe_AdminRemoteAllowedWhenConfigured(t *testing.T) {
	enabled := true
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: &enabled, AdminAllowRemote: true}}
	h := assembleHandler(cfg, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/__fakeserver/api/config", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	r := httptest.NewRecorder()
	h.ServeHTTP(r, req)
	if r.Code != http.StatusOK {
		t.Fatalf("remote allowed status=%d", r.Code)
	}
}
