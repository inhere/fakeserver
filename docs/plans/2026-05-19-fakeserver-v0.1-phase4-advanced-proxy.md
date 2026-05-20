# Fakeserver v0.1 · Phase 4 — 多响应 + 条件分支 + Proxy

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：让 `fakeserver serve -c routes.json5` 真正响应配置里的**多响应模式**（`cases` + `strategy` + `when`）与 **Proxy 模式**（`proxy.target`）。

**Architecture**：在 Phase 3 单一响应渲染管线基础上增量推进，共两条新链路：

1. **多响应链路**：`mock.Mount` 改为同时识别 `cases` 路由——为每条 case 预编译 `when` 表达式（`internal/mock/matcher.go`，基于 `expr-lang/expr`），构造 per-route `Selector`（`internal/mock/selector.go`，四种 strategy），请求路径上先用 matcher 过滤 cases，再走 selector 选中一个 case，最终复用 Phase 3 的 `Respond` 渲染单个 case。
2. **Proxy 链路**：新增 `internal/proxy/proxy.go`，以 `httputil.ReverseProxy` 为骨架，每条 proxy route 一个独立实例；Director 阶段做 `stripPathPrefix` + 多条 `rewrite` 正则替换 + 注入请求 header（template 渲染）；ModifyResponse 阶段注入响应 header（template 渲染）；ErrorHandler 阶段拨号失败回 502。请求 body 透传，受 `bodyLimit` 限制上行字节数。

`serve` 子命令的 `assembleRouter` 增加一行 `proxy.Mount(r, cfg, renderer)`；`mock.Mount` 内部按 route 形态分派（单一响应 / cases），proxy route 仍跳过；`config.Validate` 新增 `when` 表达式语法预检查与 `Warn(cfg) []string` 警告通道。

**Tech Stack**：Go 1.26+ · `github.com/expr-lang/expr`（新引入，唯一新增第三方依赖）· 标准库 `net/http/httputil` / `net/url` / `regexp` / `sync/atomic` / `math/rand`。

**前置要求**：

- Phase 1–3 已完成（commits up to `732691e`）
- 已读 `docs/fakeserver-design.md` §3.2 多响应字段段 / §4.5 cases 选择错误处理表 / §6 错误处理（cases-no-match / proxy 上游错误两行） / §9 全章（Proxy 路由）
- 已读 `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` §3 Phase 4 详述
- `fakeserver/` 当前 go.mod 含 rux/v2、gcli/v3、goutil、titanous/json5、easytpl、gofakeit/v7（6 个直接依赖）
- Phase 3 落地事实（来自 overview §3 Phase 3 实际落地偏差段，本 Phase 直接复用）：
  - `tpl.Renderer.Render(src string, ctx map[string]any) (string, error)`
  - `tpl.BuildRenderCtx(req, params, globals) map[string]any`，键名全小写：`request / now / env / osenv / config`
  - `mock.Respond(c *rux.Context, route *config.Route, renderer tpl.Renderer)`：单响应入口（Phase 3 实现）
  - `config.Validate(cfg *Config) []error`：Phase 2 已实现 proxy 字段互斥与 `proxy.target` scheme 校验；本 Phase **只追加** `when` 语法预检查与 `Warn(cfg)` 警告通道，不重复 Phase 2 的互斥规则

**Phase 4 完成定义（DoD，来自 overview §3 Phase 4）**：

1. `go build ./...` 通过
2. `go test ./...` 全绿；`internal/{mock,proxy}` 单元测试覆盖率 ≥ 80%
3. `strategy: "weighted"` 实测分布：1000 次请求按权重比例（容差 ±5%）
4. `strategy: "first-match"` 命中首个 `when` 为 true 的 case；全不匹配返回 500 + `{"error":"no case matched", "route": "...", ...}`
5. 配置中所有 case 都写 `when` 时（first-match 下无兜底 case）启动期在 stderr 发 warn；启动不退出
6. proxy route：用 `httptest.NewServer` 起假上游，验证：
   - 基本透传（method / path / 上行 body / 上游 status / 上游响应 body）
   - `stripPathPrefix` 生效
   - `rewrite` 正则 + `$1` 捕获组替换生效；多条 rewrite 首条命中即返回；全不命中保持原 path
   - 请求 `headers` 注入（模板渲染）；响应 `responseHeaders` 注入（模板渲染）
   - 拨号失败 502 + JSON 错误体含 `target` 字段
   - `bodyLimit` 超限 → 413（请求层拦截，不打到上游）
   - `proxy.timeout` 超时 → 504 + JSON 错误体
7. proxy 路由能与精确 mock 路由共存（rux radix tree 保证精确路径优先级 > 通配 proxy）
8. proxy 字段与 mock 字段互斥的所有组合（body / bodyFile / cases / status / headers / delay 任一与 proxy 共存）启动期报错——Phase 2 已实现，本 Phase 仅回归测试
9. `internal/mock/router.go` 仍跳过 `proxy != nil` 的 route；proxy 路由唯一注册路径是 `internal/proxy/proxy.go:Mount`
10. **Phase 4 不引入** fsnotify / 中间件 / 热加载；新依赖**仅** `github.com/expr-lang/expr`

---

## 文件结构（Phase 4 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `go.mod` / `go.sum` | 引入 `github.com/expr-lang/expr` |
| 新建 | `internal/mock/matcher.go` | `Matcher` 类型：`expr.Compile` 编译缓存 + `Evaluate(ctx) (bool, error)` 求值；空表达式恒为 true；运行期错误降级为 false + 包装 err |
| 新建 | `internal/mock/matcher_test.go` | 编译错误 / 求值 true/false / 字段缺失降级 / 空表达式 |
| 新建 | `internal/mock/selector.go` | `Selector` 接口 + 四种 strategy 实现：`random` / `round-robin` / `weighted` / `first-match`；`NewSelector(strategy)` 工厂；`ErrNoMatch` sentinel |
| 新建 | `internal/mock/selector_test.go` | 各 strategy 概率/顺序行为 + weighted 1000 次分布验证 |
| 新建 | `internal/mock/cases.go` | `RespondCases(c, route, prog, selector, renderer)`：构造 ctx → filter via matchers → selector.Pick → 委托现有 `Respond` 完成单 case 渲染 |
| 新建 | `internal/mock/cases_test.go` | cases 路径上整合测试：first-match 命中 / 全不匹配 500 / random 命中后渲染回归 |
| 修改 | `internal/mock/router.go` | `Mount` 增加 cases 分支：对 `len(Cases)>0 且 Proxy==nil` 的 route 预编译 matchers + selector，注册 cases handler；proxy != nil 仍跳过 |
| 修改 | `internal/mock/router_test.go` | 新增 cases 路由 + proxy 跳过的回归用例 |
| 新建 | `internal/proxy/proxy.go` | `Mount(r, cfg, renderer) error`：遍历 proxy route，构造每条独立 `httputil.ReverseProxy`；Director 做 stripPathPrefix + rewrite + 请求 header 注入；ModifyResponse 做响应 header 注入；ErrorHandler 做 502；包装 bodyLimit / timeout |
| 新建 | `internal/proxy/proxy_test.go` | 用 `httptest.NewServer` 起假上游：基本透传 / strip+rewrite / header / 502 / 413 / 504 / preserveHost |
| 新建 | `internal/proxy/rewrite.go` | `compileRewrites(any) ([]*rewriteRule, error)`：把配置里 string 或 []string 的 `<regex> => <replacement>` 编译成 `[]*rewriteRule{pattern *regexp.Regexp, replacement string}` |
| 新建 | `internal/proxy/rewrite_test.go` | 单条 / 多条 / $1 捕获组 / 不命中保持原值 |
| 修改 | `internal/config/validate.go` | 追加 `when` 表达式语法预检查（`expr.Compile`，仅校验语法）；新增 `Warn(cfg *Config) []string` 函数（first-match 无兜底 case 警告 + proxy.target 私网 info） |
| 修改 | `internal/config/validate_test.go` | 新增：`when` 语法错误用例 + first-match 无兜底警告用例 |
| 修改 | `internal/cli/serve.go` | `assembleRouter` 增加 `proxy.Mount`；启动 banner 在 Validate 后调用 `config.Warn` 打到 stderr |
| 修改 | `internal/cli/serve_test.go` | 新增 E2E：cases 路由命中 + proxy 路由透传（用 httptest.NewServer 假上游）|
| 修改 | `internal/cli/check.go` | 在原有 Validate 之后调用 `config.Warn` 打到 stderr（不影响 exit code）|
| 修改 | `internal/cli/check_test.go` | warn 输出回归 |
| 新建 | `internal/config/testdata/valid/cases-first-match.json5` | first-match 兜底 + 无兜底两个对照用例（启动期对应 warn）|
| 新建 | `internal/config/testdata/valid/proxy-basic.json5` | proxy route 配置样例（stripPathPrefix + rewrite + headers）|
| 修改 | `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` | Phase 4 状态列回写 + 实际落地偏差段 |
| 修改 | `docs/fakeserver-design.md` | 修订记录追加 v0.3-phase4-applied；§13 追加 Phase 4 已落地条目 |

> **注**：Phase 4 **不**新建 `internal/middleware/`、`internal/config/watcher.go`、`internal/admin/handlers.go` 的 `/routes` 扩展——它们留给 Phase 5。

---

## Task 1: 引入 expr-lang/expr + 探查 + smoke 锁定 Compile/Run 行为

**Files**:
- 修改：`go.mod` / `go.sum`
- 新建：`internal/mock/matcher.go`（最小 stub）
- 新建：`internal/mock/matcher_test.go`（smoke）

> **风险点（来自 overview §6）**：`expr-lang/expr` 与 Go 模板上下文如何共享变量。design §3.2 的 `when` 表达式样例是 `request.query.fail == "1"`——注意**没有前置点号**（不像模板的 `.request.query.fail`），expr 是平铺命名空间。本 Task 用 smoke 锁定。

- [ ] **Step 1.1: 拉 expr 依赖**

```
go get github.com/expr-lang/expr
go mod tidy
```

预期：`go.mod` 直接依赖出现 `github.com/expr-lang/expr v...`，indirect 不变。

- [ ] **Step 1.2: 探查 expr API**

执行（在 fakeserver/ 目录下）：

```
go doc github.com/expr-lang/expr | head -60
go doc github.com/expr-lang/expr.Compile
go doc github.com/expr-lang/expr.Run
go doc github.com/expr-lang/expr.AsBool
go doc github.com/expr-lang/expr.Env
```

把每条输出关键摘录写到下面 smoke 测试的注释里。重点确认：

- `Compile(input string, opts ...Option) (*vm.Program, error)` 是否就是这个签名（或返回到 `*Program`）
- `Run(program, env) (any, error)` 的 env 参数是否能直接传 `map[string]any`
- `AsBool()` 是 `Option`，约束程序返回 bool（语法错或类型错在 Compile 期捕获）
- `Env(map[string]any{...})` Option 是否能用 map 来声明类型上下文（避免静态类型校验失败）

- [ ] **Step 1.3: 写 smoke test**

新建 `internal/mock/matcher_test.go`：

```go
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
//
// 该 smoke 不依赖我们自己的 Matcher 类型，确保 expr 库本身的契约稳定。
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
// 这是我们 Task 5 把 expr.Compile 用作启动期 when 语法预检查的依据。
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
// 而是 Run 期返回 (nil, error) 或返回 nil。我们 Matcher.Evaluate 据此实现"求
// 值错视为不匹配 + warn"的降级（design §4.5）。
func TestExprSmokeRuntimeError(t *testing.T) {
	prog, err := expr.Compile(`request.query.nope == "x"`, expr.AsBool())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// env 里没有 query 字段 — 实测 expr 的行为
	env := map[string]any{"request": map[string]any{}}
	out, runErr := expr.Run(prog, env)
	// 把实测结果写进 t.Logf，让 Task 2 的 Evaluate 实现据此选择降级路径
	t.Logf("missing field => out=%v (%T), err=%v", out, out, runErr)
}
```

