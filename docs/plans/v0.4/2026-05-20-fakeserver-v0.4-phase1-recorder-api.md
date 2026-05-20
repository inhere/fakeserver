# Fakeserver v0.4 · Phase 1 — recorder 包 + 3 个 JSON API + adminEnabled 护栏

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。

**Goal**：让 `internal/recorder` 包从零落地——环形缓冲（Append + Snapshot），接入 `middleware.Logger` 自动把每条请求落 ring；新建 `internal/webui` 包提供 3 个只读 JSON 端点（`/__fakeserver/api/{projects,config,history}`）并按 `cfg.Server.AdminEnabled` 整体决定挂载/不挂载；启动期检测 0.0.0.0 + adminEnabled 组合发 WARNING。

**Architecture**：

1. **`internal/recorder`** 新包，单文件 + 测试：
   - `recorder.go`：Entry struct + Ring（环形 + RWMutex）+ New + Append + Snapshot
   - Subscribe/Unsubscribe **不**在本 Phase 实现（留 Phase 2 SSE）
2. **`internal/middleware/logger.go` 改造**：
   - 加可选 `ring *recorder.Ring` 参数；nil 时行为与 v0.3 一致
   - 写日志后调 `ring.Append(Entry{...})`
3. **`internal/webui`** 新包，2 文件 + 测试：
   - `mount.go`：`Mount(r *rux.Router, cfg *config.Config, regPath string, ring *recorder.Ring)` 主入口；按 `cfg.Server.AdminEnabled` 决定挂载
   - `api.go`：3 个 GET handler（projects/config/history）+ 敏感 key 脱敏助手
4. **`internal/cli/serve.go` 接入**：
   - 创建 ring → logger 传 ring → `webui.Mount` 在 admin 之后
   - 启动期 0.0.0.0 + adminEnabled 警告

**Tech Stack**：

- 标准库：`encoding/json`、`net`、`sync`、`time`
- 复用：`github.com/inhere/fakeserver/internal/{config,middleware,registry}`、`github.com/gookit/rux/v2`
- **新增第三方依赖**：无（v0.4 硬约束）

**前置要求**：

- v0.3 Phase 2 完成（commit `0e7277e..bb3b919`，registry + list/use）
- 已读 [overview](../2026-05-20-fakeserver-v0.4-overview.md) §3 Phase 1 详述
- 已读 design.md §11.3 / §11.4 / §11.6（API 端点表、Entry 字段、安全护栏）
- 熟悉 `internal/admin/handlers.go`（Phase 1 仍保留 admin 的 healthz + routes 端点；webui 与 admin 并列，前缀同 `/__fakeserver/`）

**Phase 1 完成定义（DoD）**：

1. `internal/recorder/recorder.go` `New/Append/Snapshot` 三函数全部测试覆盖
2. `Ring.Append` 在 size 满后覆盖最旧；`Snapshot` 按时间顺序（最新在最后）；并发安全（`-race` 跑全测试无 race）
3. `middleware.Logger` 在 `ring == nil` 时行为与 v0.3 一致（既有 logger_test.go 不回归）；`ring != nil` 时每个请求后 ring 多一条 Entry，含 Path/Method/Status/Duration
4. `webui.Mount` 在 `adminEnabled: false` 下完全不注册端点（GET /__fakeserver/api/projects 404）
5. 3 个 JSON 端点输出结构正确：
   - `/api/projects` → `[]registry.Project`
   - `/api/config` → 当前 cfg 的 JSON（敏感 key 脱敏：token/secret/password 子串 case-insensitive 命中 → 值替换为 `"***"`）
   - `/api/history` → `[]recorder.Entry`（Snapshot 输出）
6. 0.0.0.0 + adminEnabled 组合启动期 stderr 有 `WARNING: server.host=0.0.0.0 with adminEnabled=true exposes admin endpoints publicly`
7. `go build ./...` + `go test ./...` + `go vet ./...` 全绿；`go test -race ./internal/recorder/...` 无 race
8. `internal/recorder` ≥ 80%；`internal/webui` ≥ 70%
9. bd v0.4 Phase 1 epic 创建并关闭

---

