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
	"github.com/inhere/fakeserver/internal/tpl"
)

// TestServe_v02_FullMilestoneClosure 是 v0.2 milestone 闭环 E2E。
//
// 覆盖：mock + cases + proxy + bodyFile + env 切换 + var override + osenv 阻断
// + hot-reload。一旦此测试通过，v0.2 整体功能可宣告完整闭环。
func TestServe_v02_FullMilestoneClosure(t *testing.T) {
	// ── 上游 ──
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("UP:" + r.URL.Path))
	}))
	defer upstream.Close()

	// ── fixture：主 config + env 文件 + bodyFile fixture ──
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	envPath := filepath.Join(tmp, "fakeserver.env.json5")
	fixPath := filepath.Join(tmp, "data.txt")

	if err := os.WriteFile(fixPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	mainCfg := `{
  server: { osenvWhitelist: ["FAKESERVER_E2E_ALLOWED"] },
  routes: [
    { method: "GET", path: "/ping", body: "pong" },
    { method: "GET", path: "/u/{id}", strategy: "first-match", cases: [
        { when: "request.query.fail == \"1\"", status: 500, body: { error: "boom" } },
        { status: 200, body: { id: "{{ .request.params.id }}", token: "{{ .env.token }}" } },
    ]},
    { method: "*", path: "/api/*rest", proxy: { target: "` + upstream.URL + `", stripPathPrefix: "/api" } },
    { method: "GET", path: "/file", bodyFile: "data.txt" },
  ],
}`
	envContent := `{
  $active: "dev",
  $default: { region: "us" },
  dev: { token: "DEV-TOKEN", apiHost: "dev.local" },
  staging: { token: "STG-TOKEN", apiHost: "stage.api" },
}`
	if err := os.WriteFile(cfgPath, []byte(mainCfg), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// ── 启动 holder + watcher（用 $active=dev）──
	cfg, err := config.Load([]string{cfgPath}, "", nil) // envName="" → $active=dev
	if err != nil {
		t.Fatal(err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	opts := serveOptions{Quiet: true, NoCORS: true, EnvName: ""}
	holder := newHolderWithWatcher(t, cfg, rdr, opts, cfgPath, "")
	srv := httptest.NewServer(holder)
	defer srv.Close()

	// ── 场景 1: mock 单响应 ──
	verifyBodyContains(t, srv.URL+"/ping", "pong")

	// ── 场景 2: cases first-match 默认分支 (含 .env.token) ──
	verifyBodyContains(t, srv.URL+"/u/42", `"id":"42"`)
	verifyBodyContains(t, srv.URL+"/u/42", `"token":"DEV-TOKEN"`)

	// ── 场景 3: cases fail-branch ──
	verifyStatus(t, "cases fail-branch", srv.URL+"/u/42?fail=1", 500)

	// ── 场景 4: proxy + stripPathPrefix ──
	verifyBodyContains(t, srv.URL+"/api/orders", "UP:/orders")

	// ── 场景 5: bodyFile ──
	verifyBodyContains(t, srv.URL+"/file", "hello")

	// ── 场景 6: env 切换（修改 $active: "staging"） ──
	newEnv := strings.Replace(envContent, `$active: "dev"`, `$active: "staging"`, 1)
	if err := os.WriteFile(envPath, []byte(newEnv), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitForRouteBody(t, srv.URL+"/u/42", "STG-TOKEN", 2*time.Second) {
		t.Fatal("env switch staging never propagated")
	}

	// ── 场景 7: hot-reload 旧路由仍可用 ──
	verifyBodyContains(t, srv.URL+"/ping", "pong")
}

// TestServe_v02_OsenvWhitelistBlocking 验证 osenv 白名单在 env 文件中阻断未授权 key
// （E2E 层面，不重复 envrender_test.go 的单元覆盖）。
func TestServe_v02_OsenvWhitelistBlocking(t *testing.T) {
	os.Setenv("FAKESERVER_E2E_ALLOWED", "ALLOWED-VAL")
	os.Setenv("FAKESERVER_E2E_BLOCKED", "BLOCKED-VAL")
	defer os.Unsetenv("FAKESERVER_E2E_ALLOWED")
	defer os.Unsetenv("FAKESERVER_E2E_BLOCKED")

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	envPath := filepath.Join(tmp, "fakeserver.env.json5")

	mainCfg := `{
  server: { osenvWhitelist: ["FAKESERVER_E2E_ALLOWED"] },
  routes: [
    { method: "GET", path: "/allowed", body: "{{ .env.allowed }}" },
    { method: "GET", path: "/blocked", body: "{{ .env.blocked }}" },
  ],
}`
	envContent := `{
  dev: {
    allowed: "{{ osenv \"FAKESERVER_E2E_ALLOWED\" }}",
    blocked: "{{ osenv \"FAKESERVER_E2E_BLOCKED\" }}",
  },
}`
	_ = os.WriteFile(cfgPath, []byte(mainCfg), 0644)
	_ = os.WriteFile(envPath, []byte(envContent), 0644)

	cfg, err := config.Load([]string{cfgPath}, "dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	holder := newHolderWithWatcher(t, cfg, rdr, serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, cfgPath, "dev")
	srv := httptest.NewServer(holder)
	defer srv.Close()

	verifyBodyContains(t, srv.URL+"/allowed", "ALLOWED-VAL")
	// blocked: env value renders to "" because osenv blocked → "{{ .env.blocked }}" = ""
	resp, _ := http.Get(srv.URL + "/blocked")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(b), "BLOCKED-VAL") {
		t.Errorf("body=%q must NOT contain BLOCKED-VAL (osenv whitelist should block)", string(b))
	}
}

// TestServe_v02_VarOverride 验证 --var 顶层覆盖 + 与 env 文件值的合并顺序。
func TestServe_v02_VarOverride(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	envPath := filepath.Join(tmp, "fakeserver.env.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [{ method: "GET", path: "/var-token", body: "{{ .env.token }}-{{ .env.region }}" }] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ $default: { region: "us" }, dev: { token: "FILE-TOK" } }`), 0644)

	cfg, err := config.Load([]string{cfgPath}, "dev", map[string]string{"token": "CLI-OVERRIDE"})
	if err != nil {
		t.Fatal(err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	holder := newHolderWithWatcher(t, cfg, rdr, serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, cfgPath, "dev")
	srv := httptest.NewServer(holder)
	defer srv.Close()

	verifyBodyContains(t, srv.URL+"/var-token", "CLI-OVERRIDE-us")
}

// verifyBodyContains 是 v0.2 E2E 助手；verifyStatus / waitForRouteBody 复用
// serve_e2e_test.go 中已有的同名函数。
func verifyBodyContains(t *testing.T, url, want string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), want) {
		t.Errorf("%s body=%q want substring %q", url, string(b), want)
	}
}