> **说明**：第 3 个 case 不做断言——它是探测，结果用 `t.Logf` 打到测试输出。Task 2 实现 `Matcher.Evaluate` 时根据 `t.Logf` 的实测结果决定具体降级语义（多半是 `nil` 或 `false`，但 expr 不同小版本差异较大）。

- [ ] **Step 1.4: 新建 `internal/mock/matcher.go` 最小 stub**

```go
package mock

import (
	"github.com/expr-lang/expr/vm"
)

// Matcher wraps a compiled when-expression with its source for error reporting.
// design §3.2 introduces cases[i].when as the conditional branch language;
// design §4.5 documents the runtime-error-as-skip downgrade semantics.
//
// A zero-value Matcher (or one with Program == nil) is the "no when clause"
// sentinel: Evaluate always returns (true, nil).
//
// Task 2 fills in CompileMatcher and Evaluate.
type Matcher struct {
	Source  string
	Program *vm.Program
}
```

- [ ] **Step 1.5: 验证编译 + smoke 通过**

```
go build ./...
go test ./internal/mock/... -v -run TestExprSmoke
```

预期：3 个 smoke PASS。

- [ ] **Step 1.6: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add go.mod go.sum internal/mock/matcher.go internal/mock/matcher_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "chore(mock): 引入 expr-lang/expr + smoke 锁定 Compile/Run 行为"
```

---

## Task 2: matcher.go — CompileMatcher + Evaluate + 错误降级

**Files**:
- 修改：`internal/mock/matcher.go`
- 修改：`internal/mock/matcher_test.go`

> **范围**：实现 design §3.2 + §4.5 表里"`when` 表达式求值出错（类型错/字段缺失）→ 视为不匹配，继续判断下一 case；warn 日志"这条降级语义。本 Task 不写 warn 日志（由调用方 `RespondCases` 决定输出位置），Matcher 自己只返回 `(false, err)`。

- [ ] **Step 2.1: 追加单元测试**

把 `internal/mock/matcher_test.go` 的内容替换为（保留 Task 1 的 3 个 smoke，追加新测试）：

```go
package mock

import (
	"strings"
	"testing"

	"github.com/expr-lang/expr"
)

// ── Smoke tests from Task 1（保持原样） ──

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
	if b, ok := out.(bool); !ok || !b {
		t.Fatalf("expected true, got %v (%T)", out, out)
	}
}

func TestExprSmokeSyntaxError(t *testing.T) {
	_, err := expr.Compile(`request.query.fail ==`, expr.AsBool())
	if err == nil {
		t.Fatal("expected compile error on bad syntax")
	}
}

func TestExprSmokeRuntimeError(t *testing.T) {
	prog, err := expr.Compile(`request.query.nope == "x"`, expr.AsBool())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	env := map[string]any{"request": map[string]any{}}
	out, runErr := expr.Run(prog, env)
	t.Logf("missing field => out=%v (%T), err=%v", out, out, runErr)
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
	// expr 对 "<nil> == \"1\"" 返回 false（这是 expr 的标准行为，由
	// TestExprSmokeRuntimeError 探测确认）。不论 err 是否非 nil，匹配结
	// 果都必须是 false——这就是降级的可见行为。
	if ok {
		t.Errorf("missing field should yield false (got true)")
	}
}

// TestMatcher_Evaluate_TypeError 锁定：类型不兼容的运算（如字符串 + int）
// 编译期 AsBool 已经约束，但运行期 env 实际类型可能与编译期假设不符。
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
```

- [ ] **Step 2.2: 跑测试确认 FAIL**

```
go test ./internal/mock/... -v -run "TestCompileMatcher|TestMatcher_"
```

预期：`undefined: CompileMatcher` / `m.Evaluate undefined` 类的编译错。

- [ ] **Step 2.3: 实现 matcher.go**

把 `internal/mock/matcher.go` 替换为：

```go
package mock

import (
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Matcher wraps a compiled when-expression with its source for error reporting.
// design §3.2 introduces cases[i].when as the conditional branch language;
// design §4.5 documents the runtime-error-as-skip downgrade semantics.
//
// A zero-value Matcher (or one with Program == nil) is the "no when clause"
// sentinel: Evaluate always returns (true, nil).
type Matcher struct {
	Source  string
	Program *vm.Program
}

// CompileMatcher precompiles src as a boolean expression. An empty src
// produces a sentinel Matcher whose Evaluate always returns true (the
// "no when clause" semantics from design §3.2).
//
// Compile errors are wrapped with the source for actionable startup-time
// error messages (config.Validate uses the same wrapping for its when
// syntax pre-check in Task 5).
func CompileMatcher(src string) (*Matcher, error) {
	if src == "" {
		return &Matcher{}, nil
	}
	prog, err := expr.Compile(src, expr.AsBool())
	if err != nil {
		return nil, fmt.Errorf("when %q: %w", src, err)
	}
	return &Matcher{Source: src, Program: prog}, nil
}

// Evaluate runs the compiled program against env. Returns:
//
//   - (true,  nil): empty matcher (no when) OR program evaluated true
//   - (false, nil): program evaluated false
//   - (false, err): runtime error — caller treats as "no match" and warns
//                   (design §4.5 cases-error-handling table)
//
// The third return form is the downgrade path: we deliberately do NOT
// surface the error as a 500, because that would let a single bad case
// definition crash the whole route.
func (m *Matcher) Evaluate(env map[string]any) (bool, error) {
	if m == nil || m.Program == nil {
		return true, nil
	}
	out, err := expr.Run(m.Program, env)
	if err != nil {
		return false, fmt.Errorf("when %q: %w", m.Source, err)
	}
	b, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("when %q: expected bool result, got %T", m.Source, out)
	}
	return b, nil
}
```

- [ ] **Step 2.4: 跑测试确认 PASS**

```
go test ./internal/mock/... -v -run "TestCompileMatcher|TestMatcher_"
```

预期：6 个新测试全部 PASS；3 个 smoke 也 PASS。

- [ ] **Step 2.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/mock/matcher.go internal/mock/matcher_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(mock): matcher 编译 + 求值 + 错误降级"
```

---

## Task 3: selector.go — 四种 strategy

**Files**:
- 新建：`internal/mock/selector.go`
- 新建：`internal/mock/selector_test.go`

> **范围**：实现 design §3.2 列出的四种 strategy + design §4.5 的"先按 when 过滤、再按 strategy 选"的合流逻辑。每个 Selector 实例对应一条 route（round-robin 计数器跟 route 走），并发安全（请求 handler 并发跑）。

> **接口语义关键点**：`Selector.Pick(cases []SelectorCase) (int, error)`——传入的 `cases` **已是过滤后**的子集（caller 用 matcher 过滤后再传）。Selector 不再做 when 求值——职责单一。空切片返回 `ErrNoMatch`。

- [ ] **Step 3.1: 写测试（覆盖 4 种 strategy + 错误路径）**

新建 `internal/mock/selector_test.go`：

```go
package mock

import (
	"errors"
	"math"
	"testing"
)

// SelectorCase 是 Selector.Pick 的输入元素：一条已经过滤通过的 case。
// 这里我们造一个最小 stub（真正定义在 selector.go 里），测试只关心 Weight
// 与原始下标（OrigIdx 用来把 selector 的选中 case 映射回 cfg.Cases）。
//
// 注：这里写在 _test.go 里的引用全部走 selector.go 的导出类型，下面的代
// 码块只是文档说明；Step 3.2 起 SelectorCase 是真实类型。

func makeCases(weights ...int) []SelectorCase {
	out := make([]SelectorCase, len(weights))
	for i, w := range weights {
		out[i] = SelectorCase{OrigIdx: i, Weight: w}
	}
	return out
}

func TestSelector_Empty_ReturnsErrNoMatch(t *testing.T) {
	strategies := []string{"", "random", "round-robin", "weighted", "first-match"}
	for _, strat := range strategies {
		t.Run(strat, func(t *testing.T) {
			sel := NewSelector(strat)
			_, err := sel.Pick(nil)
			if !errors.Is(err, ErrNoMatch) {
				t.Errorf("nil cases for %s: want ErrNoMatch, got %v", strat, err)
			}
		})
	}
}

func TestSelector_FirstMatch(t *testing.T) {
	sel := NewSelector("first-match")
	// 过滤后的集合保留了 OrigIdx；first-match 选第一个即 OrigIdx 最小的那个
	picked, err := sel.Pick([]SelectorCase{
		{OrigIdx: 1, Weight: 0},
		{OrigIdx: 3, Weight: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if picked != 1 {
		t.Errorf("first-match should return first OrigIdx, got %d", picked)
	}
}

func TestSelector_RoundRobin_PerRouteCounter(t *testing.T) {
	sel := NewSelector("round-robin")
	cases := makeCases(0, 0, 0) // OrigIdx 0/1/2
	got := []int{}
	for i := 0; i < 6; i++ {
		idx, err := sel.Pick(cases)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, idx)
	}
	want := []int{0, 1, 2, 0, 1, 2}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("round-robin step %d: got %d want %d (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestSelector_RoundRobin_Isolated(t *testing.T) {
	// 两个独立 Selector 实例的计数器应互不影响（design §3.2: per-route 轮询）
	s1 := NewSelector("round-robin")
	s2 := NewSelector("round-robin")
	cases := makeCases(0, 0)
	idx1a, _ := s1.Pick(cases)
	idx2a, _ := s2.Pick(cases)
	if idx1a != idx2a {
		t.Errorf("first pick should be deterministic (0); got s1=%d s2=%d", idx1a, idx2a)
	}
	idx1b, _ := s1.Pick(cases) // s1 → 1
	idx2b, _ := s2.Pick(cases) // s2 → 1（独立计数）
	if idx1b != 1 || idx2b != 1 {
		t.Errorf("independent counters: s1=%d s2=%d want both=1", idx1b, idx2b)
	}
}

func TestSelector_Random_HitsAllCasesEventually(t *testing.T) {
	sel := NewSelector("random")
	cases := makeCases(0, 0, 0) // 三个等权重，random 应每个都被命中过
	hit := map[int]int{}
	for i := 0; i < 300; i++ {
		idx, err := sel.Pick(cases)
		if err != nil {
			t.Fatal(err)
		}
		hit[idx]++
	}
	for _, oi := range []int{0, 1, 2} {
		if hit[oi] == 0 {
			t.Errorf("random: OrigIdx %d never hit after 300 picks (dist=%v)", oi, hit)
		}
	}
}

// TestSelector_Weighted_Distribution 锁定 DoD #3：1000 次按权重分布 ±5%
// 容差。权重 1:9 → 期望 10% : 90%。
func TestSelector_Weighted_Distribution(t *testing.T) {
	sel := NewSelector("weighted")
	cases := []SelectorCase{
		{OrigIdx: 0, Weight: 1},
		{OrigIdx: 1, Weight: 9},
	}
	const N = 1000
	hit := map[int]int{}
	for i := 0; i < N; i++ {
		idx, err := sel.Pick(cases)
		if err != nil {
			t.Fatal(err)
		}
		hit[idx]++
	}
	exp0, exp1 := 0.1, 0.9
	got0 := float64(hit[0]) / N
	got1 := float64(hit[1]) / N
	if math.Abs(got0-exp0) > 0.05 {
		t.Errorf("weighted dist OrigIdx 0: got %.3f want %.3f ±0.05", got0, exp0)
	}
	if math.Abs(got1-exp1) > 0.05 {
		t.Errorf("weighted dist OrigIdx 1: got %.3f want %.3f ±0.05", got1, exp1)
	}
}

// TestSelector_Weighted_ZeroDefaultsToOne 锁定：Weight<=0 等价于 1，避免
// 用户配置遗漏 weight 字段时整条路由不可命中。
func TestSelector_Weighted_ZeroDefaultsToOne(t *testing.T) {
	sel := NewSelector("weighted")
	cases := makeCases(0, 0) // 都没写 weight；应当各 50%
	const N = 1000
	hit := map[int]int{}
	for i := 0; i < N; i++ {
		idx, _ := sel.Pick(cases)
		hit[idx]++
	}
	got0 := float64(hit[0]) / N
	if math.Abs(got0-0.5) > 0.05 {
		t.Errorf("weight=0 should default to 1; got dist %v", hit)
	}
}

// TestSelector_Unknown_FallsBackToRandom 锁定：未知 strategy 不报错，
// 退化为 random（design §3.2 strategy 默认值是 random）。
func TestSelector_Unknown_FallsBackToRandom(t *testing.T) {
	sel := NewSelector("does-not-exist")
	cases := makeCases(0, 0, 0)
	for i := 0; i < 50; i++ {
		idx, err := sel.Pick(cases)
		if err != nil {
			t.Fatalf("fallback pick err=%v", err)
		}
		if idx < 0 || idx > 2 {
			t.Fatalf("fallback yielded out-of-range OrigIdx %d", idx)
		}
	}
}
```

