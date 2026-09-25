package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadOrFatal(t *testing.T, path string) *Config {
	t.Helper()
	cfg, err := Load([]string{path}, "", nil)
	if err != nil {
		t.Fatalf("Load %q: %v", path, err)
	}
	return cfg
}

func TestValidate_HappyPath(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/valid/single-full.json5")
	errs := Validate(cfg)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_BodyAndBodyFile(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/body-and-bodyfile.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "body", "bodyFile") {
		t.Errorf("expected error mentioning body/bodyFile mutex; got %v", errs)
	}
}

func TestValidate_DuplicateRoutes(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/duplicate-routes.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "duplicate", "/users") {
		t.Errorf("expected duplicate-route error; got %v", errs)
	}
}

func TestValidate_MissingMethod(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/missing-method.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "method") {
		t.Errorf("expected missing-method error; got %v", errs)
	}
}

func TestValidate_ReservedPrefix(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/reserved-prefix.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "__fakeserver") {
		t.Errorf("expected reserved-prefix error; got %v", errs)
	}
}

func TestValidate_ProxyAndBody(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/proxy-and-body.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "proxy", "body") {
		t.Errorf("expected proxy/body mutex error; got %v", errs)
	}
}

func TestValidate_ProxyBadScheme(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/proxy-bad-scheme.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "ftp", "scheme") {
		t.Errorf("expected proxy scheme error; got %v", errs)
	}
}

func TestValidate_BodyFileMissing(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bodyfile-missing.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "no-such-file") {
		t.Errorf("expected missing-file error; got %v", errs)
	}
}

func TestValidate_BadFallback(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bad-fallback.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "fallback") {
		t.Errorf("expected fallback enum error; got %v", errs)
	}
}

func TestValidate_BadStrategy(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bad-strategy.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "strategy") {
		t.Errorf("expected strategy enum error; got %v", errs)
	}
}

func TestValidate_CaptureBadMaxBodySize(t *testing.T) {
	cfg := &Config{
		Server: ServerOpts{
			Capture: CaptureConfig{MaxBodySize: "64XB"},
		},
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"},
			Path:   "/x",
			Body:   "ok",
		}},
	}
	errs := Validate(cfg)
	if !containsErrorWith(errs, "capture.maxBodySize", "64XB") {
		t.Fatalf("expected capture.maxBodySize error, got %v", errs)
	}
}

func TestValidate_CaseNamesUniquePerRoute(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method: []string{"GET"},
		Path:   "/api/users",
		Cases: []RouteCase{
			{Name: "empty", Status: 200, Body: map[string]any{"items": []any{}}},
			{Name: "empty", Status: 500, Body: map[string]any{"error": "duplicate"}},
		},
	}}}
	applyDefaults(cfg)

	errs := Validate(cfg)
	if len(errs) == 0 {
		t.Fatal("duplicate case names should fail validation")
	}
	if !strings.Contains(errs[0].Error(), `case name "empty" duplicated`) {
		t.Fatalf("unexpected error: %v", errs[0])
	}
}

func TestValidate_ScenarioReferencesExistingCase(t *testing.T) {
	cfg := &Config{
		Server: ServerOpts{Scenario: "emptyUsers"},
		Routes: []Route{{
			Method: []string{"GET"},
			Path:   "/api/users",
			Cases: []RouteCase{
				{Name: "success", Status: 200, Body: map[string]any{"items": []any{"alice"}}},
				{Name: "empty", Status: 200, Body: map[string]any{"items": []any{}}},
			},
		}},
		Scenarios: map[string]ScenarioConfig{
			"emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
		},
	}
	applyDefaults(cfg)

	if errs := Validate(cfg); len(errs) > 0 {
		t.Fatalf("valid scenario should pass: %v", errs)
	}
}

