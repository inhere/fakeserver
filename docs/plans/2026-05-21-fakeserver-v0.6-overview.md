# Fakeserver v0.6 Web UI Debug Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 v0.4/v0.5 的只读 Web UI 升级为前端联调调试台：能看请求详情、识别命中 route/case/proxy、复制 curl、重放请求、从 Routes 页面直接测试接口。

**Architecture:** v0.6 不改变现有 mock/proxy/cases 的匹配与响应语义，主要新增 recorder 的请求详情数据结构、middleware 层的受限 body capture、route/proxy/mock 的命中信息回填，以及 Web UI 的详情/测试交互。请求重放与 route tester 第一版使用浏览器同源 `fetch` 发起，避免在服务端引入自调用 dispatcher；服务端只提供 history detail 与更丰富的 routes JSON。

**Tech Stack:** Go 标准库、现有 `rux`、现有 `recorder/middleware/mock/proxy/admin/webui/cli/config` 包；前端继续使用无构建链的原生 HTML/CSS/JS 与 embed assets；不新增第三方依赖。

---

## 1. v0.6 范围

### 1.1 包含

- History 详情抽屉
  - 展示 request method/path/query/headers/body summary。
  - 展示 response status/headers/body summary。
  - 展示 duration、client IP、route mode、route index、case index、route source、proxy target。
- recorder 命中信息补全
  - mock/cases/proxy handler 在请求上下文里写入命中信息。
  - logger/recorder 在响应完成后读取并写入 `recorder.Entry`。
- 受限 request/response body capture
  - 默认只记录 metadata。
  - `server.capture.enabled=true` 时捕获文本/JSON 前 `maxBodySize` 字节。
  - 二进制 body 不直接展示，只记录 content type、size、truncated。
  - header/body 中敏感 key 做脱敏。
- Copy as curl
  - History 详情里生成 curl。
  - 默认脱敏 `Authorization`、`Cookie`、token/password/secret 类字段。
- Replay request
  - 从 History 详情按 captured request 发起同源 `fetch`。
  - 浏览器禁止设置的 header 明确标记为 omitted。
  - 如果 body 未捕获，允许无 body replay，并在 UI 中提示。
- Route tester
  - Routes 页面选中 route 后打开测试面板。
  - 支持 path params、query、headers、body。
  - 使用同源 `fetch` 发送，展示响应 status/headers/body。
- 文档与初始化示例
  - `init --full` 示例配置加入 `server.capture`。
  - `docs/usage/frontend-workflow.md` 增补 v0.6 调试台用法。
  - `docs/fakeserver-design.md` 完成后追加 v0.6 落地记录。

### 1.2 不包含

- 服务端持久化 history。
- HAR 导出。
- Web UI 配置文件编辑器。
- 多用户权限或 admin auth。
- Scenario / case override。
- Resource CRUD。
- OpenAPI/curl/Postman import。
- SSE/WebSocket 协议 mock。
- 服务端自调用 replay dispatcher。

---

## 2. 当前代码边界

当前已存在：

- `internal/recorder/recorder.go`
  - `Entry` 已有 `RouteIndex`、`CaseIndex`、`ProxyTarget` 字段，但没有 ID、request/response detail、route mode/source。
  - `Ring.Snapshot()` 返回 entry 列表，没有按 ID 查询。
- `internal/middleware/logger.go`
  - logger 是 recorder 写入点，能捕获 status/duration/client IP。
  - 当前不 capture request/response body，也不读取 route 命中上下文。
- `internal/mock/router.go`
  - `Mount` 遍历 `cfg.Routes` 时知道 route index。
  - 当前 closure 没把 route index 传入 responder。
- `internal/mock/responder.go` / `internal/mock/cases.go`
  - 能在响应时知道 route/case。
  - 当前没有写 recorder trace。
- `internal/proxy/proxy.go`
  - `Mount` 遍历 `cfg.Routes` 时知道 route index。
  - `Build` 能知道 `route.Proxy.Target`。
  - 当前没有写 recorder trace。
- `internal/admin/handlers.go`
  - `/__fakeserver/routes` 仅返回 method/path/mode。
- `internal/webui/api.go`
  - `/__fakeserver/api/history` 返回 `ring.Snapshot()`。
  - 无 history detail API。
- `internal/webui/assets/main.js`
  - 现有 Projects/Routes/History/Config 四个只读表格视图。
  - History 行不可点击，无详情抽屉。

---