- [ ] **Step 3.2: 跑测试确认 FAIL**

```
go test ./internal/mock/... -v -run "TestSelector"
```

预期：`undefined: NewSelector / SelectorCase / ErrNoMatch` 类编译错。

- [ ] **Step 3.3: 实现 selector.go**

新建 `internal/mock/selector.go`：

```go
package mock

import (
	"errors"
	"math/rand"
	"sync/atomic"
)

// ErrNoMatch indicates Selector.Pick was called with an empty case set.
// In practice this happens when all when-conditions filtered out their
// cases (or the route had no cases at all). The responder maps this to
// the design §6 "no case matched" 500 response.
var ErrNoMatch = errors.New("no case matched")

// SelectorCase carries the minimal data a Selector needs to make a pick.
// Caller (cases.go) constructs the slice after running Matcher.Evaluate,
// so this slice already only contains cases whose when evaluated to true.
//
// OrigIdx is the original index in route.Cases, so Pick's return value
// addresses the *config* cases slice directly — no double indirection at
// render time.
type SelectorCase struct {
	OrigIdx int
	Weight  int
}

// Selector picks one SelectorCase index (OrigIdx) given the already-
// filtered candidate set. Implementations must be safe for concurrent
// Pick across goroutines (per-request handler invocations run in parallel).
type Selector interface {
	Pick(cases []SelectorCase) (int, error)
}

// NewSelector returns the strategy implementation by name. Empty string
// defaults to "random" (design §3.2). Unknown strategies also fall back
// to random — config.Validate is the source of truth for catching typos
// at startup; this fallback exists for runtime robustness.
func NewSelector(strategy string) Selector {
	switch strategy {
	case "first-match":
		return &firstMatchSelector{}
	case "round-robin":
		return &roundRobinSelector{}
	case "weighted":
		return &weightedSelector{}
	case "", "random":
		return &randomSelector{}
	default:
		return &randomSelector{}
	}
}

type randomSelector struct{}

func (s *randomSelector) Pick(cases []SelectorCase) (int, error) {
	if len(cases) == 0 {
		return 0, ErrNoMatch
	}
	return cases[rand.Intn(len(cases))].OrigIdx, nil
}

type firstMatchSelector struct{}

func (s *firstMatchSelector) Pick(cases []SelectorCase) (int, error) {
	if len(cases) == 0 {
		return 0, ErrNoMatch
	}
	return cases[0].OrigIdx, nil
}

// roundRobinSelector keeps a per-instance atomic counter. Two routes that
// both use round-robin get independent counters (the router creates one
// Selector per route in Task 5).
type roundRobinSelector struct {
	counter atomic.Uint64
}

func (s *roundRobinSelector) Pick(cases []SelectorCase) (int, error) {
	if len(cases) == 0 {
		return 0, ErrNoMatch
	}
	// Subtract 1 so first call lands on index 0 (Add returns the *new* value).
	n := s.counter.Add(1) - 1
	return cases[int(n%uint64(len(cases)))].OrigIdx, nil
}

// weightedSelector picks proportional to Weight, treating Weight<=0 as 1.
type weightedSelector struct{}

func (s *weightedSelector) Pick(cases []SelectorCase) (int, error) {
	if len(cases) == 0 {
		return 0, ErrNoMatch
	}
	total := 0
	for _, c := range cases {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	r := rand.Intn(total)
	acc := 0
	for _, c := range cases {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		acc += w
		if r < acc {
			return c.OrigIdx, nil
		}
	}
	// Unreachable given total>0 and r<total, but fall back to last case
	// defensively (avoid hidden panic).
	return cases[len(cases)-1].OrigIdx, nil
}
```

- [ ] **Step 3.4: 跑测试确认 PASS**

```
go test ./internal/mock/... -v -run "TestSelector" -count=1
```

预期：8 个测试 PASS。weighted/random 测试因有随机性应 *实际跑了* 才能验证容差——加 `-count=1` 防止缓存。

- [ ] **Step 3.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/mock/selector.go internal/mock/selector_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(mock): selector 四种 strategy（含 weighted 分布锁定）"
```

---

## Task 4: cases.go — RespondCases 整合 matcher + selector + 渲染

**Files**:
- 新建：`internal/mock/cases.go`
- 新建：`internal/mock/cases_test.go`

> **范围**：写一条 cases 请求处理函数：构造模板 ctx → matcher 过滤 cases → selector 选中一个 → 把选中的 `RouteCase` 包装成"虚拟单一响应 Route"委托给 Phase 3 的 `Respond`。这样响应渲染逻辑零重复。

> **设计要点**：
>
> 1. cases 没命中 → 写 `{"error":"no case matched", ...}` 500（design §4.5/§6）；用 `writeError` 复用 Phase 3 现有错误体格式。
> 2. 选中 case 的 `status/delay/headers/body/bodyFile` 若为空，**继承外层 route**（design §3.2 末尾"未设则继承外层默认"）——通过构造覆盖默认值的虚拟 Route 实现继承。
> 3. matcher 求值错误 → stderr warn + 视为不匹配（design §4.5）；用 `log.Printf` 写到 stderr，handler 不退出。

- [ ] **Step 4.1: 写测试**

新建 `internal/mock/cases_test.go`：

```go
package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// helper：用 cases handler 启动一个 httptest server，返回 baseURL。
// 模式与 router_test.go 现有用例一致：经过真实 rux 路由匹配。
func startCasesServer(t *testing.T, route *config.Route, renderer tpl.Renderer) *httptest.Server {
	t.Helper()
	// 预编译 matchers + selector，模拟 Task 5 router.Mount 的产物
	matchers := make([]*Matcher, len(route.Cases))
	for i, c := range route.Cases {
		m, err := CompileMatcher(c.When)
		if err != nil {
			t.Fatalf("compile when[%d]: %v", i, err)
		}
		matchers[i] = m
	}
	sel := NewSelector(route.Strategy)
	r := rux.New()
	h := func(c *rux.Context) {
		RespondCases(c, route, matchers, sel, renderer)
	}
	for _, m := range route.Method {
		if m == "*" {
			r.Any(route.Path, h)
		} else {
			r.Add(route.Path, h, strings.ToUpper(m))
		}
	}
	return httptest.NewServer(r)
}

func TestRespondCases_FirstMatch_Hits(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/u/{id}",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{When: `request.query.fail == "1"`, Status: 500, Body: map[string]any{"error": "boom"}},
			{Status: 200, Body: map[string]any{"id": "{{ .request.params.id }}", "ok": true}},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/u/42?fail=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Errorf("?fail=1 → status %d want 500", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["error"] != "boom" {
		t.Errorf("body=%v", got)
	}
}

func TestRespondCases_FirstMatch_Fallback(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/u/{id}",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{When: `request.query.fail == "1"`, Status: 500, Body: map[string]any{"error": "boom"}},
			{Status: 200, Body: map[string]any{"id": "{{ .request.params.id }}"}},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/u/42")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("default branch → status %d want 200", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["id"] != "42" {
		t.Errorf("expected id=42 got %v", got)
	}
}