## 文件结构（Phase 1 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 新建 | `internal/recorder/recorder.go` | Entry + Ring + New/Append/Snapshot |
| 新建 | `internal/recorder/recorder_test.go` | 单元 + 并发测试 |
| 修改 | `internal/middleware/logger.go` | Logger 新签名（加 ring 参数） |
| 修改 | `internal/middleware/logger_test.go` | 既有用例适配新签名 + 新增 ring 用例 |
| 新建 | `internal/webui/mount.go` | Mount + adminEnabled 护栏 + Host 警告 |
| 新建 | `internal/webui/api.go` | 3 个 handler + 脱敏助手 |
| 新建 | `internal/webui/api_test.go` | 3 端点单元测试 + 脱敏 + adminEnabled=false 404 |
| 修改 | `internal/cli/serve.go` | 创建 ring + logger 传 ring + webui.Mount + 0.0.0.0 警告 |
| 新建 | `internal/cli/serve_v04_e2e_test.go` | webui mount 后 curl 验证 history 含刚才请求 |

---

## Task 1 — `internal/recorder` 包：Entry + Ring + Append + Snapshot

**Files**: `internal/recorder/recorder.go` + `_test.go`

### Step 1.1 — TDD：先写测试

新建 `internal/recorder/recorder_test.go`：

```go
package recorder

import (
	"sync"
	"testing"
	"time"
)

func TestNew_DefaultSizeWhenZeroOrNegative(t *testing.T) {
	r := New(0)
	if r.Cap() != 200 {
		t.Errorf("Cap=%d, want 200 (zero → default)", r.Cap())
	}
	r = New(-5)
	if r.Cap() != 200 {
		t.Errorf("Cap=%d, want 200 (negative → default)", r.Cap())
	}
	r = New(10)
	if r.Cap() != 10 {
		t.Errorf("Cap=%d, want 10", r.Cap())
	}
}

func TestAppend_Snapshot_ChronologicalOrder(t *testing.T) {
	r := New(5)
	for i := 1; i <= 3; i++ {
		r.Append(Entry{Path: "/p", Status: i})
	}
	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	for i, e := range got {
		if e.Status != i+1 {
			t.Errorf("snapshot[%d].Status=%d, want %d", i, e.Status, i+1)
		}
	}
}

func TestAppend_OverwritesOldestWhenFull(t *testing.T) {
	r := New(3)
	for i := 1; i <= 5; i++ {
		r.Append(Entry{Path: "/p", Status: i})
	}
	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	// 5 entries appended into capacity 3 → oldest 2 dropped, keep 3,4,5
	wantStatus := []int{3, 4, 5}
	for i, e := range got {
		if e.Status != wantStatus[i] {
			t.Errorf("snapshot[%d].Status=%d, want %d", i, e.Status, wantStatus[i])
		}
	}
}

func TestSnapshot_ReturnsCopyNotInternalSlice(t *testing.T) {
	r := New(5)
	r.Append(Entry{Status: 200})
	got := r.Snapshot()
	got[0].Status = 999 // mutating snapshot should not affect ring
	again := r.Snapshot()
	if again[0].Status == 999 {
		t.Error("Snapshot returned shared slice; ring data corrupted by caller")
	}
}

func TestAppend_ConcurrentSafe(t *testing.T) {
	r := New(100)
	const G = 16
	const N = 1000
	var wg sync.WaitGroup
	wg.Add(G)
	for g := 0; g < G; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < N; i++ {
				r.Append(Entry{Path: "/p", Status: g, TS: time.Now()})
			}
		}()
	}
	// 并发读 Snapshot 应不撕裂
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = r.Snapshot()
		}
	}()
	wg.Wait()
	// 满后只保留最近 100 条；不验证具体内容（并发顺序非确定），只验证长度且无 panic
	if got := r.Snapshot(); len(got) != 100 {
		t.Errorf("after concurrent appends, Snapshot len=%d, want 100 (= cap)", len(got))
	}
}
```

跑：

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/recorder/... -count=1
# 预期 FAIL（包不存在）
```

### Step 1.2 — 实现

新建 `internal/recorder/recorder.go`：

```go
// Package recorder 提供请求历史的内存环形缓冲。design §11.4。
// Phase 1：Append + Snapshot；Subscribe/Unsubscribe 留 Phase 2 SSE 使用。
package recorder

import (
	"sync"
	"time"
)

