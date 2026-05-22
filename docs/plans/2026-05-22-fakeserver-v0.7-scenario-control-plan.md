# Fakeserver v0.7 Scenario Control Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让前端开发者可以稳定切换 success/empty/error/auth-expired/slow 等接口状态，并能在 Web UI 中查看和调整当前场景与单 route case override。

**Architecture:** v0.7 在现有 cases/selector/recorder/Web UI 基础上增加三层控制：配置层的 case name + scenarios，运行层的内存 scenario store，交互层的 CLI/header/UI 优先级和 Web UI 控制面板。mock handler 仍是唯一响应入口；scenario/override 只影响 cases route 的 case 选择，不改变 proxy、单响应 mock、echo 的既有语义。

**Tech Stack:** Go 标准库、现有 `rux`、`gcli`、`expr`、原生 HTML/CSS/JS embed assets；不新增第三方依赖。

---

## 1. 范围

### 1.1 包含

- `cases[].name`：为每个 case 提供稳定可读名称。
- `scenarios` 配置：按 route signature 指定 case name。
- `server.scenario` 默认场景：配置文件可指定默认启用场景。
- CLI `serve --scenario <name>`：覆盖配置默认场景。
- 请求 header `X-Fakeserver-Scenario`：单请求覆盖当前场景。
- 运行期内存 store：
  - Web UI 当前 selected scenario。
  - route 持续强制 case。
  - route 下一次强制 case。
  - route 接下来 N 次强制 case。
- Admin/Web UI API：
  - 读取 scenario 状态。
  - 设置/清除 selected scenario。
  - 设置/清除 route case override。
- Web UI：
  - 顶部 scenario selector。
  - Routes 页面展示 cases name。
  - 单 route case override 控件。
  - History 详情展示 scenario/case name/override source。
- `init --full` 示例增加 named cases 和 scenarios。
- 文档回写：usage、design、v0.7 plan checklist。

### 1.2 不包含

- 持久化 runtime scenario 状态。
- 多用户隔离 scenario。
- 全局 fault/errorRate/timeout simulation。
- resources CRUD。
- OpenAPI/curl/Postman import。
- WebSocket/SSE mock 协议扩展。
- 配置文件在线编辑。

### 1.3 行为优先级

单请求选择 scenario 的优先级：

1. 请求 header `X-Fakeserver-Scenario`
2. UI runtime selected scenario
3. CLI `--scenario`
4. `server.scenario`
5. 空场景 `""`

cases route 选择 case 的优先级：

1. route override `next`，命中一次后清除。
2. route override `count`，每次命中递减，归零后清除。
3. route override `always`。
4. 当前 scenario 对该 route signature 指定的 case name。
5. 既有 `strategy` + `when` 选择逻辑。

route signature 使用规范化格式：

```text
METHOD /path
```

示例：

```text
GET /api/users
POST /api/orders
```

---

## 2. 文件结构

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `internal/config/schema.go` | 增加 `ServerOpts.Scenario`、`Config.Scenarios`、`RouteCase.Name`、`ScenarioConfig` |
| 修改 | `internal/config/defaults.go` | 初始化 nil map/slice 的默认行为，不主动启用 scenario |
| 修改 | `internal/config/validate.go` | 校验 case name 唯一性、scenario route/case 引用有效性 |
| 修改 | `internal/config/schema_test.go` | 默认值与 schema 单测 |
| 修改 | `internal/config/validate_test.go` | scenario/case name 校验单测 |
| 新建 | `internal/scenario/store.go` | 内存 selected scenario 与 route override store |
| 新建 | `internal/scenario/store_test.go` | store 并发安全与 override 消耗规则测试 |
| 新建 | `internal/scenario/resolve.go` | request/header、store、CLI、config 的 scenario 解析 |
| 新建 | `internal/scenario/resolve_test.go` | scenario 优先级测试 |
| 修改 | `internal/recorder/recorder.go` | Entry 增加 scenario/case name/override source 字段 |
| 修改 | `internal/recorder/trace.go` | RequestTrace 增加 scenario/case name/override source |
| 修改 | `internal/recorder/trace_test.go` | trace 字段回填测试 |
| 修改 | `internal/middleware/logger_test.go` | recorder 新字段集成测试 |
| 修改 | `internal/mock/selector.go` | 增加按 case name 选择候选的 helper |
| 修改 | `internal/mock/cases.go` | 接入 scenario resolver + override store + case name trace |
| 修改 | `internal/mock/router.go` | Mount 接收 scenario runtime 参数并传入 cases handler |
| 修改 | `internal/mock/cases_test.go` | named case、scenario、override 行为测试 |
| 修改 | `internal/cli/serve.go` | 增加 `--scenario`，创建并复用 scenario store，hot reload 后保留 runtime state |
| 修改 | `internal/cli/serve_test.go` | CLI scenario 与 assembleHandler 行为测试 |
| 修改 | `internal/cli/full_templates.go` | full 示例增加 named cases 和 scenarios |
| 修改 | `internal/cli/init_test.go` | full init 示例 strict/load 校验 |
| 修改 | `internal/admin/handlers.go` | routes JSON 增加 case name/scenario case 信息 |
| 修改 | `internal/admin/handlers_test.go` | routes metadata 测试 |
| 修改 | `internal/webui/api.go` | scenario state/override API |
| 修改 | `internal/webui/mount.go` | 注册 scenario API |
| 修改 | `internal/webui/api_test.go` | scenario API 测试 |
| 修改 | `internal/webui/assets/index.html` | scenario selector 与 route override 控件 |
| 修改 | `internal/webui/assets/style.css` | scenario 控制区样式 |
| 修改 | `internal/webui/assets/main.js` | scenario API 调用、UI 状态同步、override 操作 |
| 修改 | `internal/webui/assets_test.go` | DOM/JS contract 测试 |
| 新建 | `internal/cli/serve_v07_e2e_test.go` | v0.7 主链路 E2E |
| 修改 | `docs/usage/frontend-workflow.md` | 增补 scenario 控制用法 |
| 修改 | `docs/fakeserver-design.md` | 追加 v0.7 落地记录 |
| 修改 | `docs/plans/2026-05-22-fakeserver-v0.7-scenario-control-plan.md` | 完成后勾选 acceptance checklist |

---

## 3. 数据模型

### 3.1 配置 schema

目标结构：

```go
type Config struct {
    Server    ServerOpts                `json:"server"`
    Globals   map[string]any            `json:"globals"`
    Fallback  string                     `json:"fallback"`
    Routes    []Route                    `json:"routes"`
    Scenarios map[string]ScenarioConfig  `json:"scenarios"`
    SourcePaths []string                 `json:"-"`
    Env       map[string]any             `json:"-"`
    EnvSource string                     `json:"-"`
    EnvName   string                     `json:"-"`
}

type ServerOpts struct {
    Host           string        `json:"host"`
    Port           int           `json:"port"`
    CORS           any           `json:"cors"`
    Log            *bool         `json:"log"`
    MaxBodySize    string        `json:"maxBodySize"`
    Capture        CaptureConfig `json:"capture"`
    AdminEnabled   *bool         `json:"adminEnabled"`
    OSEnvWhitelist []string      `json:"osenvWhitelist"`
    FakerSeed      int64         `json:"fakerSeed"`
    HistorySize    int           `json:"historySize"`
    ProjectName    string        `json:"projectName"`
    Scenario       string        `json:"scenario"`
}

type ScenarioConfig struct {
    Routes map[string]string `json:"routes"`
}

type RouteCase struct {
    Name     string            `json:"name"`
    When     string            `json:"when"`
    Weight   int               `json:"weight"`
    Status   int               `json:"status"`
    Delay    string            `json:"delay"`
    Headers  map[string]string `json:"headers"`
    Body     any               `json:"body"`
    BodyFile string            `json:"bodyFile"`
}
```

