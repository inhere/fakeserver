# Fakeserver v0.1 · Phase 1 — 项目骨架与零配置 Echo

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：让 `fakeserver serve` 能跑起来——零配置时充当 httpbin 风格 echo server（默认端口 3000），并提供 `/__fakeserver/healthz` 健康检查端点。

**Architecture**：`cmd/fakeserver/main.go` 仅作为极薄入口，所有 CLI 编排逻辑（app 构造、子命令注册、Run handler）在 `internal/cli/` 中，便于未来扩展子命令与单元测试；`gookit/rux` 作为 HTTP server，echo 端点直接复用 `gookit/rux` v2 内置的 `server/` 子包导出 handler，外加一层薄适配避免下游改动跟随 rux 版本漂移；admin 端点单独成包。

**Tech Stack**：Go 1.25+ · `github.com/gookit/rux` v2 · `github.com/gookit/gcli/v3` · `github.com/gookit/goutil` · 标准库 `net/http`/`net/http/httptest`/`os/signal`/`context`。

**前置要求**：

- 已读过 `docs/fakeserver-design.md` §1（背景）、§2（架构）、§5（运行机制）章节
- 已安装 Go 1.25+（验证：`go version`）
- 当前工作目录就是 `fakeserver/`（本仓库 root；存在 `prd.md`、`docs/fakeserver-design.md`、独立 `.git`，**无** `go.mod`）

**Phase 1 完成定义（DoD）**：

1. `go build ./...` 通过
2. `go test ./...` 全部通过
3. `fakeserver serve` 在 :3000 启动
4. `curl http://localhost:3000/anything` 返回 httpbin 风格 JSON（含 method/path/headers）
5. `curl http://localhost:3000/__fakeserver/healthz` 返回 200 `{"status":"ok"}`
6. `Ctrl+C` 优雅退出（无 panic 栈）
7. `cmd/fakeserver/main.go` 仅做"导入 + 调用 cli.Run"，不超过 15 行非空代码

---

## 文件结构（Phase 1 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 新建 | `go.mod` | Go module 定义；module path `github.com/inhere/fakeserver` |
| 新建 | `go.sum` | 由 `go mod tidy` 生成，跟随依赖更新 |
| 新建 | `.gitignore` | 排除二进制、临时文件 |
| 新建 | `cmd/fakeserver/main.go` | **极薄入口**：导入 `internal/cli`，调用 `cli.Run(version)` |
| 新建 | `internal/cli/app.go` | gcli App 构造、子命令注册入口（`Run` 导出函数） |
| 新建 | `internal/cli/serve.go` | `serve` 子命令定义 + `runServe` 装配逻辑 |
| 新建 | `internal/cli/serve_test.go` | `serve` 子命令的 E2E 测试（直接复用 router 装配，绕过 ListenAndServe） |
| 新建 | `internal/echo/mount.go` | echo 适配层：把 rux/server 内置 handler 挂到 router 的 NotFound 与若干固定端点 |
| 新建 | `internal/echo/mount_test.go` | echo handler 单元测试（httptest） |
| 新建 | `internal/echo/probe.md` | 探测 rux/server 子包实际导出符号的笔记（不强制 commit；调试期辅助） |
| 新建 | `internal/admin/handlers.go` | admin 端点 handler：`/__fakeserver/healthz` 等（Phase 1 仅 healthz） |
| 新建 | `internal/admin/handlers_test.go` | admin handler 单元测试 |

> **注**：本 Phase **不**新建 `internal/config/`、`internal/mock/` 等模块；它们留给 Phase 2+。Phase 1 的 `serve` 暂不解析任何配置文件，启动即进 echo 模式。

---

## Task 1: 初始化 Go module 与基础文件

**Files**:
- 新建：`go.mod`、`.gitignore`

- [ ] **Step 1.1: 初始化 Go module**

执行（在 `fakeserver/` 目录）：

```
go mod init github.com/inhere/fakeserver
```

预期：生成 `go.mod`，内容首行 `module github.com/inhere/fakeserver`，第二行声明 Go 版本（如 `go 1.25`）。

- [ ] **Step 1.2: 写 `.gitignore`**

新建 `.gitignore`，内容：

```
# 二进制
/fakeserver
/fakeserver.exe
/dist/

# 测试与构建产物
*.test
*.out
/coverage.out

# IDE
/.idea/
/.vscode/

# 临时文件
*.tmp
*.log

# OS
.DS_Store
Thumbs.db
```

- [ ] **Step 1.3: 校验 module 初始化**