// Entry 是 ring 中单条记录（design §11.4）。
// 不包含请求/响应 body（隐私 + 体积）。
type Entry struct {
	TS          time.Time `json:"ts"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Status      int       `json:"status"`
	DurationMs  float64   `json:"durationMs"`
	ClientIP    string    `json:"clientIp,omitempty"`
	RouteIndex  int       `json:"routeIndex,omitempty"`  // Phase 1 占位 0；待 mock/proxy 包协作填充
	CaseIndex   int       `json:"caseIndex,omitempty"`   // 同上
	ProxyTarget string    `json:"proxyTarget,omitempty"` // 同上
}

// Ring 是固定容量的环形缓冲。
type Ring struct {
	mu    sync.RWMutex
	buf   []Entry
	head  int // 下一个写入位置；buf[head] 是最旧的（缓冲已满时）
	size  int // 当前有效条数（≤ cap）
}

// New 创建一个容量为 size 的 Ring；size <= 0 → 默认 200（design §11.4 默认值）。
func New(size int) *Ring {
	if size <= 0 {
		size = 200
	}
	return &Ring{buf: make([]Entry, size)}
}

// Cap 返回环形缓冲的容量。
func (r *Ring) Cap() int {
	return len(r.buf)
}

// Append 写入一条新 Entry。满时覆盖最旧条目。
func (r *Ring) Append(e Entry) {
	r.mu.Lock()
	r.buf[r.head] = e
	r.head = (r.head + 1) % len(r.buf)
	if r.size < len(r.buf) {
		r.size++
	}
	r.mu.Unlock()
}

// Snapshot 返回按时间顺序的 Entry 副本（最旧在前，最新在后）。
// 调用方可自由修改返回的 slice 而不影响 ring 内部状态。
func (r *Ring) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, r.size)
	// 当 size < cap：buf[0..size) 即为时间序
	// 当 size == cap：从 buf[head] 开始（最旧），绕回到 buf[head-1]
	if r.size < len(r.buf) {
		copy(out, r.buf[:r.size])
		return out
	}
	// 已满：buf[head] 是最旧；后续按 buf[head+1] ... 绕回
	start := r.head
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}
```

跑：

```
go test ./internal/recorder/... -v -count=1
go test -race ./internal/recorder/... -count=1
# 预期 PASS + 无 race
```

### Step 1.3 — Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/recorder/
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(recorder): Ring 环形缓冲 + Entry struct（v0.4 Phase 1）"
```

---

## Task 2 — `middleware.Logger` 接入 ring

**Files**: `internal/middleware/logger.go` + `_test.go`

### Step 2.1 — 改造签名

`Logger(out io.Writer, quiet bool) func(http.Handler) http.Handler` →
`Logger(out io.Writer, quiet bool, ring *recorder.Ring) func(http.Handler) http.Handler`

- `ring == nil` → 不调用 Append（保持向后兼容）
- 在 `next.ServeHTTP` 后 + `Fprintf` 后，若 `ring != nil`：

```go
ring.Append(recorder.Entry{
    TS:         start.UTC(),
    Method:     r.Method,
    Path:       r.URL.Path,
    Status:     lr.status,
    DurationMs: float64(dur.Microseconds()) / 1000.0,
    ClientIP:   clientIP(r),
})
```

`clientIP` 辅助：从 `r.RemoteAddr` 用 `net.SplitHostPort` 拆出 host；失败则用原始字符串。

### Step 2.2 — 测试调整

`logger_test.go` 既有用例所有 `Logger(out, quiet)` 调用 → `Logger(out, quiet, nil)`（向后兼容）。新增 1 个用例：传 ring → 请求后 `ring.Snapshot()` 含 1 条且 Path/Status 正确。

### Step 2.3 — 调用方修改

`internal/cli/serve.go` 中：
```go
mws = append(mws, middleware.Logger(os.Stderr, opts.Quiet))
```
改为：
```go
mws = append(mws, middleware.Logger(os.Stderr, opts.Quiet, ring))
```
（`ring` 在 runServe 中先创建，见 Task 4 Step 4.2）

为避免本 Task 单独跑测试时 cli 编译失败，**先做 Task 4 Step 4.2 的最小改动**（创建 ring 变量传入），或直接接受 cli 包测试在 Task 2 commit 后暂时编译失败、Task 4 修复——按 TDD 节奏前者更好。

实际操作：在 Task 2 内部做完整变更（middleware + serve.go 调用点），把 ring 在 serve.go 临时硬编码 `nil`（保留向后行为），ring 创建留 Task 4。

### Step 2.4 — Commit

```
go test ./internal/middleware/... -v -count=1
go build ./...   # 验证 cli 包仍编得过（传 nil）
git -C D:/work/aidev/lite-tools/fakeserver add internal/middleware/ internal/cli/serve.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(middleware): Logger 接入可选 ring（向后兼容；ring nil 时同 v0.3）"
```

---

## Task 3 — `internal/webui` 包：mount + 3 个 JSON 端点

**Files**: `internal/webui/mount.go` + `api.go` + `api_test.go`

### Step 3.1 — TDD：先写 api_test.go

新建 `internal/webui/api_test.go`：

```go
package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/registry"
)

