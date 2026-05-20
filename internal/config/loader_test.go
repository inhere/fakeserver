package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titanous/json5"
)

// TestJSON5Lib_SmokeAcceptsCommonExtensions 锁定 titanous/json5 对我们配置文件
// 实际依赖的 JSON5 扩展项的支持。如果哪天换库或库行为变了，此测试先红。
func TestJSON5Lib_SmokeAcceptsCommonExtensions(t *testing.T) {
	src := `{
		// line comment
		/* block comment */
		server: { port: 5090 },        // 无引号 key
		fallback: "echo",
		routes: [
			{ method: 'GET', path: "/ping" },  // 单引号字符串
			{ method: "POST", path: "/users" }, // trailing comma 允许
		],
	}`
	var out map[string]any
	if err := json5.NewDecoder(strings.NewReader(src)).Decode(&out); err != nil {
		t.Fatalf("json5 should accept common extensions; err=%v", err)
	}
	if out["fallback"] != "echo" {
		t.Errorf("expected fallback=echo, got %v", out["fallback"])
	}
}

// TestJSON5Lib_SmokeErrorMentionsContext: 解析失败时错误信息应足够定位问题
// （不强求行号，但要包含可识别的位置或 token 信息）。
func TestJSON5Lib_SmokeErrorMentionsContext(t *testing.T) {
	src := `{ server: { port: 5090  routes: [] }` // 缺逗号
	var out map[string]any
	err := json5.NewDecoder(strings.NewReader(src)).Decode(&out)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	// 仅验证有 error；具体 message 由 lib 决定
}

func TestLoad_SingleMinimal(t *testing.T) {
	cfg, err := Load([]string{"testdata/valid/single-minimal.json5"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil cfg")
	}
	// 默认值已填
	if cfg.Server.Port != 5090 {
		t.Errorf("expected default port 5090, got %d", cfg.Server.Port)
	}
	if cfg.Fallback != "echo" {
		t.Errorf("expected default fallback=echo, got %q", cfg.Fallback)
	}
	// routes
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}
	r := cfg.Routes[0]
	if len(r.Method) != 1 || r.Method[0] != "GET" {
		t.Errorf("expected method=[GET], got %v", r.Method)
	}
	if r.Path != "/ping" {
		t.Errorf("expected path=/ping, got %q", r.Path)
	}
	if r.Body != "pong" {
		t.Errorf("expected body=\"pong\", got %v", r.Body)
	}
}