## 3. 文件结构

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `internal/recorder/recorder.go` | Entry 增加 ID、route mode/source、request/response detail；Ring 增加 Get |
| 新建 | `internal/recorder/trace.go` | 请求上下文 trace holder；handler 写入命中信息，logger 读取 |
| 修改 | `internal/recorder/recorder_test.go` | Entry ID、Snapshot 顺序、Get 行为 |
| 新建 | `internal/recorder/trace_test.go` | trace context helper 单测 |
| 修改 | `internal/middleware/logger.go` | 注入 trace holder；capture request/response metadata/body；写入 detail |
| 修改 | `internal/middleware/logger_test.go` | capture、脱敏、截断、二进制、trace 回填测试 |
| 修改 | `internal/config/schema.go` | 新增 `server.capture` schema |
| 修改 | `internal/config/defaults.go` | capture 默认值 |
| 修改 | `internal/config/validate.go` | capture.maxBodySize 校验 |
| 修改 | `internal/config/schema_test.go` / `internal/config/loader_test.go` | capture 配置加载与默认值 |
| 修改 | `internal/cli/full_templates.go` | `init --full` 示例启用 capture |
| 修改 | `internal/cli/init_test.go` | full init 示例 strict/load 校验仍通过，并包含 capture |
| 修改 | `internal/mock/router.go` | 给 mock/cases responder 传 route index |
| 修改 | `internal/mock/responder.go` | 单响应 route 写 trace |
| 修改 | `internal/mock/cases.go` | cases route 写 trace + case index |
| 修改 | `internal/mock/responder_test.go` / `internal/mock/cases_test.go` | trace 回填测试 |
| 修改 | `internal/proxy/proxy.go` | proxy route 写 trace |
| 修改 | `internal/proxy/proxy_test.go` | proxy trace 回填测试 |
| 修改 | `internal/admin/handlers.go` | `/__fakeserver/routes` 增加 index/source/cases/proxyTarget/params |
| 修改 | `internal/admin/handlers_test.go` | routes JSON 增强测试 |
| 修改 | `internal/webui/api.go` | 新增 `GET /__fakeserver/api/history/{id}` |
| 修改 | `internal/webui/mount.go` | 注册 history detail API |
| 修改 | `internal/webui/api_test.go` | history detail 成功/404/禁用场景 |
| 修改 | `internal/webui/assets/index.html` | 添加详情抽屉、tester 面板、额外按钮容器 |
| 修改 | `internal/webui/assets/style.css` | 抽屉、代码块、表单、响应预览样式 |
| 修改 | `internal/webui/assets/main.js` | history detail、curl、replay、route tester 交互 |
| 修改 | `internal/webui/assets_test.go` | embed assets 与关键 DOM id 回归 |
| 修改 | `internal/cli/serve_v04_e2e_test.go` 或新增 `serve_v06_e2e_test.go` | 端到端验证 history detail + route tester 基础链路 |
| 修改 | `docs/usage/frontend-workflow.md` | 增补 v0.6 UI 调试台工作流 |
| 修改 | `docs/fakeserver-design.md` | v0.6 完成后追加修订记录与 3 条落地事实 |

---

## 4. 数据模型设计

### 4.1 recorder.Entry

建议扩展为：

```go
type Entry struct {
    ID          uint64    `json:"id"`
    TS          time.Time `json:"ts"`
    Method      string    `json:"method"`
    Path        string    `json:"path"`
    Status      int       `json:"status"`
    DurationMs  float64   `json:"durationMs"`
    ClientIP    string    `json:"clientIp,omitempty"`
    RouteIndex  int       `json:"routeIndex,omitempty"`
    CaseIndex   int       `json:"caseIndex,omitempty"`
    RouteMode   string    `json:"routeMode,omitempty"`
    RouteSource string    `json:"routeSource,omitempty"`
    ProxyTarget string    `json:"proxyTarget,omitempty"`
    Request     Capture   `json:"request,omitempty"`
    Response    Capture   `json:"response,omitempty"`
}
```

注意：

- `ID` 由 `Ring.Append` 分配，单进程递增。
- `RouteIndex/CaseIndex` 现有 `omitempty` 对 0 不友好。v0.6 若要准确显示 `0`，可改成 `*int`，或保留 `int` 并在 UI 中把缺失/0 做兼容。推荐改成 `*int`，但要同步测试 JSON。
- `Request/Response` 用同一个 `Capture` 结构表达 headers/body summary。

### 4.2 recorder.Capture

```go
type Capture struct {
    Headers     map[string]string `json:"headers,omitempty"`
    ContentType string            `json:"contentType,omitempty"`
    Body        string            `json:"body,omitempty"`
    BodySize    int64             `json:"bodySize,omitempty"`
    Truncated   bool              `json:"truncated,omitempty"`
    Binary      bool              `json:"binary,omitempty"`
    Omitted     []string          `json:"omitted,omitempty"`
}
```

约定：

- `Headers` 永远脱敏后输出。
- `Body` 仅在 `server.capture.enabled=true` 且内容类型可展示时输出。
- `BodySize` 是已观察到的 body 字节数；截断时至少等于捕获字节数。
- `Binary=true` 时 `Body` 为空。
- `Omitted` 用于说明未捕获原因，例如 `capture disabled`、`binary body`、`body not read`。

### 4.3 recorder.RequestTrace

```go
type RequestTrace struct {
    RouteIndex  *int
    CaseIndex   *int
    RouteMode   string
    RouteSource string
    ProxyTarget string
}
```

logger 在请求进入时创建 `*RequestTrace` 并放入 request context；mock/cases/proxy handler 从 context 取出并填充。不要让 handler 重新赋值 `c.Req = c.Req.WithContext(...)` 作为唯一通信方式，因为外层 logger 持有的是进入时的 request 指针，重新赋值不会可靠传回外层。