func TestValidate_ScenarioUnknownRouteAndCase(t *testing.T) {
	cfg := &Config{
		Routes: []Route{{
			Method: []string{"GET"},
			Path:   "/api/users",
			Cases:  []RouteCase{{Name: "success", Status: 200}},
		}, {
			Method: []string{"GET"},
			Path:   "/api/status",
			Body:   "ok",
		}},
		Scenarios: map[string]ScenarioConfig{
			"broken": {Routes: map[string]string{
				"GET /api/missing": "empty",
				"GET /api/users":   "missingCase",
				"GET /api/status":  "success",
			}},
		},
	}
	applyDefaults(cfg)

	errs := Validate(cfg)
	if len(errs) != 3 {
		t.Fatalf("expected 3 validation errors, got %d: %v", len(errs), errs)
	}
	if !containsErrorWith(errs, `route "GET /api/status"`, "has no cases") {
		t.Fatalf("expected route without cases error, got %v", errs)
	}
}

func TestValidate_ServerScenarioUnknown(t *testing.T) {
	cfg := &Config{
		Server: ServerOpts{Scenario: "missing"},
		Routes: []Route{{
			Method: []string{"GET"},
			Path:   "/api/users",
			Cases:  []RouteCase{{Name: "success", Status: 200}},
		}},
		Scenarios: map[string]ScenarioConfig{
			"emptyUsers": {Routes: map[string]string{"GET /api/users": "success"}},
		},
	}
	applyDefaults(cfg)

	errs := Validate(cfg)
	if !containsErrorWith(errs, `server.scenario "missing"`, "does not exist in scenarios") {
		t.Fatalf("expected unknown server.scenario error, got %v", errs)
	}
}

// containsErrorWith returns true if any error message contains every one
// of the given substrings (case-sensitive).
func containsErrorWith(errs []error, subs ...string) bool {
	for _, e := range errs {
		msg := e.Error()
		ok := true
		for _, s := range subs {
			if !strings.Contains(msg, s) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func TestValidate_WhenSyntaxError(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"}, Path: "/x",
			Cases: []RouteCase{
				{When: `request.query.fail ==`, Status: 200, Body: "a"},
			},
		}},
	}
	errs := Validate(cfg)
	if !anyErrContains(errs, "when") {
		t.Errorf("expected when-syntax error, got %v", errs)
	}
}

// TestValidate_WhenSyntaxError_IncludesRouteAndCaseName 锁定 check 阶段
// when 语法错误信息带 route 标识与 case 名（case 无名字时退化为下标）。
func TestValidate_WhenSyntaxError_IncludesRouteAndCaseName(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"POST"}, Path: "/tasks",
			Cases: []RouteCase{
				{Name: "broken", When: `request.body.task_no ==`, Status: 200, Body: "a"},
			},
		}},
	}
	errs := Validate(cfg)
	if len(errs) != 1 {
		t.Fatalf("want 1 when-syntax error, got %v", errs)
	}
	got := errs[0].Error()
	for _, want := range []string{"POST /tasks", `cases[0] ("broken")`} {
		if !strings.Contains(got, want) {
			t.Errorf("when-syntax error %q should contain %q", got, want)
		}
	}
}

func TestValidate_WhenEmpty_NoError(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"}, Path: "/x",
			Cases: []RouteCase{
				{Status: 200, Body: "a"}, // no when — 永远匹配
			},
		}},
	}
	errs := Validate(cfg)
	if len(errs) > 0 {
		t.Errorf("empty when should not error, got %v", errs)
	}
}

func TestWarn_FirstMatchNoFallback(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"}, Path: "/x", Strategy: "first-match",
			Cases: []RouteCase{
				{When: `request.query.a == "1"`, Status: 200, Body: "a"},
				{When: `request.query.b == "1"`, Status: 200, Body: "b"},
			},
		}},
	}
	warns := Warn(cfg)
	if len(warns) == 0 {
		t.Fatal("expected warn for first-match with no fallback")
	}
	if !strings.Contains(warns[0], "first-match") || !strings.Contains(warns[0], "no fallback") {
		t.Errorf("warn msg=%q", warns[0])
	}
}

func TestWarn_FirstMatchWithFallback_Silent(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"}, Path: "/x", Strategy: "first-match",
			Cases: []RouteCase{
				{When: `request.query.a == "1"`, Status: 200, Body: "a"},
				{Status: 200, Body: "fallback"}, // 兜底 case
			},
		}},
	}
	warns := Warn(cfg)
	for _, w := range warns {
		if strings.Contains(w, "first-match") {
			t.Errorf("should not warn when fallback case exists; got %q", w)
		}
	}
}

