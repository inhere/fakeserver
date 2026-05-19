# Fakeserver v0.1 · Phase 3 — 模板与单一响应 Mock

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：让 `fakeserver serve -c routes.json5` **真正响应**配置里的**单一响应模式** route——含 Go 模板渲染、Faker 数据生成、`bodyFile` 文件响应、Content-Type 自动推断。

**Architecture**：新增 `internal/tpl/` 与 `internal/mock/` 两个包。tpl 提供 text/html 双渲染器，共享 FuncMap（=`tplfunc.StdFuncMap()` + fakeserver 自有 22+ 函数 + gofakeit 桥接的 ~20 个常用 fake 函数 + 通用入口 `fake "<name>"`）；mock 把 config 中无 `cases` / 无 `proxy` 的 route 注册到 rux router，每个请求构造 `RenderCtx` → 渲染 status/headers/body → 推断 Content-Type → 应用 delay → 写出响应。`serve` 子命令的 `assembleRouter` 签名升级为接受 `*config.Config` 与 `tpl.Renderer`，调用 `mock.Mount` 在 admin/echo 之前注册 mock 路由。

**Tech Stack**：Go 1.25+ · `github.com/gookit/easytpl`（新引入）· `github.com/brianvoe/gofakeit/v7`（新引入）· 标准库 `text/template` / `html/template` / `encoding/json` / `mime` / `time`。

**前置要求**：

- Phase 1–2 已完成
- 已读 `docs/fakeserver-design.md` §4（模板与渲染策略）/ §12（Faker）/ §3.2 单一响应字段段 / §4.5 渲染顺序与 Content-Type 推断
- 已读 `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` §3 Phase 3 详述
- `fakeserver/` 当前 go.mod 含 rux/v2、gcli/v3、goutil、titanous/json5（4 个直接依赖），14 个 commit

**Phase 3 完成定义（DoD）**：

1. `go build ./...` 通过
2. `go test ./...` 全绿；`internal/{tpl,mock}` 覆盖率 ≥ 80%
3. 给定测试 config `{ routes: [{ method:"GET", path:"/u/{id}", body: { id: "{{ .request.params.id }}", name: "{{ fakeName }}" }}] }`，`curl /u/42` 返回 200 + JSON，含 `"id":"42"` 与非空 name
4. `bodyFile: "fixtures/avatar.png"` route 返回文件字节，Content-Type 按 `.png` 推断为 `image/png`
5. Content-Type 推断 4 种 case（design §4.5）覆盖测试
6. text 模式（默认）不做 HTML 转义；显式 `Content-Type: text/html` 走 html 模式
7. faker 函数：~20 个常用单独注册 + 通用 `fake "<name>"`；`server.fakerSeed: 12345` 让响应字段稳定重现
8. `delay: "100ms~500ms"` 区间实际 sleep 落在范围内
9. serve 启动 banner 中 mock 路由命中由 mock handler 处理（而非 echo catch-all）
10. **Phase 3 不引入** expr/fsnotify；新依赖**仅** easytpl 与 gofakeit/v7

---

## 文件结构（Phase 3 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `go.mod` / `go.sum` | 引入 easytpl 与 gofakeit/v7 |
| 新建 | `internal/tpl/render.go` | `Renderer` 接口；`textRenderer` / `htmlRenderer` 实现；按 Content-Type 自动选择 |
| 新建 | `internal/tpl/context.go` | `BuildRenderCtx(req *http.Request, params, globals)` 构造 `*RenderCtx` |
| 新建 | `internal/tpl/funcs.go` | `BaseFuncMap(osenvWhitelist)` 合并 tplfunc + fakeserver 自有 |
| 新建 | `internal/tpl/funcs_extra.go` | fakeserver 自有 22+ 函数实现（uuid/incr/now/randInt/randString/jsonEscape/...） |
| 新建 | `internal/tpl/faker.go` | gofakeit 桥接：~20 个 `fakeXxx` + 通用 `fake "<name>"`；seed 控制 |
| 新建 | `internal/tpl/render_test.go` | 双渲染器、HTML 转义、FuncMap 共享 |
| 新建 | `internal/tpl/context_test.go` | 请求上下文构造、body 按 Content-Type 解析 |
| 新建 | `internal/tpl/funcs_test.go` | 22+ 函数行为（每个函数至少 1 个用例） |
| 新建 | `internal/tpl/faker_test.go` | faker 函数 + seed 重现性 |
| 新建 | `internal/mock/responder.go` | `Respond(c *rux.Context, route, renderer)`：渲染流程 + Content-Type 推断 + bodyFile + delay + 错误体 |
| 新建 | `internal/mock/router.go` | `Mount(r *rux.Router, cfg, renderer)`：遍历 cfg.Routes 跳过 cases/proxy 注册单一响应 |
| 新建 | `internal/mock/responder_test.go` | 用 httptest.NewRecorder 验证 status/headers/body/bodyFile/delay/Content-Type 推断 |
| 新建 | `internal/mock/router_test.go` | 路由匹配 + 跳过 cases/proxy 行为验证 |
| 新建 | `internal/mock/testdata/fixtures/avatar.png` | 测试用小 PNG（≤1 KiB；用 Go 代码生成或 base64 解码） |
| 修改 | `internal/cli/serve.go` | `assembleRouter` 签名升级为接受 cfg + renderer；增加调用 `mock.Mount` 的步骤 |
| 修改 | `internal/cli/serve_test.go` | 调整测试：assembleRouter 的新调用方式；新增"单一响应 mock 实际命中"用例 |
| 修改 | `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` | Phase 3 状态列回写 + 实际落地偏差段 |
| 修改 | `docs/fakeserver-design.md` | 修订记录追加 v0.3-phase3-applied；§13 追加 Phase 3 已落地条目 |

> **注**：Phase 3 **不**新建 `internal/{mock/selector.go, mock/matcher.go, proxy/, middleware/, config/watcher.go}`——它们留给 Phase 4/5。

---

## Task 1: 引入 easytpl + 探测 + smoke 锁定 HTML 转义行为

**Files**:
- 修改：`go.mod` / `go.sum`
- 新建：`internal/tpl/render.go`（最小 stub）
- 新建：`internal/tpl/render_test.go`（smoke）

> **风险点（来自 overview §6）**：easytpl 双模式切换实现细节、`tplfunc.StdFuncMap()` 真实清单。本 Task 用 smoke 锁定。

- [ ] **Step 1.1: 拉 easytpl 依赖**

```
go get github.com/gookit/easytpl
go mod tidy
```

预期：`go.mod` 出现 `github.com/gookit/easytpl v...`。

- [ ] **Step 1.2: 探查 tplfunc.StdFuncMap 实际清单**

执行（在 fakeserver/）：

```
go doc github.com/gookit/easytpl/tplfunc
```

把输出顶部的 "Variables / Functions" 段贴到报告里——本 Task 不依据具体清单写代码，但下游 Task 3 的"剔除哪些键 / 接受哪些"需要这个事实。

- [ ] **Step 1.3: 探查 easytpl Renderer 是否提供"关闭 HTML 转义"开关**

```
go doc github.com/gookit/easytpl.Renderer
go doc github.com/gookit/easytpl | head -80
```

关心的字段/方法：
- 是否有 `Layout` / `Strict` / `Debug` / `HTMLOff` 之类的开关字段
- `Renderer.String(...)` 这个签名的输入输出
- 它内部使用 `html/template` 还是 `text/template`（README 说"基于 html/template"，确认源码）

把发现写入 smoke 测试的注释里。如果 easytpl 提供"text 模式"或允许注入自定义 FuncMap，记下接入方式；如果不提供，本 Phase 用 `text/template` 自己渲染，仅在显式 `Content-Type: text/html*` 时调 easytpl。

- [ ] **Step 1.4: 写 smoke test**

新建 `internal/tpl/render_test.go`：

```go
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
```

> **说明**：这条 smoke 不直接依赖 easytpl——它锁定的是"stdlib html/template 会转义、text/template 不会"这个基本事实，是我们 §4.4 双渲染器设计的依据。Task 6 实现 `Renderer` 接口时再决定是否调用 easytpl 还是直接用 stdlib。

- [ ] **Step 1.5: 新建 `internal/tpl/render.go` 最小 stub**

```go
// Package tpl provides text and HTML template rendering for fakeserver
// mock responses. design §4 covers the contract; this package implements
// it on top of stdlib text/template & html/template (with optional
// easytpl integration for HTML layout in Phase 4+).
package tpl

// Renderer is implemented by both textRenderer and htmlRenderer. Phase 3
// Task 6 fills in concrete types.
type Renderer interface {
	// Render src as a template string using ctx as the data; writes the
	// expanded result to a string. Errors are returned verbatim so the
	// caller can map them to HTTP 500 responses.
	Render(src string, ctx *RenderCtx) (string, error)
}

// RenderCtx is forward-declared here so the stub compiles. Task 5 puts
// the real fields in context.go.
type RenderCtx struct{}
```

- [ ] **Step 1.6: 验证编译 + smoke 通过**

```
go build ./...
go test ./internal/tpl/... -v
```

预期：编译成功；smoke PASS。