---

## 5. Task 1: recorder entry ID、detail 查询与 trace context

**Files:**
- Modify: `internal/recorder/recorder.go`
- Create: `internal/recorder/trace.go`
- Modify: `internal/recorder/recorder_test.go`
- Create: `internal/recorder/trace_test.go`

### 5.1 Step 1: 写 failing tests

- [x] 新增 `TestRingAppend_AssignsIncreasingIDs`
  - 创建 `ring := recorder.New(3)`。
  - Append 三条无 ID entry。
  - Snapshot 期望 ID 为 1、2、3。
- [x] 新增 `TestRingGet_ReturnsEntryByID`
  - Append 两条。
  - `ring.Get(1)` 返回第一条。
  - `ring.Get(999)` 返回 `false`。
- [x] 新增 `TestRingGet_OverwrittenEntryNotFound`
  - 容量 2，Append 三条。
  - ID=1 被覆盖后 `Get(1)` 返回 false。
- [x] 新增 `TestRequestTraceContext`
  - `ctx, trace := recorder.WithRequestTrace(context.Background())`
  - `recorder.SetRouteMatch(ctx, recorder.RequestTrace{...})`
  - 断言 `trace` 被更新。

Run:

```bash
go test ./internal/recorder -run 'TestRing(Append|Get)|TestRequestTrace' -count=1
```

Expected:

- 编译失败或测试失败，因为 ID/Get/trace helper 还不存在。

### 5.2 Step 2: 扩展 Ring

- [x] `Ring` 增加 `nextEntryID uint64`。
- [x] `Append` 在持锁期间分配 ID：

```go
if e.ID == 0 {
    r.nextEntryID++
    e.ID = r.nextEntryID
}
```

- [x] 新增：

```go
func (r *Ring) Get(id uint64) (Entry, bool)
```

实现时在当前 ring buffer 有效窗口里线性扫描。history 默认 200，线性扫描足够。

### 5.3 Step 3: 新增 trace helper

- [x] 在 `internal/recorder/trace.go` 定义私有 context key。
- [x] 提供：

```go
func WithRequestTrace(ctx context.Context) (context.Context, *RequestTrace)
func TraceFromContext(ctx context.Context) (*RequestTrace, bool)
func SetRouteMatch(ctx context.Context, update RequestTrace)
```

`SetRouteMatch` 只覆盖非零语义字段：

- `RouteIndex != nil` 才覆盖。
- `CaseIndex != nil` 才覆盖。
- string 非空才覆盖。

### 5.4 Step 4: 验证并提交

Run:

```bash
go test ./internal/recorder -count=1
```

Commit:

```bash
git add internal/recorder/recorder.go internal/recorder/trace.go internal/recorder/recorder_test.go internal/recorder/trace_test.go
git commit -m "feat(recorder): add entry ids and request trace"
```

---

## 6. Task 2: server.capture 配置与 capture 工具函数

**Files:**
- Modify: `internal/config/schema.go`
- Modify: `internal/config/defaults.go`
- Modify: `internal/config/validate.go`
- Modify: `internal/config/schema_test.go`
- Modify: `internal/config/loader_test.go`
- Modify: `internal/cli/full_templates.go`
- Modify: `internal/cli/init_test.go`

### 6.1 Step 1: 写 failing tests

- [x] `TestApplyDefaults_CaptureDefaults`
  - 默认 `cfg.Server.Capture.Enabled == false`。
  - 默认 `MaxBodySize == "64KiB"`。
  - 默认 redact keys 包含 `authorization/cookie/password/token/secret`。
- [x] `TestValidate_CaptureBadMaxBodySize`
  - `server.capture.maxBodySize: "64XB"` 返回 validate error。
- [x] `TestRunInitFull_IncludesCaptureConfig`
  - full init 生成的根配置包含 `capture`。
  - 加载后 `cfg.Server.Capture.Enabled == true`。

Run:

```bash
go test ./internal/config -run 'TestApplyDefaults_Capture|TestValidate_Capture' -count=1
go test ./internal/cli -run TestRunInitFull_IncludesCaptureConfig -count=1
```

Expected:

- 编译失败或测试失败，因为 `Capture` schema 不存在。

### 6.2 Step 2: 扩展 schema

新增：

```go
type CaptureConfig struct {
    Enabled    bool     `json:"enabled"`
    MaxBodySize string  `json:"maxBodySize"`
    RedactKeys []string `json:"redactKeys"`
}
```

在 `ServerConfig` 中增加：

```go
Capture CaptureConfig `json:"capture"`
```

默认值：

```go
Capture: CaptureConfig{
    Enabled: false,
    MaxBodySize: "64KiB",
    RedactKeys: []string{"authorization", "cookie", "password", "token", "secret"},
}
```

### 6.3 Step 3: validate capture