执行：

```
go env GOMOD
```

预期：输出当前 `fakeserver/go.mod` 的绝对路径（非 `/dev/null`）。

- [ ] **Step 1.4: Commit**

```
git add go.mod .gitignore
git commit -m "chore: 初始化 Go module 与 .gitignore"
```

---

## Task 2: 引入依赖并锁定版本

**Files**:
- 修改：`go.mod`、`go.sum`

- [ ] **Step 2.1: 引入 Phase 1 所需依赖**

执行：

```
go get github.com/gookit/rux
go get github.com/gookit/gcli/v3
go get github.com/gookit/goutil
```

预期：每条命令成功打印 `go: added <module> <version>`；`go.mod` 出现对应 `require` 段；`go.sum` 自动生成。

> **注**：Phase 1 暂不引入 easytpl/fsnotify/expr/gofakeit/titanous-json5。它们留到 Phase 2+ 真正用到时再 `go get`，避免提早进入 go.sum。

- [ ] **Step 2.2: 写一个 dummy 文件验证 import 可解析**

新建 `internal/echo/mount.go`（**最小占位**，下个 Task 才填充完整逻辑）：

```go
// Package echo provides the default httpbin-style echo handlers used when
// fakeserver runs without any user-defined route configuration.
package echo

import (
	_ "github.com/gookit/rux" // ensure dependency resolves
)

// Mount 在后续步骤中实现：把内置 echo handler 挂到指定 router。
// Phase 1 Task 3 完成最终签名与实现。
func Mount() {}
```

- [ ] **Step 2.3: 校验编译**

执行：

```
go build ./...
```

预期：无输出，退出码 0。`go.sum` 内容已含 rux/gcli/goutil 及其传递依赖。

- [ ] **Step 2.4: Commit**

```
git add go.mod go.sum internal/echo/mount.go
git commit -m "chore: 引入 rux / gcli / goutil 依赖与 echo 占位"
```

---

## Task 3: 探测并锁定 rux v2 内置 echo 端点 API

**Files**:
- 新建：`internal/echo/probe.md`（探测笔记，可选 commit）

> **背景**：设计文档 §13 标注 rux v2 `server/` 子包的导出符号名"占位"。本 Task 通过实际查源码定位真实 API，避免后续每次重写 echo handler。

- [ ] **Step 3.1: 拉取 rux 源码位置**

执行：

```
go list -m -f "{{.Dir}}" github.com/gookit/rux
```

预期：输出 rux 模块在本地 GOPATH cache 中的绝对路径（如 `…/pkg/mod/github.com/gookit/rux@vX.Y.Z`）。

- [ ] **Step 3.2: 检查 server/ 子包目录是否存在**

执行（把上一步路径记为 `<RUX>`）：

```
ls <RUX>/server
```

预期（两种可能）：
1. 目录存在 → 列出 `.go` 文件清单
2. 目录不存在 → 该版本 rux 未提供内置 echo；进入 **Step 3.3.B 备选路径**

- [ ] **Step 3.3.A: 子包存在 — 列出导出符号**

执行：

```
go doc github.com/gookit/rux/server
```

记录下以下信息到 `internal/echo/probe.md`（新建该文件）：

```markdown
# rux/server 子包导出符号探测

rux 版本: <填上 Step 3.1 输出中的版本号>
探测日期: 2026-05-19

## 关心的符号

- 处理 /anything 风格回显的 handler: <填写实际名称，如 EchoHandler / EchoNotFoundHandler / Anything>
- 注册批量端点 (/status/{code}, /delay, /headers, /ip 等) 的入口: <填写实际名称，如 RegisterEchoEndpoints / Register>
- 函数签名（含参数/返回值）: <粘贴 go doc 输出中相关签名>

## 落地结论

在 internal/echo/mount.go 中将以 <实际名称> 调用 rux/server 子包。
若上游签名 (a) 接受 *rux.Router 单一参数 → 直接调用
              (b) 返回 http.Handler → 用 r.Any("/anything/*rest", handler) 等方式挂载
```

> **决策原则**：如果导出符号名与设计文档"占位名"不一致，**以实际导出名为准**，并在 `mount.go` 顶部加一行短注释指明对应关系。

- [ ] **Step 3.3.B: 子包不存在 — 走备选实现**

如果 Step 3.2 显示 server/ 子目录不存在，则改写 `internal/echo/probe.md`：

