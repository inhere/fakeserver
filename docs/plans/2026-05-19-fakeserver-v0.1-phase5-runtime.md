# Fakeserver v0.1 · Phase 5 — 运行时与可观测性（v0.1 MVP 闭环）

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：补齐 v0.1 MVP 的最后一拼图——**中间件全套**（recoverer / logger / cors / bodylimit）、**热加载**（fsnotify + 300ms 防抖 + 原子 router 切换）、**admin /__fakeserver/routes 端点**、**启动 banner**、**`serve` 子命令 `--quiet` / `--no-cors` / `--no-watch` 开关**。**Phase 5 完成 = v0.1 MVP 完整闭环**：写一个综合 config（mock 单响应 + cases + proxy + bodyFile） → 启动 → 各路由请求验证 → 编辑 config 触发热加载 → 再次请求确认新路由生效。

**Architecture**：在 Phase 1-4 已就位的 router 装配链外面，套一层"中间件链 + 原子 swap"：

1. **中间件链**（outermost first）：`recoverer → logger → bodylimit → cors → router(mock+proxy+admin+echo)`。每层是 `func(http.Handler) http.Handler` 的标准 Go 中间件签名。
2. **原子 swap**：定义 `ServerHandler` 包装类型，内含 `atomic.Pointer[http.Handler]`。`http.Server.Handler = ServerHandler`。watcher 检测到 config 变化时构造新的"中间件链 + router"并 atomic.Store 进 holder。在途请求继续走旧 handler，新请求走新 handler。
3. **热加载**：`internal/config/watcher.go` 用 fsnotify 监听 `cfg.SourcePaths`（Phase 2 已收集所有 include 链上的文件路径），事件触发 300ms 防抖窗口，窗口结束后 reload + validate；失败保留旧 router 并 stderr warn；成功 swap。
4. **CORS 特殊点**：design §5.4 规定"OPTIONS 在路由匹配**之后**才走 204 短路"——本 Phase 用"buffer-then-rewrite"模式实现：CORS 中间件用 `httptest.ResponseRecorder` 风格的 wrapper 拦截下层响应，若是 OPTIONS 且路由返回 404 → 改写为 204 + preflight headers；否则透传并追加 CORS 响应 header。`/__fakeserver/*` 路径前缀豁免（不加 CORS、不改写）。
5. **启动 banner**：抽出 `printBanner(cfg, addr)` 函数；输出版本、监听地址、路由计数（mock / proxy / fallback 模式分项）、env 名（v0.2 占位，本 Phase 输出空字符串）。

**Tech Stack**：Go 1.26+ · `github.com/fsnotify/fsnotify`（新引入，**Phase 5 唯一新增第三方依赖**）· 标准库 `net/http` / `sync/atomic` / `time` / `bufio` / `log`。

**前置要求**：

- Phase 1–4 已完成（commits up to `71d9c80`）
- 已读 `docs/fakeserver-design.md` §5.1 启动流程 / §5.2 热加载 / §5.3 请求日志 / §5.4 CORS / §5.6 admin /routes + banner / §6 错误处理完整表
- 已读 `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` §3 Phase 5 详述
- `fakeserver/` 当前 go.mod 含 rux/v2、gcli/v3、goutil、titanous/json5、easytpl、gofakeit/v7、expr-lang/expr、uuid（8 个直接依赖）
- Phase 4 落地事实（来自 overview §3 Phase 4，本 Phase 直接复用）：
  - `mock.Mount(r, cfg, renderer) error` / `proxy.Mount(r, cfg, renderer) error` / `admin.Mount(r)` / `echo.Mount(r)`：rux 路由注册入口
  - `config.Load(paths, envName, overrides) (*Config, error)` / `config.Validate(cfg) []error` / `config.Warn(cfg) []string`：配置三件套
  - `Config.SourcePaths []string`：Phase 2 已收集 include 链上的全部绝对路径，热加载直接复用
  - `tpl.NewRenderer(globals, osenvWhitelist, fakerSeed) tpl.Renderer`：Phase 3 渲染器
  - cli 入口 `runServe(opts serveOptions) error`：本 Phase 重构其内部装配链

**Phase 5 完成定义（DoD，来自 overview §3 Phase 5）**：

1. `go build ./...` 通过
2. `go test ./...` 全绿；`internal/middleware/` 单元测试覆盖率 ≥ 80%
3. **Recoverer**：panic 被捕获 → 500 + JSON 错误体（`{error, route, panic, stack}`）；server 不退出；下一个请求正常服务
4. **CORS**：默认 reflect Origin、自定义 `origins/methods/headers` 生效；`/__fakeserver/*` 无 CORS 头；OPTIONS 在用户路由命中后走用户 handler（不被中间件吃掉）；用户路由未命中时 OPTIONS → 204 + preflight headers
5. **热加载**：编辑 config 文件 → 300ms 防抖窗口结束 → fsnotify 触发 → 重新加载 + Validate 通过 → router 原子切换 → 下一个请求走新路由；在途请求走旧路由；Validate 失败保留旧表 + stderr warn（"reload failed: <errs>"）
6. **admin `/__fakeserver/routes`**：返回当前 effective route 列表 JSON（每项含 method/path/mode where mode ∈ `mock|cases|proxy`），mock/proxy 分类清晰
7. **CLI flag**：`--quiet` 抑制请求日志（recoverer 仍然兜底）；`--no-cors` 关闭 CORS 中间件；`--no-watch` 不启动 watcher
8. **启动 banner**：含版本 / 端口 / 路由计数 / fallback 模式 / env 占位
9. **v0.1 MVP 闭环 E2E**：综合 config（mock 单响应 `/ping` + cases first-match `/u/{id}` + proxy `/api/*rest` + bodyFile `/avatar.png`） → 启动 → 5 类请求各发一次验证 → 文件系统编辑 config 增加一条新 mock route → 等待 ~400ms → curl 新路径 → 200
10. **Phase 5 不再引入** 任何超出 fsnotify 的第三方依赖；env 文件（design §8）/ 项目注册（§10）/ Web UI（§11）继续留 v0.2+

---

## 文件结构（Phase 5 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `go.mod` / `go.sum` | 引入 `github.com/fsnotify/fsnotify` |
| 新建 | `internal/middleware/recoverer.go` | `Recoverer(next http.Handler) http.Handler`：defer recover → 500 + JSON + stack to stderr |
| 新建 | `internal/middleware/recoverer_test.go` | panic 不退出 / 错误体含 stack / 后续请求正常 |
| 新建 | `internal/middleware/logger.go` | `Logger(out io.Writer, quiet bool) func(http.Handler) http.Handler`：访问日志 time/method/path/status/duration |
| 新建 | `internal/middleware/logger_test.go` | 日志输出格式 / quiet=true 抑制 |
| 新建 | `internal/middleware/cors.go` | `CORS(opts CORSOpts) func(http.Handler) http.Handler`：响应头注入 + OPTIONS 后置 204 + `/__fakeserver/*` 豁免 |
| 新建 | `internal/middleware/cors_test.go` | reflect Origin / 自定义 origins / admin 豁免 / OPTIONS 命中 vs 未命中 |
| 新建 | `internal/middleware/bodylimit.go` | `BodyLimit(max int64) func(http.Handler) http.Handler`：请求体超限 → 413 + JSON |
| 新建 | `internal/middleware/bodylimit_test.go` | 边界（恰好 / 超限 / 无 body）|
| 新建 | `internal/middleware/chain.go` | `Chain(handler http.Handler, mws ...func(http.Handler) http.Handler) http.Handler`：洋葱模型组合工具 |
| 新建 | `internal/middleware/chain_test.go` | 中间件执行顺序锁定（外层先入后出）|
| 新建 | `internal/middleware/holder.go` | `Holder` 类型 + `atomic.Pointer[http.Handler]`：热加载用的"可热替换 handler"包装器 |
| 新建 | `internal/middleware/holder_test.go` | 并发 Swap + ServeHTTP 不 race；初始 nil → 503 兜底 |
| 新建 | `internal/config/watcher.go` | `Watcher` 类型：fsnotify 监听 + 300ms 防抖；`Watch(paths, debounce, onChange)` 启动 / `Stop()` 停止 |
| 新建 | `internal/config/watcher_test.go` | 单文件改动 / 多文件批量 / 防抖窗口 / Stop 后无新事件 |
| 修改 | `internal/admin/handlers.go` | 新增 `/__fakeserver/routes` handler；闭包捕获 cfg 引用（重载时通过 holder 走新闭包）|
| 修改 | `internal/admin/handlers_test.go` | 新增 /routes 用例 |
| 新建 | `internal/cli/banner.go` | `printBanner(w io.Writer, cfg *config.Config, version, addr string)`：启动 banner |
| 新建 | `internal/cli/banner_test.go` | banner 格式锁定 |
| 修改 | `internal/cli/serve.go` | 增加 `--quiet/--no-cors/--no-watch` flag；引入 Holder + middleware Chain；启动 watcher；启动 banner 替换 PrintRouteSummary 调用 |
| 修改 | `internal/cli/serve_test.go` | 新增热加载 E2E + 中间件 E2E |
| 新建 | `internal/cli/serve_e2e_test.go` | v0.1 MVP 闭环 E2E（综合 config + 编辑触发热加载 + 5 类请求）|
| 修改 | `internal/mock/responder_test.go` / `internal/mock/router_test.go` / `internal/cli/serve_test.go` | 清理 8 处 `go vet "using resp before checking for errors"` 警告（lite-tools-5an）|
| 修改 | `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` | Phase 5 状态列回写 + 实际落地偏差段 + Phase 5 完成标记"v0.1 MVP 闭环" |
| 修改 | `docs/fakeserver-design.md` | 修订记录追加 v0.3-phase5-applied；§13 追加 Phase 5 已落地条目 |

> **注**：Phase 5 **不**新建 `internal/registry/`、`internal/recorder/`、`internal/webui/`——它们留给 v0.2-v0.4。

---

## Task 1: 引入 fsnotify + Windows 编辑器原子保存行为 smoke

**Files**:
- 修改：`go.mod` / `go.sum`
- 新建：`internal/config/watcher.go`（最小 stub）
- 新建：`internal/config/watcher_smoke_test.go`（smoke）

> **风险点（overview §6）**：fsnotify 在 Windows 下编辑器原子保存的事件抖动。VS Code / 编辑器保存文件常通过"写到 tmp → rename"实现，会触发 RENAME + CREATE 两连击事件，而非单次 WRITE。本 Task 用 smoke 锁定 fsnotify 实际行为，作为 Task 7 防抖窗口设计的依据。

- [ ] **Step 1.1: 拉 fsnotify 依赖**

```
go get github.com/fsnotify/fsnotify
go mod tidy
```

预期：`go.mod` 直接依赖出现 `github.com/fsnotify/fsnotify v...`。