// anyErrContains returns true if any error message contains sub (case-sensitive).
func anyErrContains(errs []error, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), sub) {
			return true
		}
	}
	return false
}

func TestWarn_ProxyTargetPrivateHost(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		expectWarn bool
	}{
		{"localhost", "http://localhost:8080", true},
		{"127.0.0.1", "http://127.0.0.1:8080", true},
		{"192.168.x", "http://192.168.1.10:8080", true},
		{"10.x", "http://10.0.0.1:8080", true},
		{"172.16-31 lower", "http://172.16.0.1:8080", true},
		{"172.16-31 upper", "http://172.31.255.254:8080", true},
		{"172 outside range", "http://172.32.0.1:8080", false},
		{"public", "http://api.example.com:8080", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Fallback: "echo",
				Routes: []Route{{
					Method: []string{"*"}, Path: "/api/*rest",
					Proxy: &ProxyConfig{Target: tt.target},
				}},
			}
			warns := Warn(cfg)
			hasPrivateWarn := false
			for _, w := range warns {
				if strings.Contains(w, "private") || strings.Contains(w, "localhost") {
					hasPrivateWarn = true
				}
			}
			if hasPrivateWarn != tt.expectWarn {
				t.Errorf("target=%q expected warn=%v, got warns=%v", tt.target, tt.expectWarn, warns)
			}
		})
	}
}

func TestLoad_BodyFileRelativePath_ResolvedAgainstConfigDir(t *testing.T) {
	// Repro for lite-tools-gko: bodyFile relative path should resolve to
	// the config file's directory, not the CWD. Verify by chdir'ing
	// elsewhere before Load.
	wd, _ := os.Getwd()
	defer os.Chdir(wd)

	cfgPath, err := filepath.Abs("testdata/source/cfg.json5")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	otherDir := t.TempDir()
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}

	var cfg *Config
	cfg, err = Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if errs := Validate(cfg); len(errs) > 0 {
		// Pre-fix this would fail because resolveRoutePath would look
		// for data.txt in otherDir (CWD), not testdata/source/.
		t.Fatalf("validate: %v", errs)
	}
	want := filepath.Clean(cfgPath)
	got := filepath.Clean(cfg.Routes[0].SourceFile)
	if want != got {
		t.Errorf("SourceFile = %q, want %q", got, want)
	}
}

// TestLoad_BodyFile_FromIncludedFile_RelativePathResolved 验证 v0.2 Phase 1
// SourceFile 修复对 @include 链中的 bodyFile 也生效——@included routes.json5
// 中的相对 bodyFile "data.txt" 应解析为 routes.json5 所在目录的 data.txt，
// 而非主 cfg.json5 的目录。当前 fixture 两者目录相同，重点是 SourceFile
// 字段正确指向 routes.json5（让 resolveRoutePath 拿到正确的 baseDir）。
func TestLoad_BodyFile_FromIncludedFile_RelativePathResolved(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)

	cfgPath, err := filepath.Abs("testdata/source/with-include/cfg.json5")
	if err != nil {
		t.Fatal(err)
	}
	subPath, err := filepath.Abs("testdata/source/with-include/routes.json5")
	if err != nil {
		t.Fatal(err)
	}
	// 切到无关 CWD，验证相对路径不依赖 CWD
	otherDir := t.TempDir()
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}

	// SourceFile 应指向被 include 的 routes.json5（不是主 cfg.json5）
	got := filepath.Clean(cfg.Routes[0].SourceFile)
	want := filepath.Clean(subPath)
	if got != want {
		t.Errorf("SourceFile = %q, want %q (include 的 route 应保留来源文件)", got, want)
	}

	// Validate 不应报错（bodyFile 能找到 data.txt）
	if errs := Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
}

func TestValidate_HistoryBadMaxBodySize(t *testing.T) {
	cfg := &Config{
		Server:   ServerOpts{HistoryBodyMaxSize: "64XB"},
		Fallback: "echo",
		Routes:   []Route{{Method: []string{"GET"}, Path: "/x", Body: "ok"}},
	}
	errs := Validate(cfg)
	if !containsErrorWith(errs, "historyBodyMaxSize", "64XB") {
		t.Fatalf("expected historyBodyMaxSize error, got %v", errs)
	}
}