### 3.2 Runtime store

目标 package：`internal/scenario`。

```go
type Store struct {
    mu       sync.RWMutex
    selected string
    overrides map[RouteKey]Override
}

type RouteKey struct {
    Method string `json:"method"`
    Path   string `json:"path"`
}

type Override struct {
    CaseName string `json:"caseName"`
    Mode     string `json:"mode"` // always | next | count
    Remaining int  `json:"remaining,omitempty"`
}

type State struct {
    Selected  string              `json:"selected"`
    Overrides map[string]Override `json:"overrides"`
}
```

`Store.ConsumeOverride(key RouteKey)` 的规则：

- `always`：每次返回 override，不修改。
- `next`：返回 override，并删除。
- `count`：返回 override，`Remaining--`；归零后删除。

### 3.3 recorder trace

目标字段：

```go
type Entry struct {
    Scenario       string `json:"scenario,omitempty"`
    CaseName       string `json:"caseName,omitempty"`
    OverrideSource string `json:"overrideSource,omitempty"` // header | ui | cli | config | override:next | override:count | override:always | strategy
}

type RequestTrace struct {
    Scenario       string
    CaseName       string
    OverrideSource string
}
```

---

## 4. Task 1: schema 与 validate

**Files:**
- Modify: `internal/config/schema.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/validate.go`
- Modify: `internal/config/schema_test.go`
- Modify: `internal/config/validate_test.go`

- [ ] **Step 1.1: 写 failing schema tests**

在 `internal/config/schema_test.go` 增加：

```go
func TestApplyDefaults_ScenarioDefaults(t *testing.T) {
    cfg := &Config{}
    applyDefaults(cfg)

    if cfg.Server.Scenario != "" {
        t.Fatalf("server.scenario default = %q, want empty", cfg.Server.Scenario)
    }
    if cfg.Scenarios == nil {
        t.Fatalf("scenarios map should be initialized")
    }
}
```

Run:

```bash
go test ./internal/config -run TestApplyDefaults_ScenarioDefaults -count=1
```

Expected: 编译失败，提示 `ServerOpts.Scenario` 或 `Config.Scenarios` 不存在。

- [ ] **Step 1.2: 写 failing validate tests**

在 `internal/config/validate_test.go` 增加：

```go
func TestValidate_CaseNamesUniquePerRoute(t *testing.T) {
    cfg := &Config{Routes: []Route{{
        Method: []string{"GET"},
        Path: "/api/users",
        Cases: []RouteCase{
            {Name: "empty", Status: 200, Body: map[string]any{"items": []any{}}},
            {Name: "empty", Status: 500, Body: map[string]any{"error": "duplicate"}},
        },
    }}}
    applyDefaults(cfg)

    errs := Validate(cfg)
    if len(errs) == 0 {
        t.Fatal("duplicate case names should fail validation")
    }
    if !strings.Contains(errs[0].Error(), `case name "empty" duplicated`) {
        t.Fatalf("unexpected error: %v", errs[0])
    }
}

func TestValidate_ScenarioReferencesExistingCase(t *testing.T) {
    cfg := &Config{
        Server: ServerOpts{Scenario: "emptyUsers"},
        Routes: []Route{{
            Method: []string{"GET"},
            Path: "/api/users",
            Cases: []RouteCase{
                {Name: "success", Status: 200, Body: map[string]any{"items": []any{"alice"}}},
                {Name: "empty", Status: 200, Body: map[string]any{"items": []any{}}},
            },
        }},
        Scenarios: map[string]ScenarioConfig{
            "emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
        },
    }
    applyDefaults(cfg)

    if errs := Validate(cfg); len(errs) > 0 {
        t.Fatalf("valid scenario should pass: %v", errs)
    }
}

func TestValidate_ScenarioUnknownRouteAndCase(t *testing.T) {
    cfg := &Config{
        Routes: []Route{{
            Method: []string{"GET"},
            Path: "/api/users",
            Cases: []RouteCase{{Name: "success", Status: 200}},
        }},
        Scenarios: map[string]ScenarioConfig{
            "broken": {Routes: map[string]string{
                "GET /api/missing": "empty",
                "GET /api/users": "missingCase",
            }},
        },
    }
    applyDefaults(cfg)

    errs := Validate(cfg)
    if len(errs) != 2 {
        t.Fatalf("expected 2 validation errors, got %d: %v", len(errs), errs)
    }
}
```

需要给 `validate_test.go` 添加 `strings` import。

Run:

```bash
go test ./internal/config -run 'TestValidate_(CaseNames|Scenario)' -count=1
```

Expected: 编译失败或测试失败。

- [ ] **Step 1.3: 扩展 schema**

按 §3.1 修改 `internal/config/schema.go`：

- `Config` 增加 `Scenarios map[string]ScenarioConfig`。
- `ServerOpts` 增加 `Scenario string`。
- 新增 `ScenarioConfig`。
- `RouteCase` 增加 `Name string`。

- [ ] **Step 1.4: defaults 初始化 scenarios**

在 `internal/config/defaults.go` 的 `applyDefaults` 中确保：

```go
if cfg.Globals == nil {
    cfg.Globals = map[string]any{}
}
if cfg.Scenarios == nil {
    cfg.Scenarios = map[string]ScenarioConfig{}
}
```

不要默认启用任何 scenario。

- [ ] **Step 1.5: validate scenario 引用**

在 `internal/config/validate.go` 增加 helper：

```go
func routeSignature(method, path string) string {
    return strings.ToUpper(method) + " " + path
}
```

校验规则：

- 同一 route 内非空 `case.name` 不可重复。
- `server.scenario` 非空时，必须存在于 `cfg.Scenarios`。
- `scenarios.<name>.routes` 中 route signature 必须存在，且对应 route 必须有 cases。
- scenario 指定的 case name 必须存在于该 route 的 cases。

错误信息格式建议：

```text
scenario "emptyUsers": route "GET /api/users" references unknown case "missingCase"
routes[0] GET /api/users: case name "empty" duplicated
server.scenario "missing" does not exist in scenarios
```

- [ ] **Step 1.6: 验证并提交**

Run:

```bash
go test ./internal/config -count=1
```

Commit:

```bash
git add internal/config/schema.go internal/config/defaults.go internal/config/validate.go internal/config/schema_test.go internal/config/validate_test.go
git commit -m "feat(config): add scenarios schema"
```

---

## 5. Task 2: runtime scenario store

**Files:**
- Create: `internal/scenario/store.go`
- Create: `internal/scenario/store_test.go`
- Create: `internal/scenario/resolve.go`
- Create: `internal/scenario/resolve_test.go`

- [ ] **Step 2.1: 写 store failing tests**

创建 `internal/scenario/store_test.go`：