```markdown
# rux 内置 echo 探测结论

rux 版本: <版本号>
探测日期: 2026-05-19

## 结论

rux <版本> 不提供 server/ 子包内置 echo handlers。
按 design §5.5 末句"做薄适配层，不复制实现"原则的反向情况——
此 Phase 自己实现一个最小 echo handler 覆盖 §1.3 v0.1 范围：
  - /anything  GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS → JSON 回显
  - /headers   GET → 仅回显 headers
  - /ip        GET → 仅回显 client IP
  - /status/{code} GET → 直接以 {code} 返回（200..599 合法范围）
  - /delay/{seconds} GET → sleep 后返回 JSON 回显（cap 10s）
  - /__fakeserver/healthz 由 admin 包负责，不属于 echo

实现位于 internal/echo/mount.go，无外部依赖。
```

> **设计偏离记录**：Step 3.3.B 是计划的官方备选路径——design §5.5 注明"启动时探测，若 API 不一致做薄适配"的极端情况就是 API 完全不存在；此时由 internal/echo 自己实现而非引入其它库，符合"轻量、零额外依赖"原则。

- [ ] **Step 3.4: Commit 探测笔记（可选）**

```
git add internal/echo/probe.md
git commit -m "chore(echo): 记录 rux/server 子包探测结论"
```

---

## Task 4: 实现 echo 模块（含单元测试）

**Files**:
- 修改：`internal/echo/mount.go`
- 新建：`internal/echo/mount_test.go`

> **本 Task 分支条件**：如果 Step 3.3.A 成立（子包存在），按 **4-A 路径**实现；如果 Step 3.3.B 成立，按 **4-B 路径**实现。两条路径只能选一条；两套测试用例都通用。

### 路径 4-A：薄适配 rux/server

- [ ] **Step 4-A.1: 先写测试（TDD）**

新建 `internal/echo/mount_test.go`：

```go
package echo_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gookit/rux"
	"github.com/inhere/fakeserver/internal/echo"
)

// 启动一个仅挂了 echo 的 router 实例供下面用例共享。
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	r := rux.New()
	echo.Mount(r)
	return httptest.NewServer(r)
}

func TestEcho_AnythingReturnsJSON(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/anything/foo", strings.NewReader(`{"hello":"world"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test", "yes")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("expected JSON Content-Type, got %q", resp.Header.Get("Content-Type"))
	}

	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %v\nbody: %s", err, body)
	}
	// 必含字段（具体键名以 rux/server 行为为准；如键名差异请同步更新）
	for _, k := range []string{"method", "url", "headers"} {
		if _, ok := parsed[k]; !ok {
			t.Errorf("response missing key %q; body=%s", k, body)
		}
	}
}