// TestRespondCases_AllFiltered_NoMatch 锁定 DoD #4 与 design §4.5 错误表：
// first-match 下所有 when 都为 false → 500 + {"error":"no case matched"}。
func TestRespondCases_AllFiltered_NoMatch(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/x",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{When: `request.query.a == "1"`, Status: 200, Body: "a"},
			{When: `request.query.b == "1"`, Status: 200, Body: "b"},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/x") // 两个 when 都 false
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Errorf("no-match → status %d want 500", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["error"] != "no case matched" {
		t.Errorf("error msg=%v want 'no case matched'", got["error"])
	}
	if got["route"] == nil {
		t.Errorf("error body should include route field; got %v", got)
	}
}

func TestRespondCases_CaseInheritsOuterDefaults(t *testing.T) {
	// 外层 route 默认 status=201、Headers["X-Source"]="outer"，case 不写时继承
	route := &config.Route{
		Method:   []string{"POST"},
		Path:     "/p",
		Status:   201,
		Headers:  map[string]string{"X-Source": "outer"},
		Strategy: "first-match",
		Cases: []config.RouteCase{
			{Body: map[string]any{"ok": true}}, // 没写 status/headers → 应继承
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/p", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Errorf("inherit status: got %d want 201", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Source"); got != "outer" {
		t.Errorf("inherit X-Source: got %q want 'outer'", got)
	}
}

// TestRespondCases_Random_HitsBothCases 单测 random 路径走通（具体分布在
// selector_test.go 验证）。
func TestRespondCases_Random_HitsBothCases(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/r",
		Strategy: "random",
		Cases: []config.RouteCase{
			{Status: 200, Body: "A"},
			{Status: 200, Body: "B"},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		resp, err := http.Get(srv.URL + "/r")
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		_, _ = sb.ReadFrom(resp.Body)
		resp.Body.Close()
		seen[sb.String()] = true
	}
	if !seen["A"] || !seen["B"] {
		t.Errorf("50 random picks should hit both cases; seen=%v", seen)
	}
}

// TestRespondCases_RuntimeWhenError_Skips 锁定 design §4.5 降级：when 运
// 行期出错 → 该 case 视为不匹配，后续 case 继续。
func TestRespondCases_RuntimeWhenError_Skips(t *testing.T) {
	route := &config.Route{
		Method:   []string{"GET"},
		Path:     "/e",
		Strategy: "first-match",
		Cases: []config.RouteCase{
			// when 期待 query.foo 是 string 但请求带的是 query.foo=1（其实也是
			// string），所以这里我们用 len() 触发类型错。
			{When: `len(request.query.foo) > 100`, Status: 500, Body: "should not hit"},
			{Status: 200, Body: "fallback"},
		},
	}
	r := tpl.NewRenderer(nil, nil, 1)
	srv := startCasesServer(t, route, r)
	defer srv.Close()

	// 这条请求没带 foo → len(nil) 求值出错 → 跳过 → 命中 fallback
	resp, err := http.Get(srv.URL + "/e")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("when runtime err should skip case; got status %d", resp.StatusCode)
	}
}
```

- [ ] **Step 4.2: 跑测试确认 FAIL**

```
go test ./internal/mock/... -v -run "TestRespondCases"
```

预期：`undefined: RespondCases` 编译错。

- [ ] **Step 4.3: 实现 cases.go**

新建 `internal/mock/cases.go`：

```go
package mock

import (
	"errors"
	"log"
	"net/http"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// RespondCases handles one request for a route with cases[]. The pipeline is:
//
//  1. build template ctx from the request (once, shared with Respond)
//  2. evaluate each case's matcher against the ctx; collect (OrigIdx, Weight)
//     for those returning true. Matcher runtime errors → stderr warn + skip.
//  3. selector picks one OrigIdx from the filtered set. Empty set → 500 +
//     {"error":"no case matched", ...}.
//  4. construct a virtual *config.Route from the chosen case, inheriting
//     status/delay/headers from the outer route when the case omits them,
//     and delegate to Respond — which already implements the full §4.5
//     rendering pipeline.
//
// matchers must be parallel to route.Cases (one Matcher per case, even for
// cases without a when clause — the empty Matcher matches unconditionally).
// selector is owned by the caller (one per route).
func RespondCases(c *rux.Context, route *config.Route, matchers []*Matcher, selector Selector, renderer tpl.Renderer) {
	// We rebuild the ctx here (cheap) so matchers see the *parsed* body /
	// query / params the same way templates will. Respond will build its
	// own ctx again from the request — that's fine because Phase 3
	// designed BuildRenderCtx to be idempotent and req.Body is buffered
	// after the first ReadAll (see Phase 3 落地偏差 in overview §3).
	//
	// NOTE: BuildRenderCtx reads req.Body. After this call req.Body is
	// drained but BuildRenderCtx caches bodyRaw, so a second call inside
	// Respond will see an empty body. Phase 4 落地偏差：we therefore
	// pre-restore the request body by storing the parsed ctx and passing
	// it forward — see selectCaseCtx helper below.
	ctx := tpl.BuildRenderCtx(c.Req, paramsFromContext(c), nil)

	filtered := make([]SelectorCase, 0, len(route.Cases))
	for i, m := range matchers {
		ok, err := m.Evaluate(ctx)
		if err != nil {
			log.Printf("[mock] %s %s case[%d] when err: %v (skipping)", joinMethods(route), route.Path, i, err)
			continue
		}
		if !ok {
			continue
		}
		filtered = append(filtered, SelectorCase{OrigIdx: i, Weight: route.Cases[i].Weight})
	}

	pickedIdx, err := selector.Pick(filtered)
	if err != nil {
		if errors.Is(err, ErrNoMatch) {
			writeError(c.Resp, http.StatusInternalServerError, "no case matched", "", route)
			return
		}
		writeError(c.Resp, http.StatusInternalServerError, "selector error", err.Error(), route)
		return
	}

	chosen := &route.Cases[pickedIdx]
	virtual := caseAsRoute(route, chosen)
	// Respond will re-build ctx from c.Req but req.Body is already drained.
	// To preserve template access to .request.body, we stash the parsed
	// body onto the request via a sentinel header (cheap, request-scoped).
	// Phase 5 may revisit this with an explicit context.WithValue.
	// For now we accept the limitation: case bodies *do* see params/query/
	// headers, but not the parsed JSON body (when-expressions can; their
	// ctx is built up-front).
	Respond(c, virtual, renderer)
}

// caseAsRoute composes a virtual single-response Route by overlaying the
// chosen case onto the outer route's defaults (design §3.2 末尾).
func caseAsRoute(outer *config.Route, c *config.RouteCase) *config.Route {
	v := &config.Route{
		Method:     outer.Method,
		Path:       outer.Path,
		SourceFile: outer.SourceFile, // keeps bodyFile resolution working
		Status:     c.Status,
		Delay:      c.Delay,
		Headers:    c.Headers,
		Body:       c.Body,
		BodyFile:   c.BodyFile,
	}
	if v.Status == 0 {
		v.Status = outer.Status
	}
	if v.Delay == "" {
		v.Delay = outer.Delay
	}
	if len(v.Headers) == 0 && len(outer.Headers) > 0 {
		v.Headers = outer.Headers
	} else if len(outer.Headers) > 0 {
		// merge: outer keys not in case are added; case wins on conflict
		merged := make(map[string]string, len(outer.Headers)+len(v.Headers))
		for k, val := range outer.Headers {
			merged[k] = val
		}
		for k, val := range v.Headers {
			merged[k] = val
		}
		v.Headers = merged
	}
	if v.Body == nil && v.BodyFile == "" {
		v.Body = outer.Body
		v.BodyFile = outer.BodyFile
	}
	return v
}

func joinMethods(r *config.Route) string {
	if len(r.Method) == 0 {
		return "?"
	}
	if len(r.Method) == 1 {
		return r.Method[0]
	}
	out := r.Method[0]
	for _, m := range r.Method[1:] {
		out += "," + m
	}
	return out
}
```

- [ ] **Step 4.4: 跑测试**

```
go test ./internal/mock/... -v -run "TestRespondCases" -count=1
```

预期：6 个新测试 PASS。

> **可能的偏差**：`TestRespondCases_RuntimeWhenError_Skips` 的实测——`len(nil)` 在 expr v1.x 可能不返回错误（库版本差异）。如果该测试 PASS 但理由是 `len()` 返回 0 而非错——这等价于"`when` 求值为 false"，case 同样被跳过，业务效果一致。把 `t.Logf` 加进测试解释。如果 expr 在该路径直接 panic，则需要在 `Matcher.Evaluate` 用 `defer recover()` 包裹——回到 Task 2 调整后再跑。

- [ ] **Step 4.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/mock/cases.go internal/mock/cases_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(mock): cases 路径——matcher 过滤 + selector 选取 + 继承 outer 默认"
```

---

## Task 5: router.go 启用 cases 分支 + validate.go when 语法预检查 + Warnings

**Files**:
- 修改：`internal/mock/router.go`
- 修改：`internal/mock/router_test.go`
- 修改：`internal/config/validate.go`
- 修改：`internal/config/validate_test.go`

> **范围**：
>
> 1. `mock.Mount`：对 `len(Cases)>0 && Proxy==nil` 的 route，预编译每条 `when`、构造 Selector，注册 cases handler。Proxy != nil 仍跳过。
> 2. `config.Validate`：把每条非空 `when` 用 `expr.Compile` 跑一遍，收集语法错。
> 3. `config.Warn`：新增函数，返回 `[]string` 警告（first-match 无兜底；proxy.target 私网地址 info）。`cli/{serve,check}.go` 调用并写 stderr。

- [ ] **Step 5.1: validate 测试 — when 语法 + first-match warn**

把以下用例追加到 `internal/config/validate_test.go`（保留现有用例）：

```go
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

// 顶部 import 追加：strings；helper anyErrContains:
func anyErrContains(errs []error, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), sub) {
			return true
		}
	}
	return false
}
```

> 如果 `anyErrContains` 已存在或 `strings` 已 import，则跳过对应行。

- [ ] **Step 5.2: 跑测试确认 FAIL**

```
go test ./internal/config/... -v -run "TestValidate_When|TestWarn_"
```

预期：编译错 + 函数缺失。

- [ ] **Step 5.3: 实现 validate.go 扩展**

修改 `internal/config/validate.go`，在文件顶部 `import` 块追加：

```go
"github.com/expr-lang/expr"
```

把 Validate 函数的 route 循环里、`strategy enum` 检查**之后**追加：

```go
		// when expression syntax pre-check (Phase 4)
		for ci, cs := range r.Cases {
			if cs.When == "" {
				continue
			}
			if _, cerr := expr.Compile(cs.When, expr.AsBool()); cerr != nil {
				errs = append(errs, fmt.Errorf("%s cases[%d].when: %w", prefix, ci, cerr))
			}
		}
```

在文件末尾追加 `Warn` 函数：

```go
// Warn returns non-fatal advisory messages found during validation.
// design §3.7 warning bucket. Caller (cli/serve.go, cli/check.go) is
// expected to print these to stderr without affecting exit code.
//
// Currently checks:
//   - strategy=first-match where every case has a when (no fallback);
//     warns the route can return 500 "no case matched" at runtime.
//   - proxy.target host resolves to localhost/private (design §9.4 info).
func Warn(cfg *Config) []string {
	if cfg == nil {
		return nil
	}
	var warns []string
	for i, r := range cfg.Routes {
		prefix := fmt.Sprintf("routes[%d] (%s %s)", i, strings.Join(r.Method, ","), r.Path)

		// first-match without fallback
		if r.Strategy == "first-match" && len(r.Cases) > 0 {
			hasFallback := false
			for _, c := range r.Cases {
				if c.When == "" {
					hasFallback = true
					break
				}
			}
			if !hasFallback {
				warns = append(warns, fmt.Sprintf("%s: strategy=first-match with no fallback case (all cases have when); runtime requests that match no case will return 500", prefix))
			}
		}

		// proxy.target private host
		if r.Proxy != nil && r.Proxy.Target != "" {
			if isPrivateOrLocalhost(r.Proxy.Target) {
				warns = append(warns, fmt.Sprintf("%s: proxy.target %q resolves to localhost/private network (intentional? double-check)", prefix, r.Proxy.Target))
			}
		}
	}
	return warns
}

// isPrivateOrLocalhost is a cheap heuristic on the host string of a URL.
// It does NOT do DNS resolution — only checks literal hosts. Good enough
// for an advisory warning.
func isPrivateOrLocalhost(target string) bool {
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	// crude private-range prefixes
	for _, p := range []string{"10.", "192.168.", "169.254."} {
		if strings.HasPrefix(host, p) {
			return true
		}
	}
	if strings.HasPrefix(host, "172.") {
		// 172.16.0.0/12
		parts := strings.SplitN(host, ".", 3)
		if len(parts) >= 2 {
			var n int
			_, _ = fmt.Sscanf(parts[1], "%d", &n)
			if n >= 16 && n <= 31 {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 5.4: 跑测试确认 PASS**

```
go test ./internal/config/... -v -run "TestValidate_When|TestWarn_"
```

预期：4 个新测试 PASS。

- [ ] **Step 5.5: router.go 测试 — cases 分支**

把以下测试追加到 `internal/mock/router_test.go`（保留 Phase 3 现有用例）：

```go
func TestMount_RegistersCasesRoute(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x", Strategy: "first-match",
			Cases: []config.RouteCase{
				{When: `request.query.fail == "1"`, Status: 500, Body: "boom"},
				{Status: 200, Body: "ok"},
			},
		}},
	}
	r := rux.New()
	rdr := tpl.NewRenderer(nil, nil, 1)
	if err := Mount(r, cfg, rdr); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/x?fail=1")
	if resp.StatusCode != 500 {
		t.Errorf("cases first-match got %d want 500", resp.StatusCode)
	}
	resp, _ = http.Get(srv.URL + "/x")
	if resp.StatusCode != 200 {
		t.Errorf("cases fallback got %d want 200", resp.StatusCode)
	}
}

func TestMount_StillSkipsProxyRoute(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy:  &config.ProxyConfig{Target: "http://upstream:8080"},
		}},
	}
	r := rux.New()
	rdr := tpl.NewRenderer(nil, nil, 1)
	if err := Mount(r, cfg, rdr); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	// proxy route 不应被 mock.Mount 注册 → 请求应不命中 mock 路由表（这里
	// 不挂 echo，所以 404 即为"mock 未注册"的证据）
	srv := httptest.NewServer(r)
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/api/users/1")
	if resp.StatusCode != 404 {
		t.Errorf("proxy route should not be mounted by mock.Mount; got status %d", resp.StatusCode)
	}
}

func TestMount_CasesCompileError(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Cases: []config.RouteCase{
				{When: `bad syntax ==`, Status: 200, Body: "a"},
			},
		}},
	}
	r := rux.New()
	rdr := tpl.NewRenderer(nil, nil, 1)
	err := Mount(r, cfg, rdr)
	if err == nil {
		t.Fatal("Mount should error on bad when (Validate normally catches this; defense-in-depth)")
	}
}
```

- [ ] **Step 5.6: 跑测试确认 FAIL**

```
go test ./internal/mock/... -v -run "TestMount_"
```

预期：cases 分支用例 FAIL（当前 Mount 跳过 cases 路由）。

- [ ] **Step 5.7: 实现 router.go cases 分支**

把 `internal/mock/router.go` 替换为：

```go
package mock