```go
package scenario

import "testing"

func TestStoreSelectedScenario(t *testing.T) {
    s := NewStore()
    if got := s.Selected(); got != "" {
        t.Fatalf("initial selected = %q, want empty", got)
    }
    s.SetSelected("emptyUsers")
    if got := s.Selected(); got != "emptyUsers" {
        t.Fatalf("selected = %q, want emptyUsers", got)
    }
    s.SetSelected("")
    if got := s.Selected(); got != "" {
        t.Fatalf("selected after clear = %q, want empty", got)
    }
}

func TestStoreConsumeOverrideModes(t *testing.T) {
    s := NewStore()
    key := RouteKey{Method: "GET", Path: "/api/users"}

    s.SetOverride(key, Override{CaseName: "empty", Mode: "next"})
    if ov, ok := s.ConsumeOverride(key); !ok || ov.CaseName != "empty" {
        t.Fatalf("next override first consume = %#v ok=%v", ov, ok)
    }
    if _, ok := s.ConsumeOverride(key); ok {
        t.Fatal("next override should be removed after one consume")
    }

    s.SetOverride(key, Override{CaseName: "error", Mode: "count", Remaining: 2})
    for i := 0; i < 2; i++ {
        if ov, ok := s.ConsumeOverride(key); !ok || ov.CaseName != "error" {
            t.Fatalf("count override consume %d = %#v ok=%v", i, ov, ok)
        }
    }
    if _, ok := s.ConsumeOverride(key); ok {
        t.Fatal("count override should be removed after remaining reaches zero")
    }

    s.SetOverride(key, Override{CaseName: "slow", Mode: "always"})
    for i := 0; i < 3; i++ {
        if ov, ok := s.ConsumeOverride(key); !ok || ov.CaseName != "slow" {
            t.Fatalf("always override consume %d = %#v ok=%v", i, ov, ok)
        }
    }
}
```

Run:

```bash
go test ./internal/scenario -run TestStore -count=1
```

Expected: package 不存在。

- [ ] **Step 2.2: 实现 store**

创建 `internal/scenario/store.go`：

```go
package scenario

import (
    "fmt"
    "strings"
    "sync"
)

type RouteKey struct {
    Method string `json:"method"`
    Path   string `json:"path"`
}

func NewRouteKey(method, path string) RouteKey {
    return RouteKey{Method: strings.ToUpper(method), Path: path}
}

func (k RouteKey) String() string {
    return strings.ToUpper(k.Method) + " " + k.Path
}

func ParseRouteKey(sig string) (RouteKey, error) {
    method, path, ok := strings.Cut(strings.TrimSpace(sig), " ")
    if !ok || method == "" || path == "" {
        return RouteKey{}, fmt.Errorf("invalid route signature %q", sig)
    }
    return NewRouteKey(method, path), nil
}

type Override struct {
    CaseName  string `json:"caseName"`
    Mode      string `json:"mode"`
    Remaining int    `json:"remaining,omitempty"`
}

type State struct {
    Selected  string              `json:"selected"`
    Overrides map[string]Override `json:"overrides"`
}

type Store struct {
    mu        sync.RWMutex
    selected  string
    overrides map[RouteKey]Override
}

func NewStore() *Store {
    return &Store{overrides: map[RouteKey]Override{}}
}

func (s *Store) Selected() string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.selected
}

func (s *Store) SetSelected(name string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.selected = strings.TrimSpace(name)
}

func (s *Store) SetOverride(key RouteKey, ov Override) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if ov.Mode == "" {
        ov.Mode = "always"
    }
    if ov.Mode == "count" && ov.Remaining <= 0 {
        ov.Remaining = 1
    }
    s.overrides[key] = ov
}

func (s *Store) ClearOverride(key RouteKey) {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.overrides, key)
}

func (s *Store) ConsumeOverride(key RouteKey) (Override, bool) {
    s.mu.Lock()
    defer s.mu.Unlock()
    ov, ok := s.overrides[key]
    if !ok {
        return Override{}, false
    }
    switch ov.Mode {
    case "next":
        delete(s.overrides, key)
    case "count":
        ov.Remaining--
        if ov.Remaining <= 0 {
            delete(s.overrides, key)
        } else {
            s.overrides[key] = ov
        }
    }
    return ov, true
}

func (s *Store) Snapshot() State {
    s.mu.RLock()
    defer s.mu.RUnlock()
    out := State{Selected: s.selected, Overrides: map[string]Override{}}
    for k, v := range s.overrides {
        out.Overrides[k.String()] = v
    }
    return out
}
```

- [ ] **Step 2.3: 写 resolver failing tests**

创建 `internal/scenario/resolve_test.go`：

```go
package scenario

import (
    "net/http/httptest"
    "testing"

    "github.com/inhere/fakeserver/internal/config"
)

func TestResolveScenarioPriority(t *testing.T) {
    store := NewStore()
    store.SetSelected("ui")
    cfg := &config.Config{Server: config.ServerOpts{Scenario: "config"}}

    req := httptest.NewRequest("GET", "/api/users", nil)
    req.Header.Set(HeaderName, "header")
    got, source := Resolve(req, store, cfg, "cli")
    if got != "header" || source != "header" {
        t.Fatalf("header priority got scenario=%q source=%q", got, source)
    }

    req = httptest.NewRequest("GET", "/api/users", nil)
    got, source = Resolve(req, store, cfg, "cli")
    if got != "ui" || source != "ui" {
        t.Fatalf("ui priority got scenario=%q source=%q", got, source)
    }

    store.SetSelected("")
    got, source = Resolve(req, store, cfg, "cli")
    if got != "cli" || source != "cli" {
        t.Fatalf("cli priority got scenario=%q source=%q", got, source)
    }

    got, source = Resolve(req, store, cfg, "")
    if got != "config" || source != "config" {
        t.Fatalf("config priority got scenario=%q source=%q", got, source)
    }
}
```

- [ ] **Step 2.4: 实现 resolver**

创建 `internal/scenario/resolve.go`：

```go
package scenario

import (
    "net/http"
    "strings"

    "github.com/inhere/fakeserver/internal/config"
)

const HeaderName = "X-Fakeserver-Scenario"

func Resolve(req *http.Request, store *Store, cfg *config.Config, cliScenario string) (name string, source string) {
    if req != nil {
        if v := strings.TrimSpace(req.Header.Get(HeaderName)); v != "" {
            return v, "header"
        }
    }
    if store != nil {
        if v := store.Selected(); v != "" {
            return v, "ui"
        }
    }
    if strings.TrimSpace(cliScenario) != "" {
        return strings.TrimSpace(cliScenario), "cli"
    }
    if cfg != nil && strings.TrimSpace(cfg.Server.Scenario) != "" {
        return strings.TrimSpace(cfg.Server.Scenario), "config"
    }
    return "", ""
}
```

- [ ] **Step 2.5: 验证并提交**

Run:

```bash
go test ./internal/scenario -count=1
```

Commit:

```bash
git add internal/scenario
git commit -m "feat(scenario): add runtime store"
```

---

## 6. Task 3: recorder trace 增加 scenario metadata

**Files:**
- Modify: `internal/recorder/recorder.go`
- Modify: `internal/recorder/trace.go`
- Modify: `internal/recorder/trace_test.go`
- Modify: `internal/middleware/logger_test.go`

- [ ] **Step 3.1: 写 failing trace test**

在 `internal/recorder/trace_test.go` 增加：