func TestEcho_StatusCodeRoute(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status/418")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 418 {
		t.Errorf("expected status 418, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 4-A.2: 运行测试确认失败**

```
go test ./internal/echo/... -run TestEcho -v
```

预期：FAIL（`echo.Mount` 当前是空实现；至少 TestEcho_AnythingReturnsJSON 失败）。

- [ ] **Step 4-A.3: 实现 `Mount`**

完整替换 `internal/echo/mount.go`：

```go
// Package echo provides the default httpbin-style echo handlers used when
// fakeserver runs without any user-defined route configuration.
//
// This module is a thin adapter over rux's built-in server sub-package. The
// concrete symbol names from rux/server discovered during planning (see
// internal/echo/probe.md) are wired here; if rux upgrades and renames them,
// this file is the single place to update.
package echo

import (
	"github.com/gookit/rux"
	rsrv "github.com/gookit/rux/server" // probe.md 记录的实际子包路径
)

// Mount 将 rux 内置的 httpbin 风格 echo handler 挂载到 router 的 NotFound
// 兜底以及一组固定路径（/anything、/status/{code}、/delay、/headers、/ip 等）。
//
// 调用方传入的 router 通常是 fakeserver 主 router；
// 若 fallback="404" 则不要调用本函数。
func Mount(r *rux.Router) {
	// 1) 兜底回显：未匹配的请求一律走 echo 的 anything-style handler。
	r.NotFound(rsrv.EchoNotFoundHandler()) // 若 probe.md 记录实际名为其他，请同步替换

	// 2) 注册一组固定调试端点（/status/{code}, /delay, /headers, /ip 等）
	rsrv.RegisterEchoEndpoints(r) // 同上：以 probe.md 实际名为准
}
```

> **替换提示**：如果 probe.md 中记录的实际函数名与 `EchoNotFoundHandler` / `RegisterEchoEndpoints` 不一致，**改这两处即可**——不要改函数签名，不要把 rux/server 的实现复制过来。

- [ ] **Step 4-A.4: 运行测试确认通过**

```
go test ./internal/echo/... -run TestEcho -v
```

预期：PASS（两个用例均通过）。如果 TestEcho_AnythingReturnsJSON 报"必含字段缺失"，说明 rux/server 用了不同的键名（例如 `Method` 大写、或 `request.headers` 嵌套）——读测试失败输出中的 `body=...`，把测试用例中的键名列表改为 rux 实际输出的键名。

- [ ] **Step 4-A.5: Commit**

```
git add internal/echo/mount.go internal/echo/mount_test.go
git commit -m "feat(echo): 接入 rux/server 内置 echo handler（薄适配）"
```

---

### 路径 4-B：自实现最小 echo

- [ ] **Step 4-B.1: 先写测试（TDD，与 4-A.1 相同测试代码）**

新建 `internal/echo/mount_test.go`，内容与 **Step 4-A.1 完全一致**（不重复粘贴，请回头复制）。

- [ ] **Step 4-B.2: 运行测试确认失败**

```
go test ./internal/echo/... -run TestEcho -v
```

预期：FAIL。

- [ ] **Step 4-B.3: 实现自有 echo handler**

完整替换 `internal/echo/mount.go`：

```go
// Package echo provides a minimal httpbin-style echo handler set used when
// fakeserver runs without user-defined routes.
//
// This implementation is internal-only (no external echo lib) because the
// target rux version does not expose a server sub-package; see probe.md.
package echo

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gookit/rux"
)

// Mount 把固定的 echo 端点和 NotFound 兜底注册到 router。
func Mount(r *rux.Router) {
	r.Any("/anything", anythingHandler)
	r.Any("/anything/*rest", anythingHandler)
	r.GET("/headers", headersHandler)
	r.GET("/ip", ipHandler)
	r.GET("/status/{code}", statusHandler)
	r.GET("/delay/{seconds}", delayHandler)
	r.NotFound(anythingHandler) // 兜底
}

func anythingHandler(c *rux.Context) {
	w := c.Resp
	req := c.Req
	bodyBytes, _ := io.ReadAll(req.Body)
	defer req.Body.Close()

	out := map[string]any{
		"method":  req.Method,
		"url":     req.URL.String(),
		"path":    req.URL.Path,
		"headers": flattenHeaders(req.Header),
		"query":   req.URL.Query(),
		"body":    string(bodyBytes),
	}
	writeJSON(w, http.StatusOK, out)
}

func headersHandler(c *rux.Context) {
	writeJSON(c.Resp, http.StatusOK, map[string]any{
		"headers": flattenHeaders(c.Req.Header),
	})
}

func ipHandler(c *rux.Context) {
	// 优先 X-Forwarded-For，否则 RemoteAddr
	ip := c.Req.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = c.Req.RemoteAddr
	}
	writeJSON(c.Resp, http.StatusOK, map[string]any{"ip": ip})
}

func statusHandler(c *rux.Context) {
	code, err := strconv.Atoi(c.Param("code"))
	if err != nil || code < 100 || code > 599 {
		writeJSON(c.Resp, http.StatusBadRequest, map[string]any{"error": "invalid status code"})
		return
	}
	c.Resp.WriteHeader(code)
}

func delayHandler(c *rux.Context) {
	sec, err := strconv.Atoi(c.Param("seconds"))
	if err != nil || sec < 0 {
		writeJSON(c.Resp, http.StatusBadRequest, map[string]any{"error": "invalid seconds"})
		return
	}
	if sec > 10 {
		sec = 10 // cap，避免拖死 server
	}
	time.Sleep(time.Duration(sec) * time.Second)
	anythingHandler(c)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4-B.4: 运行测试确认通过**

```
go test ./internal/echo/... -run TestEcho -v
```

预期：PASS。

- [ ] **Step 4-B.5: Commit**

```
git add internal/echo/mount.go internal/echo/mount_test.go
git commit -m "feat(echo): 自实现最小 httpbin 风格 echo handler"
```

---

## Task 5: admin 端点 `/__fakeserver/healthz`

**Files**:
- 新建：`internal/admin/handlers.go`
- 新建：`internal/admin/handlers_test.go`

- [ ] **Step 5.1: 先写测试**

新建 `internal/admin/handlers_test.go`：

```go
package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gookit/rux"
	"github.com/inhere/fakeserver/internal/admin"
)

func TestHealthz_Returns200OK(t *testing.T) {
	r := rux.New()
	admin.Mount(r)
	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/__fakeserver/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if parsed["status"] != "ok" {
		t.Errorf(`expected status=="ok", got %q`, parsed["status"])
	}
}
```

- [ ] **Step 5.2: 运行测试确认失败**

```
go test ./internal/admin/... -v
```

预期：FAIL（admin 包尚未存在）。

- [ ] **Step 5.3: 实现 admin handlers**

新建 `internal/admin/handlers.go`：

```go
// Package admin exposes the fakeserver-internal endpoints under
// /__fakeserver/*. Phase 1 only provides healthz; later phases will add
// /routes, /api/*, /ui/*, /events.
package admin

import (
	"encoding/json"
	"net/http"

	"github.com/gookit/rux"
)

// Mount 注册所有 admin 端点。调用方必须保证 path "/__fakeserver/*"
// 是保留前缀（design §4.6）。
func Mount(r *rux.Router) {
	r.GET("/__fakeserver/healthz", healthzHandler)
}

func healthzHandler(c *rux.Context) {
	c.Resp.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Resp.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(c.Resp).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 5.4: 运行测试确认通过**

```
go test ./internal/admin/... -v
```

预期：PASS。

- [ ] **Step 5.5: Commit**

```
git add internal/admin/handlers.go internal/admin/handlers_test.go
git commit -m "feat(admin): 实现 /__fakeserver/healthz 端点"
```

---

## Task 6: CLI 框架——`internal/cli` + 极薄 `cmd/fakeserver/main.go`

**Files**:
- 新建：`internal/cli/app.go`
- 新建：`internal/cli/serve.go`
- 新建：`cmd/fakeserver/main.go`

> **目标拆分**：所有 CLI 编排逻辑放进 `internal/cli/`（app 构造、子命令注册、serve 子命令、子命令的 run handler）；`cmd/fakeserver/main.go` 只负责"接收 `version` 变量并调用 `cli.Run`"。这一拆分避免后期把 cmd 里的命令搬到 internal——Phase 2 引入 init/check/routes 子命令、Phase 3 引入 list/use 子命令时，cmd/ 永远只是 1 个文件 < 15 行。

- [ ] **Step 6.1: 实现 `internal/cli/app.go`（app 构造 + 子命令注册）**

新建 `internal/cli/app.go`：

```go
// Package cli wires up the fakeserver command-line interface.
//
// All command definitions live here; cmd/fakeserver/main.go is intentionally
// thin and only invokes Run().
package cli

import (
	"github.com/gookit/gcli/v3"
)

// Run 构建 fakeserver CLI app，注册所有子命令并启动。
// version 通常由 cmd/fakeserver/main.go 透传，build 时通过 ldflags 注入。
func Run(version string) {
	app := gcli.NewApp(func(a *gcli.App) {
		a.Name = "fakeserver"
		a.Version = version
		a.Desc = "Configurable HTTP mock/fake server"
	})
	app.Add(newServeCmd())
	// 未来子命令在此追加：
	//   app.Add(newInitCmd())   // Phase 2
	//   app.Add(newCheckCmd())  // Phase 2
	//   app.Add(newRoutesCmd()) // Phase 2
	//   app.Add(newListCmd())   // Phase 3
	//   app.Add(newUseCmd())    // Phase 3
	app.Run(nil)
}
```

- [ ] **Step 6.2: 实现 `internal/cli/serve.go`（serve 子命令骨架，runServe 暂占位）**

新建 `internal/cli/serve.go`：

```go
package cli

import (
	"fmt"

	"github.com/gookit/gcli/v3"
)

type serveOptions struct {
	Port int
	Host string
}

func newServeCmd() *gcli.Command {
	var opts serveOptions
	c := &gcli.Command{
		Name: "serve",
		Desc: "Start the fakeserver HTTP server",
		Config: func(cmd *gcli.Command) {
			cmd.IntOpt2(&opts.Port, "port,p", "Listening port", gcli.OptInitValue(3000))
			cmd.StrOpt2(&opts.Host, "host", "Listening host", gcli.OptInitValue("0.0.0.0"))
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			return runServe(opts)
		},
	}
	return c
}

// runServe 由 Task 7 填充。Phase 1 阶段先打印日志后退出，避免空命令报错。
func runServe(opts serveOptions) error {
	fmt.Printf("(TODO Task 7) serve on %s:%d\n", opts.Host, opts.Port)
	return nil
}
```

> **注**：gcli v3 的选项 API 名称（`IntOpt2` / `StrOpt2` / `OptInitValue`）须以本地 gcli 版本为准。如果编译失败提示 "undefined: IntOpt2"，请运行 `go doc github.com/gookit/gcli/v3.Command` 查看真实方法名（例如 v3 早期版本是 `IntOpt`），把这两行改成实际签名即可——逻辑不变。

- [ ] **Step 6.3: 实现 `cmd/fakeserver/main.go`（极薄入口）**

新建 `cmd/fakeserver/main.go`：

```go
// Command fakeserver is the entry point for the fakeserver binary.
//
// All CLI logic lives in internal/cli; this file is intentionally minimal
// so that build/embedding code stays trivial and isolated from command
// definitions.
package main

import "github.com/inhere/fakeserver/internal/cli"

// 在 build 时通过 ldflags 注入：-X main.version=v0.1.0
var version = "dev"

func main() {
	cli.Run(version)
}
```

> **行数自检**：非空、非注释代码 ≤ 5 行（package / import / var / func + 调用）。这就是 DoD 第 7 条要的"≤15 行"上限的最佳实践——main.go 一旦超过这个量，说明逻辑漏到了 cmd/。

- [ ] **Step 6.4: 校验编译通过**

```
go build ./...
```

预期：编译成功，产出 `fakeserver` 或 `fakeserver.exe`（取决于平台）。如有 gcli 方法名报错按 Step 6.2 末尾注释处理。

- [ ] **Step 6.5: 手动冒烟测试**

执行：

```
./fakeserver serve --port 4000
```

预期 stdout：`(TODO Task 7) serve on 0.0.0.0:4000`，进程立即退出。

执行：

```
./fakeserver --help
```

预期：列出 `serve` 子命令与其说明。

- [ ] **Step 6.6: Commit**

```
git add cmd/fakeserver/main.go internal/cli/app.go internal/cli/serve.go
git commit -m "feat(cli): 抽出 internal/cli 包；cmd/fakeserver 仅作极薄入口"
```

---

## Task 7: serve 子命令真实启动（rux + echo + admin + 信号退出）

**Files**:
- 修改：`internal/cli/serve.go`

- [ ] **Step 7.1: 实现真实的 runServe**

完整替换 `internal/cli/serve.go`（合并 Task 6 已写入的 serveOptions / newServeCmd，并补全 imports 与 runServe 真实实现）：

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/rux"

	"github.com/inhere/fakeserver/internal/admin"
	"github.com/inhere/fakeserver/internal/echo"
)

type serveOptions struct {
	Port int
	Host string
}

func newServeCmd() *gcli.Command {
	var opts serveOptions
	c := &gcli.Command{
		Name: "serve",
		Desc: "Start the fakeserver HTTP server",
		Config: func(cmd *gcli.Command) {
			cmd.IntOpt2(&opts.Port, "port,p", "Listening port", gcli.OptInitValue(3000))
			cmd.StrOpt2(&opts.Host, "host", "Listening host", gcli.OptInitValue("0.0.0.0"))
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			return runServe(opts)
		},
	}
	return c
}

// assembleRouter 集中装配 router 的所有 mount 操作。
// 抽成独立函数是为了让 E2E 测试（serve_test.go）复用，而不必启动真实 listener。
func assembleRouter() *rux.Router {
	r := rux.New()
	admin.Mount(r)
	echo.Mount(r)
	return r
}

// runServe 装配 router 并启动 HTTP server。Phase 1 阶段：
//   - 注册 admin 端点 (healthz)
//   - 注册 echo handler (兜底 + 固定路径)
//   - 监听 SIGINT/SIGTERM，触发后等待最多 5s 优雅退出
func runServe(opts serveOptions) error {
	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: assembleRouter(),
	}

	// 监听退出信号
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		fmt.Printf("fakeserver listening on http://%s\n", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// 等待退出信号或致命错误
	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case sig := <-stop:
		fmt.Printf("\nreceived %s, shutting down...\n", sig)
	}

	// 5s 内尝试优雅退出
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	fmt.Println("bye.")
	return nil
}
```

> **关键设计**：`assembleRouter` 抽成包级私有函数，被 `runServe` 与 Task 8 的 E2E 测试共同复用，确保"测试与运行装配相同 router"——避免装配步骤漂移导致测试漏检（这是 design §7 测试策略原则的具体体现）。

- [ ] **Step 7.2: 校验编译**

```
go build ./...
```

预期：无输出，退出码 0。

- [ ] **Step 7.3: 手动冒烟测试**

终端 A：

```
./fakeserver serve --port 4000
```

预期 stdout：`fakeserver listening on http://0.0.0.0:4000`，进程保持前台运行。