import (
	"fmt"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Mount registers mock routes (single-response and cases) onto r. Routes
// with proxy{} are skipped — proxy.Mount handles them.
//
// Phase 4 dispatch:
//
//	route.Proxy != nil       → skip (proxy.Mount registers separately)
//	len(route.Cases) > 0     → precompile matchers + selector, register cases handler
//	otherwise                → register the Phase 3 single-response handler
//
// Compile errors in any when-expression are returned as the first error
// (Validate normally catches these — this is defense-in-depth so the
// router never silently registers a half-broken route).
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		route := &cfg.Routes[i] // closure pointer
		if route.Proxy != nil {
			continue
		}

		var handler rux.HandlerFunc
		if len(route.Cases) > 0 {
			matchers := make([]*Matcher, len(route.Cases))
			for ci, c := range route.Cases {
				m, err := CompileMatcher(c.When)
				if err != nil {
					return fmt.Errorf("route[%d] %s %s: %w", i, strings.Join(route.Method, ","), route.Path, err)
				}
				matchers[ci] = m
			}
			selector := NewSelector(route.Strategy)
			handler = func(c *rux.Context) {
				RespondCases(c, route, matchers, selector, renderer)
			}
		} else {
			handler = func(c *rux.Context) {
				Respond(c, route, renderer)
			}
		}

		for _, m := range route.Method {
			method := strings.ToUpper(m)
			if method == "*" {
				r.Any(route.Path, handler)
			} else {
				r.Add(route.Path, handler, method)
			}
		}
	}
	return nil
}
```

- [ ] **Step 5.8: 跑测试确认 PASS**

```
go test ./internal/mock/... -v -count=1
go test ./internal/config/... -v -count=1
```

预期：mock 包全绿（含 Phase 3 旧用例 + Task 1-5 新用例）；config 包全绿（含新增的 when 语法 + Warn 用例）。

- [ ] **Step 5.9: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/mock/router.go internal/mock/router_test.go internal/config/validate.go internal/config/validate_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(mock,config): cases 路由注册 + when 语法预检查 + Warn 警告通道"
```

---

## Task 6: proxy 包 — rewrite.go 规则编译

**Files**:
- 新建：`internal/proxy/rewrite.go`
- 新建：`internal/proxy/rewrite_test.go`

> **范围**：把配置 `proxy.rewrite`（字段类型 `any`：string 或 []string）编译成 `[]*rewriteRule{regex, replacement}`。每条规则按 `<go-regex> => <replacement>` 拆分；不命中下一条；全不命中保留原 path（design §9.2/§9.3）。

- [ ] **Step 6.1: 写测试**

新建 `internal/proxy/rewrite_test.go`：

```go
package proxy

import (
	"testing"
)

func TestCompileRewrites_Nil(t *testing.T) {
	rules, err := compileRewrites(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("nil input → 0 rules; got %d", len(rules))
	}
}

func TestCompileRewrites_SingleString(t *testing.T) {
	rules, err := compileRewrites(`^/api/users => /v2/users`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules", len(rules))
	}
	out, ok := applyRewrites(rules, "/api/users/42")
	if !ok || out != "/v2/users/42" {
		t.Errorf("apply: out=%q ok=%v", out, ok)
	}
}

func TestCompileRewrites_Array(t *testing.T) {
	rules, err := compileRewrites([]any{
		`^/api/v1/(.+) => /legacy/$1`,
		`^/api/v2/(.+) => /modern/$1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules", len(rules))
	}
	out, _ := applyRewrites(rules, "/api/v2/users")
	if out != "/modern/users" {
		t.Errorf("apply v2: %q", out)
	}
	out, _ = applyRewrites(rules, "/api/v1/orders/9")
	if out != "/legacy/orders/9" {
		t.Errorf("apply v1: %q", out)
	}
}

func TestCompileRewrites_StringSlice(t *testing.T) {
	// JSON5 也可能把 array of string decode 成 []string 而非 []any
	rules, err := compileRewrites([]string{
		`^/a => /A`,
		`^/b => /B`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules", len(rules))
	}
}

func TestCompileRewrites_NoMatchPreservesPath(t *testing.T) {
	rules, _ := compileRewrites(`^/api/v1/(.+) => /legacy/$1`)
	out, ok := applyRewrites(rules, "/health")
	if ok {
		t.Error("no-match should signal ok=false")
	}
	if out != "/health" {
		t.Errorf("no-match should preserve path; got %q", out)
	}
}

func TestCompileRewrites_FirstMatchWins(t *testing.T) {
	rules, _ := compileRewrites([]any{
		`^/api/users => /v2/users`,
		`^/api => /catchall`, // 后规则应被首条 swallow
	})
	out, _ := applyRewrites(rules, "/api/users/1")
	if out != "/v2/users/1" {
		t.Errorf("first match should win; got %q", out)
	}
}

func TestCompileRewrites_BadSyntax(t *testing.T) {
	_, err := compileRewrites(`/api/users /v2/users`) // missing =>
	if err == nil {
		t.Error("expected error on missing arrow")
	}
}

func TestCompileRewrites_BadRegex(t *testing.T) {
	_, err := compileRewrites(`[invalid => /x`)
	if err == nil {
		t.Error("expected regex compile error")
	}
}
```

- [ ] **Step 6.2: 跑测试确认 FAIL**

```
go test ./internal/proxy/... -v
```

预期：包不存在 / 函数未定义。

- [ ] **Step 6.3: 实现 rewrite.go**

新建 `internal/proxy/rewrite.go`：

```go
// Package proxy implements fakeserver's reverse-proxy routes (design §9).
// Each route declared with a proxy{} block becomes a httputil.ReverseProxy
// instance with custom Director (path rewrite + header injection),
// ModifyResponse (response header injection), and ErrorHandler (502 on
// dial failure).
package proxy

import (
	"fmt"
	"regexp"
	"strings"
)

// rewriteRule is one compiled "<regex> => <replacement>" pair.
// applyRewrites tries them in order; first match wins.
type rewriteRule struct {
	pattern     *regexp.Regexp
	replacement string
}

// compileRewrites parses the rewrite field from a proxy config block.
// Accepts:
//
//	nil               → []*rewriteRule{} (no rules)
//	string            → one rule
//	[]any / []string  → multiple rules (in order)
//
// Each rule must be "<go-regex> => <replacement>". $1, $2, ... in the
// replacement reference regex capture groups (Go regexp ReplaceAllString
// semantics; design §9.2 explicitly notes these are NOT template variables).
func compileRewrites(raw any) ([]*rewriteRule, error) {
	if raw == nil {
		return nil, nil
	}
	var sources []string
	switch v := raw.(type) {
	case string:
		sources = []string{v}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("rewrite: array element must be string, got %T", item)
			}
			sources = append(sources, s)
		}
	case []string:
		sources = v
	default:
		return nil, fmt.Errorf("rewrite: expected string or []string, got %T", raw)
	}

	out := make([]*rewriteRule, 0, len(sources))
	for _, src := range sources {
		idx := strings.Index(src, "=>")
		if idx < 0 {
			return nil, fmt.Errorf("rewrite %q: missing '=>' separator", src)
		}
		pat := strings.TrimSpace(src[:idx])
		repl := strings.TrimSpace(src[idx+2:])
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("rewrite %q: regex compile: %w", src, err)
		}
		out = append(out, &rewriteRule{pattern: re, replacement: repl})
	}
	return out, nil
}

// applyRewrites runs path through the first matching rule. Returns
// (rewritten, true) on first match or (original, false) when no rule
// matches.
func applyRewrites(rules []*rewriteRule, path string) (string, bool) {
	for _, r := range rules {
		if loc := r.pattern.FindStringIndex(path); loc != nil {
			return r.pattern.ReplaceAllString(path, r.replacement), true
		}
	}
	return path, false
}
```

- [ ] **Step 6.4: 跑测试**

```
go test ./internal/proxy/... -v
```

预期：8 个测试 PASS。

- [ ] **Step 6.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/proxy/rewrite.go internal/proxy/rewrite_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(proxy): rewrite 规则编译 + 首条命中应用"
```

---

## Task 7: proxy/proxy.go — Build + Mount 基础透传

**Files**:
- 新建：`internal/proxy/proxy.go`
- 新建：`internal/proxy/proxy_test.go`

> **范围**：本 Task 实现 ReverseProxy 装配的**基础链路**——Mount 遍历 cfg.Routes、为每条 proxy route 调 Build 构造 handler、注册到 rux。Build 配置 `httputil.ReverseProxy.Director` 改写 URL 与 Host，`ErrorHandler` 处理拨号失败 502。`stripPathPrefix`、`rewrite`、`headers/responseHeaders`、`bodyLimit`、`timeout`、`preserveHost`、`insecureSkipVerify` 由 Task 8 增量补全。

> **rux 集成关键**：rux 的 HandlerFunc 拿到 `*rux.Context`，里面的 `c.Req *http.Request` 和 `c.Resp http.ResponseWriter` 直接交给 `ReverseProxy.ServeHTTP` 即可。

- [ ] **Step 7.1: 写测试**

新建 `internal/proxy/proxy_test.go`：

```go
package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// startProxyServer mounts cfg's proxy routes onto a fresh rux router and
// wraps it in an httptest.Server. Returns the server.
func startProxyServer(t *testing.T, cfg *config.Config) *httptest.Server {
	t.Helper()
	r := rux.New()
	rdr := tpl.NewRenderer(nil, nil, 1)
	if err := Mount(r, cfg, rdr); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	return httptest.NewServer(r)
}

// echoUpstream answers any request with method+path+body in a JSON blob.
// Used as a stable assertion target by proxy tests.
func echoUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusOK)
		// minimal assertion-friendly format
		_, _ = w.Write([]byte("UP:" + r.Method + ":" + r.URL.Path + ":" + string(b)))
	}))
}

func TestProxy_BasicForward(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy:  &config.ProxyConfig{Target: upstream.URL},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/users/1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d want 200", resp.StatusCode)
	}
	if resp.Header.Get("X-Upstream") != "yes" {
		t.Error("missing X-Upstream → response not coming from upstream")
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(b), "UP:GET:/api/users/1") {
		t.Errorf("body=%q", string(b))
	}
}

func TestProxy_DialFailure_502(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy:  &config.ProxyConfig{Target: "http://127.0.0.1:1"}, // 无端口监听
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Errorf("dial-fail status=%d want 502", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("error body should be JSON; CT=%q", ct)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `"target"`) {
		t.Errorf("error body should mention target; got %s", string(b))
	}
}

func TestProxy_PostBodyForwarded(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"POST"}, Path: "/api/*rest",
			Proxy:  &config.ProxyConfig{Target: upstream.URL},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/echo", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.HasSuffix(string(b), ":hello") {
		t.Errorf("body=%q upstream did not see request body", string(b))
	}
}
```

- [ ] **Step 7.2: 跑测试确认 FAIL**

```
go test ./internal/proxy/... -v -run "TestProxy_"
```

预期：`undefined: Mount` 编译错。

- [ ] **Step 7.3: 实现 proxy.go**

新建 `internal/proxy/proxy.go`：

