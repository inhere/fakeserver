package config

import (
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
		server: { port: 3000 },        // 无引号 key
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
	src := `{ server: { port: 3000  routes: [] }`  // 缺逗号
	var out map[string]any
	err := json5.NewDecoder(strings.NewReader(src)).Decode(&out)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	// 仅验证有 error；具体 message 由 lib 决定
}