```go
func TestRequestTraceScenarioFields(t *testing.T) {
    ctx, trace := WithRequestTrace(context.Background())
    SetRouteMatch(ctx, RequestTrace{
        Scenario: "emptyUsers",
        CaseName: "empty",
        OverrideSource: "scenario",
    })
    if trace.Scenario != "emptyUsers" {
        t.Fatalf("Scenario=%q", trace.Scenario)
    }
    if trace.CaseName != "empty" {
        t.Fatalf("CaseName=%q", trace.CaseName)
    }
    if trace.OverrideSource != "scenario" {
        t.Fatalf("OverrideSource=%q", trace.OverrideSource)
    }
}
```

Run:

```bash
go test ./internal/recorder -run TestRequestTraceScenarioFields -count=1
```

Expected: 编译失败。

- [ ] **Step 3.2: 扩展 recorder.Entry 与 RequestTrace**

在 `internal/recorder/recorder.go` 的 `Entry` 中增加：

```go
Scenario       string `json:"scenario,omitempty"`
CaseName       string `json:"caseName,omitempty"`
OverrideSource string `json:"overrideSource,omitempty"`
```

在 `internal/recorder/trace.go` 的 `RequestTrace` 中增加同名字段，并让 `SetRouteMatch` 在非空时覆盖。

- [ ] **Step 3.3: 补 logger 集成测试**

在 `internal/middleware/logger_test.go` 增加一个最小集成用例：

```go
func TestLogger_AppendsScenarioTraceFields(t *testing.T) {
    ring := recorder.New(10)
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        recorder.SetRouteMatch(r.Context(), recorder.RequestTrace{
            Scenario: "emptyUsers",
            CaseName: "empty",
            OverrideSource: "scenario",
        })
        w.WriteHeader(http.StatusOK)
    })
    wrapped := Logger(io.Discard, LoggerOptions{Quiet: true}, ring)(handler)
    wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/users", nil))

    entries := ring.Snapshot()
    if len(entries) != 1 {
        t.Fatalf("entries=%d", len(entries))
    }
    if entries[0].Scenario != "emptyUsers" || entries[0].CaseName != "empty" || entries[0].OverrideSource != "scenario" {
        t.Fatalf("scenario trace not recorded: %#v", entries[0])
    }
}
```

需要确认该文件已有 `io`、`net/http`、`net/http/httptest`、`recorder` import；缺少则补齐。

- [ ] **Step 3.4: 验证并提交**

Run:

```bash
go test ./internal/recorder ./internal/middleware -count=1
```

Commit:

```bash
git add internal/recorder internal/middleware/logger_test.go
git commit -m "feat(recorder): record scenario metadata"
```

---

## 7. Task 4: cases 选择接入 scenario 和 override

**Files:**
- Modify: `internal/mock/selector.go`
- Modify: `internal/mock/cases.go`
- Modify: `internal/mock/router.go`
- Modify: `internal/mock/cases_test.go`

- [ ] **Step 4.1: 写 selector helper tests**

在 `internal/mock/selector_test.go` 增加：

```go
func TestPickCaseByName(t *testing.T) {
    cases := []config.RouteCase{
        {Name: "success"},
        {Name: "empty"},
    }
    idx, ok := pickCaseByName(cases, "empty")
    if !ok || idx != 1 {
        t.Fatalf("pick empty = %d ok=%v", idx, ok)
    }
    if _, ok := pickCaseByName(cases, "missing"); ok {
        t.Fatal("missing case should not match")
    }
}
```

需要给 `selector_test.go` 添加 `github.com/inhere/fakeserver/internal/config` import。

- [ ] **Step 4.2: 实现 pickCaseByName**

在 `internal/mock/selector.go` 增加：

```go
func pickCaseByName(cases []config.RouteCase, name string) (int, bool) {
    if name == "" {
        return 0, false
    }
    for i := range cases {
        if cases[i].Name == name {
            return i, true
        }
    }
    return 0, false
}
```

同时给文件添加 config import。

- [ ] **Step 4.3: 写 cases behavior tests**

在 `internal/mock/cases_test.go` 增加三类用例：

```go
func TestRespondCases_UsesScenarioCase(t *testing.T) {
    route := &config.Route{
        Method: []string{"GET"},
        Path: "/api/users",
        Strategy: "first-match",
        Cases: []config.RouteCase{
            {Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
            {Name: "empty", Status: 200, Body: map[string]any{"state": "empty"}},
        },
    }
    cfg := &config.Config{
        Server: config.ServerOpts{Scenario: "emptyUsers"},
        Scenarios: map[string]config.ScenarioConfig{
            "emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
        },
    }
    rec := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/api/users", nil)
    ctx, trace := recorder.WithRequestTrace(req.Context())
    req = req.WithContext(ctx)
    c := &rux.Context{Req: req, Resp: rec}

    RespondCasesWithScenario(c, cfg, route, 0, []*Matcher{mustMatcher(t, ""), mustMatcher(t, "")}, NewSelector("first-match"), tpl.NewRenderer(nil, nil, 0), nil, nil, "")

    if !strings.Contains(rec.Body.String(), `"state":"empty"`) {
        t.Fatalf("scenario case response = %s", rec.Body.String())
    }
    if trace.CaseName != "empty" || trace.Scenario != "emptyUsers" {
        t.Fatalf("trace = %#v", trace)
    }
}
```

为避免测试依赖不存在的全局 matcher helper，如果当前测试文件没有类似 helper，新增本地 helper：

```go
func mustMatcher(t *testing.T, expr string) *Matcher {
    t.Helper()
    m, err := CompileMatcher(expr)
    if err != nil {
        t.Fatalf("CompileMatcher(%q): %v", expr, err)
    }
    return m
}
```

实际测试调用使用 `mustMatcher(t, "")`。

再补两个用例：

- `TestRespondCases_HeaderScenarioBeatsConfigScenario`：request header `X-Fakeserver-Scenario: errorUsers` 命中 header scenario。
- `TestRespondCases_OverrideNextBeatsScenarioAndConsumes`：store 设置 next override，第一次命中 override case，第二次回到 scenario case。

Run:

```bash
go test ./internal/mock -run 'TestRespondCases_.*Scenario|TestRespondCases_.*Override' -count=1
```

Expected: 编译失败，因为 `RespondCasesWithScenario` 不存在。

- [ ] **Step 4.4: 增加 scenario-aware cases 入口**

在 `internal/mock/cases.go` 保留现有 `RespondCases` 作为兼容入口，并新增：

```go
func RespondCasesWithScenario(
    c *rux.Context,
    cfg *config.Config,
    route *config.Route,
    routeIndex int,
    matchers []*Matcher,
    selector Selector,
    renderer tpl.Renderer,
    envMap map[string]any,
    scenarioStore *scenario.Store,
    cliScenario string,
) {
    scenarioName, scenarioSource := scenario.Resolve(c.Req, scenarioStore, cfg, cliScenario)
    routeKey := scenario.NewRouteKey(firstMethod(route), route.Path)

    if scenarioStore != nil {
        if ov, ok := scenarioStore.ConsumeOverride(routeKey); ok {
            if idx, found := pickCaseByName(route.Cases, ov.CaseName); found {
                respondPickedCase(c, route, routeIndex, idx, "override:"+ov.Mode, scenarioName, renderer, envMap)
                return
            }
        }
    }

    if scenarioName != "" && cfg != nil {
        if sc, ok := cfg.Scenarios[scenarioName]; ok {
            if caseName := sc.Routes[routeKey.String()]; caseName != "" {
                if idx, found := pickCaseByName(route.Cases, caseName); found {
                    respondPickedCase(c, route, routeIndex, idx, scenarioSource, scenarioName, renderer, envMap)
                    return
                }
            }
        }
    }

    // fall back to existing matcher + selector behavior
}
```

