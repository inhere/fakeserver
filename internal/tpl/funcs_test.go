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
	// 没有 default：返回空串
	out = renderInline(t, `{{ env "KEY" }}`, nil)
	if out != "" {
		t.Errorf("env without default should be empty; got %q", out)
	}
}

func TestFunc_AddDate(t *testing.T) {
	// addDate 返回 time.Time；通过 .Format 取年份验证不为空
	out := renderInline(t, `{{ (addDate 1 0 0).Format "2006" }}`, nil)
	if len(out) != 4 {
		t.Errorf("addDate.Format year: got %q", out)
	}
}

func TestFunc_RandFloat(t *testing.T) {
	for i := 0; i < 20; i++ {
		out := renderInline(t, `{{ randFloat 1.0 5.0 }}`, nil)
		if out == "" {
			t.Error("randFloat empty")
		}
	}
	// max<min 边界：返回 min
	out := renderInline(t, `{{ randFloat 5.0 1.0 }}`, nil)
	if out != "5" {
		t.Errorf("randFloat with max<min should return min; got %q", out)
	}
}

func TestFunc_RandIntMaxLessThanMin(t *testing.T) {
	out := renderInline(t, `{{ randInt 10 5 }}`, nil)
	if out != "10" {
		t.Errorf("randInt with max<min should return min; got %q", out)
	}
}

func TestFunc_RandStringCharsets(t *testing.T) {
	out := renderInline(t, `{{ randString 8 "hex" }}`, nil)
	if len(out) != 8 {
		t.Errorf("randString hex: got len %d (%q)", len(out), out)
	}
	for _, ch := range out {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Errorf("randString hex: contains non-hex char %q in %q", ch, out)
		}
	}
	out = renderInline(t, `{{ randString 8 "alpha" }}`, nil)
	if len(out) != 8 {
		t.Errorf("randString alpha: got %q", out)
	}
	out = renderInline(t, `{{ randString 8 "base62" }}`, nil)
	if len(out) != 8 {
		t.Errorf("randString base62: got %q", out)
	}
}

func TestFunc_RandChoice_Empty(t *testing.T) {
	out := renderInline(t, `{{ randChoice }}`, nil)
	if out != "" {
		t.Errorf("randChoice() with no args should be empty; got %q", out)
	}
}

func TestFunc_RandChoice_SingleSlice(t *testing.T) {
	// 单参 slice：从 slice 中选
	data := map[string]any{"opts": []any{"x", "y", "z"}}
	out := renderInline(t, `{{ randChoice .opts }}`, data)
	if out != "x" && out != "y" && out != "z" {
		t.Errorf("randChoice slice: got %q", out)
	}
}

func TestFunc_Shuffle(t *testing.T) {
	data := map[string]any{"opts": []any{"a", "b", "c", "d", "e"}}
	out := renderInline(t, `{{ shuffle .opts | len }}`, data)
	if out != "5" {
		t.Errorf("shuffle should preserve length; got %q", out)
	}
}

func TestFunc_Weighted(t *testing.T) {
	// 奇数个参数 → 返回 ""
	out := renderInline(t, `{{ weighted 1 "a" 2 }}`, nil)
	if out != "" {
		t.Errorf("weighted with odd args should return empty; got %q", out)
	}
	// 正常情况
	out = renderInline(t, `{{ weighted 1 "a" 0 "b" }}`, nil)
	if out != "a" {
		t.Errorf("weighted with non-zero weight on 'a' only: got %q", out)
	}
	// 全 0 权重：返回 args[1]
	out = renderInline(t, `{{ weighted 0 "x" 0 "y" }}`, nil)
	if out != "x" {
		t.Errorf("weighted all-zero: got %q", out)
	}
}

func TestFunc_URLDec(t *testing.T) {
	out := renderInline(t, `{{ urldec "a%20b" }}`, nil)
	if out != "a b" {
		t.Errorf("urldec: got %q", out)
	}
	// 解码失败 → 原样
	out = renderInline(t, `{{ urldec "a%ZZb" }}`, nil)
	if out != "a%ZZb" {
		t.Errorf("urldec invalid: got %q", out)
	}
}

func TestFunc_FromJSON(t *testing.T) {
	out := renderInline(t, `{{ index (fromJson "{\"k\":\"v\"}") "k" }}`, nil)
	if out != "v" {
		t.Errorf("fromJson: got %q", out)
	}
	// invalid JSON → nil
	out = renderInline(t, `{{ fromJson "not json" }}`, nil)
	if out != "<no value>" && out != "" {
		t.Errorf("fromJson invalid: got %q", out)
	}
}