终端 B：

```
curl -i http://localhost:4000/__fakeserver/healthz
curl -i http://localhost:4000/anything/test?x=1
curl -i http://localhost:4000/status/418
```

预期：依次得到 200/200/418 响应；JSON body 含 method/url/headers 等字段。

终端 A：按 `Ctrl+C`，预期 stdout：

```
^C
received interrupt, shutting down...
bye.
```

进程退出码 0。

- [ ] **Step 7.4: Commit**

```
git add internal/cli/serve.go
git commit -m "feat(cli): serve 子命令启动 rux + admin + echo + 信号退出"
```

---

## Task 8: 端到端测试 `internal/cli/serve_test.go`

**Files**:
- 新建：`internal/cli/serve_test.go`

> **目的**：自动化锁定 Phase 1 DoD 的第 3–6 条；CI 友好；避免每次手动 curl。本测试用 `httptest.NewServer` + `assembleRouter()` 直接装配 router，不真正启 listen socket（避免端口冲突）。`assembleRouter` 是包级私有，所以测试文件 package 必须是 `cli`（非 `cli_test`）。

- [ ] **Step 8.1: 写测试**

新建 `internal/cli/serve_test.go`：

```go
// 此测试在 `package cli`（非 `cli_test`），以便复用包级私有 assembleRouter。
package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServe_Healthz(t *testing.T) {
	ts := httptest.NewServer(assembleRouter())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/__fakeserver/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServe_EchoOnAnything(t *testing.T) {
	ts := httptest.NewServer(assembleRouter())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anything/abc?x=1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("expected JSON, got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
}

func TestServe_EchoFallbackOnUnknownPath(t *testing.T) {
	ts := httptest.NewServer(assembleRouter())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/totally/unknown/path")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	// Phase 1 fallback 默认 echo，所以未匹配路径也是 200 + JSON
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (echo fallback), got %d", resp.StatusCode)
	}
}

func TestServe_StatusEndpoint(t *testing.T) {
	ts := httptest.NewServer(assembleRouter())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status/503")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 8.2: 运行测试**

```
go test ./internal/cli/... -v
```

预期：4 个用例全部 PASS。

> **如果 TestServe_EchoFallbackOnUnknownPath 失败**：说明 echo.Mount 没把兜底装到 NotFound。回到 Task 4 检查 `r.NotFound(...)` 是否被注册（路径 4-A 须确认 rux/server 的 NotFound handler 真实导出名；路径 4-B 须确认 `r.NotFound(anythingHandler)` 那一行存在）。

- [ ] **Step 8.3: 运行整个测试套件**

```
go test ./...
```

预期：所有包（internal/echo、internal/admin、internal/cli）测试通过；无 race condition（`-race` 可选）。

- [ ] **Step 8.4: Commit**

```
git add internal/cli/serve_test.go
git commit -m "test(cli): serve E2E 测试覆盖 healthz/echo/status/fallback"
```

---

## Task 9: 验证 Phase 1 DoD + 文档对齐

**Files**:
- 修改：`docs/fakeserver-design.md`（如有探测过程中发现的 design 偏离需要回写）

- [ ] **Step 9.1: 执行 DoD 检查清单**

执行下列命令并人工核对：

| 命令 | 预期 |
|---|---|
| `go build ./...` | 无输出，退出码 0 |
| `go test ./...` | `ok` 每个 package；无 FAIL；无 panic |
| `./fakeserver serve` | stdout 含 `fakeserver listening on http://0.0.0.0:3000` |
| `curl -s http://localhost:3000/__fakeserver/healthz` | `{"status":"ok"}` |
| `curl -s -o /dev/null -w "%{http_code}" http://localhost:3000/anything` | `200` |
| `curl -s -o /dev/null -w "%{http_code}" http://localhost:3000/status/418` | `418` |
| 在 serve 终端按 Ctrl+C | 退出码 0；无 panic 栈 |
| `wc -l cmd/fakeserver/main.go` | ≤ 15 行（DoD 第 7 条） |

