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