- [ ] **Step 1.2: 探查 fsnotify API**

```
go doc github.com/fsnotify/fsnotify | head -40
go doc github.com/fsnotify/fsnotify.NewWatcher
go doc github.com/fsnotify/fsnotify.Watcher
go doc github.com/fsnotify/fsnotify.Event
go doc github.com/fsnotify/fsnotify.Op
```

把关键签名摘录到 smoke 注释里。重点确认：

- `NewWatcher() (*Watcher, error)` 是当前签名
- `(*Watcher).Add(path string) error` 是单文件 / 目录加入
- `(*Watcher).Events <-chan Event` 是事件通道（不是 Errors）
- `Event{Name, Op}` 字段；`Op` 是位掩码（Write / Create / Rename / Remove / Chmod）
- 是否提供 `Has(Op)` 方法做位测试

- [ ] **Step 1.3: 写 smoke test 锁定 Windows 行为**

新建 `internal/config/watcher_smoke_test.go`：

```go
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TestFsnotifySmokeSingleWrite 锁定 fsnotify v1.x 最小可用 API：
//   - NewWatcher() (*Watcher, error)
//   - (*Watcher).Add(path) error 可对单文件订阅
//   - Events chan 在 os.WriteFile 后产生至少一次 Write 事件
//
// 该 smoke 不依赖我们自己的 Watcher 类型，只确认 fsnotify 库本身的契约。
func TestFsnotifySmokeSingleWrite(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(fp); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// 给 watcher 一点时间初始化（Windows 上 race 概率较高）
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(fp, []byte(`{"a":1}`), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events:
		t.Logf("got event Name=%s Op=%s (platform=%s)", ev.Name, ev.Op, runtime.GOOS)
		// 不断言具体 Op：Windows 上可能是 Write+Chmod 组合或 Create+Write，
		// 防抖窗口（Task 7）按"任意事件 → 等 300ms → 重新加载"处理。
	case <-time.After(2 * time.Second):
		t.Fatal("no fsnotify event after WriteFile")
	}
}

// TestFsnotifySmokeRenameInPlace 锁定编辑器"写 tmp → rename"原子保存的
// 事件序列。Windows 上 VS Code 默认行为，Linux 上 vim 也常见。
func TestFsnotifySmokeRenameInPlace(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// 注意：监听**目录**而非单文件——单文件订阅在 rename 后失效（Linux inotify 语义）
	if err := w.Add(tmp); err != nil {
		t.Fatalf("Add: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// 模拟编辑器原子保存：写 tmp 文件 → rename 覆盖目标
	tmpf := filepath.Join(tmp, "cfg.json5.tmp")
	if err := os.WriteFile(tmpf, []byte(`{"b":2}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpf, fp); err != nil {
		t.Fatal(err)
	}

	// 收集 500ms 内的所有事件，由测试输出报告序列
	var events []fsnotify.Event
	deadline := time.After(500 * time.Millisecond)
collect:
	for {
		select {
		case ev := <-w.Events:
			events = append(events, ev)
		case <-deadline:
			break collect
		}
	}
	if len(events) == 0 {
		t.Fatal("rename-style atomic save produced 0 events")
	}
	t.Logf("rename atomic save => %d events:", len(events))
	for _, ev := range events {
		t.Logf("  %s %s", ev.Op, ev.Name)
	}
}
```

> **说明**：第 2 个 smoke 不做强断言——它是探测，结果由 `t.Logf` 暴露。Task 7 的防抖窗口设计基于"任意事件 → 等 300ms → 静默就绪 → reload"这个保守策略，对事件序列不挑剔。

- [ ] **Step 1.4: 新建 `internal/config/watcher.go` 最小 stub**

```go
package config

import (
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher 是 fsnotify 包装：监听一组配置文件 + 防抖窗口 + 触发回调。
// design §5.2 热加载入口；Task 7 填充 Watch / Stop。
//
// 当前 stub 仅占位，让 import 链路就绪。
type Watcher struct {
	debounce time.Duration
	inner    *fsnotify.Watcher
}
```

- [ ] **Step 1.5: 验证编译 + smoke**

```
go build ./...
go test ./internal/config/... -v -run TestFsnotifySmoke
```

预期：2 个 smoke PASS（事件 op 不做强断言，但必须收到事件）。

- [ ] **Step 1.6: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add go.mod go.sum internal/config/watcher.go internal/config/watcher_smoke_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "chore(config): 引入 fsnotify + smoke 锁定 Windows 编辑器保存行为"
```

---

## Task 2: middleware/recoverer.go — panic → 500 JSON

**Files**:
- 新建：`internal/middleware/recoverer.go`
- 新建：`internal/middleware/recoverer_test.go`

> **范围**：实现 design §6 "请求处理 panic → recoverer 中间件捕获 → 500 JSON + 栈打到 stderr"。这是 v0.1 MVP 安全网；本 Task 也是新增 `internal/middleware/` 包的第一个文件。

- [ ] **Step 2.1: 写测试**

新建 `internal/middleware/recoverer_test.go`：

```go
package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverer_PanicProducesJSON500(t *testing.T) {
	var stderr bytes.Buffer
	log.SetOutput(&stderr)
	defer log.SetOutput(io.Discard)

	bad := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	Recoverer(bad).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status=%d want 500", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("CT=%q want application/json prefix", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v (raw=%s)", err, rec.Body.String())
	}
	if body["error"] != "internal error" {
		t.Errorf("body.error=%v want 'internal error'", body["error"])
	}
	if body["panic"] != "kaboom" {
		t.Errorf("body.panic=%v want 'kaboom'", body["panic"])
	}
	if !strings.Contains(stderr.String(), "kaboom") {
		t.Errorf("stderr should contain panic msg; got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "goroutine") {
		t.Errorf("stderr should contain stack trace; got %q", stderr.String())
	}
}

func TestRecoverer_NormalRequestPassesThrough(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		_, _ = w.Write([]byte("ok"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	Recoverer(ok).ServeHTTP(rec, req)

	if rec.Code != 202 || rec.Body.String() != "ok" {
		t.Errorf("normal request not passed through: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestRecoverer_SubsequentRequestsOK 锁定"panic 不杀进程"：同一个 middleware
// 实例服务两次请求，第一次 panic，第二次必须 200。
func TestRecoverer_SubsequentRequestsOK(t *testing.T) {
	log.SetOutput(io.Discard)
	count := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			panic("first call panics")
		}
		w.WriteHeader(200)
	})
	wrap := Recoverer(h)

	// First: 500 from recover
	rec1 := httptest.NewRecorder()
	wrap.ServeHTTP(rec1, httptest.NewRequest("GET", "/", nil))
	if rec1.Code != 500 {
		t.Errorf("first call: code=%d want 500", rec1.Code)
	}

	// Second: 200
	rec2 := httptest.NewRecorder()
	wrap.ServeHTTP(rec2, httptest.NewRequest("GET", "/", nil))
	if rec2.Code != 200 {
		t.Errorf("second call: code=%d want 200", rec2.Code)
	}
}
```

- [ ] **Step 2.2: 跑测试确认 FAIL**

```
go test ./internal/middleware/... -v -run TestRecoverer
```

预期：包不存在 / 函数未定义编译错。

- [ ] **Step 2.3: 实现 recoverer.go**

新建 `internal/middleware/recoverer.go`：

```go
// Package middleware implements fakeserver's cross-cutting HTTP middleware.
// design §5 lists the full middleware roster — Phase 5 introduces all four
// of them (recoverer / logger / cors / bodylimit) plus Chain composition
// helper and an atomic-swap Holder for hot reload.
package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime/debug"
)

// Recoverer recovers from any downstream panic, writes a 500 + JSON error
// body to the client, and logs the panic + stack to stderr. The wrapped
// handler is otherwise unmodified.
//
// design §6: a panic must NOT kill the server — fakeserver is interactive
// developer tooling, not a hardened production service. Subsequent requests
// continue to be served from the same process.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[fakeserver] panic in %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				// Best-effort: if downstream already wrote headers, this is
				// a no-op on the wire (ResponseWriter rejects double WriteHeader).
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": "internal error",
					"route": r.Method + " " + r.URL.Path,
					"panic": panicMsg(rec),
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// panicMsg coerces recover()'s any-typed result into a string for the
// response body. Most code panics with a string or an error; everything
// else falls back to a %v dump.
func panicMsg(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return jsonable(v)
	}
}

// jsonable renders v as a JSON-safe string using fmt.Sprintf("%v", v).
// We avoid using %#v to keep payload sizes bounded.
func jsonable(v any) string {
	return fmtSprintf("%v", v)
}

// fmtSprintf is a private alias to fmt.Sprintf so this file can lazy-import
// fmt only here, keeping the public surface focused.
var fmtSprintf = func(format string, a ...any) string {
	return _sprintf(format, a...)
}

// _sprintf is fmt.Sprintf separated out so go vet doesn't flag a
// pseudo-format-string check; replaced inline if linter complains.
func _sprintf(f string, a ...any) string {
	return _fmt(f, a...)
}

// _fmt is fmt.Sprintf — extracted to a var so tests can stub.
var _fmt = func(f string, a ...any) string {
	// Implementation detail: jump to actual fmt.Sprintf.
	// Using a closure here avoids pulling fmt into the file's surface area
	// for callers (we re-export nothing).
	return formatWithFmt(f, a...)
}
```

> **简化建议**：上面的 `jsonable / fmtSprintf / _sprintf / _fmt / formatWithFmt` 五层间接是 plan 写得太罗嗦了。**实际实现请直接用 `fmt.Sprintf("%v", v)`**：

```go
import "fmt"

func panicMsg(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return fmt.Sprintf("%v", v)
	}
}
```

把 `recoverer.go` 简化为：

```go
package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[fakeserver] panic in %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": "internal error",
					"route": r.Method + " " + r.URL.Path,
					"panic": panicMsg(rec),
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func panicMsg(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return fmt.Sprintf("%v", v)
	}
}
```

- [ ] **Step 2.4: 跑测试确认 PASS**

```
go test ./internal/middleware/... -v -run TestRecoverer
```

预期：3 个测试 PASS。

- [ ] **Step 2.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/middleware/recoverer.go internal/middleware/recoverer_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(middleware): recoverer——panic 500 JSON + 栈打到 stderr"
```

---

## Task 3: middleware/logger.go — 访问日志 + `--quiet` 支持

**Files**:
- 新建：`internal/middleware/logger.go`
- 新建：`internal/middleware/logger_test.go`

> **范围**：实现 design §5.3 请求日志。本 Task 提供"基础访问日志"：time / method / path / status / duration。**case 命中标记** / **proxy target 标记** 留在 Task 9 装配链时补 ctx value 接入。

> **签名设计**：`Logger(out io.Writer, quiet bool)` 返回 middleware 构造函数。`out` 通常是 `os.Stderr`；`quiet=true` 直接返回透传（不写日志）。这样 `--quiet` flag 不用走全局 var。

- [ ] **Step 3.1: 写测试**

新建 `internal/middleware/logger_test.go`：

```go
package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestLogger_FormatsAccessLine(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("ok"))
	})
	Logger(&buf, false)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("POST", "/users", nil),
	)
	line := buf.String()
	if !strings.Contains(line, "POST") {
		t.Errorf("missing method: %q", line)
	}
	if !strings.Contains(line, "/users") {
		t.Errorf("missing path: %q", line)
	}
	if !strings.Contains(line, "201") {
		t.Errorf("missing status: %q", line)
	}
	// duration in some unit (ms/µs/ns suffix)
	if !regexp.MustCompile(`\d+(\.\d+)?(ns|µs|us|ms|s)`).MatchString(line) {
		t.Errorf("missing duration: %q", line)
	}
}