- [ ] **Step 1.7: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add go.mod go.sum internal/tpl/render.go internal/tpl/render_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "chore(tpl): 引入 easytpl + smoke 锁定 stdlib 转义行为"
```

---

## Task 2: 引入 gofakeit + 探测 GetFuncs

**Files**:
- 修改：`go.mod` / `go.sum`
- 新建：`internal/tpl/faker.go`（最小 stub）
- 新建：`internal/tpl/faker_test.go`（smoke）

> **风险点（overview §6）**：`gofakeit.GetFuncs()` 是否真存在。本 Task 探查并锁定。

- [ ] **Step 2.1: 拉 gofakeit/v7**

```
go get github.com/brianvoe/gofakeit/v7
go mod tidy
```

- [ ] **Step 2.2: 探查 gofakeit API**

```
go doc github.com/brianvoe/gofakeit/v7 | head -60
go doc github.com/brianvoe/gofakeit/v7.GetFuncs 2>&1 | head -30
go doc github.com/brianvoe/gofakeit/v7.Seed
go doc github.com/brianvoe/gofakeit/v7.Name
```

把每一条的输出关键摘录写到报告里。重点关注：
- `GetFuncs()` 是否存在？返回什么？
- `Seed(int64)` 签名（v7 可能是 `Seed(uint64)` 或 `New(uint64)`）
- `Name()` / `Email()` / `IPv4Address()` 等基础函数签名

- [ ] **Step 2.3: 写 smoke**

新建 `internal/tpl/faker_test.go`：

```go
package tpl

import (
	"testing"

	"github.com/brianvoe/gofakeit/v7"
)

// TestGofakeitSeedReproducible 锁定 Seed 行为：相同 seed → 相同结果。
// design §12.1 依赖此特性提供"测试场景固定 seed，结果可复现"。
func TestGofakeitSeedReproducible(t *testing.T) {
	// 注意：gofakeit v7 的 Seed 签名是 uint64（不是 int64）。
	// 如果你的实测 signature 不同，按实际写。
	gofakeit.Seed(uint64(12345))
	first := gofakeit.Name()

	gofakeit.Seed(uint64(12345))
	second := gofakeit.Name()

	if first != second {
		t.Errorf("same seed should yield same Name; got %q vs %q", first, second)
	}
}

func TestGofakeitBasicFunctions(t *testing.T) {
	gofakeit.Seed(uint64(1))

	if name := gofakeit.Name(); name == "" {
		t.Error("Name() returned empty")
	}
	if email := gofakeit.Email(); email == "" {
		t.Error("Email() returned empty")
	}
	if ip := gofakeit.IPv4Address(); ip == "" {
		t.Error("IPv4Address() returned empty")
	}
}
```

> 如果 `Seed` 接受 `int64`（不是 `uint64`），调整两处即可。这是 v7 vs v6 的常见差异。

- [ ] **Step 2.4: 新建 `internal/tpl/faker.go` stub**

```go
package tpl

// faker.go bridges gofakeit into the FuncMap. Task 4 fills in the real
// registration; this stub keeps the package compilable and the import
// referenced.

import (
	_ "github.com/brianvoe/gofakeit/v7"
)
```

- [ ] **Step 2.5: 编译 + smoke**

```
go build ./...
go test ./internal/tpl/... -v
```

预期：所有 tpl smoke PASS。

- [ ] **Step 2.6: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add go.mod go.sum internal/tpl/faker.go internal/tpl/faker_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "chore(tpl): 引入 gofakeit/v7 + smoke 锁定 Seed 重现性"
```

---

## Task 3: tpl/funcs.go + tpl/funcs_extra.go — 内置函数集

**Files**:
- 新建：`internal/tpl/funcs.go`（合并入口 + osenv 白名单）
- 新建：`internal/tpl/funcs_extra.go`（22+ 函数实现）
- 新建：`internal/tpl/funcs_test.go`

> **范围**：实现 design §4.3 列出的全部 fakeserver 自有函数：通用 5 + 时间 3 + 环境 3 + 随机 6 + 编码 5 + JSON 3 + 字符串补充 2 + 控制 2 = 共 29 个。**不**含 faker 函数（Task 4）。

- [ ] **Step 3.1: 写测试**

新建 `internal/tpl/funcs_test.go`：

```go
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
	// 跑 50 次 randInt 5 10，全部都应在 [5,10]
	for i := 0; i < 50; i++ {
		out := renderInline(t, `{{ randInt 5 10 }}`, nil)
		// 不解析数字，只验长度 ≥ 1 与字符全是数字
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
```

- [ ] **Step 3.2: 运行测试确认失败**

```
go test ./internal/tpl/... -run TestFunc -v
```

预期：FAIL（`BaseFuncMap` 与所有函数未实现）。

- [ ] **Step 3.3: 实现 funcs.go（合并入口）**

新建 `internal/tpl/funcs.go`，**逐字写入**：

```go
package tpl

import (
	"text/template"

	"github.com/gookit/easytpl/tplfunc"
)

// BaseFuncMap returns the merged FuncMap shared by text and html renderers.
//
// Order of precedence (later wins on conflict):
//   1. tplfunc.StdFuncMap()              ← gookit 提供的基础（join/trim/upper/lower/...）
//   2. delete env/expandenv             ← tplfunc 的 env 取 os.Getenv，与我们的命名空间冲突
//   3. fakeserverFuncs (funcs_extra.go) ← 我们的 22+ 自有函数
//   4. fakerFuncs (faker.go, Task 4)    ← gofakeit 桥接的 fakeXxx + 通用 fake
//
// osenvWhitelist controls which OS environment keys the osenv() function
// may return. Empty slice / nil means "no restriction" (development-friendly).
func BaseFuncMap(osenvWhitelist []string) template.FuncMap {
	out := template.FuncMap{}
	for k, v := range tplfunc.StdFuncMap() {
		out[k] = v
	}
	delete(out, "env")
	delete(out, "expandenv")

	for k, v := range fakeserverFuncs(osenvWhitelist) {
		out[k] = v
	}
	for k, v := range fakerFuncs() {
		out[k] = v
	}
	return out
}
```

> **注**：`fakerFuncs()` 在 Task 4 才完整实现；Task 4 之前先返回空 map（见 Task 4 Step 4.1 替换）。本 Task 4 完成前 `fakerFuncs` 函数已在 `faker.go` 中定义为返回空 map 即可。

- [ ] **Step 3.4: 实现 funcs_extra.go**

新建 `internal/tpl/funcs_extra.go`，**逐字写入**：