- [x] 复用 `proxy.ParseByteSize` 或迁移 byte size parser 到更中性的包。
- [x] 为避免 `config -> proxy` 反向语义依赖继续扩大，推荐新增 `internal/sizeparse`：
  - `internal/sizeparse/sizeparse.go`
  - `internal/sizeparse/sizeparse_test.go`
  - proxy 和 config 都依赖它。
- [x] 如果迁移，保留 `proxy.ParseByteSize` 包装函数，避免破坏 `cli.parseMaxBodySize` 调用：

```go
func ParseByteSize(s string) (int64, error) {
    return sizeparse.ParseByteSize(s)
}
```

### 6.4 Step 4: full init 模板启用 capture

在 `fakeserver.json5` full 模板的 `server` 中加入：

```json5
capture: {
  enabled: true,
  maxBodySize: "64KiB",
  redactKeys: ["authorization", "cookie", "password", "token", "secret"],
},
```

### 6.5 Step 5: 验证并提交

Run:

```bash
go test ./internal/config ./internal/cli ./internal/proxy -count=1
```

Commit:

```bash
git add internal/config internal/cli/full_templates.go internal/cli/init_test.go internal/proxy
git commit -m "feat(config): add capture settings"
```

---

## 7. Task 3: logger capture request/response detail

**Files:**
- Modify: `internal/middleware/logger.go`
- Modify: `internal/middleware/logger_test.go`
- Modify: `internal/cli/serve.go`
- Modify: `internal/cli/serve_test.go`

### 7.1 Step 1: 写 failing tests

新增/修改 logger tests：

- [x] `TestLogger_AppendsTraceFields`
  - handler 调用 `recorder.SetRouteMatch(r.Context(), ...)`。
  - ring entry 包含 route mode/source/index/case/proxy target。
- [x] `TestLogger_CapturesTextRequestAndResponseBody`
  - capture enabled，POST JSON。
  - entry request/response body 均可见。
- [x] `TestLogger_RedactsSensitiveHeaders`
  - request header `Authorization: Bearer abc`。
  - entry 中该 header 为 `"***"`。
- [x] `TestLogger_TruncatesBody`
  - maxBodySize=8B。
  - response 写超过 8 字节。
  - entry response truncated=true，body 只含前 8 字节。
- [x] `TestLogger_DoesNotRenderBinaryBody`
  - response content-type 为 `application/octet-stream`。
  - entry response binary=true，body 为空。

Run:

```bash
go test ./internal/middleware -run 'TestLogger_.*(Trace|Capture|Redact|Truncate|Binary)' -count=1
```

Expected:

- 测试失败，因为 Logger 尚未支持 capture 和 trace。

### 7.2 Step 2: 调整 Logger 签名

当前：

```go
func Logger(out io.Writer, quiet bool, ring *recorder.Ring) func(http.Handler) http.Handler
```

建议改为：

```go
type LoggerOptions struct {
    Quiet bool
    CaptureEnabled bool
    CaptureMaxBytes int64
    RedactKeys []string
}

func Logger(out io.Writer, opts LoggerOptions, ring *recorder.Ring) func(http.Handler) http.Handler
```

兼容改动点：

- `internal/cli/serve.go` 的 `assembleHandler` 负责从 cfg 生成 `LoggerOptions`。
- 现有测试按新签名更新。

### 7.3 Step 3: 注入 trace holder

logger 请求入口：

```go
ctx, trace := recorder.WithRequestTrace(r.Context())
r = r.WithContext(ctx)
```

响应完成后把 `trace` 合并进 `recorder.Entry`。

### 7.4 Step 4: capture request body

实现 `captureReadCloser`：

- 包装 `r.Body`。
- 下游读取时 tee 前 `CaptureMaxBytes` 字节到 buffer。
- 记录是否 truncated。
- 不主动预读整个 body，避免改变 proxy/bodylimit 行为。
- 如果 handler 没有读取 request body，则 `Omitted` 包含 `body not read`。

### 7.5 Step 5: capture response body

扩展 `loggingResponseWriter`：

- 保留 status 行为。
- `Write` 时 tee 前 `CaptureMaxBytes` 字节。
- 响应结束后从 `Header().Get("Content-Type")` 判断是否可展示。
- `Flush/Hijack` 继续透传。

文本判断规则第一版：

- `application/json`
- `text/*`
- `application/javascript`
- `application/xml`
- `application/x-www-form-urlencoded`
- content type 为空但 body 是有效 UTF-8

### 7.6 Step 6: 脱敏规则

实现 helper：

```go
func redactHeaders(h http.Header, keys []string) map[string]string
```

匹配规则：

- key 小写后等于或包含 redact key。
- 默认 redact key 用 config defaults。
- 多值 header 用 `", "` join。

body 脱敏第一版只对 JSON object 做浅/递归 key 脱敏：

- content-type 包含 `json` 时尝试 `json.Unmarshal` 到 `any`。
- 对 map key 命中敏感词的值替换为 `"***"`。
- 失败则按纯文本输出，不做正则脱敏。

### 7.7 Step 7: serve 接入

在 `assembleHandler` 中：

