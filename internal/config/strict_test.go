package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestStrictValidate_CatchesBodyTemplateSyntax(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method: []string{"GET"},
		Path:   "/bad",
		Body:   "{{ .unclosed",
	}}}

	errs := StrictValidate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected strict template problem")
	}
	got := errs[0].Error()
	for _, want := range []string{"GET /bad", "body", "unclosed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("strict error missing %q: %s", want, got)
		}
	}
}

func TestStrictValidate_CatchesHeaderTemplateSyntax(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method:  []string{"GET"},
		Path:    "/bad-header",
		Headers: map[string]string{"X-Trace-Id": "{{ .unclosed"},
		Body:    "ok",
	}}}

	errs := StrictValidate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected strict header problem")
	}
	if got := errs[0].Error(); !strings.Contains(got, "headers.X-Trace-Id") {
		t.Fatalf("strict error should mention header field, got %s", got)
	}
}

func TestStrictValidate_HintsHeaderKeyWithDash(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method: []string{"GET"},
		Path:   "/ua",
		Body:   `{{ .request.headers.User-Agent }}`,
	}}}

	errs := StrictValidate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected strict problem")
	}
	got := errs[0].Error()
	if !strings.Contains(got, `index .request.headers "User-Agent"`) {
		t.Fatalf("strict error should include header index hint, got %s", got)
	}
}

func TestStrictValidate_CaseBodyTemplateSyntax(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method: []string{"POST"},
		Path:   "/cases",
		Cases: []RouteCase{{
			Status: 200,
			Body:   map[string]any{"message": "{{ .unclosed"},
		}},
	}}}

	errs := StrictValidate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected strict case problem")
	}
	got := errs[0].Error()
	for _, want := range []string{"POST /cases", "cases[0].body.message"} {
		if !strings.Contains(got, want) {
			t.Fatalf("strict error missing %q: %s", want, got)
		}
	}
}

// TestStrictValidate_CaseBodyFileMissing 验证 case 级 bodyFile 与路由级一样
// 被 strict 校验，报错信息带 route 标识（method path）与 case 名。
func TestStrictValidate_CaseBodyFileMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "fakeserver.json5")
	if err := os.WriteFile(filepath.Join(dir, "present.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		SourcePaths: []string{src},
		Routes: []Route{{
			Method:     []string{"POST"},
			Path:       "/tasks",
			SourceFile: src,
			Strategy:   "first-match",
			Cases: []RouteCase{
				{Name: "ok", BodyFile: "present.json"},
				{Name: "gone", BodyFile: "gone.json"},
			},
		}},
	}

	errs := StrictValidate(cfg)
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 strict problem (the missing case bodyFile), got %v", errs)
	}
	got := errs[0].Error()
	for _, want := range []string{"POST /tasks", `case "gone"`, "bodyFile", strconv.Quote(filepath.Join(dir, "gone.json"))} {
		if !strings.Contains(got, want) {
			t.Errorf("strict error %q should contain %q", got, want)
		}
	}
}

// TestValidate_CaseBodyFileMissing_NotCheckedWithoutStrict 守住向后兼容：
// 非 strict 的 Validate 不因 case 级 bodyFile 缺失而拒绝配置（历史行为不变）。
func TestValidate_CaseBodyFileMissing_NotCheckedWithoutStrict(t *testing.T) {
	cfg := &Config{
		Routes: []Route{{
			Method: []string{"POST"},
			Path:   "/tasks",
			Cases:  []RouteCase{{Name: "gone", BodyFile: "gone.json"}},
		}},
	}
	for _, err := range Validate(cfg) {
		if strings.Contains(err.Error(), "bodyFile") {
			t.Fatalf("non-strict Validate must not check case bodyFile, got %v", err)
		}
	}
}

// TestStrictValidate_CaseBodyFileFromIncludedFile_ResolvedAgainstRouteDir
// 验证 @include 文件里的 case 相对 bodyFile 按「该 route 文件所在目录」解析：
// routes/tasks-ok.json 存在（不报错），routes/tasks-missing.json 不存在（报错），
// 尽管根配置目录下有一个同名诱饵文件。
func TestStrictValidate_CaseBodyFileFromIncludedFile_ResolvedAgainstRouteDir(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)

	cfgPath, err := filepath.Abs("testdata/source/with-include-cases/cfg.json5")
	if err != nil {
		t.Fatal(err)
	}
	includeDir := filepath.Dir(cfgPath) + string(filepath.Separator) + "routes"
	// 切到无关 CWD，证明解析不依赖 CWD
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	errs := StrictValidate(cfg)
	if len(errs) != 1 {
		t.Fatalf("want 1 strict problem, got %v", errs)
	}
	got := errs[0].Error()
	if !strings.Contains(got, strconv.Quote(filepath.Join(includeDir, "tasks-missing.json"))) {
		t.Errorf("case bodyFile should resolve against the include file dir, got %q", got)
	}
	if strings.Contains(got, "tasks-ok.json") {
		t.Errorf("existing include-relative case bodyFile must not be reported: %q", got)
	}
}

func TestStrictValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerOpts{OSEnvWhitelist: []string{"USER"}},
		Routes: []Route{{
			Method:  []string{"GET"},
			Path:    "/ok",
			Headers: map[string]string{"X-Trace-Id": "{{ shortid }}"},
			Body: map[string]any{
				"id": "{{ uuid }}",
				"ua": `{{ index .request.headers "User-Agent" }}`,
			},
		}},
	}

	if errs := StrictValidate(cfg); len(errs) > 0 {
		t.Fatalf("expected no strict errors, got %v", errs)
	}
}