func TestAPIProjects_ReturnsRegistryProjects(t *testing.T) {
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	reg := &registry.Registry{Version: 1, Projects: []registry.Project{
		{ID: "abc", Name: "alpha"}, {ID: "def", Name: "beta"},
	}}
	if err := registry.Save(regPath, reg); err != nil {
		t.Fatal(err)
	}

	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: true}}
	Mount(router, cfg, regPath, recorder.New(10))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/__fakeserver/api/projects", nil)
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	var got []registry.Project
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d projects, want 2", len(got))
	}
}

func TestAPIConfig_RedactsSensitiveKeys(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerOpts{AdminEnabled: true},
		Env: map[string]any{
			"apiHost":  "dev.local",
			"token":    "should-be-redacted",
			"apiSecret": "also-redacted",
			"password": "ditto",
			"someToken": "ditto",
		},
	}
	router := rux.New()
	Mount(router, cfg, "", recorder.New(10))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/__fakeserver/api/config", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"apiHost"`) || !strings.Contains(body, "dev.local") {
		t.Errorf("apiHost should be visible; got: %s", body)
	}
	if strings.Contains(body, "should-be-redacted") || strings.Contains(body, "also-redacted") {
		t.Errorf("sensitive value leaked: %s", body)
	}
	if !strings.Contains(body, `"***"`) {
		t.Errorf("redacted placeholder *** missing: %s", body)
	}
}

func TestAPIHistory_ReturnsRingSnapshot(t *testing.T) {
	ring := recorder.New(10)
	ring.Append(recorder.Entry{Method: "GET", Path: "/p", Status: 200})
	ring.Append(recorder.Entry{Method: "POST", Path: "/x", Status: 201})

	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: true}}
	Mount(router, cfg, "", ring)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/__fakeserver/api/history", nil))
	var got []recorder.Entry
	_ = json.NewDecoder(w.Body).Decode(&got)
	if len(got) != 2 || got[0].Path != "/p" || got[1].Path != "/x" {
		t.Errorf("history mismatch: %+v", got)
	}
}

func TestMount_AdminDisabled_NoEndpoints(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: false}}
	Mount(router, cfg, "", recorder.New(10))

	// 任何 webui 端点都应 404（router 没注册）
	endpoints := []string{
		"/__fakeserver/api/projects",
		"/__fakeserver/api/config",
		"/__fakeserver/api/history",
	}
	for _, ep := range endpoints {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", ep, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("endpoint %s with adminEnabled=false should be 404, got %d", ep, w.Code)
		}
	}
}
```

跑：FAIL（webui 包不存在）

### Step 3.2 — 实现 mount.go + api.go

新建 `internal/webui/mount.go`：

```go
// Package webui 提供 fakeserver 的轻量 Web UI 与配套 JSON / SSE 端点。
// design §11。
//
// Phase 1：仅 3 个 JSON 端点（projects/config/history）+ adminEnabled 护栏。
// SSE 留 Phase 2；静态资源留 Phase 3。
package webui

import (
	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
)

// Mount 注册 webui 端点到 router。
// 当 cfg.Server.AdminEnabled == false 时整体跳过（design §11.6）。
// regPath：~/.config/fakeserver/projects.json 的绝对路径，用于 /api/projects。
func Mount(r *rux.Router, cfg *config.Config, regPath string, ring *recorder.Ring) {
	if cfg == nil || !cfg.Server.AdminEnabled {
		return
	}
	r.GET("/__fakeserver/api/projects", apiProjectsHandler(regPath))
	r.GET("/__fakeserver/api/config", apiConfigHandler(cfg))
	r.GET("/__fakeserver/api/history", apiHistoryHandler(ring))
}
```

新建 `internal/webui/api.go`：

```go
package webui

import (
	"net/http"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/registry"
)