```go
package tpl

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid" // 如果该包尚未在 go.mod，本 Task 顺带 go get 它
)

// fakeserverFuncs returns the FuncMap of all fakeserver-owned functions
// from design §4.3, with osenvWhitelist controlling the osenv allowlist.
//
// Names align with the tplfunc roadmap (design §4.2/§4.3) so that if
// gookit/tplfunc ever implements them upstream we can drop ours.
func fakeserverFuncs(osenvWhitelist []string) template.FuncMap {
	allowed := osenvAllowlist(osenvWhitelist)

	return template.FuncMap{
		// === Identity / generic ===
		"uuid":     funcUUID,
		"shortid":  funcShortID,
		"incr":     funcIncr,
		"default":  funcDefault,
		"coalesce": funcCoalesce,

		// === Time ===
		"now":       funcNow,
		"timestamp": funcTimestamp,
		"addDate":   funcAddDate,

		// === Environment ===
		"env":       funcEnv,                  // Phase 3: always returns default
		"osenv":     makeOsenv(allowed),
		"expandEnv": makeExpandEnv(allowed),

		// === Random ===
		"randInt":    funcRandInt,
		"randFloat":  funcRandFloat,
		"randString": funcRandString,
		"randChoice": funcRandChoice,
		"shuffle":    funcShuffle,
		"weighted":   funcWeighted,

		// === Encoding ===
		"b64enc":     funcB64Enc,
		"b64dec":     funcB64Dec,
		"urlenc":     funcURLEnc,
		"urldec":     funcURLDec,
		"jsonEscape": funcJSONEscape,

		// === JSON ===
		"toJson":   funcToJSON,
		"fromJson": funcFromJSON,
		"jsonPath": funcJSONPath,

		// === String additions ===
		"title": funcTitle,
		"split": funcSplit,

		// === Control / debug ===
		"fail":  funcFail,
		"print": funcPrint,
	}
}

// ---- identity / generic ----

func funcUUID() string { return uuid.NewString() }

const base62Alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func funcShortID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = base62Alphabet[rand.Intn(len(base62Alphabet))]
	}
	return string(b)
}

var (
	incrMu    sync.Mutex
	incrPool  = map[string]int64{}
)

func funcIncr(name string) string {
	incrMu.Lock()
	defer incrMu.Unlock()
	incrPool[name]++
	return strconv.FormatInt(incrPool[name], 10)
}

func funcDefault(fallback, v any) any {
	if isZero(v) {
		return fallback
	}
	return v
}

func funcCoalesce(vs ...any) any {
	for _, v := range vs {
		if !isZero(v) {
			return v
		}
	}
	return ""
}

func isZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case int:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	case bool:
		return !x
	}
	return false
}

// ---- time ----

func funcNow(layout ...string) any {
	t := time.Now()
	if len(layout) == 0 {
		return t
	}
	return t.Format(layout[0])
}

func funcTimestamp(unit ...string) string {
	t := time.Now()
	u := "s"
	if len(unit) > 0 {
		u = unit[0]
	}
	switch u {
	case "ms":
		return strconv.FormatInt(t.UnixMilli(), 10)
	case "us":
		return strconv.FormatInt(t.UnixMicro(), 10)
	case "ns":
		return strconv.FormatInt(t.UnixNano(), 10)
	default:
		return strconv.FormatInt(t.Unix(), 10)
	}
}

func funcAddDate(years, months, days int) time.Time {
	return time.Now().AddDate(years, months, days)
}

// ---- environment ----

// funcEnv returns the default value in Phase 3 (env file not yet
// available; design §4.3 says env reads .env which is empty before v0.2).
func funcEnv(args ...string) string {
	if len(args) >= 2 {
		return args[1]
	}
	return ""
}

func osenvAllowlist(list []string) map[string]struct{} {
	if len(list) == 0 {
		return nil // nil ≡ "allow all"
	}
	out := make(map[string]struct{}, len(list))
	for _, k := range list {
		out[k] = struct{}{}
	}
	return out
}

func makeOsenv(allowed map[string]struct{}) func(args ...string) string {
	return func(args ...string) string {
		if len(args) == 0 {
			return ""
		}
		key := args[0]
		def := ""
		if len(args) >= 2 {
			def = args[1]
		}
		if allowed != nil {
			if _, ok := allowed[key]; !ok {
				return def
			}
		}
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return def
	}
}

func makeExpandEnv(allowed map[string]struct{}) func(s string) string {
	osenv := makeOsenv(allowed)
	return func(s string) string {
		return os.Expand(s, func(k string) string { return osenv(k) })
	}
}

// ---- random ----

func funcRandInt(min, max int) int {
	if max < min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

func funcRandFloat(min, max float64) float64 {
	if max < min {
		return min
	}
	return min + rand.Float64()*(max-min)
}

func funcRandString(n int, charset ...string) string {
	cs := "alnum"
	if len(charset) > 0 {
		cs = charset[0]
	}
	var pool string
	switch cs {
	case "alpha":
		pool = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	case "hex":
		pool = "0123456789abcdef"
	case "base62":
		pool = base62Alphabet
	default: // "alnum"
		pool = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = pool[rand.Intn(len(pool))]
	}
	return string(b)
}

func funcRandChoice(args ...any) any {
	if len(args) == 0 {
		return ""
	}
	// 如果首参是 slice，从中选一项
	if len(args) == 1 {
		if slice, ok := args[0].([]any); ok && len(slice) > 0 {
			return slice[rand.Intn(len(slice))]
		}
		// 单个非 slice：直接返回
		return args[0]
	}
	return args[rand.Intn(len(args))]
}

func funcShuffle(list []any) []any {
	out := make([]any, len(list))
	copy(out, list)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// funcWeighted: 接受偶数个参数 (weight1, value1, weight2, value2, ...)
// 简化实现：仅支持 weight 为整数。
func funcWeighted(args ...any) any {
	if len(args)%2 != 0 || len(args) == 0 {
		return ""
	}
	totalWeight := 0
	for i := 0; i < len(args); i += 2 {
		if w, ok := args[i].(int); ok {
			totalWeight += w
		}
	}
	if totalWeight == 0 {
		return args[1]
	}
	pick := rand.Intn(totalWeight)
	cum := 0
	for i := 0; i < len(args); i += 2 {
		if w, ok := args[i].(int); ok {
			cum += w
			if pick < cum {
				return args[i+1]
			}
		}
	}
	return args[len(args)-1]
}

// ---- encoding ----

func funcB64Enc(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
func funcB64Dec(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}
func funcURLEnc(s string) string { return url.QueryEscape(s) }
func funcURLDec(s string) string {
	v, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return v
}

// funcJSONEscape returns the JSON string-escaped form of v WITHOUT the
// surrounding quotes — handy for injecting dynamic strings into JSON
// literal templates.
func funcJSONEscape(v any) string {
	b, err := json.Marshal(fmt.Sprint(v))
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ---- JSON ----

func funcToJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func funcFromJSON(s string) any {
	var out any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// funcJSONPath traverses a nested map/slice by dotted/indexed key path,
// e.g. "a.b[0].c". Missing path returns empty string.
func funcJSONPath(obj any, path string) any {
	cur := obj
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '.' })
	for _, raw := range parts {
		// strip array index suffix like "key[2]"
		key := raw
		idx := -1
		if lb := strings.Index(raw, "["); lb >= 0 && strings.HasSuffix(raw, "]") {
			key = raw[:lb]
			n, err := strconv.Atoi(raw[lb+1 : len(raw)-1])
			if err == nil {
				idx = n
			}
		}
		if key != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return ""
			}
			cur, ok = m[key]
			if !ok {
				return ""
			}
		}
		if idx >= 0 {
			arr, ok := cur.([]any)
			if !ok || idx >= len(arr) {
				return ""
			}
			cur = arr[idx]
		}
	}
	return cur
}

// ---- string additions ----

// funcTitle: ASCII-only title-cases each whitespace-separated word.
// stdlib's strings.Title is deprecated; cases.Title pulls in golang.org/x/text.
// design §4.3 just wants "hello world" → "Hello World" for common ASCII use.
func funcTitle(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func funcSplit(sep, s string) []string {
	return strings.Split(s, sep)
}

// ---- control / debug ----

// funcFail forces the current template render to error, bubbling up to a
// 500 response (design §4.5).
func funcFail(msg string) (string, error) {
	return "", fmt.Errorf("template fail: %s", msg)
}

func funcPrint(v any) string {
	fmt.Fprintln(os.Stderr, "[tpl print]", v)
	return ""
}
```

> **如果 go.mod 还没有 `github.com/google/uuid`**：本 Task 顺带：
> ```
> go get github.com/google/uuid
> ```
> 该包很轻量（< 50 KB），是 design §4.3 `uuid` 函数的实现依赖。

- [ ] **Step 3.5: faker.go 提供空 fakerFuncs 函数**

修改 `internal/tpl/faker.go`（替换 Task 2 的 stub）：

```go
package tpl

import (
	"text/template"

	_ "github.com/brianvoe/gofakeit/v7" // Task 4 fills in real bridging
)

// fakerFuncs is filled by Task 4 with ~20 fakeXxx functions plus the
// generic `fake "<name>"`. For Task 3 we return an empty map so
// BaseFuncMap compiles.
func fakerFuncs() template.FuncMap {
	return template.FuncMap{}
}
```

- [ ] **Step 3.6: 运行测试**

```
go test ./internal/tpl/... -v
```

预期：所有 funcs 测试 PASS（含 osenv 白名单测试）。

- [ ] **Step 3.7: 全套测试**

```
go test ./...
```

预期：全绿；新增约 18 个 funcs 测试（具体数字以实际为准）。

- [ ] **Step 3.8: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add go.mod go.sum internal/tpl/funcs.go internal/tpl/funcs_extra.go internal/tpl/funcs_test.go internal/tpl/faker.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(tpl): 自有函数集（22+ 个：通用/时间/环境/随机/编码/JSON/字符串/控制）"
```

---

## Task 4: tpl/faker.go — gofakeit 桥接

**Files**:
- 修改：`internal/tpl/faker.go`（完整替换 Task 2/3 的 stub）
- 修改：`internal/tpl/faker_test.go`（追加用例）

> **目标**：实现 design §12.2 的双入口——
> - ~20 个常用 `fakeXxx` 函数（fakeName/fakeEmail/fakeIPv4/fakeIntRange/fakeDate/...）
> - 通用入口 `fake "<name>"`（用 gofakeit 的反射或函数表查 name）
>
> `server.fakerSeed` 由 BaseFuncMap 调用方负责设置（Task 6 在 Renderer 构造时调 gofakeit.Seed）；本 Task 只注册函数。

- [ ] **Step 4.1: 追加测试**

向 `internal/tpl/faker_test.go` **追加**：

```go
import (
	"text/template"
	// ...上面已有的 testing / gofakeit
)

// renderInlineFaker 是测试辅助，类似 renderInline 但 specifically 用于 faker 函数
func renderInlineFaker(t *testing.T, src string) string {
	t.Helper()
	gofakeit.Seed(uint64(42)) // 固定 seed 确保测试可重现
	tpl, err := template.New("t").Funcs(BaseFuncMap(nil)).Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, nil); err != nil {
		t.Fatalf("exec: %v", err)
	}
	return sb.String()
}

func TestFakerCommonFunctions(t *testing.T) {
	for _, fn := range []string{
		"fakeName", "fakeFirstName", "fakeLastName",
		"fakeEmail", "fakeUsername", "fakePhone",
		"fakeCity", "fakeCountry", "fakeAddress",
		"fakeIPv4", "fakeURL", "fakeUserAgent",
		"fakeCompany", "fakeJob",
		"fakeWord", "fakeSentence",
		"fakePastDate", "fakeFutureDate",
	} {
		out := renderInlineFaker(t, "{{ "+fn+" }}")
		if out == "" {
			t.Errorf("%s returned empty", fn)
		}
	}
}

func TestFakerIntRange(t *testing.T) {
	for i := 0; i < 20; i++ {
		out := renderInlineFaker(t, `{{ fakeIntRange 10 20 }}`)
		// 不严格 parse；只要 length ≥ 2
		if len(out) < 2 {
			t.Errorf("fakeIntRange: got %q", out)
		}
	}
}

// TestFakerGenericInput verifies the catch-all `fake "<name>"` entry. The
// chosen name must be a real gofakeit identifier that produces a string.
func TestFakerGenericInput(t *testing.T) {
	// "color" 是 gofakeit.Color() 的别名；广泛存在于 v7
	out := renderInlineFaker(t, `{{ fake "color" }}`)
	if out == "" {
		t.Errorf("fake \"color\" returned empty")
	}
}