- 读取 `cfg.Server.Capture`。
- parse max size，失败时默认 64KiB 并 stderr warn。
- cfg nil 时 capture disabled。

### 7.8 Step 8: 验证并提交

Run:

```bash
go test ./internal/middleware -count=1
go test ./internal/cli -run 'TestAssemble|TestRunServe|TestServe' -count=1
```

Commit:

```bash
git add internal/middleware internal/cli/serve.go internal/cli/serve_test.go
git commit -m "feat(middleware): capture request and response details"
```

---

## 8. Task 4: mock/cases/proxy 写入 route 命中信息

**Files:**
- Modify: `internal/mock/router.go`
- Modify: `internal/mock/responder.go`
- Modify: `internal/mock/cases.go`
- Modify: `internal/mock/responder_test.go`
- Modify: `internal/mock/cases_test.go`
- Modify: `internal/proxy/proxy.go`
- Modify: `internal/proxy/proxy_test.go`

### 8.1 Step 1: 写 failing tests

- [x] `TestRespond_SetsRecorderTrace`
  - 请求单响应 mock route。
  - handler 返回后从 trace 断言 `RouteMode=mock`、`RouteSource`、`RouteIndex`。
- [x] `TestRespondCases_SetsCaseTrace`
  - 命中 case index 1。
  - trace 中 `RouteMode=cases`、`CaseIndex=1`。
- [x] `TestProxy_SetsRecorderTrace`
  - 代理请求。
  - trace 中 `RouteMode=proxy`、`ProxyTarget`。

Run:

```bash
go test ./internal/mock ./internal/proxy -run 'Test.*Sets.*Trace' -count=1
```

Expected:

- 测试失败，因为 handler 尚未写 trace。

### 8.2 Step 2: mock.Mount 捕获 route index

`mock.Mount` 中：

```go
routeIndex := i
```

传入：

```go
Respond(c, route, routeIndex, renderer, envMap)
RespondCases(c, route, routeIndex, matchers, selector, renderer, envMap)
```

### 8.3 Step 3: responder 写 trace

单响应 route 响应开始前：

```go
recorder.SetRouteMatch(c.Req.Context(), recorder.RequestTrace{
    RouteIndex: ptr(routeIndex),
    RouteMode: "mock",
    RouteSource: route.SourceFile,
})
```

cases route 在 selector 选出 case 后写：

```go
recorder.SetRouteMatch(c.Req.Context(), recorder.RequestTrace{
    RouteIndex: ptr(routeIndex),
    CaseIndex: ptr(caseIndex),
    RouteMode: "cases",
    RouteSource: route.SourceFile,
})
```

注意：

- no case matched 时仍写 `RouteMode=cases` 和 `RouteIndex`，`CaseIndex` 留 nil。
- 不要为了 trace 改变错误响应语义。

### 8.4 Step 4: proxy 写 trace

`proxy.Mount` 传 `routeIndex` 给 `Build`：

```go
handler, err := Build(route, i, renderer, cfg.Env)
```

`Build` 返回 handler 入口处写：

```go
recorder.SetRouteMatch(c.Req.Context(), recorder.RequestTrace{
    RouteIndex: ptr(routeIndex),
    RouteMode: "proxy",
    RouteSource: route.SourceFile,
    ProxyTarget: p.Target,
})
```

### 8.5 Step 5: 验证并提交

Run:

```bash
go test ./internal/mock ./internal/proxy -count=1
go test ./internal/cli -run 'TestServe.*E2E|TestAssemble' -count=1
```

Commit:

```bash
git add internal/mock internal/proxy
git commit -m "feat(recorder): record route match metadata"
```

---

## 9. Task 5: routes API 与 history detail API

**Files:**
- Modify: `internal/admin/handlers.go`
- Modify: `internal/admin/handlers_test.go`
- Modify: `internal/webui/api.go`
- Modify: `internal/webui/mount.go`
- Modify: `internal/webui/api_test.go`

### 9.1 Step 1: 写 failing tests

Admin routes:

- [ ] `TestRoutesHandler_IncludesDebugMetadata`
  - 返回字段包含 `index/source/method/path/mode/cases/proxyTarget/params`。
  - 多 method route 仍一 method 一条。

History detail:

- [ ] `TestAPIHistoryDetail_ReturnsEntry`
  - ring append 一条 entry。
  - `GET /__fakeserver/api/history/1` 返回该 entry。
- [ ] `TestAPIHistoryDetail_NotFound`
  - 请求不存在 ID 返回 404 JSON。
- [ ] `TestAPIHistoryDetail_BadID`
  - 请求非数字 ID 返回 400 JSON。

Run:

```bash
go test ./internal/admin -run TestRoutesHandler_IncludesDebugMetadata -count=1
go test ./internal/webui -run 'TestAPIHistoryDetail' -count=1
```

Expected:

- 测试失败，因为字段和 endpoint 尚未实现。

### 9.2 Step 2: routes API 增强

输出结构建议：

