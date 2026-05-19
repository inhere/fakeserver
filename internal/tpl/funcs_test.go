package tpl

import (
	"strings"
	"testing"
	"text/template"
)

// renderInline 是测试辅助：用我们的 FuncMap 执行一段模板字符串。
func renderInline(t *testing.T, src string, data any) string {
	t.Helper()
	tpl, err := template.New("t").Funcs(BaseFuncMap(nil)).Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, data); err != nil {
		t.Fatalf("exec: %v", err)
	}
	return sb.String()
}

func TestFunc_UUID(t *testing.T) {
	out := renderInline(t, `{{ uuid }}`, nil)
	if len(out) != 36 {
		t.Errorf("uuid length: got %d, want 36; value=%q", len(out), out)
	}
	if strings.Count(out, "-") != 4 {
		t.Errorf("uuid should have 4 hyphens; got %q", out)
	}
}

func TestFunc_Shortid(t *testing.T) {
	out := renderInline(t, `{{ shortid }}`, nil)
	if len(out) != 8 {
		t.Errorf("shortid length: got %d, want 8; value=%q", len(out), out)
	}
}

func TestFunc_Incr_PerName(t *testing.T) {
	// 同名 counter 应递增
	a := renderInline(t, `{{ incr "x" }}`, nil)
	b := renderInline(t, `{{ incr "x" }}`, nil)
	if a == b {
		t.Errorf("incr should produce distinct values; got %q twice", a)
	}
	// 不同名 counter 独立
	c := renderInline(t, `{{ incr "y" }}`, nil)
	if c != "1" {
		t.Errorf("first incr 'y' should be 1; got %q", c)
	}
}

func TestFunc_Default(t *testing.T) {
	out := renderInline(t, `{{ default "fallback" .v }}`, map[string]any{"v": ""})
	if out != "fallback" {
		t.Errorf("default with empty v: got %q, want fallback", out)
	}
	out = renderInline(t, `{{ default "fallback" .v }}`, map[string]any{"v": "real"})
	if out != "real" {
		t.Errorf("default with non-empty v: got %q, want real", out)
	}
}

func TestFunc_Coalesce(t *testing.T) {
	out := renderInline(t, `{{ coalesce "" nil "third" }}`, nil)
	if out != "third" {
		t.Errorf("coalesce: got %q, want third", out)
	}
}

func TestFunc_NowFormatsLayout(t *testing.T) {
	out := renderInline(t, `{{ now "2006-01-02" }}`, nil)
	if len(out) != 10 || out[4] != '-' || out[7] != '-' {
		t.Errorf("now with layout: got %q", out)
	}
}

func TestFunc_Timestamp(t *testing.T) {
	out := renderInline(t, `{{ timestamp }}`, nil)
	if len(out) < 10 {
		t.Errorf("timestamp seconds should be ≥ 10 digits; got %q", out)
	}
	outMs := renderInline(t, `{{ timestamp "ms" }}`, nil)
	if len(outMs) < 13 {
		t.Errorf("timestamp ms should be ≥ 13 digits; got %q", outMs)
	}
}

func TestFunc_RandInt(t *testing.T) {
	for i := 0; i < 50; i++ {
		out := renderInline(t, `{{ randInt 5 10 }}`, nil)
		if out == "" {
			t.Error("randInt empty")
		}
	}
}

func TestFunc_RandString(t *testing.T) {
	out := renderInline(t, `{{ randString 16 }}`, nil)
	if len(out) != 16 {
		t.Errorf("randString(16): got len %d", len(out))
	}
}

func TestFunc_RandChoice(t *testing.T) {
	out := renderInline(t, `{{ randChoice "a" "b" "c" }}`, nil)
	if out != "a" && out != "b" && out != "c" {
		t.Errorf("randChoice: got %q", out)
	}
}

func TestFunc_B64EncDec(t *testing.T) {
	out := renderInline(t, `{{ b64enc "hello" }}`, nil)
	if out != "aGVsbG8=" {
		t.Errorf("b64enc: got %q, want aGVsbG8=", out)
	}
	out = renderInline(t, `{{ b64dec "aGVsbG8=" }}`, nil)
	if out != "hello" {
		t.Errorf("b64dec: got %q, want hello", out)
	}
}

func TestFunc_URLEncDec(t *testing.T) {
	out := renderInline(t, `{{ urlenc "a b" }}`, nil)
	if out != "a+b" && out != "a%20b" {
		t.Errorf("urlenc: got %q", out)
	}
}

func TestFunc_JSONEscape(t *testing.T) {
	out := renderInline(t, `{{ jsonEscape "a\"b" }}`, nil)
	if !strings.Contains(out, `\"`) {
		t.Errorf("jsonEscape should escape quotes; got %q", out)
	}
}

func TestFunc_ToJsonFromJson(t *testing.T) {
	out := renderInline(t, `{{ toJson .v }}`, map[string]any{"v": map[string]any{"k": 1}})
	if !strings.Contains(out, `"k":1`) {
		t.Errorf("toJson: got %q", out)
	}
}

func TestFunc_JSONPath(t *testing.T) {
	data := map[string]any{
		"obj": map[string]any{"a": map[string]any{"b": "deep"}},
	}
	out := renderInline(t, `{{ jsonPath .obj "a.b" }}`, data)
	if out != "deep" {
		t.Errorf("jsonPath a.b: got %q, want deep", out)
	}
}

func TestFunc_TitleSplit(t *testing.T) {
	out := renderInline(t, `{{ title "hello world" }}`, nil)
	if out != "Hello World" {
		t.Errorf("title: got %q", out)
	}
}

func TestFunc_OSEnvWhitelist(t *testing.T) {
	t.Setenv("FAKESERVER_TEST_VAR", "secret")
	// 无白名单 → 放行所有
	tpl, _ := template.New("t").Funcs(BaseFuncMap(nil)).Parse(`{{ osenv "FAKESERVER_TEST_VAR" }}`)
	var sb strings.Builder
	_ = tpl.Execute(&sb, nil)
	if sb.String() != "secret" {
		t.Errorf("osenv no whitelist: got %q", sb.String())
	}
	// 白名单不含 → 拒绝（空串）
	sb.Reset()
	tpl, _ = template.New("t").Funcs(BaseFuncMap([]string{"ALLOWED_KEY"})).Parse(`{{ osenv "FAKESERVER_TEST_VAR" }}`)
	_ = tpl.Execute(&sb, nil)
	if sb.String() != "" {
		t.Errorf("osenv with whitelist excluding key: expected empty, got %q", sb.String())
	}
}

func TestFunc_EnvReturnsDefaultInPhase3(t *testing.T) {
	// design §4.3: env 在 v0.2 才接 .env，v0.1 始终返回 default
	out := renderInline(t, `{{ env "KEY" "fallback" }}`, nil)
	if out != "fallback" {
		t.Errorf("env Phase 3 should return default; got %q", out)
	}
}