func apiProjectsHandler(regPath string) rux.HandlerFunc {
	return func(c *rux.Context) {
		if regPath == "" {
			c.JSON(http.StatusOK, []registry.Project{})
			return
		}
		reg, err := registry.Load(regPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, reg.Projects)
	}
}

func apiConfigHandler(cfg *config.Config) rux.HandlerFunc {
	return func(c *rux.Context) {
		if cfg == nil {
			c.JSON(http.StatusOK, map[string]any{})
			return
		}
		// 深拷贝并脱敏 env 段中的敏感 key
		redacted := redactConfig(cfg)
		c.JSON(http.StatusOK, redacted)
	}
}

func apiHistoryHandler(ring *recorder.Ring) rux.HandlerFunc {
	return func(c *rux.Context) {
		if ring == nil {
			c.JSON(http.StatusOK, []recorder.Entry{})
			return
		}
		c.JSON(http.StatusOK, ring.Snapshot())
	}
}

// redactConfig 返回 cfg 的可序列化拷贝，env 段中 key 含 token/secret/password
// 子串（不区分大小写）的值被替换为 "***"。返回 map 而非 *config.Config，
// 避免修改原对象 + 避免 json:"-" 字段污染输出。
func redactConfig(cfg *config.Config) map[string]any {
	out := map[string]any{
		"server":  cfg.Server,
		"globals": cfg.Globals,
		"routes":  cfg.Routes,
	}
	if cfg.Env != nil {
		out["env"] = redactMap(cfg.Env)
	}
	return out
}

func redactMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if isSensitiveKey(k) {
			out[k] = "***"
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			out[k] = redactMap(nested)
			continue
		}
		out[k] = v
	}
	return out
}

func isSensitiveKey(k string) bool {
	lk := strings.ToLower(k)
	for _, needle := range []string{"token", "secret", "password"} {
		if strings.Contains(lk, needle) {
			return true
		}
	}
	return false
}
```

跑：

```
go test ./internal/webui/... -v -count=1
# 预期 PASS（4 用例）
```

### Step 3.3 — Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/webui/
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(webui): Mount + 3 JSON 端点 (projects/config/history) + 敏感 key 脱敏"
```

---

## Task 4 — `serve.go` 接入 ring + webui.Mount + 0.0.0.0 警告

**Files**: `internal/cli/serve.go` + `internal/cli/serve_v04_e2e_test.go`

### Step 4.1 — 接入点位置

`runServe` 现状（v0.3 后）：

1. loadServeConfig → cfg
2. v0.3 registry Upsert + WritePIDFile
3. 创建 renderer
4. 创建 holder + assembleHandler
5. watcher
6. srv + signal handler

接入点：

- **创建 ring** 在 #2 之后、#3 之前；容量 = `cfg.Server.HistorySize`（cfg=nil → 默认 200）
- **0.0.0.0 + adminEnabled WARNING** 在 #2 之后；条件：`cfg != nil && cfg.Server.Host == "0.0.0.0" && cfg.Server.AdminEnabled`
- **logger 传 ring**：`assembleHandler` 当前签名 `(cfg, renderer, opts)` 不接收 ring。两种方案：
  - (A) `assembleHandler(cfg, renderer, opts, ring)` 改签名
  - (B) 把 ring 塞到 opts 里
  - 选 (A)：opts 当前是 CLI flag 集合，加 ring 字段语义不符。改签名只影响 cli 包内部。

- **webui.Mount** 在 `assembleHandler` 中，紧贴 `admin.Mount(r, cfg)` 之后：

```go
webui.Mount(r, cfg, userRegistryPath(), ring)
```

### Step 4.2 — 编辑 serve.go

import 加 `recorder` 与 `webui`。

`assembleHandler` 改签名：

```go
func assembleHandler(cfg *config.Config, renderer tpl.Renderer, opts serveOptions, ring *recorder.Ring) http.Handler {
	r := rux.New()
	_ = mock.Mount(r, cfg, renderer)
	_ = proxy.Mount(r, cfg, renderer)
	admin.Mount(r, cfg)
	webui.Mount(r, cfg, userRegistryPath(), ring)
	echo.Mount(r)
	// ... 既有 middleware 装配；logger 已在 Task 2 取 ring
```

`runServe` 中（v0.3 registry 段之后）插入：