```json
{
  "index": 2,
  "method": "GET",
  "path": "/api/users/{id}",
  "mode": "cases",
  "source": ".fakeserver/routes/users.json5",
  "cases": [
    {"index": 0, "name": "", "when": "request.query.empty == \"1\"", "status": 200},
    {"index": 1, "name": "", "when": "", "status": 200}
  ],
  "proxyTarget": "",
  "params": ["id"]
}
```

`params` 第一版从 path 字符串提取：

- `{id}` -> `id`
- `*rest` -> `rest`

### 9.3 Step 3: history detail API

新增：

```go
func apiHistoryDetailHandler(ring *recorder.Ring) rux.HandlerFunc
```

路由：

```go
r.GET("/__fakeserver/api/history/{id}", apiHistoryDetailHandler(ring))
```

行为：

- ring nil -> 404。
- id 非 uint -> 400。
- not found -> 404。
- found -> 200 entry JSON。

### 9.4 Step 4: 验证并提交

Run:

```bash
go test ./internal/admin ./internal/webui -count=1
```

Commit:

```bash
git add internal/admin internal/webui/api.go internal/webui/mount.go internal/webui/api_test.go
git commit -m "feat(webui): add route metadata and history detail api"
```

---

## 10. Task 6: Web UI History 详情抽屉与 Copy as curl

**Files:**
- Modify: `internal/webui/assets/index.html`
- Modify: `internal/webui/assets/style.css`
- Modify: `internal/webui/assets/main.js`
- Modify: `internal/webui/assets_test.go`

### 10.1 Step 1: 写 asset regression tests

在 `assets_test.go` 中新增断言 HTML/JS/CSS 包含关键元素：

- [ ] `history-detail-drawer`
- [ ] `history-detail-body`
- [ ] `copy-curl-button`
- [ ] `replay-button`
- [ ] `route-tester`

Run:

```bash
go test ./internal/webui -run TestAssets -count=1
```

Expected:

- 测试失败，因为 DOM id 不存在。

### 10.2 Step 2: HTML 增加抽屉

在 History view 中加入：

```html
<aside id="history-detail-drawer" class="drawer" hidden>
  <div class="drawer-header">
    <h2 id="history-detail-title">Request</h2>
    <button id="history-detail-close" type="button">Close</button>
  </div>
  <div id="history-detail-body" class="drawer-body"></div>
  <div class="drawer-actions">
    <button id="copy-curl-button" type="button">Copy curl</button>
    <button id="replay-button" type="button">Replay</button>
  </div>
</aside>
```

按钮文案可以后续换 icon；v0.6 先保证可用性。

### 10.3 Step 3: JS 行点击加载详情

实现：

- history table row 增加 `data-history-id`。
- click 后调用：

```js
const entry = await getJSON(`/__fakeserver/api/history/${id}`);
```

- 渲染分区：
  - Summary
  - Matched route
  - Request headers/body
  - Response headers/body

Body 展示：

- JSON 字符串尝试 `JSON.parse` + `JSON.stringify(..., null, 2)`。
- `binary=true` 显示 `Binary body omitted`。
- `truncated=true` 显示 `truncated at <n> bytes`。

### 10.4 Step 4: Copy as curl

实现 helper：

```js
function buildCurl(entry) {}
```

规则：

- URL 使用 `location.origin + entry.path`。
- method 非 GET/HEAD 时加 `-X METHOD`。
- headers 来自 `entry.request.headers`。
- 跳过或脱敏值为 `"***"` 的敏感 header。
- body 存在时加 `--data-raw`。
- 单引号 body 用 shell 安全拼接：`'abc'\''def'`。

Clipboard：

```js
await navigator.clipboard.writeText(curl);
```

失败时 fallback 到 `<textarea>` select + `document.execCommand("copy")`。

### 10.5 Step 5: 样式

抽屉要求：

- 桌面右侧固定宽度，最大 560px。
- 移动端全屏覆盖。
- body code block 可横向滚动。
- 不让长 header/body 撑破布局。

### 10.6 Step 6: 验证并提交

Run:

```bash
go test ./internal/webui -run TestAssets -count=1
go test ./internal/webui -count=1
```

Manual:

```bash
go run ./cmd/fakeserver serve -c fakeserver.json5 --env dev
```

打开 `http://127.0.0.1:5090/__fakeserver/ui/`，发起任意 mock 请求后确认 History 行可打开详情，Copy curl 可复制。

Commit:

```bash
git add internal/webui/assets internal/webui/assets_test.go
git commit -m "feat(webui): add history detail drawer"
```

---

## 11. Task 7: Web UI Replay request

**Files:**
- Modify: `internal/webui/assets/main.js`
- Modify: `internal/webui/assets/style.css`
- Modify: `internal/webui/assets_test.go`

### 11.1 Step 1: 写 asset regression tests

在 assets test 中断言 JS 包含：

- [ ] `function replayEntry`
- [ ] `replay-result`
- [ ] `forbiddenHeaders`

Run:

```bash
go test ./internal/webui -run TestAssets -count=1
```

Expected:

- 测试失败，因为 replay 逻辑尚未存在。

### 11.2 Step 2: replay 策略

第一版使用浏览器同源 fetch：

