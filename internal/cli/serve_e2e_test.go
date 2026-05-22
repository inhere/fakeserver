package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/middleware"
	"github.com/inhere/fakeserver/internal/tpl"
)

// TestServe_v01_MVPClosure 是 Phase 5 DoD #9：v0.1 MVP 闭环 E2E。
//
// 综合 config 含 mock 单响应 + cases first-match + proxy + bodyFile，
// 启动 fakeserver，请求每类路由验证响应，然后文件系统编辑 config 增加一条
// 新路由，等待 watcher 防抖窗口结束 + swap，再次请求新路径，验证生效。
func TestServe_v01_MVPClosure(t *testing.T) {
	// 1. Spin up a fake upstream for the proxy route
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("UP:" + r.URL.Path))
	}))
	defer upstream.Close()

	// 2. Write initial config + a bodyFile fixture
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	fixPath := filepath.Join(tmp, "hi.txt")
	if err := os.WriteFile(fixPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// Use forward slashes for the bodyFile absolute path to avoid JSON5 escape issues
	fixPathFwd := filepath.ToSlash(fixPath)
	initialCfg := `{
  fallback: "echo",
  routes: [
    { method: "GET", path: "/ping", body: "pong" },
    { method: "GET", path: "/u/{id}", strategy: "first-match", cases: [
        { when: "request.query.fail == \"1\"", status: 500, body: { error: "boom" } },
        { status: 200, body: { id: "{{ .request.params.id }}" } },
    ]},
    { method: "*", path: "/api/*rest", proxy: { target: "` + upstream.URL + `", stripPathPrefix: "/api" } },
    { method: "GET", path: "/file", bodyFile: "` + fixPathFwd + `" },
  ],
}`
	if err := os.WriteFile(cfgPath, []byte(initialCfg), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Load + assemble + Holder + watcher
	cfg, err := config.Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	opts := serveOptions{Quiet: true, NoCORS: true, NoWatch: false}

	holder := newHolderWithWatcher(t, cfg, rdr, opts, cfgPath, "")
	srv := httptest.NewServer(holder)
	defer srv.Close()

	// 4. Verify 4 initial routes
	verifyOK := func(label, url, wantSubstring string) {
		t.Helper()
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("[%s] %v", label, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(b), wantSubstring) {
			t.Errorf("[%s] body=%q want substring %q", label, string(b), wantSubstring)
		}
	}
	verifyOK("mock single", srv.URL+"/ping", "pong")
	verifyOK("cases default", srv.URL+"/u/42", `"id":"42"`)
	verifyStatus(t, "cases fail-branch", srv.URL+"/u/42?fail=1", 500)
	verifyOK("proxy", srv.URL+"/api/orders", "UP:/orders")
	verifyOK("bodyFile", srv.URL+"/file", "hello")

	// 5. Edit config: add a new route /v2/new → 200 "added"
	// We insert after the bodyFile route line using the path-specific marker.
	marker := `{ method: "GET", path: "/file", bodyFile: "` + fixPathFwd + `" },`
	editedCfg := strings.Replace(initialCfg,
		marker,
		marker+"\n    { method: \"GET\", path: \"/v2/new\", body: \"added\" },",
		1)
	if err := os.WriteFile(cfgPath, []byte(editedCfg), 0644); err != nil {
		t.Fatal(err)
	}

	// 6. Wait for debounce + swap. Poll instead of fixed sleep for resilience
	// on slow CI machines. 2s budget = 300ms debounce + plenty of safety.
	// We check for "added" in the body to distinguish the new mock route from
	// the echo fallback (which also returns 200 but with a different body).
	if !waitForRouteBody(t, srv.URL+"/v2/new", "added", 2*time.Second) {
		t.Fatal("new route never became live after config edit")
	}

	// 7. Verify new route is live
	verifyOK("new route after reload", srv.URL+"/v2/new", "added")

	// 8. Verify old routes still work after reload
	verifyOK("ping still works", srv.URL+"/ping", "pong")
}

func verifyStatus(t *testing.T, label, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("[%s] %v", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Errorf("[%s] status=%d want %d", label, resp.StatusCode, want)
	}
}

// waitForRouteBody polls url until the response body contains wantSubstr OR
// budget expires. Returns true if found, false if timeout. We check the body
// rather than just status because the echo fallback also returns 200 — we need
// to distinguish "new mock route live" from "echo fallback still serving".
// Initial polls are fast (10ms); interval grows to amortize over slow startup.
func waitForRouteBody(t *testing.T, url, wantSubstr string, budget time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(budget)
	interval := 10 * time.Millisecond
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if strings.Contains(string(b), wantSubstr) {
				return true
			}
		}
		time.Sleep(interval)
		if interval < 100*time.Millisecond {
			interval *= 2
		}
	}
	return false
}

// TestServe_v02_EnvFile_HotReload 是 Phase 2 DoD #8：env 文件 hot-reload E2E。
//
// 写主配置（含 {{ .env.token }} 模板）+ env 文件（dev 段 token=DEV），启动
// fakeserver，验证初始值为 "DEV"，然后修改 env 文件将 token 改为 "DEV2"，
// 等待 watcher 防抖窗口结束 + holder swap，验证新值生效。
func TestServe_v02_EnvFile_HotReload(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	envPath := filepath.Join(tmp, "fakeserver.env.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [{ method: "GET", path: "/t", body: "{{ .env.token }}" }] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ dev: { token: "DEV" } }`), 0644)

	cfg, err := config.Load([]string{cfgPath}, "dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("%v", errs)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	holder := newHolderWithWatcher(t, cfg, rdr, serveOptions{Quiet: true, NoCORS: true}, cfgPath, "dev")
	srv := httptest.NewServer(holder)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/t")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "DEV" {
		t.Errorf("initial body=%q want DEV", string(b))
	}

	_ = os.WriteFile(envPath, []byte(`{ dev: { token: "DEV2" } }`), 0644)
	if !waitForRouteBody(t, srv.URL+"/t", "DEV2", 2*time.Second) {
		t.Fatal("new env value never appeared after edit")
	}
}

// newHolderWithWatcher 模拟 runServe 的 holder + watcher 装配（只是不起 HTTP
// server，由 httptest 接管）。envName 传 "" 表示按 $active/首段自动选择。
func newHolderWithWatcher(t *testing.T, cfg *config.Config, rdr tpl.Renderer, opts serveOptions, cfgPath string, envName string) http.Handler {
	t.Helper()
	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, rdr, opts, nil, nil))
	watcher, err := config.NewWatcher(cfg.SourcePaths, 300*time.Millisecond, func() {
		newCfg, lerr := config.Load([]string{cfgPath}, envName, nil)
		if lerr != nil {
			t.Logf("reload load err: %v", lerr)
			return
		}
		if errs := config.Validate(newCfg); len(errs) > 0 {
			t.Logf("reload validate failed: %v", errs)
			return
		}
		newRdr := tpl.NewRenderer(newCfg.Globals, newCfg.Server.OSEnvWhitelist, newCfg.Server.FakerSeed)
		holder.Swap(assembleHandler(newCfg, newRdr, opts, nil, nil))
	})
	if err != nil {
		t.Fatalf("watcher: %v", err)
	}
	t.Cleanup(func() { _ = watcher.Stop() })
	return holder
}