func TestFunc_Split(t *testing.T) {
	out := renderInline(t, `{{ split "," "a,b,c" | len }}`, nil)
	if out != "3" {
		t.Errorf("split: got %q", out)
	}
}

func TestFunc_Print(t *testing.T) {
	// print 输出到 stderr，返回空串
	out := renderInline(t, `{{ print "debug" }}`, nil)
	_ = out // tplfunc.StdFuncMap 可能也提供 print；忽略具体值，只为执行覆盖率
}

func TestFunc_B64DecError(t *testing.T) {
	// 非法 base64 → 空串
	out := renderInline(t, `{{ b64dec "not-base64!@#$" }}`, nil)
	if out != "" {
		t.Errorf("b64dec invalid: got %q", out)
	}
}

func TestFunc_Default_NilAndZero(t *testing.T) {
	// nil 视作 zero
	out := renderInline(t, `{{ default "fb" .v }}`, map[string]any{"v": nil})
	if out != "fb" {
		t.Errorf("default(nil): got %q", out)
	}
	// 数字 0
	out = renderInline(t, `{{ default "fb" .v }}`, map[string]any{"v": 0})
	if out != "fb" {
		t.Errorf("default(int 0): got %q", out)
	}
	// false
	out = renderInline(t, `{{ default "fb" .v }}`, map[string]any{"v": false})
	if out != "fb" {
		t.Errorf("default(false): got %q", out)
	}
}

func TestFunc_Coalesce_NonZeroFirst(t *testing.T) {
	out := renderInline(t, `{{ coalesce "first" "second" }}`, nil)
	if out != "first" {
		t.Errorf("coalesce: got %q", out)
	}
	out = renderInline(t, `{{ coalesce }}`, nil)
	if out != "" {
		t.Errorf("coalesce empty: got %q", out)
	}
}

func TestFunc_ExpandEnv(t *testing.T) {
	t.Setenv("EXPAND_TEST_KEY", "EXPANDED")
	out := renderInline(t, `{{ expandEnv "v=$EXPAND_TEST_KEY" }}`, nil)
	if out != "v=EXPANDED" {
		t.Errorf("expandEnv: got %q", out)
	}
	// 白名单不含 → 空
	tpl, _ := template.New("t").Funcs(BaseFuncMap([]string{"ALLOWED"})).Parse(`{{ expandEnv "v=$EXPAND_TEST_KEY" }}`)
	var sb strings.Builder
	_ = tpl.Execute(&sb, nil)
	if sb.String() != "v=" {
		t.Errorf("expandEnv with whitelist excluding key: got %q", sb.String())
	}
}

func TestFunc_OSEnvDefault(t *testing.T) {
	// 不存在的 key + default
	out := renderInline(t, `{{ osenv "DOES_NOT_EXIST_KEY_XYZ" "fb" }}`, nil)
	if out != "fb" {
		t.Errorf("osenv missing key with default: got %q", out)
	}
	// 无参 → 空串
	out = renderInline(t, `{{ osenv }}`, nil)
	if out != "" {
		t.Errorf("osenv no args: got %q", out)
	}
}

func TestFunc_JSONPath_Index(t *testing.T) {
	data := map[string]any{
		"obj": map[string]any{"arr": []any{"x", "y", "z"}},
	}
	out := renderInline(t, `{{ jsonPath .obj "arr[1]" }}`, data)
	if out != "y" {
		t.Errorf("jsonPath arr[1]: got %q", out)
	}
	// 越界
	out = renderInline(t, `{{ jsonPath .obj "arr[99]" }}`, data)
	if out != "" {
		t.Errorf("jsonPath out-of-range: got %q", out)
	}
	// 路径不存在
	out = renderInline(t, `{{ jsonPath .obj "missing.key" }}`, data)
	if out != "" {
		t.Errorf("jsonPath missing: got %q", out)
	}
}

func TestFunc_Title_EmptyWord(t *testing.T) {
	// 多空格 / 空 word 不应 panic
	out := renderInline(t, `{{ title "  hello   world  " }}`, nil)
	if !strings.Contains(out, "Hello") || !strings.Contains(out, "World") {
		t.Errorf("title: got %q", out)
	}
}

func TestFunc_NowNoLayout(t *testing.T) {
	// 无 layout 应返回 time.Time（模板会用默认 String 表示）
	out := renderInline(t, `{{ (now).Year }}`, nil)
	if out == "" {
		t.Errorf("now no layout (.Year): got empty")
	}
}

func TestFunc_TimestampVariants(t *testing.T) {
	for _, u := range []string{"us", "ns"} {
		out := renderInline(t, `{{ timestamp "`+u+`" }}`, nil)
		if len(out) < 13 {
			t.Errorf("timestamp %s: got %q", u, out)
		}
	}
}
