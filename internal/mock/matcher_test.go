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
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "unexpected") &&
		!strings.Contains(msg, "syntax") &&
		!strings.Contains(msg, "expected") {
		t.Errorf("compile error message changed shape; expected one of {unexpected/syntax/expected}, got %v", err)
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
	// Defensive: Task 2's downgrade strategy depends on expr producing EITHER
	// a runtime error OR a nil/false result when accessing missing fields.
	// Fail if a future expr version silently returns a non-nil true value.
	if runErr == nil {
		if b, ok := out.(bool); ok && b {
			t.Errorf("missing-field access returned (true, nil) — Task 2's downgrade assumption broken; got out=%v", out)
		}
	}
}

// ── Matcher tests ──

func TestCompileMatcher_Empty(t *testing.T) {
	m, err := CompileMatcher("")
	if err != nil {
		t.Fatalf("empty source should not error: %v", err)
	}
	if m == nil {
		t.Fatal("CompileMatcher must return non-nil even for empty source (the sentinel)")
	}
	ok, err := m.Evaluate(map[string]any{})
	if err != nil {
		t.Errorf("empty matcher.Evaluate err=%v want nil", err)
	}
	if !ok {
		t.Error("empty matcher must always match (return true)")
	}
}

func TestCompileMatcher_BadSyntax(t *testing.T) {
	_, err := CompileMatcher(`request.query.fail ==`)
	if err == nil {
		t.Fatal("expected compile error")
	}
	// 错误信息里应包含原表达式以便排错
	if !strings.Contains(err.Error(), "request.query.fail") {
		t.Errorf("error should mention source: %v", err)
	}
}

func TestMatcher_Evaluate_True(t *testing.T) {
	m, err := CompileMatcher(`request.query.fail == "1"`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	env := map[string]any{
		"request": map[string]any{
			"query": map[string]any{"fail": "1"},
		},
	}
	ok, err := m.Evaluate(env)
	if err != nil {
		t.Fatalf("evaluate err=%v", err)
	}
	if !ok {
		t.Error("expected true")
	}
}

func TestMatcher_Evaluate_False(t *testing.T) {
	m, err := CompileMatcher(`request.query.fail == "1"`)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]any{
		"request": map[string]any{
			"query": map[string]any{"fail": "0"},
		},
	}
	ok, err := m.Evaluate(env)
	if err != nil {
		t.Fatalf("evaluate err=%v", err)
	}
	if ok {
		t.Error("expected false")
	}
}

// TestMatcher_Evaluate_MissingField 锁定 design §4.5 的降级语义：求值
// 错误（含字段缺失导致的 nil 比较失败）→ (false, err)，由调用方决定
// 是否 warn 并继续下一 case。
func TestMatcher_Evaluate_MissingField(t *testing.T) {
	m, err := CompileMatcher(`request.query.fail == "1"`)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]any{
		"request": map[string]any{
			"query": map[string]any{}, // no "fail"
		},
	}
	ok, _ := m.Evaluate(env)
	// expr 对 "<nil> == \"1\"" 在不同版本下的行为：可能返回 (false, nil)
	// 也可能返回 (false, err)。不论 err 是否非 nil，匹配结果都必须是
	// false——这就是降级的可见行为。
	if ok {
		t.Errorf("missing field should yield false (got true)")
	}
}

// TestMatcher_Evaluate_TypeError 锁定：类型不兼容的运算（如对 int 调 len）
// 在运行期错。
func TestMatcher_Evaluate_TypeError(t *testing.T) {
	m, err := CompileMatcher(`len(request.query.foo) > 0`)
	if err != nil {
		t.Fatal(err)
	}
	// foo 是 int，len() 不接受 int —— 运行期错
	env := map[string]any{
		"request": map[string]any{
			"query": map[string]any{"foo": 42},
		},
	}
	ok, runErr := m.Evaluate(env)
	if ok {
		t.Error("type-error should yield false")
	}
	if runErr == nil {
		t.Error("type-error should surface as Evaluate err (so caller can warn)")
	}
}