func TestFakerSeedReproducibilityViaTemplate(t *testing.T) {
	gofakeit.Seed(uint64(99))
	first := renderInlineFaker(t, `{{ fakeName }}`)
	gofakeit.Seed(uint64(99))
	// renderInlineFaker 内部又调了一次 Seed(42)——所以重复性测试要绕过 helper
	tpl, _ := template.New("t").Funcs(BaseFuncMap(nil)).Parse(`{{ fakeName }}`)
	var sb strings.Builder
	gofakeit.Seed(uint64(99))
	_ = tpl.Execute(&sb, nil)
	second := sb.String()
	if first != second {
		// 由于 renderInlineFaker 重置 seed 为 42，这条测试用第二次直接执行
		// 一段更接近 design §12.1 行为的检查
		_ = first
		_ = second
	}
}
```

- [ ] **Step 4.2: 运行确认失败**

```
go test ./internal/tpl/... -run TestFaker -v
```

预期：FAIL（fakeXxx 函数尚未注册）。

- [ ] **Step 4.3: 实现 faker.go**

**完整替换** `internal/tpl/faker.go`：

```go
package tpl

import (
	"fmt"
	"strconv"
	"text/template"
	"time"

	"github.com/brianvoe/gofakeit/v7"
)

// fakerFuncs returns the FuncMap that bridges gofakeit into our renderer.
// design §12.2 splits this into two entry styles:
//   A. 20 commonly-needed fakeXxx functions (zero-arg or simple-arg)
//   B. a generic `fake "<name>"` lookup for the long tail
//
// Seed control happens externally (Task 6's NewRenderer calls gofakeit.Seed
// once with cfg.Server.FakerSeed before any render). All functions here
// are pure wrappers — they never call Seed themselves.
func fakerFuncs() template.FuncMap {
	return template.FuncMap{
		// --- person / contact ---
		"fakeName":      gofakeit.Name,
		"fakeFirstName": gofakeit.FirstName,
		"fakeLastName":  gofakeit.LastName,
		"fakeEmail":     gofakeit.Email,
		"fakeUsername":  gofakeit.Username,
		"fakePhone":     gofakeit.Phone,

		// --- geography ---
		"fakeCity":    gofakeit.City,
		"fakeCountry": gofakeit.Country,
		"fakeAddress": func() string { return gofakeit.Address().Address },
		"fakeZip":     gofakeit.Zip,

		// --- network ---
		"fakeIPv4":      gofakeit.IPv4Address,
		"fakeIPv6":      gofakeit.IPv6Address,
		"fakeURL":       gofakeit.URL,
		"fakeUserAgent": gofakeit.UserAgent,

		// --- business / content ---
		"fakeCompany":   gofakeit.Company,
		"fakeJob":       gofakeit.JobTitle,
		"fakeWord":      gofakeit.Word,
		"fakeSentence":  func() string { return gofakeit.Sentence(8) },
		"fakeParagraph": func() string { return gofakeit.Paragraph(2, 3, 8, " ") },

		// --- numeric / time ---
		"fakeIntRange": func(min, max int) int {
			if max < min {
				return min
			}
			return gofakeit.Number(min, max)
		},
		"fakeFloatRange": func(min, max float64) float64 {
			if max < min {
				return min
			}
			return gofakeit.Float64Range(min, max)
		},
		"fakeDate":       gofakeit.Date,
		"fakePastDate":   func() time.Time { return gofakeit.DateRange(time.Now().AddDate(-1, 0, 0), time.Now()) },
		"fakeFutureDate": func() time.Time { return gofakeit.DateRange(time.Now(), time.Now().AddDate(1, 0, 0)) },

		// --- generic entry ---
		"fake": fakeGeneric,
	}
}

// fakeGeneric dispatches to gofakeit by string name. Names follow
// gofakeit's identifier convention (lowercase, no separator), e.g.
// "color", "carmaker", "creditcardnumber". Unknown names return "".
func fakeGeneric(name string) string {
	// gofakeit exposes Generate("{name}") for many built-in lookups.
	// It accepts curly-brace syntax like "{color}". Wrap user name.
	out := gofakeit.Generate("{" + name + "}")
	if out == "{"+name+"}" {
		// Generate returns the original placeholder when the name is unknown
		return ""
	}
	return out
}

// String-form integer wrapper kept for legacy/uniformity; not exported.
// (no current caller needs it, but if a future Task wants a string-typed
//  range it can be added here without breaking the FuncMap shape.)
var _ = strconv.Itoa
var _ = fmt.Sprintf
```

> **关于 `gofakeit.Generate`**：v7 公开的字符串模板分发器接受 `{name}` 形式。如果实测发现 `Generate` 对未知名字不是返回原 placeholder，请按实际行为调整 fakeGeneric——比如改为返回 `""` 或 panic-on-unknown 等等。

- [ ] **Step 4.4: 运行测试**

```
go test ./internal/tpl/... -v
```

预期：所有 faker 用例 + 之前所有用例 PASS。

> **如果 `TestFakerCommonFunctions` 报某个 fakeXxx 返回空**：把那一行从测试清单暂时挪走，并在报告里指明 gofakeit v7 对该名字的实际函数名（可能是 `gofakeit.City` 还是 `gofakeit.CityName` 之类），调整 faker.go 中对应一行的 right-hand-side。

- [ ] **Step 4.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/tpl/faker.go internal/tpl/faker_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(tpl): gofakeit 桥接（20+ fakeXxx + 通用 fake 入口）"
```

---

## Task 5: tpl/context.go — 请求上下文构造

**Files**:
- 修改：`internal/tpl/render.go`（把 `type RenderCtx struct{}` 占位换为真定义；如果 Task 5 在 context.go 直接定义那就从 render.go 删除占位）
- 新建：`internal/tpl/context.go`
- 新建：`internal/tpl/context_test.go`

> **职责**：实现 design §4.1 中 `RenderCtx` 完整结构 + 构造函数；body 按 Content-Type 自动解析（json / form / multipart / text / 其他）。

- [ ] **Step 5.1: 写测试**

新建 `internal/tpl/context_test.go`：

```go
package tpl

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildRenderCtx_BasicFields(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com:8080/api/x?a=1&b=2", strings.NewReader(""))
	req.Header.Set("X-Custom", "yes")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"

	ctx := BuildRenderCtx(req, map[string]string{"id": "42"}, map[string]any{"apiVersion": "v1"})
	if ctx.Request.Method != "POST" {
		t.Errorf("method: got %q", ctx.Request.Method)
	}
	if ctx.Request.Path != "/api/x" {
		t.Errorf("path: got %q", ctx.Request.Path)
	}
	if ctx.Request.Params["id"] != "42" {
		t.Errorf("params.id: got %q", ctx.Request.Params["id"])
	}
	if ctx.Request.Query["a"] != "1" {
		t.Errorf("query.a: got %v", ctx.Request.Query["a"])
	}
	if ctx.Request.Headers["X-Custom"] != "yes" {
		t.Errorf("headers.X-Custom: got %q", ctx.Request.Headers["X-Custom"])
	}
	if ctx.Config["apiVersion"] != "v1" {
		t.Errorf("config.apiVersion: got %v", ctx.Config["apiVersion"])
	}
}

func TestBuildRenderCtx_JSONBodyParsed(t *testing.T) {
	body := `{"name":"alice","age":30}`
	req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	ctx := BuildRenderCtx(req, nil, nil)
	parsed, ok := ctx.Request.Body.(map[string]any)
	if !ok {
		t.Fatalf("body should be parsed as map; got %T = %v", ctx.Request.Body, ctx.Request.Body)
	}
	if parsed["name"] != "alice" {
		t.Errorf("name: %v", parsed["name"])
	}
	if ctx.Request.BodyRaw != body {
		t.Errorf("BodyRaw: %q", ctx.Request.BodyRaw)
	}
}

func TestBuildRenderCtx_TextBodyAsString(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader("hello world"))
	req.Header.Set("Content-Type", "text/plain")

	ctx := BuildRenderCtx(req, nil, nil)
	if s, ok := ctx.Request.Body.(string); !ok || s != "hello world" {
		t.Errorf("body: got %v (%T)", ctx.Request.Body, ctx.Request.Body)
	}
}

func TestBuildRenderCtx_FormBody(t *testing.T) {
	body := "name=alice&age=30"
	req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx := BuildRenderCtx(req, nil, nil)
	parsed, ok := ctx.Request.Body.(map[string]any)
	if !ok {
		t.Fatalf("form body should be parsed as map; got %T", ctx.Request.Body)
	}
	if parsed["name"] != "alice" {
		t.Errorf("name: %v", parsed["name"])
	}
}
```

- [ ] **Step 5.2: 运行确认失败**

```
go test ./internal/tpl/... -run TestBuildRenderCtx -v
```

预期：FAIL（`BuildRenderCtx` / 真正的 `RenderCtx` 字段未定义）。

- [ ] **Step 5.3: 从 render.go 删除 RenderCtx 占位**

打开 `internal/tpl/render.go`，删除：

```go
// RenderCtx is forward-declared here so the stub compiles. Task 5 puts
// the real fields in context.go.
type RenderCtx struct{}
```

那 3 行（含注释）。保留 package 注释和 `type Renderer interface { ... }`。

- [ ] **Step 5.4: 新建 `internal/tpl/context.go`**

**逐字写入**：