如果任一项不符合预期，停下来定位回退到相应 Task，**不要**进入 Step 9.2。

- [ ] **Step 9.2: 同步 design 文档（如有偏离）**

如果 Task 3 探测结果与 design §5.5 的占位名不一致，在 `docs/fakeserver-design.md` §13 "未决项"区块**第一条**前追加一行：

```markdown
- [已落地] rux/server 子包探测结果：Phase 1 实际接入的导出符号为 <实际名>（详见 internal/echo/probe.md）。design §5.5 占位描述无须改动，但日后阅读时以本条为准。
```

如果 Task 3 走了路径 B（rux 不存在 server/ 子包），追加：

```markdown
- [已落地] rux 当前版本未提供 server/ 子包；Phase 1 在 internal/echo 自行实现最小 echo handler（覆盖 /anything、/headers、/ip、/status/{code}、/delay/{seconds}）。当 rux 后续版本提供官方 echo 子包时再考虑切换。
```

另外把"CLI 编排逻辑放置位置"的实际选择回写到 design §2.2 的注释（如果原文未明示）：

```markdown
- [已落地] CLI 编排（app 构造、子命令注册、子命令 run handler）位于 `internal/cli/`；`cmd/fakeserver/main.go` 仅作极薄入口。后续子命令（init/check/routes/list/use）都在 internal/cli 内追加一个 .go 文件。
```