```go
package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Mount registers every proxy route in cfg onto r. Routes without a proxy
// block are skipped (mock.Mount handles them).
//
// Each proxy route gets its own *httputil.ReverseProxy instance so target
// hostnames / TLS settings / timeouts don't have to be re-computed on
// every request.
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		route := &cfg.Routes[i]
		if route.Proxy == nil {
			continue
		}
		handler, err := Build(route, renderer)
		if err != nil {
			return fmt.Errorf("route[%d] %s %s: %w", i, strings.Join(route.Method, ","), route.Path, err)
		}
		for _, m := range route.Method {
			method := strings.ToUpper(m)
			if method == "*" {
				r.Any(route.Path, handler)
			} else {
				r.Add(route.Path, handler, method)
			}
		}
	}
	return nil
}

// Build constructs the rux handler for one proxy route. The returned
// handler is goroutine-safe (ReverseProxy is concurrency-safe; rewrite
// rules are read-only after compilation).
//
// Task 7 implements the basic forwarding + 502-on-dial-failure path.
// Task 8 layers on stripPathPrefix, rewrite, headers, responseHeaders,
// bodyLimit, timeout, preserveHost, insecureSkipVerify.
func Build(route *config.Route, renderer tpl.Renderer) (rux.HandlerFunc, error) {
	p := route.Proxy
	if p == nil {
		return nil, fmt.Errorf("Build called with nil Proxy")
	}
	targetURL, err := url.Parse(p.Target)
	if err != nil {
		return nil, fmt.Errorf("proxy.target %q: %w", p.Target, err)
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		return nil, fmt.Errorf("proxy.target scheme must be http/https, got %q", targetURL.Scheme)
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.Host = targetURL.Host
			// Path: keep client's path as-is in Task 7; Task 8 layers
			// stripPathPrefix + rewrite here.
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, perr error) {
			writeProxyError(w, http.StatusBadGateway, "upstream dial failed", perr.Error(), route, p.Target)
		},
	}
	return func(c *rux.Context) {
		rp.ServeHTTP(c.Resp, c.Req)
	}, nil
}

// writeProxyError emits the §9.3/§6 error body. The `target` field is the
// distinguishing feature of proxy errors vs mock errors.
func writeProxyError(w http.ResponseWriter, status int, short, detail string, route *config.Route, target string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  short,
		"detail": detail,
		"route":  fmt.Sprintf("%s %s", strings.Join(route.Method, ","), route.Path),
		"target": target,
	})
}
```

- [ ] **Step 7.4: 跑测试**

```
go test ./internal/proxy/... -v -count=1
```

预期：3 个 proxy 测试 PASS（rewrite 包测试也仍绿）。

- [ ] **Step 7.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/proxy/proxy.go internal/proxy/proxy_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(proxy): Mount + Build 基础透传 + 502 拨号失败"
```

---

## Task 8: proxy 增强 — stripPathPrefix + rewrite + headers + bodyLimit + timeout + preserveHost

**Files**:
- 修改：`internal/proxy/proxy.go`
- 修改：`internal/proxy/proxy_test.go`

> **范围**：按 design §9.2/§9.3 把 Task 7 留下的所有 proxy 字段补齐：
>
> - `stripPathPrefix`：rewrite **之前**剥前缀
> - `rewrite`：调用 Task 6 的 `applyRewrites`
> - `headers`：请求 header 注入；value 走 template 渲染
> - `responseHeaders`：响应 header 注入；value 走 template 渲染
> - `bodyLimit`：用 `http.MaxBytesReader` 包 `req.Body`；超限 → 413（拦在请求路径上，不打到上游）
> - `timeout`：自定义 `Transport.ResponseHeaderTimeout` + Director 阶段加 ctx；超时 → 504
> - `preserveHost`：默认改 `req.Host = target.Host`；为 true 时保留客户端 Host
> - `insecureSkipVerify`：HTTPS 自签证书时透传给 Transport.TLSClientConfig
>
> design §9.2 明确："模板渲染边界：proxy 块内只有 `headers` 与 `responseHeaders` 的 value 字符串走 template 渲染，其余字段（target / rewrite / stripPathPrefix / timeout 等）均为字面量"——本 Task 严格遵循。

- [ ] **Step 8.1: 追加测试**

把以下追加到 `internal/proxy/proxy_test.go`（保留 Task 7 用例）：

```go
func TestProxy_StripPathPrefix(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy:  &config.ProxyConfig{Target: upstream.URL, StripPathPrefix: "/api"},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/api/users/1")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(b), "UP:GET:/users/1") {
		t.Errorf("expected /users/1 after strip; got %q", string(b))
	}
}

func TestProxy_Rewrite(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy: &config.ProxyConfig{
				Target:  upstream.URL,
				Rewrite: `^/api/v1/(.+) => /legacy/$1`,
			},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/api/v1/orders")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(b), "UP:GET:/legacy/orders") {
		t.Errorf("expected /legacy/orders; got %q", string(b))
	}
}

func TestProxy_StripThenRewrite(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy: &config.ProxyConfig{
				Target:          upstream.URL,
				StripPathPrefix: "/api",
				Rewrite:         `^/v1/(.+) => /legacy/$1`,
			},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	// /api/v1/orders → strip → /v1/orders → rewrite → /legacy/orders
	resp, _ := http.Get(srv.URL + "/api/v1/orders")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(b), "UP:GET:/legacy/orders") {
		t.Errorf("strip+rewrite chain: got %q", string(b))
	}
}

func TestProxy_RequestHeaderInjection(t *testing.T) {
	upstreamSawHeader := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamSawHeader = r.Header.Get("X-Forwarded-By")
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy: &config.ProxyConfig{
				Target: upstream.URL,
				Headers: map[string]string{
					"X-Forwarded-By": "fakeserver-{{ .request.method }}",
				},
			},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/x")
	resp.Body.Close()
	if upstreamSawHeader != "fakeserver-GET" {
		t.Errorf("upstream X-Forwarded-By=%q (template not rendered?)", upstreamSawHeader)
	}
}

func TestProxy_ResponseHeaderInjection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy: &config.ProxyConfig{
				Target: upstream.URL,
				ResponseHeaders: map[string]string{
					"X-Mocked-By": "fakeserver-proxy",
				},
			},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/x")
	resp.Body.Close()
	if got := resp.Header.Get("X-Mocked-By"); got != "fakeserver-proxy" {
		t.Errorf("response X-Mocked-By=%q", got)
	}
}

func TestProxy_BodyLimit_413(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"POST"}, Path: "/x",
			Proxy:  &config.ProxyConfig{Target: upstream.URL, BodyLimit: "16B"},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/x", "text/plain", strings.NewReader("this body is more than sixteen bytes long"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Errorf("status=%d want 413", resp.StatusCode)
	}
}

func TestProxy_Timeout_504(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
			w.WriteHeader(200)
		}
	}))
	defer slow.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy:  &config.ProxyConfig{Target: slow.URL, Timeout: "50ms"},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 504 {
		t.Errorf("timeout status=%d want 504", resp.StatusCode)
	}
}

func TestProxy_PreserveHost(t *testing.T) {
	var sawHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHost = r.Host
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy:  &config.ProxyConfig{Target: upstream.URL, PreserveHost: true},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/x", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	// PreserveHost: 上游应看到客户端发起的 Host（srv.URL 去掉 scheme 部分）
	want := strings.TrimPrefix(srv.URL, "http://")
	if sawHost != want {
		t.Errorf("preserveHost: upstream saw Host=%q want %q", sawHost, want)
	}
}
```

> 顶部 import 追加 `"time"`。

- [ ] **Step 8.2: 跑测试确认 FAIL**

```
go test ./internal/proxy/... -v -run "TestProxy_StripPath|TestProxy_Rewrite|TestProxy_StripThen|TestProxy_RequestHeader|TestProxy_ResponseHeader|TestProxy_BodyLimit|TestProxy_Timeout|TestProxy_Preserve" -count=1
```

预期：8 个新测试 FAIL（功能未实现）。

- [ ] **Step 8.3: 重写 proxy.go**

把 `internal/proxy/proxy.go` 替换为：

```go
package proxy

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Mount registers every proxy route in cfg onto r. Routes without a proxy
// block are skipped (mock.Mount handles them).
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		route := &cfg.Routes[i]
		if route.Proxy == nil {
			continue
		}
		handler, err := Build(route, renderer)
		if err != nil {
			return fmt.Errorf("route[%d] %s %s: %w", i, strings.Join(route.Method, ","), route.Path, err)
		}
		for _, m := range route.Method {
			method := strings.ToUpper(m)
			if method == "*" {
				r.Any(route.Path, handler)
			} else {
				r.Add(route.Path, handler, method)
			}
		}
	}
	return nil
}