```js
async function replayEntry(entry) {
  const headers = replayableHeaders(entry.request?.headers || {});
  const init = { method: entry.method || "GET", headers };
  if (!["GET", "HEAD"].includes(init.method) && entry.request?.body) {
    init.body = entry.request.body;
  }
  const resp = await fetch(entry.path, init);
  return readFetchResponse(resp);
}
```

跳过 forbidden headers：

- `host`
- `connection`
- `content-length`
- `cookie`
- `origin`
- `referer`
- `sec-*`

UI 显示：

- replay status。
- response headers。
- response body preview。
- omitted headers 列表。
- body 未捕获时提示 `request body was not captured; replay sent without body`。

### 11.3 Step 3: 防止 admin 自递归

如果 `entry.path` 以 `/__fakeserver/` 开头：

- Replay 按钮 disabled。
- 提示 `admin requests are not replayed from the UI`。

### 11.4 Step 4: 验证并提交

Run:

```bash
go test ./internal/webui -run TestAssets -count=1
```

Manual:

- 对 `GET /api/users` replay，确认产生新的 history entry。
- 对 `POST /api/users` replay，capture enabled 时带 body。
- 对 `/__fakeserver/api/history` replay 按钮禁用。

Commit:

```bash
git add internal/webui/assets internal/webui/assets_test.go
git commit -m "feat(webui): replay history requests"
```

---

## 12. Task 8: Web UI Route tester

**Files:**
- Modify: `internal/webui/assets/index.html`
- Modify: `internal/webui/assets/style.css`
- Modify: `internal/webui/assets/main.js`
- Modify: `internal/webui/assets_test.go`

### 12.1 Step 1: 写 asset regression tests

断言 assets 包含：

- [ ] `route-tester`
- [ ] `tester-method`
- [ ] `tester-path`
- [ ] `tester-query`
- [ ] `tester-headers`
- [ ] `tester-body`
- [ ] `tester-send`
- [ ] `tester-response`

Run:

```bash
go test ./internal/webui -run TestAssets -count=1
```

Expected:

- 测试失败，因为 tester DOM 不存在。

### 12.2 Step 2: Routes 表格可选择

Routes row 增加：

```html
<button type="button" data-test-route-index="...">Test</button>
```

点击后打开 tester panel，并填充：

- method
- path template
- mode/source/cases summary
- path params 输入框

### 12.3 Step 3: path params 替换

实现：

```js
function buildPath(route, params, queryText) {}
```

规则：

- `/api/users/{id}` 用 params.id 替换。
- `/files/*rest` 用 params.rest 替换，允许包含 `/`。
- query textarea 支持：

```text
page=1
limit=20
role=admin
```

转换为 URLSearchParams。

### 12.4 Step 4: headers/body 编辑

Headers textarea 格式：

```text
Content-Type: application/json
X-Debug: 1
```

Body textarea 原样发送。

默认：

- GET/HEAD body disabled。
- POST/PATCH/PUT 自动填 `{}` 和 `Content-Type: application/json`。

### 12.5 Step 5: 发送与响应展示

```js
const resp = await fetch(path, { method, headers, body });
```

展示：

- status。
- response headers。
- response body pretty JSON 或文本。
- duration。

发送完成后 history SSE 会追加一条 entry；不需要手动插入 history。

### 12.6 Step 6: 验证并提交

Run:

```bash
go test ./internal/webui -run TestAssets -count=1
go test ./internal/admin ./internal/webui -count=1
```

Manual:

- 在 Routes 页面测试 `GET /api/health`。
- 测试带 `{id}` 的 route。
- 测试 POST JSON route。

Commit:

```bash
git add internal/webui/assets internal/webui/assets_test.go
git commit -m "feat(webui): add route tester"
```

---

## 13. Task 9: v0.6 E2E 与文档收尾

**Files:**
- Create: `internal/cli/serve_v06_e2e_test.go`
- Modify: `docs/usage/frontend-workflow.md`
- Modify: `docs/fakeserver-design.md`
- Modify: `docs/plans/2026-05-21-fakeserver-v0.6-overview.md`

### 13.1 Step 1: 写 E2E

新增 `TestServeV06_WebUIDebugConsoleE2E`：

流程：

1. temp dir 执行 `runInit(initOptions{full:true})`。
2. 启动 assemble handler 或 httptest server。
3. 请求 `GET /api/users`。
4. 请求 `GET /__fakeserver/api/history`。
5. 取第一条 ID。
6. 请求 `GET /__fakeserver/api/history/{id}`。
7. 断言：
   - ID 非 0。
   - method/path/status 存在。
   - routeMode 非空。
   - routeIndex 存在。
   - request/response capture 字段结构存在。
8. 请求 `GET /__fakeserver/routes`。
9. 断言 route metadata 包含 index/source/mode。

Run:

```bash
go test ./internal/cli -run TestServeV06_WebUIDebugConsoleE2E -count=1
```

Expected:

- 如果前面任务完整，测试通过；否则补齐缺口。

### 13.2 Step 2: 更新 usage 文档