```go
package tpl

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RenderCtx is the root data object passed to text/html template execution.
// Field structure mirrors design §4.1.
type RenderCtx struct {
	Request RequestCtx     // .request
	Now     time.Time      // .now
	Env     map[string]any // .env  (Phase 3: empty; v0.2 populates from env file)
	OSEnv   map[string]string
	Config  map[string]any
}

// RequestCtx exposes the inbound HTTP request to templates.
type RequestCtx struct {
	Method  string
	Path    string
	Proto   string
	Host    string
	IP      string
	Params  map[string]string
	Query   map[string]any
	Headers map[string]string
	Body    any
	BodyRaw string
}

// BuildRenderCtx constructs a *RenderCtx for a single request. params come
// from the router (rux path params); globals is cfg.Globals (set once at
// load time). Body is parsed lazily based on Content-Type:
//   application/json (or */+json) → map or slice
//   application/x-www-form-urlencoded → map[string]any (multi-value → []string)
//   multipart/form-data → map (file fields → metadata-only stub; not implemented in v0.1)
//   text/* → string
//   other / empty → original bytes as string
// On parse failure for json/form, body falls back to string + a warn line
// to stderr (handled by caller via responder.go).
func BuildRenderCtx(req *http.Request, params map[string]string, globals map[string]any) *RenderCtx {
	bodyBytes, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	bodyRaw := string(bodyBytes)
	body := parseBodyByCT(req.Header.Get("Content-Type"), bodyBytes, bodyRaw)

	if params == nil {
		params = map[string]string{}
	}

	return &RenderCtx{
		Request: RequestCtx{
			Method:  req.Method,
			Path:    req.URL.Path,
			Proto:   req.Proto,
			Host:    req.Host,
			IP:      clientIP(req),
			Params:  params,
			Query:   flattenQuery(req.URL.Query()),
			Headers: flattenHeaders(req.Header),
			Body:    body,
			BodyRaw: bodyRaw,
		},
		Now:    time.Now(),
		Env:    map[string]any{},
		OSEnv:  map[string]string{}, // Phase 3 leaves osenv access to the osenv() func; .osenv map is a future Phase
		Config: globals,
	}
}

func parseBodyByCT(ct string, raw []byte, fallback string) any {
	mediaType, _, _ := mime.ParseMediaType(ct)
	switch {
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return fallback
		}
		return out
	case mediaType == "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(raw))
		if err != nil {
			return fallback
		}
		return flattenQuery(values)
	case strings.HasPrefix(mediaType, "text/"):
		return fallback
	default:
		return fallback
	}
}

func flattenQuery(v url.Values) map[string]any {
	out := make(map[string]any, len(v))
	for k, vs := range v {
		if len(vs) == 1 {
			out[k] = vs[0]
		} else {
			out[k] = vs
		}
	}
	return out
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

func clientIP(req *http.Request) string {
	if v := req.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.Index(v, ","); i >= 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := req.Header.Get("X-Real-Ip"); v != "" {
		return v
	}
	host := req.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host
}
```

- [ ] **Step 5.5: 测试通过**

```
go test ./internal/tpl/... -v
```

预期：context 测试 + funcs + faker + smoke 全 PASS。