实现时建议抽出：

```go
func respondPickedCase(...)
func firstMethod(route *config.Route) string
```

`respondPickedCase` 必须写 trace：

```go
recorder.SetRouteMatch(c.Req.Context(), recorder.RequestTrace{
    RouteIndex: ptr(routeIndex),
    CaseIndex: ptr(caseIndex),
    RouteMode: "cases",
    RouteSource: route.SourceFile,
    Scenario: scenarioName,
    CaseName: chosen.Name,
    OverrideSource: source,
})
```

fallback selector 路径的 `OverrideSource` 使用 `"strategy"`，`CaseName` 使用 chosen case name，scenarioName 若有仍写入 trace。

- [ ] **Step 4.5: router.Mount 传入 runtime 依赖**

为了不破坏所有调用点，新增：

```go
type RuntimeOptions struct {
    ScenarioStore *scenario.Store
    CLIScenario string
}

func MountWithRuntime(r *rux.Router, cfg *config.Config, renderer tpl.Renderer, runtime RuntimeOptions) error
```

现有 `Mount` 改为：

```go
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
    return MountWithRuntime(r, cfg, renderer, RuntimeOptions{})
}
```

`MountWithRuntime` 在 cases handler 中调用 `RespondCasesWithScenario`。

- [ ] **Step 4.6: 验证并提交**

Run:

```bash
go test ./internal/mock -count=1
```

Commit:

```bash
git add internal/mock
git commit -m "feat(mock): select cases by scenario"
```

---

## 8. Task 5: serve CLI 接入 scenario store

**Files:**
- Modify: `internal/cli/serve.go`
- Modify: `internal/cli/serve_test.go`

- [ ] **Step 5.1: 写 failing CLI tests**

在 `internal/cli/serve_test.go` 增加：

```go
func TestAssembleHandler_UsesCLIScenario(t *testing.T) {
    cfg := &config.Config{
        Routes: []config.Route{{
            Method: []string{"GET"},
            Path: "/api/users",
            Strategy: "first-match",
            Cases: []config.RouteCase{
                {Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
                {Name: "empty", Status: 200, Body: map[string]any{"state": "empty"}},
            },
        }},
        Scenarios: map[string]config.ScenarioConfig{
            "emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
        },
    }
    enabled := true
    cfg.Server.AdminEnabled = &enabled
    rdr := tpl.NewRenderer(nil, nil, 0)
    store := scenario.NewStore()
    handler := assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true, Scenario: "emptyUsers"}, recorder.New(10), store)

    rr := httptest.NewRecorder()
    handler.ServeHTTP(rr, httptest.NewRequest("GET", "/api/users", nil))
    if !strings.Contains(rr.Body.String(), `"state":"empty"`) {
        t.Fatalf("response = %s", rr.Body.String())
    }
}
```

不要为了这个测试新增 config 包 public API；测试只需要显式设置 `AdminEnabled`，`cfg.Globals`、`cfg.Env` 可保持 nil。

Run:

```bash
go test ./internal/cli -run TestAssembleHandler_UsesCLIScenario -count=1
```

Expected: 编译失败，因为 `serveOptions.Scenario` 和 assembleHandler 签名不存在。

- [ ] **Step 5.2: serveOptions 增加 Scenario**

修改 `internal/cli/serve.go`：

```go
type serveOptions struct {
    Port int
    Host string
    ConfigFlag string
    Quiet bool
    NoCORS bool
    NoWatch bool
    EnvName string
    Scenario string
    VarOverrides gcli.Strings
}
```

在 `newServeCmd` 注册：

```go
cmd.StrOpt2(&opts.Scenario, "scenario", "Default scenario name for this serve process")
```

- [ ] **Step 5.3: assembleHandler 接收 store**

修改签名：

```go
func assembleHandler(cfg *config.Config, renderer tpl.Renderer, opts serveOptions, ring *recorder.Ring, scenarioStore *scenario.Store) http.Handler
```

调用 mock：

```go
_ = mock.MountWithRuntime(r, cfg, renderer, mock.RuntimeOptions{
    ScenarioStore: scenarioStore,
    CLIScenario: opts.Scenario,
})
```

所有现有 `assembleHandler(...)` 调用点补最后一个参数 `nil`，需要 scenario 的调用传入 store。

- [ ] **Step 5.4: runServe 创建并复用 store**

在 `runServe` 中创建：

```go
scenarioStore := scenario.NewStore()
```

首次 assemble 和 hot reload re-assemble 都传同一个 `scenarioStore`，确保 UI runtime 状态热加载后保留。

- [ ] **Step 5.5: 验证并提交**

Run:

```bash
go test ./internal/cli -run 'TestAssemble|TestRunServe|TestServe' -count=1
```

Commit:

```bash
git add internal/cli/serve.go internal/cli/serve_test.go
git commit -m "feat(cli): wire scenario selection into serve"
```

---

## 9. Task 6: admin/routes 与 Web UI scenario API

**Files:**
- Modify: `internal/admin/handlers.go`
- Modify: `internal/admin/handlers_test.go`
- Modify: `internal/webui/api.go`
- Modify: `internal/webui/mount.go`
- Modify: `internal/webui/api_test.go`

- [ ] **Step 6.1: routes metadata failing test**

在 `internal/admin/handlers_test.go` 增加或扩展：

```go
func TestRoutesHandler_IncludesCaseNamesAndScenarioCases(t *testing.T) {
    cfg := &config.Config{
        Routes: []config.Route{{
            Method: []string{"GET"},
            Path: "/api/users",
            Cases: []config.RouteCase{
                {Name: "success", Status: 200},
                {Name: "empty", Status: 200},
            },
        }},
        Scenarios: map[string]config.ScenarioConfig{
            "emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
        },
    }
    r := rux.New()
    Mount(r, cfg)
    rr := httptest.NewRecorder()
    r.ServeHTTP(rr, httptest.NewRequest("GET", "/__fakeserver/routes", nil))

    body := rr.Body.String()
    for _, want := range []string{`"name":"empty"`, `"scenarios"`, `"emptyUsers":"empty"`} {
        if !strings.Contains(body, want) {
            t.Fatalf("routes body missing %s: %s", want, body)
        }
    }
}
```

- [ ] **Step 6.2: 实现 routes metadata**

`routesHandler` 的 case item 增加：

```go
"name": cs.Name,
```

每条 route item 增加 `scenarios`：

```go
scenarioCases := map[string]string{}
sig := strings.ToUpper(m) + " " + route.Path
for name, sc := range cfg.Scenarios {
    if caseName := sc.Routes[sig]; caseName != "" {
        scenarioCases[name] = caseName
    }
}
if len(scenarioCases) > 0 {
    item["scenarios"] = scenarioCases
}
```

- [ ] **Step 6.3: Web UI API failing tests**

在 `internal/webui/api_test.go` 增加：

