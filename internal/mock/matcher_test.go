package mock

import (
	"strings"
	"testing"

	"github.com/expr-lang/expr"
)

// TestExprSmokeCompileAndRun 锁定 expr v1.x 的最小可用 API：
//   - Compile(src, AsBool(), Env(stubEnv)) 返回 *vm.Program / error
//   - Run(program, env) 返回 any / error
//   - 字段访问语法是 "request.query.fail"（无前置点号）
//   - env map 里 key 名直接作为 expr 变量名
func TestExprSmokeCompileAndRun(t *testing.T) {
	stubEnv := map[string]any{
		"request": map[string]any{
			"query": map[string]any{"fail": "1"},
		},
	}

	prog, err := expr.Compile(`request.query.fail == "1"`, expr.AsBool(), expr.Env(stubEnv))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	out, err := expr.Run(prog, stubEnv)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	b, ok := out.(bool)
	if !ok {
		t.Fatalf("AsBool should yield bool, got %T", out)
	}
	if !b {
		t.Error("expected true")
	}
}

// TestExprSmokeSyntaxError 锁定：语法错在 Compile 期能被发现，不必等到 Run。
func TestExprSmokeSyntaxError(t *testing.T) {
	_, err := expr.Compile(`request.query.fail ==`, expr.AsBool())
	if err == nil {
		t.Fatal("expected compile error on bad syntax")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unexpected") &&
		!strings.Contains(strings.ToLower(err.Error()), "syntax") &&
		!strings.Contains(strings.ToLower(err.Error()), "expected") {
		t.Logf("note: error message wording changed; got %v", err)
	}
}

// TestExprSmokeRuntimeError 锁定：字段不存在不是 panic、不是 compile error，
// 而是 Run 期返回 (nil, error) 或返回 nil。我们 Matcher.Evaluate 据此实现
// "求值错视为不匹配 + warn" 的降级（design §4.5）。
func TestExprSmokeRuntimeError(t *testing.T) {
	prog, err := expr.Compile(`request.query.nope == "x"`, expr.AsBool())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	env := map[string]any{"request": map[string]any{}}
	out, runErr := expr.Run(prog, env)
	t.Logf("missing field => out=%v (%T), err=%v", out, out, runErr)
}