```go
historySize := 200
if cfg != nil && cfg.Server.HistorySize > 0 {
    historySize = cfg.Server.HistorySize
}
ring := recorder.New(historySize)

if cfg != nil && cfg.Server.Host == "0.0.0.0" && cfg.Server.AdminEnabled {
    fmt.Fprintln(os.Stderr, "WARNING: server.host=0.0.0.0 with adminEnabled=true exposes admin endpoints publicly")
}
```

assembleHandler 调用点（含 watcher onReload 路径中的）全部传 ring。

logger 调用从 `middleware.Logger(os.Stderr, opts.Quiet, nil)` 改为 `middleware.Logger(os.Stderr, opts.Quiet, ring)`。

### Step 4.3 — 集成 E2E

新建 `internal/cli/serve_v04_e2e_test.go`：

```go
package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/middleware"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/tpl"
)

// TestServe_v04_WebUIAPIs 验证 v0.4 Phase 1 DoD #4/#5：
// webui.Mount 注册的 3 个 JSON 端点可达，且 /api/history 包含刚刚打过的请求。
func TestServe_v04_WebUIAPIs(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ server: { adminEnabled: true }, routes: [{ method: "GET", path: "/hello", body: "world" }] }`), 0644)

	cfg, err := config.Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	ring := recorder.New(50)

	// 用与 runServe 一样的装配方式（assembleHandler 接收 ring）
	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true}, ring))
	srv := httptest.NewServer(holder)
	defer srv.Close()

	// 打一个 mock 请求
	resp, _ := http.Get(srv.URL + "/hello")
	resp.Body.Close()

	// /api/history 应含 1 条
	resp, _ = http.Get(srv.URL + "/__fakeserver/api/history")
	var entries []recorder.Entry
	_ = json.NewDecoder(resp.Body).Decode(&entries)
	resp.Body.Close()
	if len(entries) < 1 {
		t.Errorf("api/history should contain at least 1 entry; got %d", len(entries))
	}
	found := false
	for _, e := range entries {
		if e.Path == "/hello" && e.Status == 200 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("/hello request not in history: %+v", entries)
	}

	// /api/config 应可达
	resp, _ = http.Get(srv.URL + "/__fakeserver/api/config")
	if resp.StatusCode != 200 {
		t.Errorf("api/config status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestServe_v04_AdminDisabled_NoUIEndpoints 验证 adminEnabled=false 全部 404。
func TestServe_v04_AdminDisabled_NoUIEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ server: { adminEnabled: false }, routes: [{ method: "GET", path: "/p", body: "x" }] }`), 0644)

	cfg, err := config.Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	ring := recorder.New(50)
	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true}, ring))
	srv := httptest.NewServer(holder)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/__fakeserver/api/history")
	if resp.StatusCode != 404 {
		t.Errorf("api/history with adminEnabled=false should be 404; got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
```

跑：

```
go build ./... 2>&1 | tail -5
go test ./... -count=1 2>&1 | tail -12
go test -race ./internal/recorder/... ./internal/webui/... -count=1
```

### Step 4.4 — Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve.go internal/cli/serve_v04_e2e_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): serve 接入 recorder.Ring + webui.Mount + 0.0.0.0 警告（v0.4 Phase 1）"
```

---

## Task 5 — DoD 核对 + 文档回写 + bd close

**Files**: `docs/plans/2026-05-20-fakeserver-v0.4-overview.md` + `docs/fakeserver-design.md`

### Step 5.1 — 完整核对

```
cd D:/work/aidev/lite-tools/fakeserver
go build ./...
go test ./... -count=1
go test -race ./internal/{recorder,webui}/... -count=1
go test -cover ./internal/{recorder,webui,middleware,cli} 
go vet ./...
```

DoD 9 项核对（见本 plan 顶部 DoD 列表）。覆盖率应 recorder ≥ 80% / webui ≥ 70%。

### Step 5.2 — 手动验证（可选）

```
cd /tmp && mkdir -p v04-check && cd v04-check
cat > fakeserver.json5 <<'EOF'
{
  server: { host: "127.0.0.1", port: 5095, adminEnabled: true },
  routes: [{ method: "GET", path: "/hello", body: "world" }],
}
EOF
D:/work/aidev/lite-tools/fakeserver/fakeserver.exe serve --quiet &
sleep 1
curl -s http://127.0.0.1:5095/hello
curl -s http://127.0.0.1:5095/__fakeserver/api/history | head -100
curl -s http://127.0.0.1:5095/__fakeserver/api/projects
curl -s http://127.0.0.1:5095/__fakeserver/api/config | head -50
kill %1
```

### Step 5.3 — 回写 overview

v0.4 overview §2 表 Phase 1 行：`待开始` → `✅ 已完成 (commit <T1>..<T5>)`

§3 Phase 1 详述末尾追加"实际落地偏差" + commit 流水（按 Phase 1 实测情况）。

### Step 5.4 — 回写 design.md（严格 3 条事实）

修订记录追加：

```
| 2026-05-XX | v0.4-phase0.4.1-applied | inhere | v0.4 Phase 1：recorder 包 + middleware logger 接入 + 3 个 JSON API + adminEnabled 护栏 |
```

§13 追加（**严格 3 条事实**）：

```
### 已落地（v0.4 Phase 1 阶段确认）

1. **recorder 环形缓冲零侵入接入 middleware.Logger**：Logger 新签名加 `*recorder.Ring` 可选参数，nil → 行为同 v0.3；非 nil → 每条请求日志同时 Append 一条 Entry（TS/Method/Path/Status/DurationMs/ClientIP）。RouteIndex/CaseIndex/ProxyTarget 字段结构占位但值待 v1.x mock/proxy 包协作填充。
2. **webui 包按 adminEnabled 整体挂载**：cfg.Server.AdminEnabled=false → Mount 直接 return；3 个端点 (projects/config/history) 全部 404。config 端点对 env 段做敏感 key 脱敏（token/secret/password 子串 case-insensitive 命中 → 值 "***"）。
3. **0.0.0.0 + adminEnabled 启动期 WARNING**：design §11.6 安全护栏的"启动时若检测到该组合，打 WARNING 日志"落地；用户希望关闭警告需显式改 host 为 127.0.0.1 或关 adminEnabled。
```

### Step 5.5 — Commit + bd close

```
git -C D:/work/aidev/lite-tools/fakeserver add docs/plans/2026-05-20-fakeserver-v0.4-overview.md docs/fakeserver-design.md docs/plans/v0.4/2026-05-20-fakeserver-v0.4-phase1-recorder-api.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "docs(v0.4): 回写 Phase 1 落地 + design §13 阶段确认"

# bd
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd create --title="fakeserver v0.4 Phase 1 — recorder + 3 JSON API + adminEnabled 护栏" --description="5 Task：recorder 包 / middleware 接入 / webui mount + 3 端点 / serve 接入 / docs + bd" --type=feature --priority=2
# 取 <id> 后立即关闭：
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd close <id> --reason="v0.4 Phase 1 完整落地：recorder 包 + middleware logger 接入 + webui 3 JSON 端点 + adminEnabled 护栏。"
```

---

## Phase 1 完成 · 下一步

仓库具备：

- ✅ `internal/recorder/recorder.go` Append/Snapshot 完整 + 单测 + 并发安全
- ✅ `internal/middleware/logger.go` 接入 ring（向后兼容）
- ✅ `internal/webui/` 包 + 3 个 JSON 端点 + 敏感 key 脱敏
- ✅ serve 启动期 0.0.0.0 + adminEnabled 警告
- ✅ 全包绿 + `-race` 无 race + recorder ≥ 80% / webui ≥ 70%
- ✅ design.md §13 v0.4 Phase 1 阶段确认（严格 3 条）
- ✅ bd v0.4 Phase 1 epic 关闭

**Phase 2 预告**：

- `recorder.Subscribe / Unsubscribe`（多订阅者管理）
- `/__fakeserver/events` SSE 端点（心跳 15s + 慢客户端非阻塞 + 客户端断开自动 unsub）
- `event: reload` 推送（watcher onReload 集成）

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| TDD：先测后写 | ✓（Task 1/3 测试先行；Task 2 改造保留向后兼容测试）|
| 频繁提交 | ✓（Task 1-5 各一个 commit）|
| 无新增第三方依赖 | ✓ |
| 失败策略明确（adminEnabled=false 整体跳过）| ✓ |
| 覆盖 design §11.3 前 3 端点 + §11.4 + §11.6 | ✓ |
| design.md 回写**严格 3 条事实** | ✓ |
| Phase 1 / 2 / 3 边界清晰（SSE 留 Phase 2、UI 留 Phase 3）| ✓ |
| 跨平台问题（embed FS 在 Phase 3 才用，Phase 1 不涉及） | ✓ |