```go
func TestAPIScenarioState(t *testing.T) {
    store := scenario.NewStore()
    store.SetSelected("emptyUsers")
    r := rux.New()
    Mount(r, testAdminCfg(), "", recorder.New(10), store)

    rr := httptest.NewRecorder()
    r.ServeHTTP(rr, httptest.NewRequest("GET", "/__fakeserver/api/scenario", nil))
    if rr.Code != http.StatusOK {
        t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
    }
    if !strings.Contains(rr.Body.String(), `"selected":"emptyUsers"`) {
        t.Fatalf("body=%s", rr.Body.String())
    }
}

func TestAPIScenarioSetSelected(t *testing.T) {
    store := scenario.NewStore()
    r := rux.New()
    Mount(r, testAdminCfg(), "", recorder.New(10), store)

    req := httptest.NewRequest("PUT", "/__fakeserver/api/scenario", strings.NewReader(`{"selected":"errorUsers"}`))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    r.ServeHTTP(rr, req)
    if rr.Code != http.StatusOK {
        t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
    }
    if got := store.Selected(); got != "errorUsers" {
        t.Fatalf("selected=%q", got)
    }
}
```

如果 `testAdminCfg` 不存在，新增本地 helper：

```go
func testAdminCfg() *config.Config {
    enabled := true
    return &config.Config{Server: config.ServerOpts{AdminEnabled: &enabled}}
}
```

- [ ] **Step 6.4: 实现 API**

修改 `webui.Mount` 签名：

```go
func Mount(r *rux.Router, cfg *config.Config, regPath string, ring *recorder.Ring, scenarioStore *scenario.Store)
```

新增路由：

```go
r.GET("/__fakeserver/api/scenario", apiScenarioStateHandler(scenarioStore))
r.PUT("/__fakeserver/api/scenario", apiScenarioSetSelectedHandler(scenarioStore))
r.PUT("/__fakeserver/api/scenario/overrides", apiScenarioSetOverrideHandler(scenarioStore))
r.DELETE("/__fakeserver/api/scenario/overrides", apiScenarioClearOverrideHandler(scenarioStore))
```

`api.go` 中新增 request structs：

```go
type scenarioSetRequest struct {
    Selected string `json:"selected"`
}

type overrideRequest struct {
    Method string `json:"method"`
    Path string `json:"path"`
    CaseName string `json:"caseName"`
    Mode string `json:"mode"`
    Remaining int `json:"remaining"`
}
```

响应约定：

- store nil 时 GET 返回 `{selected:"", overrides:{}}`。
- PUT/DELETE store nil 时返回 404 `{error:"scenario store disabled"}`。
- invalid JSON 返回 400。
- override mode 仅允许 `always|next|count`，否则 400。

- [ ] **Step 6.5: 调整 assembleHandler 调用**

`cli.assembleHandler` 调用 `webui.Mount` 时传入同一个 `scenarioStore`。

所有测试调用补参数。

- [ ] **Step 6.6: 验证并提交**

Run:

```bash
go test ./internal/admin ./internal/webui ./internal/cli -run 'Test.*Scenario|TestRoutesHandler' -count=1
```

Commit:

```bash
git add internal/admin internal/webui internal/cli/serve.go
git commit -m "feat(webui): add scenario control api"
```

---

## 10. Task 7: Web UI scenario controls

**Files:**
- Modify: `internal/webui/assets/index.html`
- Modify: `internal/webui/assets/style.css`
- Modify: `internal/webui/assets/main.js`
- Modify: `internal/webui/assets_test.go`

- [ ] **Step 7.1: 写 asset contract tests**

在 `internal/webui/assets_test.go` 增加：

```go
func TestUIAssets_ScenarioControlContracts(t *testing.T) {
    data := readAsset(t, "assets/index.html")
    for _, want := range []string{
        `scenario-select`,
        `scenario-clear`,
        `route-override-panel`,
        `override-case`,
        `override-mode`,
        `override-remaining`,
        `override-apply`,
        `override-clear`,
    } {
        if !strings.Contains(data, want) {
            t.Fatalf("index missing %q", want)
        }
    }

    js := readAsset(t, "assets/main.js")
    for _, want := range []string{
        `loadScenarioState`,
        `setSelectedScenario`,
        `applyRouteOverride`,
        `clearRouteOverride`,
        `/__fakeserver/api/scenario`,
    } {
        if !strings.Contains(js, want) {
            t.Fatalf("main.js missing %q", want)
        }
    }
}
```

Run:

```bash
go test ./internal/webui -run TestUIAssets_ScenarioControlContracts -count=1
```

Expected: FAIL。

- [ ] **Step 7.2: HTML 增加全局 scenario 控制**

在 `index.html` 顶部 toolbar 或 header 区域增加：

```html
<label class="scenario-control">
  <span>Scenario</span>
  <select id="scenario-select"></select>
</label>
<button id="scenario-clear" type="button">Clear</button>
```

在 Routes view 中增加：

```html
<section id="route-override-panel" class="panel" hidden>
  <h2 id="override-title">Route override</h2>
  <label>Case <select id="override-case"></select></label>
  <label>Mode
    <select id="override-mode">
      <option value="always">Always</option>
      <option value="next">Next request</option>
      <option value="count">Next N requests</option>
    </select>
  </label>
  <label>Count <input id="override-remaining" type="number" min="1" value="1"></label>
  <button id="override-apply" type="button">Apply</button>
  <button id="override-clear" type="button">Clear route override</button>
  <pre id="override-status"></pre>
</section>
```

- [ ] **Step 7.3: JS state 扩展**

`main.js` 顶部 state 改为：

```js
const state = {
  history: [],
  routes: [],
  selectedEntry: null,
  config: {},
  scenario: { selected: "", overrides: {} },
  selectedRoute: null,
};
```

`loadAll` 中同时读取：

```js
getJSON("/__fakeserver/api/scenario")
```

并调用：

```js
renderScenarioControls();
```

- [ ] **Step 7.4: 渲染 scenario selector**

实现：

```js
function scenarioNames() {
  const scenarios = state.config && state.config.scenarios ? Object.keys(state.config.scenarios) : [];
  return ["", ...scenarios.sort()];
}

function renderScenarioControls() {
  const select = $("scenario-select");
  select.innerHTML = scenarioNames().map((name) => {
    const label = name || "(none)";
    return `<option value="${escapeHTML(name)}">${escapeHTML(label)}</option>`;
  }).join("");
  select.value = state.scenario.selected || "";
}

async function setSelectedScenario(name) {
  const resp = await fetch("/__fakeserver/api/scenario", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ selected: name }),
  });
  if (!resp.ok) throw new Error(`set scenario failed: ${resp.status}`);
  state.scenario = await resp.json();
  renderScenarioControls();
}
```

- [ ] **Step 7.5: Routes row 展示 cases 与 override**

`renderRoutes` 中对 cases route 增加按钮：

```js
`<button type="button" data-override-route-index="${escapeHTML(r.index)}" data-override-route-method="${escapeHTML(r.method)}">Override</button>`
```

`openRouteOverride(index, method)`：

- 找到 route。
- 填充 `override-case` 为 `route.cases[].name`，没有 name 的 case 显示 `case #<index>` 但不可作为 scenario/override 目标。
- 设置 `state.selectedRoute`。
- 显示 panel。

- [ ] **Step 7.6: apply/clear override**

实现：