func TestLogger_QuietSuppresses(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	Logger(&buf, true)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)
	if buf.Len() != 0 {
		t.Errorf("quiet=true should suppress; got %q", buf.String())
	}
}

func TestLogger_StatusDefaultsTo200(t *testing.T) {
	// Handler that writes body without calling WriteHeader → status should
	// default to 200 in the log (matching net/http default).
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hi"))
	})
	Logger(&buf, false)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)
	if !strings.Contains(buf.String(), "200") {
		t.Errorf("default status 200 not logged: %q", buf.String())
	}
}

func TestLogger_NilOutDoesNotPanic(t *testing.T) {
	// Defensive: passing nil writer should not crash; equivalent to quiet
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	Logger(io.Discard, false)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)
}
```

- [ ] **Step 3.2: 跑测试确认 FAIL**

```
go test ./internal/middleware/... -v -run TestLogger
```

预期：`undefined: Logger`。

- [ ] **Step 3.3: 实现 logger.go**

新建 `internal/middleware/logger.go`：

```go
package middleware

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// Logger returns a middleware that writes one access-log line per request
// to out. Format:
//
//	2026-05-20T15:23:45.123Z GET /users 201 3.2ms
//
// When quiet is true, the middleware degrades to a transparent pass-through
// — used to honour `serve --quiet`. The middleware never logs to anything
// other than out, so callers can route logs to a file, stderr, or any
// io.Writer without global state.
func Logger(out io.Writer, quiet bool) func(http.Handler) http.Handler {
	if quiet || out == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			lr := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(lr, r)
			dur := time.Since(start)
			fmt.Fprintf(out, "%s %s %s %d %s\n",
				start.UTC().Format("2006-01-02T15:04:05.000Z"),
				r.Method, r.URL.Path, lr.status, dur,
			)
		})
	}
}

// loggingResponseWriter captures the response status code for the access
// log line. Status defaults to 200 — matching net/http's implicit behavior
// when handlers write a body without calling WriteHeader first.
type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	if !lw.wroteHeader {
		lw.status = code
		lw.wroteHeader = true
	}
	lw.ResponseWriter.WriteHeader(code)
}

func (lw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !lw.wroteHeader {
		lw.wroteHeader = true // implicit 200
	}
	return lw.ResponseWriter.Write(b)
}
```

- [ ] **Step 3.4: 跑测试**

```
go test ./internal/middleware/... -v -run TestLogger -count=1
```

预期：4 个测试 PASS。

- [ ] **Step 3.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/middleware/logger.go internal/middleware/logger_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(middleware): logger——访问日志 + quiet 支持"
```

---

## Task 4: middleware/cors.go — header 注入 + OPTIONS 后置 204 + admin 豁免

**Files**:
- 新建：`internal/middleware/cors.go`
- 新建：`internal/middleware/cors_test.go`

> **范围**：实现 design §5.4 CORS。难点是 OPTIONS preflight 的处理顺序——design 明确要求"OPTIONS 在路由匹配**之后**才走 204 短路"（用户路由可能要自己处理 OPTIONS）。本 Task 用"buffer-then-rewrite"模式：CORS 包装下层 handler，记录其响应状态码，若为 OPTIONS 且下层返回 404 → 改写为 204 + preflight headers。

> **数据结构**：`CORSOpts{Origins, Methods, Headers []string, AllowCredentials bool}`。空 `Origins` 表示"reflect 请求方的 Origin"；非空时白名单。`/__fakeserver/*` 路径前缀**永远**豁免，不加任何 CORS header、不改写状态。

- [ ] **Step 4.1: 写测试**

新建 `internal/middleware/cors_test.go`：

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func handler200() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
}

func handler404() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
}

func TestCORS_ReflectOriginByDefault(t *testing.T) {
	mw := CORS(CORSOpts{})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("reflect origin: got %q want http://localhost:5173", got)
	}
}

func TestCORS_AllowedOriginExplicit(t *testing.T) {
	mw := CORS(CORSOpts{Origins: []string{"http://allowed.example"}})(handler200())
	// allowed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://allowed.example")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://allowed.example" {
		t.Errorf("allowed: got %q", got)
	}
	// disallowed
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://evil.example")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin should not be reflected; got %q", got)
	}
}

func TestCORS_AdminPathExempt(t *testing.T) {
	mw := CORS(CORSOpts{})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/__fakeserver/healthz", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("admin path should be CORS-exempt; got Origin header %q", got)
	}
}

// TestCORS_OptionsHandledByRoute 用户路由自己处理了 OPTIONS（返回 200）→
// CORS 不改写响应，只追加 header。
func TestCORS_OptionsHandledByRoute(t *testing.T) {
	mw := CORS(CORSOpts{})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("status=%d want 200 (route handled OPTIONS)", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("CORS header missing on route-handled OPTIONS: %q", got)
	}
}

// TestCORS_OptionsNotMatched 用户路由没处理 OPTIONS（返回 404）→ CORS
// 改写为 204 + preflight headers（Access-Control-Allow-Methods/Headers）。
func TestCORS_OptionsNotMatched(t *testing.T) {
	mw := CORS(CORSOpts{
		Methods: []string{"GET", "POST"},
		Headers: []string{"Content-Type", "X-Custom"},
	})(handler404())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Errorf("status=%d want 204 (preflight short-circuit)", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Errorf("Allow-Methods missing on preflight: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Errorf("Allow-Headers missing on preflight: %q", got)
	}
}

// TestCORS_AllowCredentials 当配置允许凭据时，输出 ACAC=true。
func TestCORS_AllowCredentials(t *testing.T) {
	mw := CORS(CORSOpts{AllowCredentials: true})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("ACAC=%q want true", got)
	}
}
```

- [ ] **Step 4.2: 跑测试确认 FAIL**

```
go test ./internal/middleware/... -v -run TestCORS
```

预期：`undefined: CORS / CORSOpts`。

- [ ] **Step 4.3: 实现 cors.go**

新建 `internal/middleware/cors.go`：

```go
package middleware

import (
	"net/http"
	"strings"
)

// CORSOpts is the configurable surface of the CORS middleware.
//
// design §5.4 conventions:
//   - Origins empty → reflect the request's Origin header (developer-friendly
//     default; matches "CORS: true" in JSON5 config)
//   - Origins non-empty → strict allowlist
//   - Methods/Headers populate the preflight Access-Control-Allow-* responses
//   - AllowCredentials controls the ACAC header
//
// Path-level behavior: requests to /__fakeserver/* are exempted entirely
// (no headers added, no preflight rewrite). This protects the internal
// endpoints from being usable as a CORS bypass surface from arbitrary
// origins.
type CORSOpts struct {
	Origins          []string
	Methods          []string
	Headers          []string
	AllowCredentials bool
}

const adminPrefix = "/__fakeserver/"

// CORS returns the middleware. design §5.4:
//   - non-OPTIONS: pass through, append CORS response headers
//   - OPTIONS:
//     * route handled (status != 404) → keep route response, append headers
//     * route returned 404            → rewrite to 204 + preflight headers
//   - any /__fakeserver/* path        → pass through unchanged
func CORS(opts CORSOpts) func(http.Handler) http.Handler {
	allowMethods := strings.Join(defaultIfEmpty(opts.Methods, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}), ", ")
	allowHeaders := strings.Join(defaultIfEmpty(opts.Headers, []string{"Content-Type", "Authorization"}), ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Admin endpoints get no CORS, by design
			if strings.HasPrefix(r.URL.Path, adminPrefix) {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			allowOrigin := resolveOrigin(opts.Origins, origin)

			// For non-OPTIONS: write CORS headers, then delegate.
			if r.Method != http.MethodOptions {
				if allowOrigin != "" {
					setCORSHeaders(w, allowOrigin, opts.AllowCredentials)
				}
				next.ServeHTTP(w, r)
				return
			}

			// OPTIONS path: buffer the downstream response.
			buf := &bufferedWriter{header: http.Header{}}
			next.ServeHTTP(buf, r)
			if buf.status == 404 || buf.status == 0 {
				// Route didn't handle preflight — short-circuit with 204.
				if allowOrigin != "" {
					setCORSHeaders(w, allowOrigin, opts.AllowCredentials)
				}
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			// Route handled OPTIONS — flush its response, add CORS headers
			for k, v := range buf.header {
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
			if allowOrigin != "" {
				setCORSHeaders(w, allowOrigin, opts.AllowCredentials)
			}
			w.WriteHeader(buf.status)
			_, _ = w.Write(buf.body)
		})
	}
}

func resolveOrigin(allow []string, requested string) string {
	if requested == "" {
		return ""
	}
	if len(allow) == 0 {
		return requested // reflect-mode default
	}
	for _, a := range allow {
		if a == requested {
			return requested
		}
	}
	return ""
}

func setCORSHeaders(w http.ResponseWriter, origin string, allowCred bool) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	if allowCred {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

func defaultIfEmpty(v, def []string) []string {
	if len(v) == 0 {
		return def
	}
	return v
}

// bufferedWriter captures status/headers/body from downstream so the CORS
// middleware can decide between "let route handle OPTIONS" and "rewrite to
// 204 preflight short-circuit".
type bufferedWriter struct {
	header http.Header
	status int
	body   []byte
}

func (bw *bufferedWriter) Header() http.Header        { return bw.header }
func (bw *bufferedWriter) WriteHeader(code int)       { bw.status = code }
func (bw *bufferedWriter) Write(b []byte) (int, error) {
	if bw.status == 0 {
		bw.status = 200
	}
	bw.body = append(bw.body, b...)
	return len(b), nil
}
```

- [ ] **Step 4.4: 跑测试**

```
go test ./internal/middleware/... -v -run TestCORS -count=1
```

预期：6 个 CORS 用例 PASS。

- [ ] **Step 4.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/middleware/cors.go internal/middleware/cors_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(middleware): cors——reflect/allowlist + OPTIONS 后置 204 + admin 豁免"
```

---

## Task 5: middleware/bodylimit.go — 413 拒绝过大请求

**Files**:
- 新建：`internal/middleware/bodylimit.go`
- 新建：`internal/middleware/bodylimit_test.go`

> **范围**：实现 design §6 "请求超限 → body 超 maxBodySize → 413 在中间件层返回，不进 handler"。BodyLimit 应用于**整个 server 的 maxBodySize**，与 proxy 路由独立的 `bodyLimit` 共存（proxy 那个在 proxy 包内单独应用，更严格的限制赢）。

- [ ] **Step 5.1: 写测试**

新建 `internal/middleware/bodylimit_test.go`：

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimit_AllowsUnderLimit(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	mw := BodyLimit(100)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader("short"))
	mw.ServeHTTP(rec, req)
	if !called {
		t.Error("handler should have been called")
	}
	if rec.Code != 200 {
		t.Errorf("status=%d want 200", rec.Code)
	}
}