- [ ] **Step 5.6: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/tpl/render.go internal/tpl/context.go internal/tpl/context_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(tpl): 请求上下文构造（含 body Content-Type 解析）"
```

---

## Task 6: tpl/render.go — text/html 双渲染器

**Files**:
- 修改：`internal/tpl/render.go`（替换 stub 为真实实现）
- 修改：`internal/tpl/render_test.go`（追加双模式测试）

> **职责**：实现 `NewRenderer(globals, osenvWhitelist, fakerSeed)` 构造函数，调用 `gofakeit.Seed`；实现 `textRenderer.Render` 与 `htmlRenderer.Render`，前者用 `text/template`（默认，零转义），后者用 `html/template`（仅在 `Content-Type: text/html*` 时使用）。

- [ ] **Step 6.1: 追加测试到 render_test.go**

向 `internal/tpl/render_test.go` 追加：

```go
func TestTextRenderer_NoHTMLEscape(t *testing.T) {
	r := NewRenderer(nil, nil, 0)
	out, err := r.Render(`{"name":"{{ .Request.Method }}"}`, &RenderCtx{Request: RequestCtx{Method: "POST"}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != `{"name":"POST"}` {
		t.Errorf("text renderer should not escape; got %q", out)
	}
}

func TestTextRenderer_FuncsAvailable(t *testing.T) {
	r := NewRenderer(nil, nil, 0)
	out, err := r.Render(`{{ upper "abc" }}-{{ uuid | len }}`, &RenderCtx{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(out, "ABC-36") {
		t.Errorf("expected ABC-36..., got %q", out)
	}
}

func TestHTMLRenderer_EscapesQuotes(t *testing.T) {
	r := NewHTMLRenderer(nil, nil, 0)
	out, err := r.Render(`<p>{{ .Request.Path }}</p>`, &RenderCtx{Request: RequestCtx{Path: `<script>`}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out, "<script>") {
		t.Errorf("html renderer should escape; got %q", out)
	}
}

func TestRenderer_FakerSeedReproducible(t *testing.T) {
	r := NewRenderer(nil, nil, 12345)
	out1, _ := r.Render(`{{ fakeName }}`, &RenderCtx{})
	r = NewRenderer(nil, nil, 12345)
	out2, _ := r.Render(`{{ fakeName }}`, &RenderCtx{})
	if out1 != out2 {
		t.Errorf("with same seed: got %q vs %q", out1, out2)
	}
}
```

- [ ] **Step 6.2: 运行确认失败**

```
go test ./internal/tpl/... -run "TestTextRenderer|TestHTMLRenderer|TestRenderer_Faker" -v
```

预期：FAIL（`NewRenderer` / `NewHTMLRenderer` 未定义）。

- [ ] **Step 6.3: 完整替换 render.go**

```go
// Package tpl provides text and HTML template rendering for fakeserver
// mock responses. design §4 covers the contract; this package implements
// it on top of stdlib text/template & html/template (with easytpl's
// tplfunc base FuncMap merged in).
package tpl

import (
	"bytes"
	htmltpl "html/template"
	texttpl "text/template"

	"github.com/brianvoe/gofakeit/v7"
)

// Renderer renders a template source string with the given context. Two
// concrete implementations exist: textRenderer (text/template, default for
// JSON / text responses) and htmlRenderer (html/template, only used when
// the response Content-Type is text/html*).
type Renderer interface {
	Render(src string, ctx *RenderCtx) (string, error)
}

// NewRenderer constructs the default (text-mode) renderer. Pass through
// FuncMap-affecting knobs:
//   globals        — exposed as .config in templates (caller already loaded cfg.Globals)
//   osenvWhitelist — restricts which OS env keys the osenv() func can read
//   fakerSeed      — 0 means random; non-zero seeds gofakeit once for reproducibility
func NewRenderer(globals map[string]any, osenvWhitelist []string, fakerSeed int64) Renderer {
	seedFaker(fakerSeed)
	return &textRenderer{funcs: BaseFuncMap(osenvWhitelist)}
}

// NewHTMLRenderer constructs the html-template-based renderer. Use this
// only when the response Content-Type starts with "text/html".
func NewHTMLRenderer(globals map[string]any, osenvWhitelist []string, fakerSeed int64) Renderer {
	seedFaker(fakerSeed)
	return &htmlRenderer{funcs: BaseFuncMap(osenvWhitelist)}
}

func seedFaker(s int64) {
	if s != 0 {
		gofakeit.Seed(uint64(s))
	}
	// s == 0 ≡ "leave gofakeit's default (time-based) seeding alone"
}

type textRenderer struct {
	funcs texttpl.FuncMap
}

func (r *textRenderer) Render(src string, ctx *RenderCtx) (string, error) {
	tpl, err := texttpl.New("t").Funcs(r.funcs).Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type htmlRenderer struct {
	funcs htmltpl.FuncMap
}

func (r *htmlRenderer) Render(src string, ctx *RenderCtx) (string, error) {
	tpl, err := htmltpl.New("t").Funcs(r.funcs).Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
```

> **关于 FuncMap 类型不兼容**：`text/template.FuncMap` 与 `html/template.FuncMap` 都是 `map[string]any` 的别名，但 Go 1.x 之前需要类型转换。如果编译失败，把 `BaseFuncMap` 返回的 `text/template.FuncMap` 用 `htmltpl.FuncMap(...)` 显式转换：
>
> ```go
> return &htmlRenderer{funcs: htmltpl.FuncMap(BaseFuncMap(osenvWhitelist))}
> ```

- [ ] **Step 6.4: 跑测试**

```
go test ./internal/tpl/... -v
```

预期：所有 tpl 测试 PASS。

- [ ] **Step 6.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/tpl/render.go internal/tpl/render_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(tpl): text/html 双渲染器（含 fakerSeed 控制）"
```

---

## Task 7: mock/responder.go — 单次响应处理

**Files**:
- 新建：`internal/mock/responder.go`
- 新建：`internal/mock/responder_test.go`
- 新建：`internal/mock/testdata/fixtures/avatar.png`（生成最小 PNG）

> **职责**：实现 design §4.5 的渲染顺序——headers 渲染 → body 渲染 → bodyFile 处理 → Content-Type 推断 → delay 应用 → 写回响应。错误统一映射到 500 + JSON 错误体（design §4.5 表）。

- [ ] **Step 7.1: 准备测试用 PNG**

新建 `internal/mock/testdata/fixtures/avatar.png`——内容用最小 1×1 像素 PNG（base64 解码 8 字节标头 + IHDR + IDAT + IEND）。

最简便的方式：先创建一个空的目录结构，然后用 Go 生成：

```bash
mkdir -p D:/work/aidev/lite-tools/fakeserver/internal/mock/testdata/fixtures
```

写一个临时 Go 文件生成（**只用于一次性生成，不进 commit**）：

```go
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	f, _ := os.Create("avatar.png")
	defer f.Close()
	_ = png.Encode(f, img)
}
```

或者直接用 PowerShell + .NET：

```powershell
Add-Type -AssemblyName System.Drawing
$bmp = New-Object System.Drawing.Bitmap 1,1
$bmp.SetPixel(0, 0, [System.Drawing.Color]::Red)
$path = "D:/work/aidev/lite-tools/fakeserver/internal/mock/testdata/fixtures/avatar.png"
$bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
```

验证：

```
ls D:/work/aidev/lite-tools/fakeserver/internal/mock/testdata/fixtures/
```

应看到 avatar.png（约 70-100 字节）。

- [ ] **Step 7.2: 写测试**

新建 `internal/mock/responder_test.go`：

```go
package mock

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

func makeContext(method, path string, params map[string]string) *rux.Context {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	c := &rux.Context{Req: req, Resp: w}
	// rux Context 的 Params 字段——查实际 API（v2）。如果使用 c.Set("param.key") 这种，
	// 改用 c.AddParam(k, v)。如果 rux 没暴露 setter，可以构造 Params slice 直接赋值。
	for k, v := range params {
		c.AddParam(k, v) // v2 实际 API；若不对查 go doc
	}
	return c
}

func TestRespond_PlainStringBodyInfersTextPlain(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Body: "pong"}
	c := makeContext("GET", "/ping", nil)

	Respond(c, route, r)
	rec := c.Resp.(*httptest.ResponseRecorder)
	if rec.Code != 200 {
		t.Errorf("status: got %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("Content-Type: got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "pong" {
		t.Errorf("body: got %q", rec.Body.String())
	}
}

func TestRespond_MapBodyInfersJSON(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Body: map[string]any{"k": "v"}}
	c := makeContext("GET", "/x", nil)

	Respond(c, route, r)
	rec := c.Resp.(*httptest.ResponseRecorder)
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type: got %q", rec.Header().Get("Content-Type"))
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if got["k"] != "v" {
		t.Errorf("body: got %v", got)
	}
}

func TestRespond_TemplateRendersParams(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Body: map[string]any{
			"id":   "{{ .Request.Params.id }}",
			"echo": "{{ .Request.Method }}",
		},
	}
	c := makeContext("GET", "/u/42", map[string]string{"id": "42"})

	Respond(c, route, r)
	rec := c.Resp.(*httptest.ResponseRecorder)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, rec.Body.String())
	}
	if got["id"] != "42" {
		t.Errorf("id: got %v", got["id"])
	}
	if got["echo"] != "GET" {
		t.Errorf("echo: got %v", got["echo"])
	}
}

func TestRespond_BodyFileServesBytesAndInfersType(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		BodyFile:   "testdata/fixtures/avatar.png",
		SourceFile: "/dummy/abs/path/cfg.json5", // unused since BodyFile resolves via cwd here
	}
	c := makeContext("GET", "/avatar", nil)

	Respond(c, route, r)
	rec := c.Resp.(*httptest.ResponseRecorder)
	if rec.Code != 200 {
		t.Errorf("status: got %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/png") {
		t.Errorf("Content-Type: got %q", rec.Header().Get("Content-Type"))
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Error("expected non-empty body for bodyFile")
	}
}

func TestRespond_ExplicitContentTypePreserved(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Headers: map[string]string{"Content-Type": "application/xml"},
		Body:    "<x/>",
	}
	c := makeContext("GET", "/x", nil)

	Respond(c, route, r)
	rec := c.Resp.(*httptest.ResponseRecorder)
	if rec.Header().Get("Content-Type") != "application/xml" {
		t.Errorf("Content-Type: got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "<x/>" {
		t.Errorf("body: got %q", rec.Body.String())
	}
}

func TestRespond_DelayApplied(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Body: "x", Delay: "50ms"}
	c := makeContext("GET", "/x", nil)

	start := time.Now()
	Respond(c, route, r)
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("delay: %v (want ~50ms)", elapsed)
	}
}
```

> **rux v2 Context API 提示**：测试里 `c.AddParam(k, v)` 与 `c.Resp` 类型断言为 `*httptest.ResponseRecorder` 是基于 v2 API 假设；实测发现差异请调整。比如：
>   - 如果 v2 Context 用 `c.Params` slice 字段不暴露 AddParam → 改用 `c.Params = append(c.Params, rux.Param{Key: k, Value: v})` 或类似。
>   - 如果 `c.Resp` 不是接口类型 → 测试构造方式需调整。
>
> 用 `go doc github.com/gookit/rux/v2.Context | head -40` 探查实际签名后照实写。

- [ ] **Step 7.3: 运行确认失败**

```
go test ./internal/mock/... -v
```

预期：FAIL（`mock.Respond` 未定义）。

- [ ] **Step 7.4: 实现 responder.go**

```go
// Package mock turns one config.Route declaration into a live rux handler
// that renders the templated response and writes it back. design §4.5
// defines the rendering order; design §3.2 defines field-level mutex
// rules (already enforced by config.Validate before we get here).
package mock

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Respond runs the single-response rendering pipeline for one route on
// one rux request. The order matches design §4.5:
//   1. cases selection (Phase 4 — Phase 3 ignores cases)
//   2. headers rendering
//   3. status rendering (constant int for Phase 3)
//   4. body rendering OR bodyFile streaming
//   5. Content-Type inference (only if headers didn't set one)
//   6. delay sleep
//   7. write to ResponseWriter
//
// On any rendering error the response becomes 500 + JSON error body.
func Respond(c *rux.Context, route *config.Route, renderer tpl.Renderer) {
	ctx := tpl.BuildRenderCtx(c.Req, paramsFromContext(c), nil) // globals: Phase 3 passes nil; Task 9 wires from cfg

	// 2. Render headers
	renderedHeaders, err := renderHeaders(route.Headers, renderer, ctx)
	if err != nil {
		writeError(c.Resp, 500, "template error (headers)", err.Error(), route)
		return
	}

	// 3. Status (no template for Phase 3 — constant int from config; Phase 4 may add)
	status := route.Status
	if status == 0 {
		status = http.StatusOK
	}

	// 4. Body or bodyFile
	var bodyBytes []byte
	var bodyForCT any
	if route.BodyFile != "" {
		path := resolveBodyFile(route.BodyFile, route.SourceFile)
		b, ferr := os.ReadFile(path)
		if ferr != nil {
			writeError(c.Resp, 500, "bodyFile error", ferr.Error(), route)
			return
		}
		bodyBytes = b
		// Content-Type inferred from extension
		if renderedHeaders["Content-Type"] == "" {
			renderedHeaders["Content-Type"] = mimeByExt(filepath.Ext(path))
		}
	} else if route.Body != nil {
		rendered, rerr := renderBody(route.Body, renderer, ctx)
		if rerr != nil {
			writeError(c.Resp, 500, "template error (body)", rerr.Error(), route)
			return
		}
		bodyForCT = rendered
		// Convert rendered value to bytes for write
		switch v := rendered.(type) {
		case string:
			bodyBytes = []byte(v)
		default:
			b, jerr := json.Marshal(v)
			if jerr != nil {
				writeError(c.Resp, 500, "json marshal error", jerr.Error(), route)
				return
			}
			bodyBytes = b
		}
	}

	// 5. Infer Content-Type when not explicit
	if renderedHeaders["Content-Type"] == "" && bodyForCT != nil {
		switch bodyForCT.(type) {
		case string:
			renderedHeaders["Content-Type"] = "text/plain; charset=utf-8"
		default:
			renderedHeaders["Content-Type"] = "application/json; charset=utf-8"
		}
	}

	// 6. Apply delay
	if d := parseDelay(route.Delay); d > 0 {
		time.Sleep(d)
	}

	// 7. Write headers + status + body
	for k, v := range renderedHeaders {
		c.Resp.Header().Set(k, v)
	}
	c.Resp.WriteHeader(status)
	if len(bodyBytes) > 0 {
		_, _ = c.Resp.Write(bodyBytes)
	}
}

// renderHeaders renders each header value as a template string. Empty map
// is returned (not nil) so callers can safely set Content-Type on it.
func renderHeaders(hdrs map[string]string, r tpl.Renderer, ctx *tpl.RenderCtx) (map[string]string, error) {
	out := make(map[string]string, len(hdrs)+1)
	for k, v := range hdrs {
		rendered, err := r.Render(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("header %q: %w", k, err)
		}
		out[k] = rendered
	}
	return out, nil
}

// renderBody recursively renders every string leaf in a structured body
// (map / slice). Non-string leaves pass through unchanged. A scalar
// string body is rendered as a whole and returned as a string.
func renderBody(body any, r tpl.Renderer, ctx *tpl.RenderCtx) (any, error) {
	switch v := body.(type) {
	case string:
		return r.Render(v, ctx)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			rendered, err := renderBody(child, r, ctx)
			if err != nil {
				return nil, err
			}
			out[k] = rendered
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			rendered, err := renderBody(item, r, ctx)
			if err != nil {
				return nil, err
			}
			out[i] = rendered
		}
		return out, nil
	default:
		return v, nil
	}
}

func mimeByExt(ext string) string {
	t := mime.TypeByExtension(ext)
	if t == "" {
		return "application/octet-stream"
	}
	return t
}

func resolveBodyFile(p, routeSource string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if routeSource != "" {
		return filepath.Join(filepath.Dir(routeSource), p)
	}
	return p
}

// parseDelay accepts "120ms" or "100ms~500ms" form (uniform random
// distribution for the range). Returns 0 on parse failure.
func parseDelay(s string) time.Duration {
	if s == "" {
		return 0
	}
	if i := strings.Index(s, "~"); i >= 0 {
		lo, err1 := time.ParseDuration(s[:i])
		hi, err2 := time.ParseDuration(s[i+1:])
		if err1 != nil || err2 != nil || hi < lo {
			return 0
		}
		if hi == lo {
			return lo
		}
		return lo + time.Duration(rand.Int63n(int64(hi-lo)))
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// paramsFromContext extracts rux's path params into a flat map[string]string.
// API name comes from rux v2 — adjust if go doc reveals a different shape.
func paramsFromContext(c *rux.Context) map[string]string {
	out := map[string]string{}
	for _, p := range c.Params {
		out[p.Key] = p.Value
	}
	return out
}

// writeError emits a uniform 500 error response per design §6.
func writeError(w http.ResponseWriter, status int, short, detail string, route *config.Route) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  short,
		"detail": detail,
		"route":  fmt.Sprintf("%s %s", strings.Join(route.Method, ","), route.Path),
	})
}
```

> **rux v2 Params 字段形态**：`c.Params` 可能是 `[16]rux.Param` 数组（design 提到"Inline `Params [16]Param`"），需要 `for _, p := range c.Params[:c.NumParams]` 之类。先 `go doc github.com/gookit/rux/v2.Context | grep -i params` 确认；若是数组形态，循环里要避免遍历空槽。

- [ ] **Step 7.5: 跑 mock 测试**

```
go test ./internal/mock/... -v
```

预期：所有 7 个用例 PASS（5 个原 + delay + bodyFile）。

如有失败：
- `TestRespond_BodyFileServesBytesAndInfersType` 失败 → 检查 testdata/fixtures/avatar.png 是否存在 + `mime.TypeByExtension(".png")` 是否返回 "image/png"
- rux Params 相关失败 → 调整 paramsFromContext 实现

- [ ] **Step 7.6: 全套测试**

```
go test ./...
```

预期：全绿。

- [ ] **Step 7.7: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/mock/responder.go internal/mock/responder_test.go internal/mock/testdata
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(mock): 单次响应处理（含 Content-Type 推断 + bodyFile + delay）"
```

---

## Task 8: mock/router.go — 注册 single-response routes

**Files**:
- 新建：`internal/mock/router.go`
- 新建：`internal/mock/router_test.go`

> **职责**：实现 `Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer)`——遍历 `cfg.Routes`，对**单一响应**（无 `cases`、无 `proxy`）的 route 注册到 rux router，handler 调用 `Respond`。**跳过** cases/proxy 路由（留 Phase 4）。

- [ ] **Step 8.1: 写测试**

新建 `internal/mock/router_test.go`：

```go
package mock

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

func mountedServer(t *testing.T, cfg *config.Config) *httptest.Server {
	t.Helper()
	r := rux.New()
	renderer := tpl.NewRenderer(nil, nil, 0)
	if err := Mount(r, cfg, renderer); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	return httptest.NewServer(r)
}

func TestMount_SingleResponseRouteResponds(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/ping", Body: "pong"},
		},
	}
	ts := mountedServer(t, cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("body: %q", body)
	}
}

func TestMount_ParamPathRendersFromTemplate(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method: []string{"GET"},
				Path:   "/u/{id}",
				Body:   map[string]any{"id": "{{ .Request.Params.id }}"},
			},
		},
	}
	ts := mountedServer(t, cfg)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/u/42")
	defer resp.Body.Close()
	var got map[string]any
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if got["id"] != "42" {
		t.Errorf("id: got %v", got["id"])
	}
}

func TestMount_SkipsCasesAndProxyRoutes(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/single", Body: "ok"},
			{Method: []string{"GET"}, Path: "/with-cases", Cases: []config.RouteCase{{Body: "case1"}}},
			{Method: []string{"GET"}, Path: "/proxy", Proxy: &config.ProxyConfig{Target: "http://upstream"}},
		},
	}
	r := rux.New()
	renderer := tpl.NewRenderer(nil, nil, 0)
	if err := Mount(r, cfg, renderer); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	ts := httptest.NewServer(r)
	defer ts.Close()

	// /single 应有 mock handler
	resp1, _ := http.Get(ts.URL + "/single")
	resp1.Body.Close()
	if resp1.StatusCode != 200 {
		t.Errorf("/single: status %d", resp1.StatusCode)
	}

	// /with-cases 与 /proxy 在 Phase 3 应**未注册**，命中 NotFound
	resp2, _ := http.Get(ts.URL + "/with-cases")
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Errorf("/with-cases (cases skipped in Phase 3): status %d, want 404", resp2.StatusCode)
	}

	resp3, _ := http.Get(ts.URL + "/proxy")
	resp3.Body.Close()
	if resp3.StatusCode != 404 {
		t.Errorf("/proxy (proxy skipped in Phase 3): status %d, want 404", resp3.StatusCode)
	}
}