在 `docs/usage/frontend-workflow.md` 增加：

- 如何启用 `server.capture`。
- 如何打开 History 详情。
- Copy as curl 的脱敏说明。
- Replay 的浏览器限制说明。
- Route tester 的 path/query/header/body 写法。

### 13.3 Step 3: 回写 design

v0.6 完成后追加：

- 修订记录 `v0.6-debug-console-applied`。
- `### 已落地（v0.6 Web UI 调试台阶段确认）`。
- 严格 3 条事实：
  1. recorder detail/capture/trace。
  2. History detail + copy curl + replay。
  3. Route tester + routes metadata。

注意：

- 当前工作区可能仍有用户/历史未提交的 `docs/fakeserver-design.md` 变更。提交时必须只 stage v0.6 新增 hunk，不要混入既有删除。

### 13.4 Step 4: 更新本计划 checklist

把 §16 Acceptance Checklist 中已完成项改为 `[x]`。

### 13.5 Step 5: 验证并提交

Run:

```bash
go test ./... -count=1
go vet ./...
go build ./...
```

Commit:

```bash
git add internal/cli/serve_v06_e2e_test.go docs/usage/frontend-workflow.md docs/plans/2026-05-21-fakeserver-v0.6-overview.md
git add -p docs/fakeserver-design.md
git commit -m "docs: complete v0.6 debug console"
```

---

## 14. Final Quality Gates

在宣布 v0.6 完成前必须运行：

```bash
go test ./... -count=1
go vet ./...
go build ./...
go test -cover ./internal/recorder ./internal/middleware ./internal/webui ./internal/admin ./internal/mock ./internal/proxy
```

真实 CLI/UI 验证：

```bash
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ('fakeserver-v06-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null
go build -o (Join-Path $tmp 'fakeserver.exe') ./cmd/fakeserver
Push-Location $tmp
.\fakeserver.exe init --full
.\fakeserver.exe check --strict -c fakeserver.json5
.\fakeserver.exe doctor -c fakeserver.json5 --env dev
.\fakeserver.exe serve -c fakeserver.json5 --env dev
```

浏览器手工验证：

- 打开 `http://127.0.0.1:5090/__fakeserver/ui/`。
- 访问 `http://127.0.0.1:5090/api/users` 产生 history。
- History 行可打开详情。
- Copy curl 可复制。
- Replay 可产生新 history。
- Routes 页面 tester 可发送 GET/POST。

如果环境可用，使用 Browser/Playwright 截图验证：

- 桌面 1280x800。
- 移动 390x844。
- History 抽屉不遮挡主导航。
- Route tester 表单不溢出。
- 长 body/code block 可滚动。

---

## 15. Beads / Git 收尾

开始实现时创建并 claim v0.6 issue：

```bash
bd create "Implement fakeserver v0.6 Web UI debug console" --type feature --priority 2 --description "执行 docs/plans/2026-05-21-fakeserver-v0.6-overview.md：history detail、route metadata、capture、copy curl、replay、route tester。"
bd update <issue-id> --claim
```

每个 Task 完成后：

```bash
git status --short
git add <task files>
git commit -m "<task commit message>"
```

v0.6 全部完成后：

```bash
bd close <issue-id> --reason="fakeserver v0.6 Web UI debug console completed"
git status --short --branch
```

如果仓库没有 remote，最终说明无法 push；如果配置了 remote，按项目规则执行：

```bash
git pull --rebase
git push
git status
```

---

## 16. Acceptance Checklist

- [x] recorder entry 有稳定递增 ID。
- [x] `Ring.Get(id)` 可查询未覆盖 history entry。
- [x] logger 能从 request context 读取 route/case/proxy 命中信息。
- [x] mock 单响应 route 写入 `routeMode=mock`、route index、source。
- [x] cases route 写入 `routeMode=cases`、route index、case index、source。
- [x] proxy route 写入 `routeMode=proxy`、route index、source、proxy target。
- [x] `server.capture` schema/defaults/validate 完成。
- [x] capture enabled 时 request/response 文本 body 有限捕获。
- [x] capture 对敏感 headers 和 JSON body key 脱敏。
- [x] capture 对二进制 body 不直接展示。
- [ ] `/__fakeserver/routes` 返回 index/source/cases/proxyTarget/params。
- [ ] `/__fakeserver/api/history/{id}` 返回单条详情，支持 400/404。
- [ ] History 行可打开详情抽屉。
- [ ] History 详情展示 request/response headers/body 与 route metadata。
- [ ] Copy as curl 可用，默认脱敏敏感 header。
- [ ] Replay request 可用，并说明浏览器 forbidden header/body 未捕获限制。
- [ ] Routes 页面 route tester 可发送 GET/POST 请求。
- [x] `init --full` 示例启用 capture。
- [ ] `docs/usage/frontend-workflow.md` 更新 v0.6 用法。
- [ ] `docs/fakeserver-design.md` 追加 v0.6 落地记录。
- [ ] `go test ./... -count=1` 通过。
- [ ] `go vet ./...` 通过。
- [ ] `go build ./...` 通过。