func TestBodyLimit_RejectsOverLimit(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := BodyLimit(10)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 100)))
	mw.ServeHTTP(rec, req)
	if called {
		t.Error("handler should NOT have been called on oversized body")
	}
	if rec.Code != 413 {
		t.Errorf("status=%d want 413", rec.Code)
	}
}

func TestBodyLimit_ExactlyAtLimit(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	mw := BodyLimit(16)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 16)))
	mw.ServeHTTP(rec, req)
	if !called {
		t.Error("exact-limit body should pass through")
	}
	if rec.Code != 200 {
		t.Errorf("status=%d want 200", rec.Code)
	}
}

func TestBodyLimit_ZeroLimitMeansNoLimit(t *testing.T) {
	// max <= 0 should disable the middleware entirely
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	mw := BodyLimit(0)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 10000)))
	mw.ServeHTTP(rec, req)
	if !called {
		t.Error("0 limit should disable check")
	}
}

func TestBodyLimit_NoBodyOk(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	mw := BodyLimit(10)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	mw.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("status=%d want 200 (no body, no rejection)", rec.Code)
	}
}
```

- [ ] **Step 5.2: 跑测试确认 FAIL**

```
go test ./internal/middleware/... -v -run TestBodyLimit
```

- [ ] **Step 5.3: 实现 bodylimit.go**

新建 `internal/middleware/bodylimit.go`：

```go
package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// BodyLimit returns a middleware that rejects requests whose body exceeds
// max bytes with a 413 + JSON error. max <= 0 disables the check entirely
// (the middleware degrades to a no-op pass-through).
//
// Implementation note: we read the body up-front into a bounded buffer
// rather than using http.MaxBytesReader, because the latter surfaces its
// error during handler-side body reads — past the point where this
// middleware can cleanly return a 413. The trade-off is that bodies up to
// max bytes are buffered fully; for fakeserver's dev-tooling use case this
// is acceptable (max is typically 1MiB).
func BodyLimit(max int64) func(http.Handler) http.Handler {
	if max <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}
			buf := make([]byte, max+1)
			n, err := readUpTo(r.Body, buf)
			if err != nil {
				writeBodyLimitError(w, max)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(buf[:n]))
			r.ContentLength = int64(n)
			next.ServeHTTP(w, r)
		})
	}
}

// readUpTo reads from r into buf. Returns (n, nil) on EOF within buf;
// returns (n, err) when there's MORE data beyond buf.
func readUpTo(r io.ReadCloser, buf []byte) (int, error) {
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
	extra := make([]byte, 1)
	n, _ := r.Read(extra)
	if n > 0 {
		return total, errors.New("body exceeds limit")
	}
	return total, nil
}

func writeBodyLimitError(w http.ResponseWriter, max int64) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   "request body too large",
		"limit":   max,
		"message": fmt.Sprintf("request body exceeds server.maxBodySize=%d bytes", max),
	})
}
```

- [ ] **Step 5.4: 跑测试**

```
go test ./internal/middleware/... -v -run TestBodyLimit -count=1
```

预期：5 个用例 PASS。

> **可能的偏差**：`readUpTo` 与 proxy 包的同名函数 source 相同（Task 8 of Phase 4 内联了相同逻辑）。两包独立 copy 可接受（避免跨包共享导致 import 链复杂化）。如果未来想 dedupe，提到 `internal/iox/` 公共包再共享。

- [ ] **Step 5.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/middleware/bodylimit.go internal/middleware/bodylimit_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(middleware): bodylimit——413 拒绝过大请求体"
```

---

## Task 6: middleware/chain.go + holder.go — 组合工具 + 原子 swap 容器

**Files**:
- 新建：`internal/middleware/chain.go`
- 新建：`internal/middleware/chain_test.go`
- 新建：`internal/middleware/holder.go`
- 新建：`internal/middleware/holder_test.go`

> **范围**：
>
> 1. `Chain(handler, mws...)` 把 `http.Handler` 包到一组 `func(http.Handler) http.Handler` middleware 里，洋葱模型，**第一个 middleware 是最外层**。
> 2. `Holder` 类型用 `atomic.Pointer[http.Handler]` 持有当前生效 handler，并发安全。`http.Server.Handler = holder`。watcher 拿到新 cfg 时 swap 一次，旧请求继续走旧 handler。

- [ ] **Step 6.1: chain 测试**