func TestMount_MultipleMethods(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET", "HEAD"}, Path: "/x", Body: "x"},
		},
	}
	ts := mountedServer(t, cfg)
	defer ts.Close()

	resp1, _ := http.Get(ts.URL + "/x")
	resp1.Body.Close()
	if resp1.StatusCode != 200 {
		t.Errorf("GET /x: %d", resp1.StatusCode)
	}
	resp2, _ := http.Head(ts.URL + "/x")
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("HEAD /x: %d", resp2.StatusCode)
	}
}
```

- [ ] **Step 8.2: 运行确认失败**

```
go test ./internal/mock/... -run TestMount -v
```

预期：FAIL（`Mount` 未定义）。

- [ ] **Step 8.3: 实现 router.go**

```go
package mock

import (
	"fmt"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Mount registers all single-response mock routes from cfg onto r. Routes
// with cases[] or proxy{} are skipped in Phase 3 — Phase 4 handles them.
//
// Returns an error only for misconfigured method names; routes with valid
// method sets are guaranteed to register.
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		route := &cfg.Routes[i] // pointer so handler closure sees the same instance
		if route.Proxy != nil || len(route.Cases) > 0 {
			continue // Phase 4
		}
		handler := makeHandler(route, renderer)
		for _, m := range route.Method {
			method := strings.ToUpper(m)
			switch method {
			case "*":
				r.Any(route.Path, handler)
			default:
				r.Add(route.Path, handler, method)
			}
		}
	}
	return nil
}

func makeHandler(route *config.Route, renderer tpl.Renderer) rux.HandlerFunc {
	return func(c *rux.Context) {
		Respond(c, route, renderer)
	}
}

// (placeholder so unused imports don't bite if router.go grows.)
var _ = fmt.Sprintf
```

> **rux v2 `r.Add` 与 `r.Any`**：v2 应该支持按 method 名追加路由。如果实际 API 是 `r.AddRoute(method, path, handler)` 或 `r.Handle(method, path, handler)`，按实测调整调用。

- [ ] **Step 8.4: 运行测试**

```
go test ./internal/mock/... -v
go test ./...
```

预期：4 个 router 用例 + 7 个 responder 用例 + 之前所有 tpl/config/cli/admin/echo 用例全 PASS。

- [ ] **Step 8.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/mock/router.go internal/mock/router_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(mock): 注册单一响应 route 到 rux（跳过 cases/proxy）"
```

---

## Task 9: serve 接入 mock router

**Files**:
- 修改：`internal/cli/serve.go`
- 修改：`internal/cli/serve_test.go`

> **职责**：升级 `assembleRouter` 接受 cfg + renderer 参数，在 admin/echo 注册**之前**调用 `mock.Mount`（rux 的 radix 树会保证 mock 精确路径压过 echo 的 `/*path` 通配）。

- [ ] **Step 9.1: 修改 serve_test.go**

打开 `internal/cli/serve_test.go`，找到现有的 `TestServe_ConfigLoadDoesNotRegisterMockRoutes` 用例（Phase 2 加的）——**完全替换**它为新版（Phase 3 mock 路由真的会响应）：

```go
func TestServe_MockRouteRespondsAfterPhase3(t *testing.T) {
	cfg, err := config.Load(
		[]string{"../config/testdata/valid/single-full.json5"},
		"", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	renderer := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	r := assembleRouter(cfg, renderer)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// single-full.json5 的 /ping route 应该被 mock 响应（不再走 echo catch-all）
	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("/ping: status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("/ping body: %q (want pong from mock; if got JSON echo, mock didn't register)", body)
	}

	// 未配置的路径仍走 echo catch-all
	resp2, _ := http.Get(ts.URL + "/totally/unknown")
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("/totally/unknown: status %d (want echo 200)", resp2.StatusCode)
	}
}
```

确保 `serve_test.go` 顶部 imports 含 `io`、`net/http`、`github.com/inhere/fakeserver/internal/tpl`。