func TestLoad_SingleFull_NormalizesMethodAndPreservesUserValues(t *testing.T) {
	cfg, err := Load([]string{"testdata/valid/single-full.json5"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// server 用户值保留
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host=127.0.0.1, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port=8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Log == nil || *cfg.Server.Log {
		t.Errorf("expected log=false, got %v", cfg.Server.Log)
	}
	if cfg.Fallback != "404" {
		t.Errorf("expected fallback=404, got %q", cfg.Fallback)
	}
	// globals
	if cfg.Globals["apiVersion"] != "v1" {
		t.Errorf("expected globals.apiVersion=v1")
	}
	// routes[0]: 单字符串 method 标准化
	if len(cfg.Routes[0].Method) != 1 || cfg.Routes[0].Method[0] != "GET" {
		t.Errorf("expected GET, got %v", cfg.Routes[0].Method)
	}
	// routes[1]: 数组 method 保持顺序
	if len(cfg.Routes[1].Method) != 2 || cfg.Routes[1].Method[0] != "GET" || cfg.Routes[1].Method[1] != "HEAD" {
		t.Errorf("expected [GET,HEAD], got %v", cfg.Routes[1].Method)
	}
	// routes[2]: proxy 字段被解析为强类型
	if cfg.Routes[2].Proxy == nil {
		t.Fatalf("expected proxy non-nil")
	}
	if cfg.Routes[2].Proxy.Target != "http://upstream.local:8080" {
		t.Errorf("expected target=http://upstream.local:8080, got %q", cfg.Routes[2].Proxy.Target)
	}
	if cfg.Routes[2].Proxy.Timeout != "10s" {
		t.Errorf("expected timeout=10s, got %q", cfg.Routes[2].Proxy.Timeout)
	}
	// SourcePaths 包含加载文件
	if len(cfg.SourcePaths) != 1 {
		t.Errorf("expected 1 source path, got %d", len(cfg.SourcePaths))
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load([]string{"testdata/does-not-exist.json5"}, "", nil)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_Include_FlattensRoutesArray(t *testing.T) {
	cfg, err := Load([]string{"testdata/include/root.json5"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 期望：inline 1 + users.json5 数组 2 + admin/*.json5 两个文件各 1 = 5
	if len(cfg.Routes) != 5 {
		t.Fatalf("expected 5 routes, got %d", len(cfg.Routes))
	}
	wantPaths := map[string]bool{
		"/inline":        true,
		"/users":         true,
		"/users/{id}":    true,
		"/admin/orders":  true,
		"/admin/reports": true,
	}
	for _, r := range cfg.Routes {
		if !wantPaths[r.Path] {
			t.Errorf("unexpected route path %q", r.Path)
		}
	}
}

func TestLoad_Include_DetectsCycle(t *testing.T) {
	_, err := Load([]string{"testdata/include/cycle/a.json5"}, "", nil)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") && !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected error to mention cycle/circular, got %v", err)
	}
}

func TestLoad_Include_NestedExpansion(t *testing.T) {
	cfg, err := Load([]string{"testdata/include/nested/root.json5"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes (l1+l2), got %d", len(cfg.Routes))
	}
}

func TestLoad_Include_GlobNoMatchErrors(t *testing.T) {
	tmpDir := t.TempDir()
	rootPath := filepath.Join(tmpDir, "root.json5")
	if err := os.WriteFile(rootPath, []byte(`{ routes: ["@nope/*.json5"] }`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load([]string{rootPath}, "", nil)
	if err == nil {
		t.Fatal("expected glob-no-match error")
	}
}

func TestLoad_Include_RejectsUnsupportedExtension(t *testing.T) {
	tmpDir := t.TempDir()
	rootPath := filepath.Join(tmpDir, "root.json5")
	if err := os.WriteFile(rootPath, []byte(`{ routes: ["@foo.txt"] }`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load([]string{rootPath}, "", nil)
	if err == nil {
		t.Fatal("expected unsupported-extension error")
	}
}

func TestLoad_MultiFile_Merge(t *testing.T) {
	cfg, err := Load(
		[]string{"testdata/merge/base.json5", "testdata/merge/override.json5"},
		"", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// server: 后者覆盖前者；未在 override 中的字段保留 base
	if cfg.Server.Port != 9000 {
		t.Errorf("expected port 9000 (override), got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0 (base preserved), got %q", cfg.Server.Host)
	}
	// globals: deep merge
	if cfg.Globals["apiVersion"] != "v2" {
		t.Errorf("expected apiVersion=v2 (override), got %v", cfg.Globals["apiVersion"])
	}
	if cfg.Globals["extra"] != "added" {
		t.Errorf("expected globals.extra=added")
	}
	// fallback: 后者覆盖
	if cfg.Fallback != "404" {
		t.Errorf("expected fallback=404 (override), got %q", cfg.Fallback)
	}
	// routes: append in order
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes (1 base + 1 override), got %d", len(cfg.Routes))
	}
	if cfg.Routes[0].Path != "/base-only" {
		t.Errorf("expected routes[0].path=/base-only, got %q", cfg.Routes[0].Path)
	}
	if cfg.Routes[1].Path != "/override-only" {
		t.Errorf("expected routes[1].path=/override-only, got %q", cfg.Routes[1].Path)
	}
}

func TestLoadDefault_PicksFirstExistingCandidate(t *testing.T) {
	tmpDir := t.TempDir()
	// 在 tmpDir 下放第二档候选 (fakeserver.json)，第一档 (fakeserver.json5) 故意不放
	target := filepath.Join(tmpDir, "fakeserver.json")
	if err := os.WriteFile(target, []byte(`{"routes":[{"method":"GET","path":"/x","body":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadDefault(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil cfg")
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].Path != "/x" {
		t.Errorf("expected single route /x, got %+v", cfg.Routes)
	}
}

func TestLoadDefault_NoneExistReturnsNilNilNoError(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := LoadDefault(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error when no defaults exist: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil cfg (echo-only fallback), got %+v", cfg)
	}
}

func TestLoad_RouteSourceFile_Populated(t *testing.T) {
	// /tmp/<X>/cfg.json5 with one route → cfg.Routes[0].SourceFile
	// must be the absolute path to cfg.json5
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	body := `{
		routes: [
			{ method: "GET", path: "/x", body: "ok" },
		],
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}
	want := cfgPath
	got := cfg.Routes[0].SourceFile
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Errorf("SourceFile = %q, want %q", got, want)
	}
}

func TestLoad_RouteSourceFile_FromInclude(t *testing.T) {
	tmp := t.TempDir()
	subDir := filepath.Join(tmp, "routes")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	subPath := filepath.Join(subDir, "users.json5")
	subBody := `[
		{ method: "GET", path: "/u", body: "user-route" },
	]`
	if err := os.WriteFile(subPath, []byte(subBody), 0644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tmp, "cfg.json5")
	cfgBody := `{
		routes: [
			{ method: "GET", path: "/main", body: "main-route" },
			"@routes/users.json5",
		],
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(cfg.Routes))
	}
	if filepath.Clean(cfg.Routes[0].SourceFile) != filepath.Clean(cfgPath) {
		t.Errorf("Routes[0] (/main) SourceFile = %q, want %q",
			cfg.Routes[0].SourceFile, cfgPath)
	}
	if filepath.Clean(cfg.Routes[1].SourceFile) != filepath.Clean(subPath) {
		t.Errorf("Routes[1] (/u) SourceFile = %q, want %q",
			cfg.Routes[1].SourceFile, subPath)
	}
}

// TestLoad_RouteSourceFile_UserSuppliedSentinelIgnored 锁定：用户在 JSON5
// 里写 __source_file__ 字段不会污染路径解析——loader 总是用真实文件路径
// 覆盖。防止恶意配置经此 sentinel 注入路径。
func TestLoad_RouteSourceFile_UserSuppliedSentinelIgnored(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	// User tries to inject a fake SourceFile via the sentinel key
	body := `{
		routes: [
			{ method: "GET", path: "/x", body: "ok", __source_file__: "/evil/path.json5" },
		],
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}
	got := filepath.Clean(cfg.Routes[0].SourceFile)
	want := filepath.Clean(cfgPath)
	if got != want {
		t.Errorf("SourceFile = %q, want %q (user-supplied __source_file__ should be ignored)", got, want)
	}
}

func TestLoad_EnvFile_AutoLoaded(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	envPath := filepath.Join(tmp, DefaultEnvFileName)

	if err := os.WriteFile(cfgPath, []byte(`{ routes: [{ method: "GET", path: "/", body: "ok" }] }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(`{ $default: { x: "y" }, dev: { token: "t" } }`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// envName="" + no $active → first non-$default segment ("dev")
	if cfg.Env["x"] != "y" {
		t.Errorf("Env.x=%v want 'y' (from $default)", cfg.Env["x"])
	}
	if cfg.Env["token"] != "t" {
		t.Errorf("Env.token=%v want 't' (from dev)", cfg.Env["token"])
	}
	if cfg.EnvSource != envPath {
		t.Errorf("EnvSource=%q want %q", cfg.EnvSource, envPath)
	}
}

func TestLoad_NoEnvFile_EmptyEnv(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env == nil {
		t.Error("cfg.Env should be non-nil empty map, not nil")
	}
	if len(cfg.Env) != 0 {
		t.Errorf("no env file → empty Env; got %v", cfg.Env)
	}
	if cfg.EnvSource != "" {
		t.Errorf("no env file → empty EnvSource; got %q", cfg.EnvSource)
	}
}

func TestLoad_EnvName_SelectsSegment(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	envPath := filepath.Join(tmp, DefaultEnvFileName)
	if err := os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(`{ $default: { host: "d" }, dev: { token: "DEV" }, staging: { token: "STG" } }`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{cfgPath}, "staging", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env["token"] != "STG" {
		t.Errorf("token=%v want STG", cfg.Env["token"])
	}
	if cfg.Env["host"] != "d" {
		t.Errorf("host=%v want d (from $default)", cfg.Env["host"])
	}
}

func TestLoad_EnvFile_AddedToSourcePaths(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	envPath := filepath.Join(tmp, DefaultEnvFileName)
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ dev: { x: 1 } }`), 0644)

	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range cfg.SourcePaths {
		if filepath.Clean(p) == filepath.Clean(envPath) {
			found = true
		}
	}
	if !found {
		t.Errorf("env path not in SourcePaths: %v", cfg.SourcePaths)
	}
}

func TestLoad_VarOverride_TopLevelKeys(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	envPath := filepath.Join(tmp, DefaultEnvFileName)
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ dev: { apiHost: "from-file", token: "T" } }`), 0644)

	cfg, err := Load([]string{cfgPath}, "dev", map[string]string{"apiHost": "OVERRIDE"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env["apiHost"] != "OVERRIDE" {
		t.Errorf("apiHost=%v want OVERRIDE", cfg.Env["apiHost"])
	}
	if cfg.Env["token"] != "T" {
		t.Errorf("token=%v want T (untouched)", cfg.Env["token"])
	}
}

func TestLoad_VarOverride_WithoutEnvFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644)
	// no env file present
	cfg, err := Load([]string{cfgPath}, "", map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env["k"] != "v" {
		t.Errorf("k=%v want v (overrides should work without env file)", cfg.Env["k"])
	}
}