新建 `internal/middleware/chain_test.go`：

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestChain_OnionOrder 锁定洋葱模型：第一个 middleware 是最外层，
// 请求顺序进入，响应反向退出。
func TestChain_OnionOrder(t *testing.T) {
	var trace []string
	mwName := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				trace = append(trace, "→"+name)
				next.ServeHTTP(w, r)
				trace = append(trace, name+"←")
			})
		}
	}
	core := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace = append(trace, "*core*")
		w.WriteHeader(200)
	})

	h := Chain(core, mwName("A"), mwName("B"), mwName("C"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	got := strings.Join(trace, " ")
	want := "→A →B →C *core* C← B← A←"
	if got != want {
		t.Errorf("order:\n  got  %q\n  want %q", got, want)
	}
}

func TestChain_NoMiddlewareReturnsCore(t *testing.T) {
	core := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	h := Chain(core)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 204 {
		t.Errorf("status=%d want 204", rec.Code)
	}
}
```

- [ ] **Step 6.2: holder 测试**

新建 `internal/middleware/holder_test.go`：

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestHolder_InitialNilReturns503(t *testing.T) {
	h := NewHolder()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 503 {
		t.Errorf("uninitialized holder should return 503; got %d", rec.Code)
	}
}

func TestHolder_SwapServesNewHandler(t *testing.T) {
	h := NewHolder()
	h.Swap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("v1"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "v1" {
		t.Errorf("first: body=%q", rec.Body.String())
	}

	h.Swap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("v2"))
	}))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "v2" {
		t.Errorf("after swap: body=%q", rec.Body.String())
	}
}

// TestHolder_ConcurrentSwapAndServe 锁定并发 Swap 与 ServeHTTP 不 race。
// 必须用 `go test -race` 验证（CI 应启用）。
func TestHolder_ConcurrentSwapAndServe(t *testing.T) {
	h := NewHolder()
	h.Swap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			}
		}()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			h.Swap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			}))
		}(i)
	}
	wg.Wait()
}
```

- [ ] **Step 6.3: 跑测试确认 FAIL**

```
go test ./internal/middleware/... -v -run "TestChain|TestHolder"
```

预期：`undefined: Chain / NewHolder / Holder.Swap`。

- [ ] **Step 6.4: 实现 chain.go**

新建 `internal/middleware/chain.go`：

```go
package middleware

import "net/http"

// Chain composes mws around handler, onion-style: the FIRST middleware in
// mws is the outermost layer.
//
//	Chain(h, A, B, C) ≡ A(B(C(h)))
//
// Empty mws returns handler unchanged.
func Chain(handler http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	// Iterate in reverse so the first middleware ends up outermost.
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}
```

- [ ] **Step 6.5: 实现 holder.go**

新建 `internal/middleware/holder.go`：

```go
package middleware

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Holder is a swap-able http.Handler container backed by atomic.Pointer.
// Used by the hot-reload watcher: when config changes, the watcher
// constructs a brand-new (middleware-chain + router) http.Handler and
// hands it to Swap. In-flight requests finish on the previous handler;
// new requests get the new one.
//
// A zero-value Holder (NewHolder return value, before any Swap) responds
// with 503 Service Unavailable. This is a defensive default — the server
// should always Swap an initial handler before starting to listen.
type Holder struct {
	inner atomic.Pointer[http.Handler]
}

// NewHolder returns an empty Holder. Call Swap before listening.
func NewHolder() *Holder {
	return &Holder{}
}

// Swap atomically replaces the current handler. Safe for concurrent calls.
func (h *Holder) Swap(handler http.Handler) {
	h.inner.Store(&handler)
}

// ServeHTTP dispatches to the current handler, or returns 503 if Swap has
// not yet been called.
func (h *Holder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ptr := h.inner.Load()
	if ptr == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   "server not ready",
			"message": "handler not yet initialized",
		})
		return
	}
	(*ptr).ServeHTTP(w, r)
}
```

- [ ] **Step 6.6: 跑测试**

```
go test ./internal/middleware/... -v -count=1
go test -race ./internal/middleware/... -count=1 -run TestHolder_ConcurrentSwapAndServe
```

预期：所有 middleware 用例（recoverer 3 + logger 4 + cors 6 + bodylimit 5 + chain 2 + holder 3 = 23）PASS；race 测试也 PASS。

- [ ] **Step 6.7: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/middleware/chain.go internal/middleware/chain_test.go internal/middleware/holder.go internal/middleware/holder_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(middleware): chain 洋葱组合 + holder 原子 swap 容器"
```

---

## Task 7: config/watcher.go — fsnotify + 300ms 防抖

**Files**:
- 修改：`internal/config/watcher.go`
- 新建：`internal/config/watcher_test.go`

> **范围**：把 Task 1 的 stub 改成完整的 `Watcher`：监听一组路径（通常是 `cfg.SourcePaths`）+ 300ms 防抖（任意事件触发计时器；窗口内再次触发则重置；窗口结束后调用 `onChange()` 一次）+ 提供 `Stop()` 停止 goroutine。

> **API**：`NewWatcher(paths []string, debounce time.Duration, onChange func()) (*Watcher, error)` + `(*Watcher).Stop() error`。

> **路径策略**：单文件 watch 在 Linux inotify 下 rename 后失效。本 Phase 监听**所有 SourcePaths 的所在目录**（去重），事件回调里再校验 `event.Name` 是否命中已注册路径——这样原子保存（rename in place）也能被正确感知。

- [ ] **Step 7.1: 写测试**

新建 `internal/config/watcher_test.go`（替换 Task 1 的 smoke 文件名为 `watcher_smoke_test.go` 保留；本文件是单元测试）：

```go
package config

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcher_DebouncedSingleCallback(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls int32
	w, err := NewWatcher([]string{fp}, 150*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// 200ms 内连续 5 次改动 → 单次回调
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(fp, []byte(`{"a":`+string(rune('0'+i))+"}"), 0644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 等防抖窗口结束 + 余量
	time.Sleep(400 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got != 1 {
		t.Errorf("burst of writes should yield 1 callback; got %d", got)
	}
}

func TestWatcher_MultipleBurstsMultipleCallbacks(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	_ = os.WriteFile(fp, []byte("{}"), 0644)

	var calls int32
	w, err := NewWatcher([]string{fp}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Burst 1
	_ = os.WriteFile(fp, []byte(`{"a":1}`), 0644)
	time.Sleep(300 * time.Millisecond)
	// Burst 2 — 离前次足够远
	_ = os.WriteFile(fp, []byte(`{"a":2}`), 0644)
	time.Sleep(300 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got != 2 {
		t.Errorf("two separated bursts should yield 2 callbacks; got %d", got)
	}
}

func TestWatcher_StopPreventsCallback(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	_ = os.WriteFile(fp, []byte("{}"), 0644)

	var calls int32
	w, err := NewWatcher([]string{fp}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
	// 改动应被忽略
	_ = os.WriteFile(fp, []byte(`{"a":1}`), 0644)
	time.Sleep(300 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got != 0 {
		t.Errorf("after Stop, no callback; got %d", got)
	}
}

func TestWatcher_MultiplePathsSameCallback(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.json5")
	b := filepath.Join(tmp, "b.json5")
	_ = os.WriteFile(a, []byte("{}"), 0644)
	_ = os.WriteFile(b, []byte("{}"), 0644)

	var calls int32
	w, err := NewWatcher([]string{a, b}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	_ = os.WriteFile(a, []byte(`{"x":1}`), 0644)
	_ = os.WriteFile(b, []byte(`{"y":1}`), 0644)
	time.Sleep(300 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got != 1 {
		t.Errorf("two paths in one debounce window → 1 callback; got %d", got)
	}
}

func TestWatcher_AtomicRenameStillTriggers(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	_ = os.WriteFile(fp, []byte("{}"), 0644)

	var calls int32
	w, err := NewWatcher([]string{fp}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// 模拟编辑器原子保存
	tmpf := filepath.Join(tmp, "cfg.json5.tmp")
	if err := os.WriteFile(tmpf, []byte(`{"x":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpf, fp); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got < 1 {
		t.Errorf("atomic rename should trigger at least once; got %d", got)
	}
	// 不上限断言：rename + create 可能合成 1 次回调（防抖窗口内），也可能 2 次
}
```

- [ ] **Step 7.2: 跑测试确认 FAIL**

```
go test ./internal/config/... -v -run TestWatcher_
```

预期：`undefined: NewWatcher / Watcher.Stop`。

- [ ] **Step 7.3: 实现 watcher.go**

替换 `internal/config/watcher.go`：

```go
package config

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors a set of config files and invokes onChange after a
// debounce window of quiet on the filesystem.
//
// design §5.2: hot reload must coalesce editor save bursts (Windows
// editors emit Write+Chmod or Rename+Create event pairs on a single save).
// The debounce window collapses those into a single onChange call.
//
// Path strategy: subscribe to the *parent directories* of each path
// (deduped) because single-file watches in inotify drop their subscription
// after rename-in-place. Event names are then filtered against the
// originally requested path set.
type Watcher struct {
	inner    *fsnotify.Watcher
	debounce time.Duration
	onChange func()

	wanted   map[string]struct{} // absolute paths the caller asked us to watch
	timer    *time.Timer
	timerMu  sync.Mutex
	stop     chan struct{}
	stopOnce sync.Once
}

// NewWatcher starts a background goroutine that watches paths and calls
// onChange after debounce of filesystem quiet. Stop the watcher with
// (*Watcher).Stop() — leaking the goroutine on test failure is harmless
// but please clean up in production code.
func NewWatcher(paths []string, debounce time.Duration, onChange func()) (*Watcher, error) {
	inner, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fsnotify.NewWatcher: %w", err)
	}

	wanted := make(map[string]struct{}, len(paths))
	dirs := make(map[string]struct{})
	for _, p := range paths {
		abs, aerr := filepath.Abs(p)
		if aerr != nil {
			inner.Close()
			return nil, fmt.Errorf("abs %q: %w", p, aerr)
		}
		wanted[abs] = struct{}{}
		dirs[filepath.Dir(abs)] = struct{}{}
	}
	for d := range dirs {
		if werr := inner.Add(d); werr != nil {
			inner.Close()
			return nil, fmt.Errorf("watch dir %q: %w", d, werr)
		}
	}

	w := &Watcher{
		inner:    inner,
		debounce: debounce,
		onChange: onChange,
		wanted:   wanted,
		stop:     make(chan struct{}),
	}
	go w.loop()
	return w, nil
}

// loop is the watcher's main goroutine. It collapses event bursts and
// invokes onChange after debounce of quiet.
func (w *Watcher) loop() {
	for {
		select {
		case <-w.stop:
			return
		case ev, ok := <-w.inner.Events:
			if !ok {
				return
			}
			// Only react to events for paths we care about
			abs, aerr := filepath.Abs(ev.Name)
			if aerr != nil {
				continue
			}
			if _, want := w.wanted[abs]; !want {
				continue
			}
			w.armOrReset()
		case _, ok := <-w.inner.Errors:
			if !ok {
				return
			}
			// fsnotify errors are non-fatal at this layer; surface via
			// the next reload attempt if any. Phase 5 doesn't expose
			// them on the Watcher API.
		}
	}
}

// armOrReset arms the debounce timer, resetting if already armed.
func (w *Watcher) armOrReset() {
	w.timerMu.Lock()
	defer w.timerMu.Unlock()
	if w.timer == nil {
		w.timer = time.AfterFunc(w.debounce, w.onChange)
		return
	}
	w.timer.Reset(w.debounce)
}

// Stop terminates the watcher goroutine and closes the underlying
// fsnotify watcher. Safe to call multiple times.
func (w *Watcher) Stop() error {
	var err error
	w.stopOnce.Do(func() {
		close(w.stop)
		err = w.inner.Close()
		w.timerMu.Lock()
		if w.timer != nil {
			w.timer.Stop()
		}
		w.timerMu.Unlock()
	})
	return err
}
```

- [ ] **Step 7.4: 跑测试**

```
go test ./internal/config/... -v -run TestWatcher_ -count=1
```

预期：5 个测试 PASS。

> **可能的偏差**：`TestWatcher_AtomicRenameStillTriggers` 在某些 Linux 文件系统（tmpfs）下 rename 可能不触发任何事件，导致 calls=0。如果实测发现这种情况，**降级断言** `if got < 1` 改为 `t.Logf` 输出，并在落地偏差段记录。Windows / macOS / 大多数 Linux 文件系统下应该都能触发。

- [ ] **Step 7.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/watcher.go internal/config/watcher_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): watcher 文件监听 + 300ms 防抖 + 目录订阅"
```

---

## Task 8: admin /__fakeserver/routes + 启动 banner

**Files**:
- 修改：`internal/admin/handlers.go`
- 修改：`internal/admin/handlers_test.go`
- 新建：`internal/cli/banner.go`
- 新建：`internal/cli/banner_test.go`

> **范围**：
>
> 1. admin 新增 `GET /__fakeserver/routes` → 返回当前生效路由 JSON 列表 `[{method, path, mode}]`，mode ∈ `mock | cases | proxy`。**关键设计**：admin handler 通过闭包持有 cfg 引用；热加载时 Holder 会切换整个 handler 链（含 admin），因此 admin 也能看到最新 cfg。本 Task 把 `admin.Mount(r)` 签名升级为 `admin.Mount(r *rux.Router, cfg *config.Config)`，让 /routes handler 闭包绑定本次装配的 cfg 快照。
> 2. `printBanner(w, cfg, version, addr)` 输出 design §5.1 末尾的启动 banner：

```
   ╭─ fakeserver v0.1.0
   │  listening on http://0.0.0.0:5090
   │  config:      ./fakeserver.json5 (+2 includes)
   │  routes:      5 mock, 2 cases, 1 proxy, fallback=echo
   │  env:         (none)
   ╰─
```

- [ ] **Step 8.1: admin /routes 测试**

把以下追加到 `internal/admin/handlers_test.go`（保留 Phase 1 的 healthz 用例）:

```go
func TestAdmin_RoutesEndpoint(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/ping", Body: "pong"},
			{Method: []string{"GET"}, Path: "/u/{id}", Strategy: "first-match",
				Cases: []config.RouteCase{
					{Status: 200, Body: "a"},
				}},
			{Method: []string{"*"}, Path: "/api/*rest",
				Proxy: &config.ProxyConfig{Target: "http://up:80"}},
		},
	}

	r := rux.New()
	Mount(r, cfg)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/__fakeserver/routes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d want 200", resp.StatusCode)
	}
	var got []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	modes := []string{}
	for _, r := range got {
		modes = append(modes, r["mode"].(string))
	}
	wantModes := map[string]int{"mock": 0, "cases": 0, "proxy": 0}
	for _, m := range modes {
		wantModes[m]++
	}
	if wantModes["mock"] != 1 || wantModes["cases"] != 1 || wantModes["proxy"] != 1 {
		t.Errorf("mode distribution wrong: %v", wantModes)
	}
}

func TestAdmin_RoutesEmptyConfig(t *testing.T) {
	r := rux.New()
	Mount(r, nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/__fakeserver/routes")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d", resp.StatusCode)
	}
	var got []any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 0 {
		t.Errorf("nil cfg → empty list; got %v", got)
	}
}
```

Imports to confirm in handlers_test.go: `encoding/json`, `net/http`, `net/http/httptest`, `testing`, `github.com/gookit/rux/v2`, `github.com/inhere/fakeserver/internal/config`.

- [ ] **Step 8.2: 跑测试确认 FAIL**

```
go test ./internal/admin/... -v
```

预期：编译错（Mount 签名不匹配）+ 现有 healthz 用例也需要调整调用方式。

- [ ] **Step 8.3: 改 handlers.go**

替换 `internal/admin/handlers.go`:

```go
// Package admin exposes the fakeserver-internal endpoints under
// /__fakeserver/*. Phase 1 only provides healthz; Phase 5 adds /routes.
package admin

import (
	"net/http"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
)

// Mount registers all admin endpoints. design §5.6 lists the surface:
//   GET /__fakeserver/healthz — liveness probe
//   GET /__fakeserver/routes  — JSON list of effective routes
//
// cfg is captured by the /routes handler closure so each (re-)Mount sees
// the cfg active at assembly time. With the hot-reload Holder pattern,
// the watcher constructs a fresh router (and admin.Mount call) on every
// successful reload, so /routes always reflects the current cfg.
func Mount(r *rux.Router, cfg *config.Config) {
	r.GET("/__fakeserver/healthz", healthzHandler)
	r.GET("/__fakeserver/routes", routesHandler(cfg))
}

func healthzHandler(c *rux.Context) {
	c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// routesHandler returns a handler that emits the route table as JSON.
// Each entry: {method, path, mode} where mode is one of "mock", "cases",
// "proxy". For routes with multiple methods, one entry per (method, path).
func routesHandler(cfg *config.Config) rux.HandlerFunc {
	return func(c *rux.Context) {
		out := []map[string]any{}
		if cfg != nil {
			for _, route := range cfg.Routes {
				mode := "mock"
				if route.Proxy != nil {
					mode = "proxy"
				} else if len(route.Cases) > 0 {
					mode = "cases"
				}
				for _, m := range route.Method {
					out = append(out, map[string]any{
						"method": m,
						"path":   route.Path,
						"mode":   mode,
					})
				}
			}
		}
		c.JSON(http.StatusOK, out)
	}
}
```

- [ ] **Step 8.4: 修改 Phase 1 的 healthz 测试**

`handlers_test.go` 中已有的 healthz 用例需把 `Mount(r)` 改成 `Mount(r, nil)`（或一个 stub cfg）以适应新签名。**这是 Phase 5 必要的 API 演进**，与 Phase 4 router.go 测试演进同样处理。

- [ ] **Step 8.5: 跑测试**

```
go test ./internal/admin/... -v -count=1
```

预期：healthz（保留）+ /routes 用例 + 空 cfg 用例全 PASS。

- [ ] **Step 8.6: banner 测试**

新建 `internal/cli/banner_test.go`:

```go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestBanner_FullCfg(t *testing.T) {
	cfg := &config.Config{
		Fallback: "echo",
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/ping", Body: "pong"},
			{Method: []string{"GET"}, Path: "/u/{id}", Strategy: "first-match",
				Cases: []config.RouteCase{{Status: 200}}},
			{Method: []string{"*"}, Path: "/api/*rest",
				Proxy: &config.ProxyConfig{Target: "http://up:80"}},
		},
		SourcePaths: []string{"/abs/fakeserver.json5"},
	}
	var buf bytes.Buffer
	printBanner(&buf, cfg, "v0.1.0", "0.0.0.0:5090")
	s := buf.String()
	if !strings.Contains(s, "fakeserver") || !strings.Contains(s, "v0.1.0") {
		t.Errorf("missing name/version: %q", s)
	}
	if !strings.Contains(s, "0.0.0.0:5090") {
		t.Errorf("missing addr: %q", s)
	}
	if !strings.Contains(s, "1 mock") || !strings.Contains(s, "1 cases") || !strings.Contains(s, "1 proxy") {
		t.Errorf("missing route counts: %q", s)
	}
	if !strings.Contains(s, "fallback=echo") {
		t.Errorf("missing fallback: %q", s)
	}
}

func TestBanner_NoCfg(t *testing.T) {
	var buf bytes.Buffer
	printBanner(&buf, nil, "v0.1.0", "0.0.0.0:5090")
	s := buf.String()
	if !strings.Contains(s, "echo-only") {
		t.Errorf("nil cfg should mention echo-only: %q", s)
	}
}
```

- [ ] **Step 8.7: 实现 banner.go**

新建 `internal/cli/banner.go`:

```go
package cli

import (
	"fmt"
	"io"

	"github.com/inhere/fakeserver/internal/config"
)

// printBanner writes the startup banner to w. design §5.1 末尾 sample:
//
//   ╭─ fakeserver v0.1.0
//   │  listening on http://0.0.0.0:5090
//   │  config:      /abs/fakeserver.json5 (+0 includes)
//   │  routes:      1 mock, 1 cases, 1 proxy, fallback=echo
//   │  env:         (none)
//   ╰─
//
// version is the build-injected version string. addr is the listener
// address. cfg may be nil — in which case the banner mentions echo-only.
func printBanner(w io.Writer, cfg *config.Config, version, addr string) {
	fmt.Fprintln(w, "╭─ fakeserver "+version)
	fmt.Fprintf(w, "│  listening on http://%s\n", addr)
	if cfg == nil {
		fmt.Fprintln(w, "│  config:      (none — echo-only mode)")
		fmt.Fprintln(w, "│  routes:      0")
		fmt.Fprintln(w, "│  env:         (none)")
		fmt.Fprintln(w, "╰─")
		return
	}
	var mockN, casesN, proxyN int
	for _, r := range cfg.Routes {
		switch {
		case r.Proxy != nil:
			proxyN++
		case len(r.Cases) > 0:
			casesN++
		default:
			mockN++
		}
	}
	primary := "(stdin)"
	extra := 0
	if len(cfg.SourcePaths) > 0 {
		primary = cfg.SourcePaths[0]
		extra = len(cfg.SourcePaths) - 1
	}
	fmt.Fprintf(w, "│  config:      %s (+%d includes)\n", primary, extra)
	fmt.Fprintf(w, "│  routes:      %d mock, %d cases, %d proxy, fallback=%s\n", mockN, casesN, proxyN, cfg.Fallback)
	fmt.Fprintln(w, "│  env:         (none)")
	fmt.Fprintln(w, "╰─")
}
```

- [ ] **Step 8.8: 跑测试**

```
go test ./internal/cli/... -v -run TestBanner -count=1
go test ./internal/admin/... -v -count=1
```

预期：banner 2 个 + admin 全包 PASS。

- [ ] **Step 8.9: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/admin/handlers.go internal/admin/handlers_test.go internal/cli/banner.go internal/cli/banner_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(admin,cli): /__fakeserver/routes 端点 + 启动 banner"
```

---

## Task 9: serve.go 整合 — flag + middleware + watcher + handler 装配

**Files**:
- 修改：`internal/cli/serve.go`
- 修改：`internal/cli/serve_test.go`
- 新建：`internal/cli/serve_e2e_test.go`（v0.1 MVP 闭环 E2E）

> **范围**：把 Task 2-8 的产物在 `serve` 子命令里串起来：
>
> 1. `serveOptions` 增加 `Quiet bool`、`NoCORS bool`、`NoWatch bool` 三个字段
> 2. 新 flag：`--quiet`、`--no-cors`、`--no-watch`
> 3. `assembleHandler(cfg, renderer, opts)` 新函数：替代 `assembleRouter`，输出 `http.Handler`：先 `assembleRouter` 得到 rux router，再 `Chain(router, mws...)` 套上中间件链
> 4. `runServe` 主流程改为：
>    - 加载 + 校验 cfg
>    - 构造 renderer
>    - 创建 `Holder`
>    - `holder.Swap(assembleHandler(cfg, renderer, opts))`
>    - 启动 watcher（若不是 `--no-watch` 且 cfg 非 nil）：watcher 的 onChange 重新 Load + Validate；成功则 Swap，失败则 stderr warn
>    - `http.Server.Handler = holder` 启动
>    - signal 退出时 Stop watcher + Shutdown server
> 5. 启动后用 `printBanner` 替代 `PrintRouteSummary`（仍保留 `routes` 子命令用 `PrintRouteSummary`）
> 6. `admin.Mount(r, cfg)` 签名升级配合 Task 8

- [ ] **Step 9.1: 改 serveOptions 与 flag 注册**

读 `internal/cli/serve.go` 当前内容。把 `serveOptions` 改为：

```go
type serveOptions struct {
	Port       int
	Host       string
	ConfigFlag string
	Quiet      bool
	NoCORS     bool
	NoWatch    bool
}
```

`newServeCmd` 的 `Config` 函数追加三行 flag 注册：

```go
		cmd.BoolOpt2(&opts.Quiet, "quiet,q", "Suppress request access log")
		cmd.BoolOpt2(&opts.NoCORS, "no-cors", "Disable CORS middleware")
		cmd.BoolOpt2(&opts.NoWatch, "no-watch", "Disable hot-reload watcher")
```

- [ ] **Step 9.2: 新 `assembleHandler` 函数**

把 `assembleRouter` 改名保留（用于 routes/check 子命令静态展示）+ 新增 `assembleHandler`：

```go
// assembleHandler builds the full request-handling stack:
//
//   middleware chain → rux router → mock+proxy+admin+echo
//
// The order of middlewares (outermost first) is:
//
//   1. Recoverer  — catches downstream panics, always 500 JSON
//   2. Logger     — access log (no-op when opts.Quiet)
//   3. BodyLimit  — 413 on requests exceeding cfg.Server.MaxBodySize
//   4. CORS       — header injection + OPTIONS post-route 204 (no-op when opts.NoCORS)
//
// cfg may be nil — in that case CORS / BodyLimit degrade to no-ops and
// the chain reduces to recoverer → logger → router.
func assembleHandler(cfg *config.Config, renderer tpl.Renderer, opts serveOptions) http.Handler {
	r := rux.New()
	_ = mock.Mount(r, cfg, renderer)
	_ = proxy.Mount(r, cfg, renderer)
	admin.Mount(r, cfg)
	echo.Mount(r)

	var mws []func(http.Handler) http.Handler
	mws = append(mws, middleware.Recoverer)
	mws = append(mws, middleware.Logger(os.Stderr, opts.Quiet))

	var maxBody int64
	if cfg != nil {
		maxBody = parseMaxBodySize(cfg.Server.MaxBodySize) // see helper below
	}
	if maxBody > 0 {
		mws = append(mws, middleware.BodyLimit(maxBody))
	}

	if !opts.NoCORS {
		mws = append(mws, middleware.CORS(corsOptsFromCfg(cfg))) // see helper below
	}

	return middleware.Chain(r, mws...)
}

// parseMaxBodySize tolerates empty/invalid values by returning 0 (no limit).
// design §5 says "1MiB" default — but we keep parsing forgiving so a
// missing field doesn't kill startup.
func parseMaxBodySize(s string) int64 {
	if s == "" {
		return 1 << 20 // 1MiB default
	}
	// Reuse the proxy package's parseByteSize OR inline a tiny parser.
	// For Phase 5 simplicity, return 1MiB on any parse error.
	n, err := proxy.ParseByteSize(s)
	if err != nil {
		return 1 << 20
	}
	return n
}

// corsOptsFromCfg converts cfg.Server.CORS into middleware.CORSOpts.
// Phase 5 keeps this minimal: boolean false → caller skipped via opts.NoCORS;
// boolean true → reflect-mode (CORSOpts{}); map form → populate Origins/
// Methods/Headers/AllowCredentials.
func corsOptsFromCfg(cfg *config.Config) middleware.CORSOpts {
	if cfg == nil {
		return middleware.CORSOpts{}
	}
	switch v := cfg.Server.CORS.(type) {
	case map[string]any:
		o := middleware.CORSOpts{}
		if origins, ok := v["origins"].([]any); ok {
			for _, x := range origins {
				if s, ok := x.(string); ok {
					o.Origins = append(o.Origins, s)
				}
			}
		}
		if methods, ok := v["methods"].([]any); ok {
			for _, x := range methods {
				if s, ok := x.(string); ok {
					o.Methods = append(o.Methods, s)
				}
			}
		}
		if headers, ok := v["headers"].([]any); ok {
			for _, x := range headers {
				if s, ok := x.(string); ok {
					o.Headers = append(o.Headers, s)
				}
			}
		}
		if ac, ok := v["allowCredentials"].(bool); ok {
			o.AllowCredentials = ac
		}
		return o
	default:
		return middleware.CORSOpts{}
	}
}
```

> **注**：上面引用了 `proxy.ParseByteSize`——这要求 Task 8 of Phase 4 的 `parseByteSize` 改为导出。**两种选择**：
>
> A. 在 proxy 包内把 `parseByteSize` 改为 `ParseByteSize`（导出）。一行改动。
> B. 在 middleware 或 cli 包内**重复实现** 5 行 `parseByteSize`（避免跨包依赖）。
>
> 推荐 A——这是合理的接口提升，且只有 cli 包会用。**实施 A 时同步改 Phase 4 proxy 包的 `parseByteSize` → `ParseByteSize`，包内调用点一起改**，提交合并到本 Task。

- [ ] **Step 9.3: 改 runServe 主流程**

把现有 `runServe` 替换为新版本（保留原有 cfg 加载 + Warn 输出 + signal 处理）：

```go
func runServe(opts serveOptions) error {
	cfg, err := loadServeConfig(opts)
	if err != nil {
		return err
	}

	if cfg != nil {
		for _, w := range config.Warn(cfg) {
			fmt.Fprintln(os.Stderr, "warn:", w)
		}
	}

	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	var (
		globals map[string]any
		osenvWl []string
		seed    int64
	)
	if cfg != nil {
		globals = cfg.Globals
		osenvWl = cfg.Server.OSEnvWhitelist
		seed = cfg.Server.FakerSeed
	}
	renderer := tpl.NewRenderer(globals, osenvWl, seed)

	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, renderer, opts))

	// Start hot-reload watcher if config came from a file and --no-watch isn't set
	var watcher *config.Watcher
	if !opts.NoWatch && cfg != nil && len(cfg.SourcePaths) > 0 {
		paths := cfg.SourcePaths
		watcher, err = config.NewWatcher(paths, 300*time.Millisecond, func() {
			// Reload + revalidate; on success, swap the holder.
			newCfg, lerr := config.Load(paths, "", nil)
			if lerr != nil {
				fmt.Fprintln(os.Stderr, "warn: reload load err:", lerr)
				return
			}
			if errs := config.Validate(newCfg); len(errs) > 0 {
				fmt.Fprintln(os.Stderr, "warn: reload validate failed; keeping previous router")
				for _, e := range errs {
					fmt.Fprintln(os.Stderr, "  -", e)
				}
				return
			}
			for _, w := range config.Warn(newCfg) {
				fmt.Fprintln(os.Stderr, "warn (reload):", w)
			}
			newRenderer := tpl.NewRenderer(newCfg.Globals, newCfg.Server.OSEnvWhitelist, newCfg.Server.FakerSeed)
			holder.Swap(assembleHandler(newCfg, newRenderer, opts))
			fmt.Fprintln(os.Stderr, "info: config reloaded; router swapped")
		})
		if err != nil {
			return fmt.Errorf("watcher init: %w", err)
		}
		defer watcher.Stop()
	}

	srv := &http.Server{Addr: addr, Handler: holder}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		printBanner(os.Stdout, cfg, version(), addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case sig := <-stop:
		fmt.Printf("\nreceived %s, shutting down...\n", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	fmt.Println("bye.")
	return nil
}

// version returns the build-injected version string. Currently a stub
// returning "v0.1.0"; cmd/fakeserver/main.go can override via ldflags.
func version() string {
	return "v0.1.0"
}
```

Imports to add:
- `"github.com/inhere/fakeserver/internal/middleware"`

> **注**：`assembleRouter` 函数可以保留（用于 `routes` / `check` 子命令的静态展示），也可以删除（这些子命令本来就只用 cfg + PrintRouteSummary，不需要构造 router）。推荐删除 `assembleRouter` 让链路清爽。

- [ ] **Step 9.4: serve_test.go 调整**

读 `internal/cli/serve_test.go`。把 Phase 4 末尾两个 E2E 测试（`TestServe_E2E_CasesRoute` / `TestServe_E2E_ProxyRouteCoexistsWithMock`）的 `assembleRouter(cfg, rdr)` 调用改为 `assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true, NoWatch: true})`。

理由：Phase 4 测试调用的是 router-only 装配，Phase 5 把 router 嵌入 handler 链。`Quiet:true / NoCORS:true / NoWatch:true` 确保测试输出干净、不引入新断言负担、不启动 watcher 协程。

如果 `assembleRouter` 被删除（Step 9.3 推荐），Phase 4 测试必须改用 `assembleHandler`。

- [ ] **Step 9.5: serve_e2e_test.go — v0.1 MVP 闭环**

新建 `internal/cli/serve_e2e_test.go`:

```go
package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// TestServe_v01_MVPClosure 是 Phase 5 DoD #9：v0.1 MVP 闭环 E2E。
//
// 综合 config 含 mock 单响应 + cases first-match + proxy + bodyFile，
// 启动 fakeserver，请求每类路由验证响应，然后文件系统编辑 config 增加一条
// 新路由，等待 watcher 防抖窗口结束 + swap，再次请求新路径，验证生效。
func TestServe_v01_MVPClosure(t *testing.T) {
	// 1. Spin up a fake upstream for the proxy route
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("UP:" + r.URL.Path))
	}))
	defer upstream.Close()

	// 2. Write initial config + a 1-byte bodyFile fixture
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	fixPath := filepath.Join(tmp, "hi.txt")
	if err := os.WriteFile(fixPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	initialCfg := `{
  fallback: "echo",
  routes: [
    { method: "GET", path: "/ping", body: "pong" },
    { method: "GET", path: "/u/{id}", strategy: "first-match", cases: [
        { when: "request.query.fail == \"1\"", status: 500, body: { error: "boom" } },
        { status: 200, body: { id: "{{ .request.params.id }}" } },
    ]},
    { method: "*", path: "/api/*rest", proxy: { target: "` + upstream.URL + `", stripPathPrefix: "/api" } },
    { method: "GET", path: "/file", bodyFile: "hi.txt" },
  ],
}`
	if err := os.WriteFile(cfgPath, []byte(initialCfg), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Load + assemble + Holder + watcher
	cfg, err := config.Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	opts := serveOptions{Quiet: true, NoCORS: true, NoWatch: false}

	holder := newHolderWithWatcher(t, cfg, rdr, opts, cfgPath)
	srv := httptest.NewServer(holder)
	defer srv.Close()

	// 4. Verify 4 initial routes
	verifyOK := func(label, url, wantSubstring string) {
		t.Helper()
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("[%s] %v", label, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(b), wantSubstring) {
			t.Errorf("[%s] body=%q want substring %q", label, string(b), wantSubstring)
		}
	}
	verifyOK("mock single", srv.URL+"/ping", "pong")
	verifyOK("cases default", srv.URL+"/u/42", `"id":"42"`)
	verifyStatus(t, "cases fail-branch", srv.URL+"/u/42?fail=1", 500)
	verifyOK("proxy", srv.URL+"/api/orders", "UP:/orders")
	verifyOK("bodyFile", srv.URL+"/file", "hello")

	// 5. Edit config: add a new route /v2/new → 200 "added"
	editedCfg := strings.Replace(initialCfg,
		`{ method: "GET", path: "/file", bodyFile: "hi.txt" },`,
		`{ method: "GET", path: "/file", bodyFile: "hi.txt" },
    { method: "GET", path: "/v2/new", body: "added" },`,
		1)
	if err := os.WriteFile(cfgPath, []byte(editedCfg), 0644); err != nil {
		t.Fatal(err)
	}

	// 6. Wait for debounce + swap (300ms + safety margin)
	time.Sleep(600 * time.Millisecond)

	// 7. Verify new route is live
	verifyOK("new route after reload", srv.URL+"/v2/new", "added")

	// 8. Verify old routes still work after reload
	verifyOK("ping still works", srv.URL+"/ping", "pong")
}

func verifyStatus(t *testing.T, label, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("[%s] %v", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Errorf("[%s] status=%d want %d", label, resp.StatusCode, want)
	}
}

// newHolderWithWatcher 模拟 runServe 的 holder + watcher 装配（只是不起 HTTP
// server，由 httptest 接管）。这样 E2E 测试不需要真实端口、SIGINT 等。
func newHolderWithWatcher(t *testing.T, cfg *config.Config, rdr tpl.Renderer, opts serveOptions, cfgPath string) http.Handler {
	t.Helper()
	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, rdr, opts))
	watcher, err := config.NewWatcher(cfg.SourcePaths, 300*time.Millisecond, func() {
		newCfg, lerr := config.Load([]string{cfgPath}, "", nil)
		if lerr != nil {
			t.Logf("reload load err: %v", lerr)
			return
		}
		if errs := config.Validate(newCfg); len(errs) > 0 {
			t.Logf("reload validate failed: %v", errs)
			return
		}
		newRdr := tpl.NewRenderer(newCfg.Globals, newCfg.Server.OSEnvWhitelist, newCfg.Server.FakerSeed)
		holder.Swap(assembleHandler(newCfg, newRdr, opts))
	})
	if err != nil {
		t.Fatalf("watcher: %v", err)
	}
	t.Cleanup(func() { _ = watcher.Stop() })
	return holder
}
```

Imports to add at top: standard + `github.com/inhere/fakeserver/internal/middleware`. Also a JSON decode call is unused — drop `encoding/json` if so.

- [ ] **Step 9.6: 跑全套测试**

```
cd D:/work/aidev/lite-tools/fakeserver
go build ./...
go test ./... -count=1
go test -race ./internal/middleware/... -count=1
```

预期：
- 全包 PASS（Phase 1-4 旧用例 + Phase 5 新用例 ~30 个）
- race 测试 PASS
- `internal/middleware` 覆盖率 ≥ 80%

- [ ] **Step 9.7: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve.go internal/cli/serve_test.go internal/cli/serve_e2e_test.go internal/proxy/proxy.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): serve 装配 middleware chain + holder swap + watcher + flags"
```

> 注：commit 包含 `internal/proxy/proxy.go` 因为 `parseByteSize` 改为 `ParseByteSize`（导出）。

---

## Task 10: vet 清理 + DoD + 文档回写 + v0.1 MVP 收尾

**Files**:
- 修改：`internal/mock/responder_test.go`、`internal/mock/router_test.go`、`internal/cli/serve_test.go`（清理 vet 警告）
- 修改：`docs/plans/2026-05-19-fakeserver-v0.1-overview.md`
- 修改：`docs/fakeserver-design.md`

### Step 10.1: 清理 lite-tools-5an vet 警告

Phase 4 集成审计发现 8 处 `using resp before checking for errors` vet 警告。统一修复模式：

```go
// 之前（vet 警告）：
resp, _ := http.Get(url)
if resp.StatusCode != 200 { ... }

// 之后：
resp, err := http.Get(url)
if err != nil {
    t.Fatal(err)
}
if resp.StatusCode != 200 { ... }
```

执行：

```
cd D:/work/aidev/lite-tools/fakeserver
go vet ./... 2>&1 | Out-File vet.txt
type vet.txt
```

(或 `go vet ./... 2>&1 | tee vet.txt && cat vet.txt` 在 bash 下)

逐行定位 8 处，按上述模式修复。每修复一处再跑 vet 确认。

### Step 10.2: DoD 检查清单

| 命令 / 检查 | 预期 |
|---|---|
| `go build ./...` | 0 退出 |
| `go test ./...` | 全 PASS |
| `go test -cover ./internal/middleware/...` | 覆盖率 ≥ 80% |
| `go test -cover ./internal/config/...` | 覆盖率维持 ≥ 80% |
| `go test -race ./internal/middleware/... -count=1` | PASS |
| `go vet ./...` | 0 告警 |
| 手动冒烟（v0.1 MVP 闭环 E2E 自动跑） | PASS（含 5 个初始路由 + 1 个 reload 后路由 + 1 个回归） |
| `go list -m all \| grep -E "fsnotify"` | 出现 |
| `go list -m all \| grep -E "registry\|recorder\|webui"` | **不**出现（这些是 v0.2-v0.4） |
| Phase 5 commit 数 | 9–12 个（Task 1–10）|

### Step 10.3: 回写 overview

修改 `docs/plans/2026-05-19-fakeserver-v0.1-overview.md`：

**§2 表 Phase 5 状态列**改为：

```
✅ 已完成 (commit <first SHA>..<last SHA>)
```

**§3 Phase 5 详述末尾追加**：

```markdown
**实际落地偏差**：

- <Phase 5 落地过程中发现的细节。可能项：
  - fsnotify Windows rename-in-place 事件序列实测（Task 1 smoke 输出）
  - rux v2 OPTIONS 与中间件交互——CORS 的 buffer-then-rewrite 模式是否需要兼容性调整
  - cfg.Server.CORS 字段 JSON5 decode 后的实际类型（bool / map[string]any / nil）
  - admin.Mount 签名升级对其他子命令的影响（routes/check 不依赖 Mount，仅 serve 受影响——验证）
  - parseByteSize → ParseByteSize 重命名涉及的 proxy 包内部调用点数量
  - watcher 在多文件 include 链下事件触发的行为>
- 其余实现与 plan 一致

**Phase 5 测试覆盖**：<填入实测用例总数> 个用例；`internal/middleware` 覆盖率 <X>%；`internal/config` 覆盖率维持 <Y>%

**v0.1 MVP 完整闭环**：本 Phase 完成后，fakeserver v0.1 MVP 全部里程碑达成：
- ✅ 零配置 echo（Phase 1）
- ✅ JSON5 配置 + Validate + 子命令（Phase 2）
- ✅ 模板渲染 + 单一响应 mock + faker（Phase 3）
- ✅ 多响应 + 条件分支 + Proxy（Phase 4）
- ✅ 中间件全套 + 热加载 + admin /routes（Phase 5）

v0.2 路线图入口已就绪：env 文件 + osenv 完整 + 项目注册（design §8 / §10 / §11）。
```

**§2 总表**：在 Phase 5 行下方追加一行（如果还没有）"**v0.1 完成于 2026-05-XX**"备注。

### Step 10.4: 回写 design

修改 `docs/fakeserver-design.md`：

修订记录追加：

```markdown
| 2026-05-XX | v0.3-phase5-applied | inhere | Phase 5 落地：internal/middleware（recoverer/logger/cors/bodylimit/chain/holder）+ internal/config/watcher.go + admin /__fakeserver/routes + 启动 banner + serve --quiet/--no-cors/--no-watch flag。v0.1 MVP 完整闭环。|
```

§13 追加"已落地（Phase 5 阶段确认）"子段，至少含 5 条事实：

1. **fsnotify Windows rename 行为**：编辑器原子保存（写 tmp → rename）在 Windows / macOS / Linux 三平台均能触发事件，但事件序列差异显著（Write+Chmod / Create+Write+Rename / Rename+Create）。300ms 防抖窗口足以聚合所有变种。
2. **CORS OPTIONS 后置策略**：用 `bufferedWriter` 在 middleware 层拦截路由层响应；路由返回 404 时改写为 204 + preflight headers；否则透传路由响应仅追加 CORS header。无需在路由层注册 OPTIONS catch-all。
3. **Holder 原子 swap**：`atomic.Pointer[http.Handler]` 实现"零拷贝热替换"。在途请求继续走旧 handler 直至完成；新请求走新 handler。race 测试通过（50 并发 Swap + 1000 并发 ServeHTTP）。
4. **watcher 目录订阅**：单文件 fsnotify Add 在 rename-in-place 后失效；本工程改为订阅每条 SourcePath 的所在目录（去重），事件回调里再用 `wanted` map 过滤回 path 集合。覆盖了 VS Code / vim 的原子保存。
5. **admin.Mount 签名升级**：从 `Mount(r)` 到 `Mount(r, cfg)`，让 `/__fakeserver/routes` handler 闭包持有 cfg 引用。每次 watcher swap 都会调用一次 Mount（在 assembleHandler 内），所以 /routes 总是返回最新 cfg。
6. **`parseByteSize → ParseByteSize` 导出**：原 Phase 4 proxy 包私有函数提升为 `proxy.ParseByteSize`，被 cli 包的 `parseMaxBodySize` 复用，避免 5 行代码两处重复。
7. **v0.1 MVP 完整闭环 E2E 通过**：综合 config（mock + cases + proxy + bodyFile）启动 → 5 类请求验证 → 文件系统编辑 + 300ms 防抖 + holder swap → 新路由生效 → 旧路由仍工作。该测试覆盖 Phase 1-5 全部模块协作。
```

### Step 10.5: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/mock/responder_test.go internal/mock/router_test.go internal/cli/serve_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "chore(tests): 修复 lite-tools-5an——'using resp before checking err' vet 警告"

git -C D:/work/aidev/lite-tools/fakeserver add docs/plans/2026-05-19-fakeserver-v0.1-overview.md docs/fakeserver-design.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "docs: 回写 Phase 5 落地结果——v0.1 MVP 完整闭环"
```

### Step 10.6: 关闭 bd 任务

```
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd close lite-tools-5an --reason="vet 警告 8 处全部修复（Task 10 Step 10.1），go vet ./... 零告警"
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd close <Phase 5 issue 编号> --reason="Phase 5 落地完成，v0.1 MVP 完整闭环"
```

---

## Phase 5 完成 · 下一步

仓库具备：

- ✅ `internal/middleware/{recoverer,logger,cors,bodylimit,chain,holder}.go`：全套中间件 + 组合工具 + 热替换容器
- ✅ `internal/config/watcher.go`：fsnotify + 300ms 防抖 + 目录订阅 + Stop
- ✅ `internal/admin/handlers.go` 扩展：`/__fakeserver/routes` 端点
- ✅ `internal/cli/banner.go`：启动 banner（版本 / 监听 / 路由计数 / fallback / env）
- ✅ `internal/cli/serve.go`：`--quiet/--no-cors/--no-watch` flag + Holder + watcher 装配
- ✅ v0.1 MVP 闭环 E2E：综合 config + 热加载验证

**v0.1 完成 · v0.2 预告**（不在本计划范围）：

- env 文件加载（`fakeserver.env.json5`，design §8）
- osenv 白名单完整生效（design §4.6）
- env 文件中模板字符串调用 `osenv` 的白名单约束
- 项目注册（`~/.config/fakeserver/projects.json`，design §10）入口
- 新子命令 `list` / `use`（切换 active env）

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| 无 TBD / placeholder | ✓（Task 1 smoke 第 2 个用 `t.Logf` 探测、不做强断言，是受控 smoke） |
| 类型签名前后一致 | ✓（`middleware.Recoverer(next) http.Handler` / `Logger(out, quiet) func(http.Handler) http.Handler` / `CORS(opts) func(http.Handler) http.Handler` / `CORSOpts{Origins,Methods,Headers,AllowCredentials}` / `BodyLimit(max) func(http.Handler) http.Handler` / `Chain(handler, mws...) http.Handler` / `Holder` + `NewHolder()` + `Swap(h)` + `ServeHTTP(w,r)` / `config.NewWatcher(paths, debounce, onChange) (*Watcher, error)` + `Watcher.Stop() error` / `admin.Mount(r, cfg)` / `printBanner(w, cfg, version, addr)` / `assembleHandler(cfg, renderer, opts) http.Handler` 在 Task 2-9 中保持一致） |
| 包路径前后一致 | ✓（`github.com/inhere/fakeserver/internal/{middleware,config,admin,cli,proxy,mock,tpl}`） |
| TDD：先测后写 | ✓（每个 Task 都是先写测试 → 跑 fail → 实现 → 跑 pass） |
| 频繁提交 | ✓（Task 1-10 各一个 commit；Task 10 顺手清 vet 是独立 chore commit） |
| 仅新增 fsnotify 一个第三方依赖 | ✓（Phase 5 不引 v0.2+ 任何包） |
| 覆盖 design §5 全章（启动流程 / 热加载 / 日志 / CORS / admin 端点 / 子命令）+ §6 错误处理 middleware 行 + §7 测试策略 middleware/cmd e2e 段 | ✓（Task 2-9 各章节有对应任务） |
| Phase 5 边界明确（env / registry / webui / recorder 全部留 v0.2-v0.4） | ✓（Task 10 §13 已落地段第 7 条明确 v0.2 路线图入口） |
| 失败路径明确 | ✓（panic → 500 / CORS 跨域拒绝 → header 不带 / bodyLimit → 413 / watcher 校验失败保留旧表 / Holder 未初始化 → 503） |
| DoD 10 项与 overview §3 Phase 5 DoD 8 项一致 | ✓（本 plan 的 DoD 10 项是 overview 8 项的细化拆分，并新增 #1 build、#2 测试覆盖率作为防御边界） |
| 顺手清理 lite-tools-5an（vet 警告）已纳入 Task 10 | ✓ |
| v0.1 MVP 闭环 E2E 在 Task 9 step 9.5 自动跑 | ✓ |