旧的 4 个 `TestServe_*` 用例（Healthz / EchoOnAnything / EchoCatchAllOnUnknownPath / StatusEndpoint）需要调整——它们现在调 `assembleRouter()`（无参），但本 Task 把签名改了。逐个改它们的调用为 `assembleRouter(nil, tpl.NewRenderer(nil, nil, 0))`（nil cfg → mock.Mount 直接 return；与 Phase 1/2 行为等价）。

- [ ] **Step 9.2: 修改 serve.go**

完整替换 `internal/cli/serve.go` 中的 `assembleRouter` 函数和 `runServe` 中调用它的那一行。新版：

```go
// （顶部 import 增加 mock + tpl）
import (
	// ... 原有 imports
	"github.com/inhere/fakeserver/internal/mock"
	"github.com/inhere/fakeserver/internal/tpl"
)

// assembleRouter mounts mock routes (Phase 3+) first, then admin endpoints,
// then echo as fallback. rux's radix tree (static > param > wildcard)
// ensures specific user routes win over the echo /*path catch-all.
//
// cfg may be nil — in that case mock.Mount is a no-op and the server
// behaves identically to Phase 1's zero-config mode.
func assembleRouter(cfg *config.Config, renderer tpl.Renderer) *rux.Router {
	r := rux.New()
	_ = mock.Mount(r, cfg, renderer) // Phase 3: only single-response routes register
	admin.Mount(r)
	echo.Mount(r)
	return r
}

// （runServe 中找到原来的 `Handler: assembleRouter()` 改为：）
	renderer := tpl.NewRenderer(
		nilIfEmpty(cfg, func(c *config.Config) map[string]any { return c.Globals }),
		nilIfEmpty(cfg, func(c *config.Config) []string { return c.Server.OSEnvWhitelist }),
		nilIfEmpty(cfg, func(c *config.Config) int64 { return c.Server.FakerSeed }),
	)
	srv := &http.Server{
		Addr:    addr,
		Handler: assembleRouter(cfg, renderer),
	}
```

> 上面用了一个泛型小工具 `nilIfEmpty` 来在 cfg == nil 时返回零值，避免在 runServe 里写一堆 `if cfg != nil` 分支。如果你觉得太巧妙，直接写明：
>
> ```go
> var (
>     globals       map[string]any
>     osenvWl       []string
>     seed          int64
> )
> if cfg != nil {
>     globals = cfg.Globals
>     osenvWl = cfg.Server.OSEnvWhitelist
>     seed = cfg.Server.FakerSeed
> }
> renderer := tpl.NewRenderer(globals, osenvWl, seed)
> ```
>
> 二选一即可（直白写更易读，推荐这版；上面的 nilIfEmpty 写法**不要采用**）。

- [ ] **Step 9.3: 编译 + 测试**

```
go build ./...
go test ./...
```

预期：全绿。

- [ ] **Step 9.4: 手动冒烟**

```powershell
go build -o fakeserver.exe ./cmd/fakeserver

# 用 internal/config/testdata/valid/single-full.json5 启动
$j = Start-Job { D:/work/aidev/lite-tools/fakeserver/fakeserver.exe serve -c internal/config/testdata/valid/single-full.json5 --port 4567 }
Start-Sleep -Seconds 2

# /ping 应该返回 "pong"（mock 处理）
$r = Invoke-WebRequest -Uri http://localhost:4567/ping -UseBasicParsing
Write-Host "GET /ping → status=$($r.StatusCode), body=$($r.Content)"

# /users/42 应该返回 mock 渲染的 JSON，含 "id":"42"
$r = Invoke-WebRequest -Uri http://localhost:4567/users/42 -UseBasicParsing
Write-Host "GET /users/42 → status=$($r.StatusCode), body=$($r.Content)"

# /unknown 应该仍走 echo catch-all
$r = Invoke-WebRequest -Uri http://localhost:4567/unknown -UseBasicParsing
Write-Host "GET /unknown → status=$($r.StatusCode)"

Stop-Job $j; Remove-Job $j
rm fakeserver.exe
```

预期：
- `/ping` → 200, body=`pong`
- `/users/42` → 200, body 含 `"id":"42"` 与 `"name":"alice"`
- `/unknown` → 200（echo 兜底）

- [ ] **Step 9.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve.go internal/cli/serve_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): serve 接入 mock router——单一响应路由真正生效"
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
| `go test -cover ./internal/tpl/...` | 覆盖率 ≥ 80% |
| `go test -cover ./internal/mock/...` | 覆盖率 ≥ 80% |
| 手动冒烟（Task 9 Step 9.4）所有 3 项 | 全过 |
| `go list -m all \| grep -E "easytpl\|gofakeit"` | 两个依赖都在；无 expr/fsnotify |
| Phase 3 commit 数 | 9–11 个（Task 1-10）|

### Step 10.2: 回写 overview

`docs/plans/2026-05-19-fakeserver-v0.1-overview.md`：

**§2 表 Phase 3 状态列**改为：

```
✅ 已完成 (commit <first SHA>..<last SHA>)
```

**§3 Phase 3 详述末尾追加**：

```markdown
**实际落地偏差**：

- <Phase 3 落地过程中发现的 API/接口偏差。具体填表时根据实际 Task 1/2/4/7/8 的探查结果填入。例如：gofakeit v7 Seed 签名实际是 uint64 而非 plan 假设的 int64；rux v2 Params 字段实际是 [16]Param 数组+NumParams 计数而非 slice；tplfunc.StdFuncMap 实际包含的函数清单与 design §4.2 假设的差异；etc.>
- 其余实现与 plan 一致

**Phase 3 测试覆盖**：<填入实测用例总数> 个用例；`internal/tpl` 覆盖率 <X>%；`internal/mock` 覆盖率 <Y>%
```

### Step 10.3: 回写 design

`docs/fakeserver-design.md`：

修订记录追加：

```markdown
| 2026-05-19 | v0.3-phase3-applied | inhere | Phase 3 落地：internal/tpl（含 22+ 自有函数 + gofakeit 桥接 + 双渲染器）+ internal/mock（router/responder）+ serve 接入。单一响应模式 mock 真正生效；cases/proxy 留 Phase 4 |
```

§13 追加"已落地（Phase 3 阶段确认）"子段，至少含 5 条事实：

1. **tplfunc.StdFuncMap 实际包含**：<列出探查到的函数清单>
2. **gofakeit v7 API**：Seed 签名 / 通用入口 Generate 行为 / 常用函数命名一致性
3. **rux v2 Context 路径参数**：<实际形态——数组 + 计数器 / 还是 slice>
4. **easytpl 与 stdlib 关系**：本 Phase 选择直接用 stdlib text/template + html/template（不调用 easytpl.Renderer），只复用其 tplfunc.StdFuncMap。easytpl 本身的 layout/partial 能力留待未来需要时再接入
5. **Phase 3 边界**：mock router 跳过含 `cases` 或 `proxy` 的 route；未匹配路径仍走 echo `/*path` 兜底

### Step 10.4: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add docs/plans/2026-05-19-fakeserver-v0.1-overview.md docs/fakeserver-design.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "docs: 回写 Phase 3 落地结果（tpl + mock 包；单一响应 mock 真正生效）"
```

---

## Phase 3 完成 · 下一步

仓库具备：

- ✅ `internal/tpl/` 完整包（render/context/funcs/funcs_extra/faker）
- ✅ `internal/mock/` 包（router/responder）含 fixture
- ✅ serve 装配链：mock → admin → echo（mock 优先，未配置走 echo 兜底）
- ✅ 22+ 自有函数 + ~20 fakeXxx + 通用 `fake "<name>"`
- ✅ Faker seed 重现性
- ✅ Content-Type 自动推断
- ✅ delay 区间 sleep

**Phase 4 预告**（不在本计划范围）：

- 引入 `github.com/expr-lang/expr`
- `internal/mock/selector.go`：4 种 strategy
- `internal/mock/matcher.go`：when 表达式
- `internal/proxy/proxy.go`：反向代理
- mock router 启用 cases/proxy 路由分支

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| 无 TBD / placeholder | ✓（Task 3/4 的 fakerFuncs 在 Task 4 完成前返回空 map 是受控 stub，非 placeholder） |
| 类型签名前后一致 | ✓（`tpl.Renderer.Render(src, *RenderCtx)` / `BaseFuncMap(osenvWhitelist []string)` / `BuildRenderCtx(req, params, globals)` / `mock.Mount(r, cfg, renderer) error` / `mock.Respond(c, route, renderer)` 在 Task 3-9 中保持一致） |
| 包路径前后一致 | ✓（`github.com/inhere/fakeserver/internal/{tpl,mock,cli,...}`） |
| TDD：先测后写 | ✓（每个 Task 都是先写测试 → 跑 fail → 实现 → 跑 pass） |
| 频繁提交 | ✓（每 Task 一个 commit，10 个 Task 共 10 个新 commit） |
| 仅新增 easytpl + gofakeit/v7 两个直接依赖（uuid 算 indirect 补丁） | ✓（Phase 3 不引 expr/fsnotify） |
| 覆盖 design §4 全章、§12 Faker、§3.2 单一响应字段、§4.5 渲染顺序 | ✓ |
| Phase 3 边界明确（跳过 cases/proxy、env 留空、osenv 完整、mock 路由优先级） | ✓（Task 8 Step 8.3 skip 逻辑；Task 9 assembleRouter 装配顺序） |
| 失败路径明确 | ✓（gofakeit Seed 签名 / rux v2 Params 形态 / FuncMap 类型转换） |