```js
async function applyRouteOverride() {
  const route = state.selectedRoute;
  if (!route) return;
  const payload = {
    method: route.method,
    path: route.path,
    caseName: $("override-case").value,
    mode: $("override-mode").value,
    remaining: Number($("override-remaining").value || "1"),
  };
  const resp = await fetch("/__fakeserver/api/scenario/overrides", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!resp.ok) throw new Error(`override failed: ${resp.status}`);
  state.scenario = await resp.json();
  $("override-status").textContent = JSON.stringify(state.scenario.overrides, null, 2);
}

async function clearRouteOverride() {
  const route = state.selectedRoute;
  if (!route) return;
  const query = new URLSearchParams({ method: route.method, path: route.path });
  const resp = await fetch(`/__fakeserver/api/scenario/overrides?${query}`, { method: "DELETE" });
  if (!resp.ok) throw new Error(`clear override failed: ${resp.status}`);
  state.scenario = await resp.json();
  $("override-status").textContent = JSON.stringify(state.scenario.overrides, null, 2);
}
```

- [ ] **Step 7.7: CSS 控制布局**

新增样式要求：

- scenario selector 在 header 内横向排列。
- route override panel 不嵌套 card，使用现有 panel/card 风格最接近的样式。
- 移动端 input/select/button 不溢出。
- override status 用 `white-space: pre-wrap; overflow:auto;`。

- [ ] **Step 7.8: 验证并提交**

Run:

```bash
go test ./internal/webui -run TestUIAssets -count=1
```

Commit:

```bash
git add internal/webui/assets internal/webui/assets_test.go
git commit -m "feat(webui): add scenario controls"
```

---

## 11. Task 8: full init 示例与 E2E

**Files:**
- Modify: `internal/cli/full_templates.go`
- Modify: `internal/cli/init_test.go`
- Create: `internal/cli/serve_v07_e2e_test.go`

- [ ] **Step 8.1: full init failing test**

在 `internal/cli/init_test.go` 增加：

```go
func TestRunInitFull_IncludesScenarios(t *testing.T) {
    tmp := t.TempDir()
    opts := initOptions{cwd: tmp, full: true, out: io.Discard}
    if err := runInit(opts); err != nil {
        t.Fatalf("runInit full: %v", err)
    }
    cfg, err := config.Load([]string{filepath.Join(tmp, "fakeserver.json5")}, "dev", nil)
    if err != nil {
        t.Fatalf("load full config: %v", err)
    }
    if len(cfg.Scenarios) == 0 {
        t.Fatalf("full init should include scenarios")
    }
    if _, ok := cfg.Scenarios["emptyUsers"]; !ok {
        t.Fatalf("full init should include emptyUsers scenario: %#v", cfg.Scenarios)
    }
    if errs := config.Validate(cfg); len(errs) > 0 {
        t.Fatalf("full config should validate: %v", errs)
    }
}
```

- [ ] **Step 8.2: 更新 full template**

在 `internal/cli/full_templates.go` 的 users/orders/auth cases 中补 name，例如：

```json5
cases: [
  { name: "success", status: 200, body: { items: [...] } },
  { name: "empty", when: "request.query.empty == \"1\"", status: 200, body: { items: [] } },
  { name: "server-error", when: "request.query.fail == \"1\"", status: 500, body: { error: "failed" } },
],
```

在根配置增加：

```json5
scenarios: {
  emptyUsers: {
    routes: {
      "GET /api/users": "empty",
    },
  },
  authExpired: {
    routes: {
      "GET /api/profile": "unauthorized",
    },
  },
  serverErrors: {
    routes: {
      "GET /api/users": "server-error",
    },
  },
},
```

确保引用的 route/case 都真实存在。

- [ ] **Step 8.3: 新增 v0.7 E2E**

创建 `internal/cli/serve_v07_e2e_test.go`：

```go
package cli

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/inhere/fakeserver/internal/config"
    "github.com/inhere/fakeserver/internal/recorder"
    "github.com/inhere/fakeserver/internal/scenario"
    "github.com/inhere/fakeserver/internal/tpl"
)

func TestServeV07_ScenarioControlE2E(t *testing.T) {
    enabled := true
    cfg := &config.Config{
        Server: config.ServerOpts{AdminEnabled: &enabled, Capture: config.CaptureConfig{Enabled: true, MaxBodySize: "64KiB"}},
        Routes: []config.Route{{
            Method: []string{"GET"},
            Path: "/api/users",
            Strategy: "first-match",
            Cases: []config.RouteCase{
                {Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
                {Name: "empty", Status: 200, Body: map[string]any{"state": "empty"}},
                {Name: "server-error", Status: 500, Body: map[string]any{"state": "error"}},
            },
        }},
        Scenarios: map[string]config.ScenarioConfig{
            "emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
        },
    }
    store := scenario.NewStore()
    ring := recorder.New(20)
    handler := assembleHandler(cfg, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true}, ring, store)
    srv := httptest.NewServer(handler)
    defer srv.Close()

    req, _ := http.NewRequest("GET", srv.URL+"/api/users", nil)
    req.Header.Set(scenario.HeaderName, "emptyUsers")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("request: %v", err)
    }
    body := readBody(t, resp)
    if !strings.Contains(body, `"state":"empty"`) {
        t.Fatalf("header scenario response = %s", body)
    }

    rr := httptest.NewRecorder()
    apiReq := httptest.NewRequest("PUT", "/__fakeserver/api/scenario/overrides", strings.NewReader(`{"method":"GET","path":"/api/users","caseName":"server-error","mode":"next"}`))
    apiReq.Header.Set("Content-Type", "application/json")
    handler.ServeHTTP(rr, apiReq)
    if rr.Code != http.StatusOK {
        t.Fatalf("override api code=%d body=%s", rr.Code, rr.Body.String())
    }

    resp, err = http.Get(srv.URL + "/api/users")
    if err != nil {
        t.Fatalf("request override: %v", err)
    }
    body = readBody(t, resp)
    if !strings.Contains(body, `"state":"error"`) {
        t.Fatalf("next override response = %s", body)
    }

    entries := ring.Snapshot()
    if len(entries) == 0 {
        t.Fatal("expected history entries")
    }
    last := entries[len(entries)-1]
    if last.CaseName != "server-error" || last.OverrideSource != "override:next" {
        t.Fatalf("last history = %#v", last)
    }
}
```

如果 `readBody` helper 已存在于其他 `_test.go` 且同包可用，复用；否则在本文件追加：

```go
func readBody(t *testing.T, resp *http.Response) string {
    t.Helper()
    defer resp.Body.Close()
    data, err := io.ReadAll(resp.Body)
    if err != nil {
        t.Fatalf("read body: %v", err)
    }
    return string(data)
}
```

并添加 `io` import。

- [ ] **Step 8.4: 验证并提交**

Run:

```bash
go test ./internal/cli -run 'TestRunInitFull_IncludesScenarios|TestServeV07_ScenarioControlE2E' -count=1
```

Commit:

```bash
git add internal/cli/full_templates.go internal/cli/init_test.go internal/cli/serve_v07_e2e_test.go
git commit -m "feat(cli): add scenario examples and e2e"
```

---

## 12. Task 9: 文档收尾与质量门禁

**Files:**
- Modify: `docs/usage/frontend-workflow.md`
- Modify: `docs/fakeserver-design.md`
- Modify: `docs/plans/2026-05-22-fakeserver-v0.7-scenario-control-plan.md`