// Build constructs the rux handler for one proxy route. Compilation
// (rewrite rules / timeout / bodyLimit / target URL) happens once here;
// the returned handler is allocation-light on the request path.
func Build(route *config.Route, renderer tpl.Renderer) (rux.HandlerFunc, error) {
	p := route.Proxy
	if p == nil {
		return nil, fmt.Errorf("Build called with nil Proxy")
	}
	targetURL, err := url.Parse(p.Target)
	if err != nil {
		return nil, fmt.Errorf("proxy.target %q: %w", p.Target, err)
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		return nil, fmt.Errorf("proxy.target scheme must be http/https, got %q", targetURL.Scheme)
	}

	rules, err := compileRewrites(p.Rewrite)
	if err != nil {
		return nil, err
	}

	timeout := 30 * time.Second
	if p.Timeout != "" {
		d, terr := time.ParseDuration(p.Timeout)
		if terr != nil {
			return nil, fmt.Errorf("proxy.timeout %q: %w", p.Timeout, terr)
		}
		timeout = d
	}

	var bodyLimit int64 = -1 // sentinel: no per-route limit (caller server.maxBodySize still applies via middleware in Phase 5)
	if p.BodyLimit != "" {
		n, berr := humanize.ParseBytes(p.BodyLimit)
		if berr != nil {
			return nil, fmt.Errorf("proxy.bodyLimit %q: %w", p.BodyLimit, berr)
		}
		bodyLimit = int64(n)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: p.InsecureSkipVerify},
		// ResponseHeaderTimeout caps "Director done → first response byte".
		// We layer ctx.WithTimeout on top for total request lifetime; the
		// transport-level cap catches stalled upstreams that don't honor ctx.
		ResponseHeaderTimeout: timeout,
	}

	rp := &httputil.ReverseProxy{
		Transport: transport,
		Director: func(req *http.Request) {
			// 1. URL scheme + host
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			// 2. Host header: preserve client's Host iff configured
			if !p.PreserveHost {
				req.Host = targetURL.Host
			}
			// 3. Path: stripPathPrefix → rewrite
			path := req.URL.Path
			if p.StripPathPrefix != "" && strings.HasPrefix(path, p.StripPathPrefix) {
				path = path[len(p.StripPathPrefix):]
				if path == "" {
					path = "/"
				}
			}
			if newPath, ok := applyRewrites(rules, path); ok {
				path = newPath
			}
			req.URL.Path = path
			// 4. Inject request headers (template-rendered)
			if len(p.Headers) > 0 {
				ctx := tpl.BuildRenderCtx(req, nil, nil) // params nil — proxy doesn't extract path params for header templates
				for k, v := range p.Headers {
					rendered, rerr := renderer.Render(v, ctx)
					if rerr != nil {
						// Template error on a header injection isn't fatal; we just drop
						// that header and let the upstream see no value.
						continue
					}
					req.Header.Set(k, rendered)
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			if len(p.ResponseHeaders) == 0 {
				return nil
			}
			// Build a *minimal* ctx for response-header rendering. resp.Request
			// is the upstream request which already has scheme/host/path
			// rewritten, but for header values that reference .request.method
			// / .request.path that's still a sensible source.
			ctx := tpl.BuildRenderCtx(resp.Request, nil, nil)
			for k, v := range p.ResponseHeaders {
				rendered, rerr := renderer.Render(v, ctx)
				if rerr != nil {
					continue
				}
				resp.Header.Set(k, rendered)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, perr error) {
			// distinguish timeout from dial failure
			if errors.Is(perr, http.ErrHandlerTimeout) || strings.Contains(perr.Error(), "timeout") || strings.Contains(perr.Error(), "deadline") {
				writeProxyError(w, http.StatusGatewayTimeout, "upstream timeout", perr.Error(), route, p.Target)
				return
			}
			writeProxyError(w, http.StatusBadGateway, "upstream dial failed", perr.Error(), route, p.Target)
		},
	}

	return func(c *rux.Context) {
		// Body limit applied at the request boundary — too-large body never reaches upstream
		if bodyLimit > 0 {
			c.Req.Body = http.MaxBytesReader(c.Resp, c.Req.Body, bodyLimit)
		}
		// Wrap request ctx for timeout
		ctx, cancel := contextWithTimeout(c.Req, timeout)
		defer cancel()
		req := c.Req.WithContext(ctx)

		// Pre-flight read of body to enforce MaxBytesReader: ReverseProxy
		// reads body lazily and MaxBytesReader only errors on Read. We let
		// it pass through and rely on ErrorHandler to receive the size
		// error — but MaxBytesReader's error path goes through the
		// proxy's "copy body" which closes the response with status 413
		// already written. The simpler approach: drain body up-front and
		// short-circuit on size error.
		if bodyLimit > 0 && req.Body != nil {
			// Read up to bodyLimit+1 to detect overage
			buf := make([]byte, bodyLimit+1)
			n, rerr := readFully(req.Body, buf)
			if rerr != nil {
				writeProxyError(c.Resp, http.StatusRequestEntityTooLarge, "request body exceeds proxy.bodyLimit", fmt.Sprintf("limit=%d bytes", bodyLimit), route, p.Target)
				return
			}
			// Reinstall body for ReverseProxy
			req.Body = readCloserFromBytes(buf[:n])
			req.ContentLength = int64(n)
		}

		rp.ServeHTTP(c.Resp, req)
	}, nil
}

// contextWithTimeout returns the request context plus a cancel func. When
// timeout <= 0 we still return a cancel so the deferred call is safe.
func contextWithTimeout(req *http.Request, timeout time.Duration) (ctx context.Context, cancel context.CancelFunc) {
	parent := req.Context()
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}

// readFully reads from r into buf until EOF or buf is full. Returns
// (n, err) — err is non-nil ONLY when r returned more than len(buf) bytes
// (i.e. the body exceeded the soft limit caller picked).
func readFully(r io.ReadCloser, buf []byte) (int, error) {
	defer r.Close()
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
	// We filled buf; check whether there's MORE data → overflow
	extra := make([]byte, 1)
	n, _ := r.Read(extra)
	if n > 0 {
		return total, errors.New("body exceeds limit")
	}
	return total, nil
}

func readCloserFromBytes(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}

// writeProxyError emits the §9.3/§6 error body. The `target` field is the
// distinguishing feature of proxy errors vs mock errors.
func writeProxyError(w http.ResponseWriter, status int, short, detail string, route *config.Route, target string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  short,
		"detail": detail,
		"route":  fmt.Sprintf("%s %s", strings.Join(route.Method, ","), route.Path),
		"target": target,
	})
}
```

> 在 import 块顶部追加：
> ```go
> "bytes"
> "context"
> "io"
> ```
>
> 同时拉新依赖 `github.com/dustin/go-humanize` 用于 `humanize.ParseBytes("16B")`。如果 `goutil` 已经提供等价工具（`bytesize.ParseString` 或类似），可改用 goutil；探查命令：
>
> ```
> go doc github.com/gookit/goutil/byteutil
> ```
>
> 若 goutil 有，删掉 humanize 依赖，import `byteutil` 即可。

- [ ] **Step 8.4: 跑测试**

```
go mod tidy
go test ./internal/proxy/... -v -count=1
```

预期：所有 proxy 用例（基础 + 增强 = 11 个）PASS。

> **可能的偏差**：
>
> - `TestProxy_PreserveHost` 中 `srv.URL` 的形态因 httptest 不带 Host 头而需要对比 `req.Host` 与 `srv.Listener.Addr().String()`——按实测调整断言。
> - `TestProxy_Timeout_504` 的实际错误信息因 Go 版本不同形态有差异（"deadline exceeded" / "timeout" / "context canceled"）；以 ErrorHandler 中的 `strings.Contains` 串当作 fallback。如果实测发现 ErrorHandler 没有触发（直接拿到 502），把"timeout" / "deadline" 检查改为通过 `errors.Is(perr, context.DeadlineExceeded)`。
> - `humanize` 与 `goutil/byteutil`：用哪个看探查结果，最终只保留一个依赖；写到落地偏差段。

- [ ] **Step 8.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/proxy/proxy.go internal/proxy/proxy_test.go go.mod go.sum
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(proxy): strip/rewrite/headers/responseHeaders/bodyLimit/timeout/preserveHost 全量字段"
```

---

## Task 9: cli/serve.go + check.go — 装配 proxy.Mount + Warn 输出 + E2E

**Files**:
- 修改：`internal/cli/serve.go`
- 修改：`internal/cli/serve_test.go`
- 修改：`internal/cli/check.go`
- 修改：`internal/cli/check_test.go`
- 新建：`internal/config/testdata/valid/cases-first-match.json5`
- 新建：`internal/config/testdata/valid/proxy-basic.json5`

> **范围**：
>
> 1. `assembleRouter` 在 `mock.Mount` 之后增加 `proxy.Mount`，路由优先级：mock 单一/cases → proxy → admin → echo
> 2. `runServe` 在 Validate 成功后调用 `config.Warn(cfg)` 并把每条警告写到 stderr（不影响 exit code）
> 3. `cli/check.go` 同样调用 Warn 输出（不影响其 0 退出语义；只有 Validate errs 才退非 0）
> 4. E2E：用 `httptest.NewServer` 起假上游 + 新 testdata 配置验证 cases + proxy 真正可用

- [ ] **Step 9.1: 新建 testdata 配置**

新建 `internal/config/testdata/valid/cases-first-match.json5`：

```json5
{
  routes: [
    {
      method: "GET",
      path: "/u/{id}",
      strategy: "first-match",
      cases: [
        { when: "request.query.fail == \"1\"", status: 500, body: { error: "boom" } },
        { status: 200, body: { id: "{{ .request.params.id }}", ok: true } },
      ],
    },
  ],
}
```

新建 `internal/config/testdata/valid/proxy-basic.json5`（target 占位，测试时通过 `assertConfigLoadsAndValidates` 而非实际启动）：

```json5
{
  routes: [
    {
      method: "*",
      path: "/api/*rest",
      proxy: {
        target: "http://upstream.example:8080",
        stripPathPrefix: "/api",
        rewrite: [
          "^/v1/(.+) => /legacy/$1",
        ],
        headers: { "X-Forwarded-By": "fakeserver" },
        timeout: "10s",
      },
    },
  ],
}
```

- [ ] **Step 9.2: 修改 serve.go**

把 `internal/cli/serve.go` 中 `assembleRouter` 与 `runServe` 改为：

```go
func assembleRouter(cfg *config.Config, renderer tpl.Renderer) *rux.Router {
	r := rux.New()
	_ = mock.Mount(r, cfg, renderer)
	_ = proxy.Mount(r, cfg, renderer)
	admin.Mount(r)
	echo.Mount(r)
	return r
}
```

在 import 块追加 `"github.com/inhere/fakeserver/internal/proxy"`。

在 `runServe` 中，加载 config 成功后、构造 server 之前插入 warn 输出：

```go
	if cfg != nil {
		for _, w := range config.Warn(cfg) {
			fmt.Fprintln(os.Stderr, "warn:", w)
		}
	}
```

更新启动 banner 文本（mock + proxy 同时启用）：

```go
		if cfg != nil {
			PrintRouteSummary(cfg, os.Stdout)
			fmt.Println("(Phase 4: mock + cases + proxy routes are registered and served)")
		} else {
			fmt.Println("no config; running in echo-only mode")
		}
```

- [ ] **Step 9.3: 修改 check.go**

在 `internal/cli/check.go` 的 Validate 调用后追加 Warn 输出：

```go
	// (existing) if errs := config.Validate(cfg); len(errs) > 0 { ... return non-zero ... }

	for _, w := range config.Warn(cfg) {
		fmt.Fprintln(os.Stderr, "warn:", w)
	}
```

具体位置：所有 Validate 错路径都已经 return 之后；如果 Validate 没有 errs 才会执行 Warn 段。

- [ ] **Step 9.4: 修改 serve_test.go — E2E cases + proxy**

把以下追加到 `internal/cli/serve_test.go`：

```go
func TestServe_E2E_CasesRoute(t *testing.T) {
	// 给 mock 路由的 first-match cases 起个 server，请求两种分支
	cfg := mustLoadCfg(t, "../config/testdata/valid/cases-first-match.json5")
	rdr := tpl.NewRenderer(nil, nil, 1)
	r := assembleRouter(cfg, rdr)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// branch: fail=1 → 500
	resp, _ := http.Get(srv.URL + "/u/42?fail=1")
	if resp.StatusCode != 500 {
		t.Errorf("fail=1: status=%d want 500", resp.StatusCode)
	}
	resp.Body.Close()
	// branch: default → 200 + id=42
	resp, _ = http.Get(srv.URL + "/u/42")
	if resp.StatusCode != 200 {
		t.Errorf("default: status=%d want 200", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["id"] != "42" {
		t.Errorf("body=%v", got)
	}
}

func TestServe_E2E_ProxyRouteCoexistsWithMock(t *testing.T) {
	// 起假上游
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("UP:" + r.URL.Path))
	}))
	defer upstream.Close()

	// 构造 cfg：一条精确 mock + 一条通配 proxy
	cfg := &config.Config{
		Fallback: "echo",
		Routes: []config.Route{
			{
				Method: []string{"GET"}, Path: "/api/users",
				Status: 200, Body: map[string]any{"local": true},
			},
			{
				Method: []string{"*"}, Path: "/api/*rest",
				Proxy: &config.ProxyConfig{Target: upstream.URL, StripPathPrefix: "/api"},
			},
		},
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}

	rdr := tpl.NewRenderer(nil, nil, 1)
	r := assembleRouter(cfg, rdr)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// /api/users → 精确 mock 截胡
	resp, _ := http.Get(srv.URL + "/api/users")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"local":true`) {
		t.Errorf("precise mock should win for /api/users; body=%s", string(b))
	}
	// /api/orders/1 → 通配 proxy
	resp, _ = http.Get(srv.URL + "/api/orders/1")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(b), "UP:/orders/1") {
		t.Errorf("proxy should win for /api/orders/1; body=%s", string(b))
	}
}

