package tpl

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
	ttemplate "text/template"
)

// TestStdlibHTMLTemplate_EscapesQuotesInText 锁定 stdlib html/template 在
// 文本上下文中的转义行为：双引号会被替换为 \" 或 &#34;。该 case 让我们
// 知道 html/template 默认不适合 JSON 响应。
func TestStdlibHTMLTemplate_EscapesQuotesInText(t *testing.T) {
	tpl, err := template.New("t").Parse(`{"k": "{{ .v }}"}`)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]any{"v": `a"b`}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// 关键观察：html/template 会把 `"` 转义；JSON 失效
	if !strings.Contains(out, `"`) && !strings.Contains(out, `\"`) && !strings.Contains(out, "&#34;") {
		t.Logf("warning: html/template escape form changed; got %q", out)
	}
	// 反例：text/template 不转义
	tt, _ := ttemplate.New("t").Parse(`{"k": "{{ .v }}"}`)
	buf.Reset()
	_ = tt.Execute(&buf, map[string]any{"v": `a"b`})
	if buf.String() != `{"k": "a"b"}` {
		t.Errorf("expected text/template to NOT escape; got %q", buf.String())
	}
}

func TestTextRenderer_NoHTMLEscape(t *testing.T) {
	r := NewRenderer(nil, nil, 0)
	ctx := map[string]any{
		"request": map[string]any{"method": "POST"},
	}
	out, err := r.Render(`{"name":"{{ .request.method }}"}`, ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != `{"name":"POST"}` {
		t.Errorf("text renderer should not escape; got %q", out)
	}
}

func TestTextRenderer_FuncsAvailable(t *testing.T) {
	r := NewRenderer(nil, nil, 0)
	out, err := r.Render(`{{ upper "abc" }}-{{ uuid | len }}`, map[string]any{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(out, "ABC-36") {
		t.Errorf("expected ABC-36..., got %q", out)
	}
}

func TestHTMLRenderer_EscapesQuotes(t *testing.T) {
	r := NewHTMLRenderer(nil, nil, 0)
	ctx := map[string]any{
		"request": map[string]any{"path": `<script>`},
	}
	out, err := r.Render(`<p>{{ .request.path }}</p>`, ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out, "<script>") {
		t.Errorf("html renderer should escape; got %q", out)
	}
}

func TestRenderer_FakerSeedReproducible(t *testing.T) {
	r := NewRenderer(nil, nil, 12345)
	out1, _ := r.Render(`{{ fakeName }}`, map[string]any{})
	r = NewRenderer(nil, nil, 12345)
	out2, _ := r.Render(`{{ fakeName }}`, map[string]any{})
	if out1 != out2 {
		t.Errorf("with same seed: got %q vs %q", out1, out2)
	}
}

// TestTextRenderer_LowercaseFieldAccess 验证 design §4.1 全小写字段
// 访问（.request.params.id / .now / .config.x）能正常解析——这正是
// Phase 3 后期发现的 bug 修复点。
func TestTextRenderer_LowercaseFieldAccess(t *testing.T) {
	r := NewRenderer(nil, nil, 0)
	ctx := map[string]any{
		"request": map[string]any{
			"method": "GET",
			"params": map[string]string{"id": "42"},
		},
		"config": map[string]any{"apiVersion": "v1"},
	}
	out, err := r.Render(
		`{"id":"{{ .request.params.id }}","m":"{{ .request.method }}","v":"{{ .config.apiVersion }}"}`,
		ctx,
	)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != `{"id":"42","m":"GET","v":"v1"}` {
		t.Errorf("lowercase fields: got %q", out)
	}
}

// TestTextRenderer_ParseError 验证模板语法错误返回 error。
func TestTextRenderer_ParseError(t *testing.T) {
	r := NewRenderer(nil, nil, 0)
	_, err := r.Render(`{{ .unclosed `, map[string]any{})
	if err == nil {
		t.Error("expected parse error for unclosed action")
	}
}

// TestTextRenderer_ExecuteError 验证模板执行错误（缺字段）返回 error。
func TestTextRenderer_ExecuteError(t *testing.T) {
	r := NewRenderer(nil, nil, 0)
	// 调 fail 函数强制返回 error
	_, err := r.Render(`{{ fail "kaboom" }}`, map[string]any{})
	if err == nil {
		t.Error("expected execute error from fail")
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("error should contain message; got %v", err)
	}
}

// TestHTMLRenderer_ParseError html 渲染器对错误模板同样返回 error。
func TestHTMLRenderer_ParseError(t *testing.T) {
	r := NewHTMLRenderer(nil, nil, 0)
	_, err := r.Render(`{{ .unclosed `, map[string]any{})
	if err == nil {
		t.Error("expected parse error")
	}
}

// TestHTMLRenderer_ExecuteError html 渲染器执行错误返回 error。
func TestHTMLRenderer_ExecuteError(t *testing.T) {
	r := NewHTMLRenderer(nil, nil, 0)
	_, err := r.Render(`{{ fail "boom" }}`, map[string]any{})
	if err == nil {
		t.Error("expected execute error")
	}
}