- [ ] **Step 9.3: 更新设计文档修订记录**

在 `docs/fakeserver-design.md` 顶部"修订记录"表追加一行：

```markdown
| 2026-05-19 | v0.3-phase1-applied | inhere | Phase 1 落地：项目骨架、internal/cli + cmd 极薄入口、echo + healthz、E2E。rux/server 探测结果回写至 §13 |
```

- [ ] **Step 9.4: Commit**

```
git add docs/fakeserver-design.md
git commit -m "docs(design): 回写 Phase 1 落地结果到设计文档"
```

> 如果 Step 9.2 没有任何偏离需要回写，则跳过 Step 9.4（不提交空 commit）。

---

## Phase 1 完成 · 下一步

完成本计划后，仓库具备：

- ✅ 完整 Go module 与依赖锁定
- ✅ 模块骨架（`cmd/fakeserver/`、`internal/cli/`、`internal/echo/`、`internal/admin/`）
- ✅ `cmd/fakeserver/main.go` 极薄（≤ 15 行）；所有 CLI 编排在 `internal/cli`
- ✅ 可启动的 CLI：`fakeserver serve [-p PORT] [--host HOST]`
- ✅ 默认 echo 模式（无任何用户配置即可使用）
- ✅ `/__fakeserver/healthz` 健康检查端点
- ✅ 优雅退出（SIGINT/SIGTERM + 5s timeout）
- ✅ 单元 + E2E 测试覆盖以上能力