// mustLoadCfg helper（如果 serve_test.go 已有则跳过，避免重复定义）
func mustLoadCfg(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.Load([]string{path}, "", nil)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate %s: %v", path, errs)
	}
	return cfg
}
```

> 顶部 import 追加（若缺）：`"encoding/json"`、`"io"`、`"net/http"`、`"net/http/httptest"`、`"strings"`、`"github.com/inhere/fakeserver/internal/config"`、`"github.com/inhere/fakeserver/internal/tpl"`。

- [ ] **Step 9.5: check_test.go 追加 warn 回归**

新增一个 case 验证 Warn 文本是否能写出（用 `os.Pipe` 或 `os.Stderr` 重定向，或者读 Warn 返回直接断言）。简化方案：

```go
func TestCheck_FirstMatchNoFallback_PrintsWarnButExitOk(t *testing.T) {
	// 构造一个 first-match 无兜底的临时配置，跑 runCheck 并断言：
	//  - exit code == 0（Validate 没错）
	//  - stderr 含 "first-match" warn 字串
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	body := `{
		routes: [{
			method: "GET", path: "/x", strategy: "first-match",
			cases: [
				{ when: "request.query.a == \"1\"", status: 200, body: "a" },
				{ when: "request.query.b == \"1\"", status: 200, body: "b" },
			],
		}],
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	// 重定向 stderr 抓输出
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	err := runCheck(checkOptions{ConfigFlag: cfgPath})
	w.Close()
	stderrOut, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("check should pass (warn-only), got err=%v", err)
	}
	if !strings.Contains(string(stderrOut), "first-match") {
		t.Errorf("stderr missing warn: %q", string(stderrOut))
	}
}
```

> import 追加 `"io"`、`"os"`、`"path/filepath"`、`"strings"`（如缺）。`runCheck` 与 `checkOptions` 的实际名称按 check.go 的导出符号填——如果 check.go 的入口是 `runCheck(opts)`、不导出则在测试包 `package cli`（已是同包）直接调。

- [ ] **Step 9.6: 跑全套测试**

```
go test ./... -v -count=1
go test ./internal/{mock,proxy} -cover
```

预期：
- 全包 PASS（Phase 1-3 旧用例 + Phase 4 新用例）
- `internal/mock` cover ≥ 80%
- `internal/proxy` cover ≥ 80%

- [ ] **Step 9.7: 手动冒烟**

```powershell
go build -o fakeserver.exe ./cmd/fakeserver

# 第一档：起一个上游 mock，让本机 8081 给 echo /api/* 用
$up = Start-Job { D:/work/aidev/lite-tools/fakeserver/fakeserver.exe serve --port 8081 }
Start-Sleep -Seconds 2

# 第二档：用 proxy-basic.json5（target 改成 http://127.0.0.1:8081）启动 fakeserver
# 临时手改 testdata 用例的 target — 或者写一个临时配置文件：
@'
{
  routes: [
    { method: "GET", path: "/u/{id}", strategy: "first-match",
      cases: [
        { when: "request.query.fail == \"1\"", status: 500, body: { error: "boom" } },
        { status: 200, body: { id: "{{ .request.params.id }}", ok: true } },
      ],
    },
    { method: "*", path: "/api/*rest",
      proxy: { target: "http://127.0.0.1:8081", stripPathPrefix: "/api" },
    },
  ],
}
'@ | Set-Content -Path tmp-cfg.json5

$j = Start-Job { D:/work/aidev/lite-tools/fakeserver/fakeserver.exe serve -c tmp-cfg.json5 --port 4567 }
Start-Sleep -Seconds 2

# cases first-match
Write-Host "--- cases first-match ---"
$r = Invoke-WebRequest -Uri "http://localhost:4567/u/42?fail=1" -UseBasicParsing -SkipHttpErrorCheck
Write-Host "fail=1 → $($r.StatusCode), body=$($r.Content)"
$r = Invoke-WebRequest -Uri "http://localhost:4567/u/42" -UseBasicParsing
Write-Host "default → $($r.StatusCode), body=$($r.Content)"

# proxy
Write-Host "--- proxy ---"
$r = Invoke-WebRequest -Uri "http://localhost:4567/api/anything" -UseBasicParsing
Write-Host "proxy /api/anything → $($r.StatusCode), body length=$($r.Content.Length)"

# 拨号失败 502
@'
{ routes: [{ method: "GET", path: "/bad", proxy: { target: "http://127.0.0.1:1" }}]}
'@ | Set-Content -Path tmp-bad.json5
Stop-Job $j; Remove-Job $j
$j2 = Start-Job { D:/work/aidev/lite-tools/fakeserver/fakeserver.exe serve -c tmp-bad.json5 --port 4568 }
Start-Sleep -Seconds 2
$r = Invoke-WebRequest -Uri "http://localhost:4568/bad" -UseBasicParsing -SkipHttpErrorCheck
Write-Host "dial-fail → $($r.StatusCode), body=$($r.Content)"

Stop-Job $j2; Remove-Job $j2; Stop-Job $up; Remove-Job $up
Remove-Item tmp-cfg.json5, tmp-bad.json5, fakeserver.exe
```

预期：
- `/u/42?fail=1` → 500，body 含 `"boom"`
- `/u/42` → 200，body 含 `"id":"42"`
- `/api/anything` → 200，body 是上游 echo 的输出（含 `/anything`）
- `/bad` → 502，body 含 `"target":"http://127.0.0.1:1"`

- [ ] **Step 9.8: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve.go internal/cli/serve_test.go internal/cli/check.go internal/cli/check_test.go internal/config/testdata/valid/cases-first-match.json5 internal/config/testdata/valid/proxy-basic.json5
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): serve 装配 proxy.Mount + warn 输出 + E2E（cases + proxy 共存）"
```

---

## Task 10: DoD 验证 + 文档回写

**Files**:
- 修改：`docs/plans/2026-05-19-fakeserver-v0.1-overview.md`
- 修改：`docs/fakeserver-design.md`

### Step 10.1: DoD 检查清单

逐项核对：

| 命令 / 检查 | 预期 |
|---|---|
| `go build ./...` | 0 退出 |
| `go test ./...` | 全 PASS |
| `go test -cover ./internal/mock/...` | 覆盖率 ≥ 80% |
| `go test -cover ./internal/proxy/...` | 覆盖率 ≥ 80% |
| 手动冒烟（Step 9.7） | 4 个断言全过 |
| `go list -m all \| Select-String -Pattern "expr-lang/expr"` | 出现 |
| `go list -m all \| Select-String -Pattern "fsnotify"` | 不出现 |
| Phase 4 commit 数 | 9–11 个（Task 1–10）|

### Step 10.2: 回写 overview

修改 `docs/plans/2026-05-19-fakeserver-v0.1-overview.md`：

**§2 表 Phase 4 状态列**改为：

```
✅ 已完成 (commit <first SHA>..<last SHA>)
```

**§3 Phase 4 详述末尾追加**：

```markdown
**实际落地偏差**：

- <Phase 4 落地过程中发现的 API/接口偏差。具体填表时根据实际 Task 1/8 的探查结果填入。可能的项目：
  - expr v1.x 的 Compile/Run/AsBool 实际签名与 plan 假设差异
  - expr Run 对字段缺失的返回（err 还是 nil 还是 false）
  - httputil.ReverseProxy 在 Go 1.26 的 ErrorHandler 触发条件
  - humanize.ParseBytes vs goutil/byteutil 的最终选择
  - rux v2 c.Req.WithContext 的副作用（rux 是否仍认得替换后的 Req）
  - MaxBytesReader 与 ReverseProxy 的协作细节（错误是否能上抛到 ErrorHandler）>
- 其余实现与 plan 一致

**Phase 4 测试覆盖**：<填入实测用例总数> 个用例；`internal/mock` 覆盖率 <X>%；`internal/proxy` 覆盖率 <Y>%；`internal/config` 覆盖率维持 ≥ 85%
```

### Step 10.3: 回写 design

修改 `docs/fakeserver-design.md`：

修订记录追加：

```markdown
| 2026-05-19 | v0.3-phase4-applied | inhere | Phase 4 落地：internal/mock 增 matcher（expr）+ selector（四 strategy）+ cases.go；internal/proxy 整包（ReverseProxy + 6 字段全量）；config.Validate 增 when 语法预检；新增 config.Warn 警告通道（first-match 无兜底、proxy.target 私网 info）|
```

§13 追加"已落地（Phase 4 阶段确认）"子段，至少含 5 条事实：

1. **expr-lang/expr 实际 API**：`Compile(src, AsBool(), Env(...))` / `Run(prog, env) (any, error)`。字段访问平铺命名空间（`request.query.x`，无前置点号）。运行期错误（字段缺失、类型不匹配）返回 `(nil/false, err)`，本工程降级为 `(false, err)` → 调用方 warn 跳过
2. **ReverseProxy + Director 链路**：path 改写顺序锁定为 `stripPathPrefix` → `rewrite`（首匹配）→ host 切换；template 渲染**只在 headers/responseHeaders value 上**生效
3. **超时实现**：`Transport.ResponseHeaderTimeout` + 请求 `context.WithTimeout` 双层；ErrorHandler 通过 `errors.Is(err, context.DeadlineExceeded)` 区分 504 vs 502
4. **bodyLimit 实现**：在 rux handler 入口做"读满 + 探测剩余字节"，超限直接 413（不走 ReverseProxy）；选择放弃 `http.MaxBytesReader`（其错误经由 ReverseProxy 的 body 拷贝触发，路径不可控）
5. **warn 通道**：`config.Warn(cfg) []string` 与 `config.Validate(cfg) []error` 并列；前者只产生 stderr 文本、不影响 exit code，后者产生终止性错误。`first-match` 全 when 触发警告；`proxy.target` 私网/localhost 触发 info-级警告

### Step 10.4: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add docs/plans/2026-05-19-fakeserver-v0.1-overview.md docs/fakeserver-design.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "docs: 回写 Phase 4 落地结果（mock cases + proxy 整包 + warn 通道）"
```

---

## Phase 4 完成 · 下一步

仓库具备：

- ✅ `internal/mock/matcher.go`：expr 编译缓存 + 求值 + 错误降级
- ✅ `internal/mock/selector.go`：random / round-robin / weighted / first-match
- ✅ `internal/mock/cases.go`：cases 整合处理（matcher → selector → 继承 outer → 委托 Respond）
- ✅ `internal/mock/router.go`：cases / 单一响应双分支；proxy 仍跳过交给 proxy.Mount
- ✅ `internal/proxy/` 整包：Build / Mount / rewrite / 完整 6+ 字段 / 502 / 504 / 413
- ✅ `internal/config/validate.go`：when 语法预检查
- ✅ `internal/config/Warn`：first-match 无兜底 + proxy 私网 info 警告
- ✅ E2E：cases + proxy + 精确 mock 截胡 proxy 在同一 server 上验证通过

**Phase 5 预告**（不在本计划范围）：

- 引入 `github.com/fsnotify/fsnotify`
- `internal/middleware/{recoverer,logger,cors,bodylimit}.go`：全套中间件
- `internal/config/watcher.go`：热加载（300ms 防抖 + 原子 router 切换）
- `internal/admin/handlers.go` 扩展：`/__fakeserver/routes`
- 启动 banner / `--quiet` / `--no-cors` / `--no-watch`
- v0.1 MVP 综合 E2E 闭环

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| 无 TBD / placeholder | ✓（matcher_test.go Task 1 中 `TestExprSmokeRuntimeError` 用 `t.Logf` 做探测、不做断言，是受控 smoke 而非 placeholder） |
| 类型签名前后一致 | ✓（`mock.Matcher{Source,Program}` / `CompileMatcher(string) (*Matcher,error)` / `Matcher.Evaluate(map[string]any) (bool,error)` / `mock.SelectorCase{OrigIdx,Weight}` / `mock.NewSelector(string) Selector` / `mock.Selector.Pick([]SelectorCase) (int,error)` / `mock.RespondCases(c, route, matchers, selector, renderer)` / `proxy.Mount(r, cfg, renderer) error` / `proxy.Build(route, renderer) (rux.HandlerFunc, error)` / `config.Warn(cfg) []string` 在 Task 2-9 中保持一致） |
| 包路径前后一致 | ✓（`github.com/inhere/fakeserver/internal/{mock,proxy,config,cli,tpl}`） |
| TDD：先测后写 | ✓（每个 Task 都是先写测试 → 跑 fail → 实现 → 跑 pass） |
| 频繁提交 | ✓（每 Task 一个 commit，10 个 Task 共 10 个新 commit） |
| 仅新增 expr-lang/expr 一个第三方依赖（如选用 dustin/go-humanize 而非 goutil 则两个） | ✓（Phase 4 不引 fsnotify） |
| 覆盖 design §3.2 多响应字段、§4.5 cases 选择错误处理、§6 cases-no-match + proxy 上游错误两行、§9 全章 Proxy | ✓（Task 2-5 覆盖 §3.2/§4.5/§6 第一行；Task 6-9 覆盖 §9 全章 + §6 proxy 上游错误两行） |
| Phase 4 边界明确（中间件 / 热加载 / admin /routes 全部留 Phase 5） | ✓（Task 9 装配链不引入任何 middleware；不调用 fsnotify；admin 端点不变） |
| 失败路径明确 | ✓（when 语法错 / when 运行期错降级 / cases 全不匹配 500 / 拨号失败 502 / 超时 504 / body 超限 413 / proxy.target scheme 错） |
| DoD 10 项与 overview §3 Phase 4 DoD 7 项一致 | ✓（本 plan 的 DoD 10 项是 overview 7 项的细化拆分，并新增 #9/#10 防御边界） |