- [ ] **Step 9.1: 更新 usage 文档**

在 `docs/usage/frontend-workflow.md` 的“模拟异常态”前后增加：

- `server.scenario` 默认场景。
- `serve --scenario emptyUsers`。
- `X-Fakeserver-Scenario` header。
- Web UI scenario selector。
- Routes 页面 route override 的 `always/next/count` 说明。

示例：

```bash
fakeserver serve -c fakeserver.json5 --env dev --scenario emptyUsers
curl -H 'X-Fakeserver-Scenario: authExpired' http://127.0.0.1:5090/api/profile
```

- [ ] **Step 9.2: 回写 design**

在 `docs/fakeserver-design.md` 顶部修订表追加：

```text
| 2026-05-22 | v0.7-scenario-control-applied | inhere | v0.7：场景与异常态控制，含 case name、scenario 配置、CLI/header/UI 优先级、runtime override store、Web UI 控制 |
```

在落地确认区追加：

```markdown
### 已落地（v0.7 场景与异常态控制阶段确认）

1. **case name + scenarios 成为稳定状态切换层**：cases 支持 `name`，`scenarios` 可按 `METHOD /path` 指定 case name，validate 会拦截重复 case name、未知 route、未知 case。
2. **请求级与运行期优先级明确**：`X-Fakeserver-Scenario` > UI selected scenario > CLI `--scenario` > `server.scenario` > 默认策略；route override 支持 always/next/count 并优先于 scenario。
3. **Web UI 可操作异常态**：UI 可切换 selected scenario，并可对单 route 设置/清除 case override；history detail 记录 scenario、case name 与 override source。
```

- [ ] **Step 9.3: 勾选本计划 Acceptance Checklist**

完成实现后，把 §15 中已完成项改为 `[x]`。

- [ ] **Step 9.4: 运行质量门禁**

Run:

```bash
go test ./... -count=1
go vet ./...
go build ./...
go test -cover ./internal/config ./internal/scenario ./internal/mock ./internal/webui ./internal/cli ./internal/recorder
```

Expected:

- 所有命令通过。
- 如果 coverage 输出中某包低于既有阶段要求，不要求本阶段强行补齐全项目覆盖，但新增 `internal/scenario` 应有高覆盖。

- [ ] **Step 9.5: 提交文档收尾**

Commit:

```bash
git add docs/usage/frontend-workflow.md docs/fakeserver-design.md docs/plans/2026-05-22-fakeserver-v0.7-scenario-control-plan.md
git commit -m "docs: complete v0.7 scenario control"
```

---

## 13. Beads / Git 工作流

开始实现 v0.7 时，在 `fakeserver` 仓库内创建并领取 feature issue：

```bash
bd create --title="Implement fakeserver v0.7 scenario control" --type=feature --priority=2 --description="执行 docs/plans/2026-05-22-fakeserver-v0.7-scenario-control-plan.md：case name、scenarios、CLI/header/UI 优先级、runtime override store、Web UI 场景控制、E2E 与文档收尾。"
bd update <issue-id> --claim
```

每个 Task 完成后提交一次。每次提交前检查：

```bash
git status --short --branch
```

v0.7 完成后：

```bash
bd close <issue-id> --reason="fakeserver v0.7 scenario control completed"
git status --short --branch
```

当前仓库如果没有 remote，不执行 push，并在最终说明中明确；如果配置了 remote，按项目规范执行：

```bash
git pull --rebase
git push
git status --short --branch
```

---

## 14. 手工验证

构建并初始化示例：

```bash
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ('fakeserver-v07-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null
go build -o (Join-Path $tmp 'fakeserver.exe') ./cmd/fakeserver
Push-Location $tmp
.\fakeserver.exe init --full
.\fakeserver.exe check --strict -c fakeserver.json5
.\fakeserver.exe serve -c fakeserver.json5 --env dev --scenario emptyUsers
```

浏览器验证：

- 打开 `http://127.0.0.1:5090/__fakeserver/ui/`。
- Header 区域能看到 scenario selector。
- 选择 `emptyUsers` 后请求 `/api/users` 返回 empty case。
- Routes 页面打开某条 route override，设置 `server-error` + `next`。
- 下一次请求返回 server-error，再下一次回到 scenario 或默认策略。
- History 详情显示 scenario、case name、override source。

curl 验证：

```bash
curl http://127.0.0.1:5090/api/users
curl -H "X-Fakeserver-Scenario: emptyUsers" http://127.0.0.1:5090/api/users
curl http://127.0.0.1:5090/__fakeserver/api/scenario
```

---

## 15. Acceptance Checklist

- [ ] `RouteCase.Name` 可从 JSON5 加载。
- [ ] `Config.Scenarios` 可从 JSON5 加载。
- [ ] `server.scenario` 可从 JSON5 加载。
- [ ] validate 拦截同 route 重复 case name。
- [ ] validate 拦截 unknown `server.scenario`。
- [ ] validate 拦截 scenario unknown route signature。
- [ ] validate 拦截 scenario unknown case name。
- [ ] `internal/scenario.Store` 支持 selected scenario。
- [ ] `Store` 支持 always override。
- [ ] `Store` 支持 next override 并消费后清除。
- [ ] `Store` 支持 count override 并按剩余次数递减。
- [ ] scenario resolver 实现 header > UI > CLI > config > empty。
- [ ] cases route 优先使用 route override。
- [ ] cases route 其次使用 scenario 指定 case。
- [ ] cases route fallback 保持既有 strategy/when 逻辑。
- [ ] recorder history entry 记录 scenario。
- [ ] recorder history entry 记录 case name。
- [ ] recorder history entry 记录 override source。
- [ ] `serve --scenario` 生效。
- [ ] hot reload 后 scenario runtime store 不丢失。
- [ ] `/__fakeserver/routes` 返回 case name 和 scenario case 映射。
- [ ] `/__fakeserver/api/scenario` GET 返回 selected/overrides。
- [ ] `/__fakeserver/api/scenario` PUT 可设置 selected scenario。
- [ ] `/__fakeserver/api/scenario/overrides` PUT 可设置 route override。
- [ ] `/__fakeserver/api/scenario/overrides` DELETE 可清除 route override。
- [ ] Web UI 有 scenario selector。
- [ ] Web UI Routes 页面可设置 route case override。
- [ ] Web UI History 详情显示 scenario/case/override source。
- [ ] `init --full` 示例包含 named cases。
- [ ] `init --full` 示例包含 scenarios。
- [ ] `docs/usage/frontend-workflow.md` 说明 v0.7 用法。
- [ ] `docs/fakeserver-design.md` 追加 v0.7 落地记录。
- [ ] `go test ./... -count=1` 通过。
- [ ] `go vet ./...` 通过。
- [ ] `go build ./...` 通过。

---

## 16. Plan Self-Review

- Spec coverage: v0.7 roadmap 中的 case name、scenario 配置、header/UI/CLI/config 优先级、runtime override store、UI 控制均有对应任务；全局 fault/errorRate/timeout simulation 被明确排除在 v0.7 首轮之外。
- Placeholder scan: 本计划不包含未定占位、未指定测试或未指定文件路径的实现步骤。
- Type consistency: `scenario.Store`、`scenario.RouteKey`、`scenario.Override`、`mock.RuntimeOptions`、`serveOptions.Scenario`、recorder scenario 字段在后续任务中使用的名称保持一致。