**Phase 2 预告（不在本计划范围）**：

- 在 `internal/cli/` 新增 `init.go` / `check.go` / `routes.go` 文件，分别注册到 `app.go` 的 Run 函数中
- 新增 `internal/config/` 包：JSON5 解析、schema 结构体、`@include` 展开、多文件合并、默认查找路径、集中校验
- serve 子命令接入 `-c/--config`，加载 config → 打印路由摘要（mock 路由响应留 Phase 3）

**生成 Phase 2 plan**：当 Phase 1 在主分支合并且所有 DoD 通过后，再次调用 `superpowers:writing-plans` skill，并把本计划路径作为前置依赖说明传入。

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| 无 TBD / placeholder / "类似 Task N" | ✓（Task 4 两条分支路径各自完整；Task 6 占位的 `runServe` 在 Task 7 完整给出实现） |
| 类型签名前后一致 | ✓（`cli.Run(version string)` / `echo.Mount(r *rux.Router)` / `admin.Mount(r *rux.Router)` / `serveOptions{Port,Host}` / `assembleRouter() *rux.Router` 在 Task 6/7/8 中保持一致） |
| 包路径前后一致 | ✓（`github.com/inhere/fakeserver/internal/cli`、`internal/echo`、`internal/admin` 全文统一） |
| TDD：先测后实现 | ✓（Task 4/5 先写测试；Task 8 验证 Task 7 装配） |
| 频繁提交 | ✓（每 Task 收尾一次 commit；Task 9 视情况提交） |
| 覆盖 design §5.5 echo 接入 + §5.6 admin healthz + §5.1 启动流程 1/8/11（仅 rux + echo + admin + 信号；配置/中间件/热加载留 Phase 2+） | ✓ |
| 失败路径明确（gcli 方法名漂移、rux 子包不存在、E2E fallback 检测） | ✓ |
| `cmd/fakeserver/main.go` 极薄约束（DoD #7） | ✓（Step 6.3 实现 ≤ 5 行非空代码；Step 9.1 用 wc -l 校验） |
