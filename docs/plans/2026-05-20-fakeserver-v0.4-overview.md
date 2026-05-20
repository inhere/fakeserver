# Fakeserver v0.4 阶段规划总览

> 本文档把 v0.4（Web UI + 请求历史 + SSE 实时推送）按"独立可测可交付"原则拆成 3 个 Phase。每个 Phase 对应一份 `phase<N>-*.md` 详细计划，由 `superpowers:writing-plans` 在执行前展开。
>
> **前置**：v0.3 已于 2026-05-20 完整闭环（见 [2026-05-20-fakeserver-v0.3-overview.md](2026-05-20-fakeserver-v0.3-overview.md)）。

## 修订记录

| 日期 | 版本 | 作者 | 变更说明 |
|---|---|---|---|
| 2026-05-20 | v0.4-overview | inhere | 初稿。固化 v0.4 内部的 3 Phase 拆分（内核 + SSE + UI 资源）|
| 2026-05-21 | v0.4-overview-phase1-applied | inhere | Phase 1 落地：recorder 包 + middleware logger 接入 + webui 3 JSON API + adminEnabled 护栏 |

后续修订：每完成一个 Phase 后在对应行回写 commit 摘要与实际偏差。

---

## 1. v0.4 范围与拆分原则

**v0.4 目标**：把 design §11 列出的轻量 Web UI 完整接入——请求历史环形缓冲、3 个 JSON API 端点、1 个 SSE 实时推送端点、4 个只读 UI 页面（项目/路由/历史/配置）。

design §14 路线图明确 v0.4 范围**仅含 Web UI + 历史 + SSE**——WS/SSE 协议 mock、录制-回放、json-server 风格 CRUD 留 v1.0+。

把 v0.4 切成 3 个 Phase 的依据（与 v0.1/v0.2/v0.3 拆分原则一致）：

1. **每个 Phase 末尾必须能 `go run ./cmd/fakeserver serve` 跑出一个对用户可用的产物**——Phase 1 完成后 3 个 admin JSON API 可用 curl 验证；Phase 2 完成后 `curl -N /__fakeserver/events` 看到实时事件流；Phase 3 完成后浏览器打开 `http://host:port/__fakeserver/ui/` 看到完整 UI。
2. **每个 Phase 内部模块依赖封闭**：Phase 1 = `internal/recorder` 包独立 + `webui/api.go` 注册 JSON 端点；Phase 2 = `webui` 新增 SSE handler + recorder.Subscribe 集成（recorder 包加 Subscribe 接口，Phase 1 已有 Append/Snapshot）；Phase 3 = `webui/assets/` embed.FS 静态资源 + UI 页面，不动后端。
3. **每个 Phase 自带完整测试**——单元 + 集成 + E2E，不靠后续 Phase 兜底。
4. **无新增第三方依赖（硬约束）**：design §2.4 + §14 明确 v0.4 全部用标准库（`embed` / `net/http` / `encoding/json`）。Phase 1 Task 1 spike 验证 SSE 用 `http.Flusher` 是否在 rux v2 链路上可用——这是唯一可能踩坑的点（middleware/loggingResponseWriter 已实现 Flush 透传，logger_test.go 有对应测试）。

## 2. Phase 拆分总览

| Phase | 一句话目标 | 主要新增模块 / 子命令 | 新增第三方依赖 | 前置依赖 | 估计代码量 | 状态 |
|---|---|---|---|---|---|---|
| **1** | `internal/recorder` 包（环形缓冲 + Append/Snapshot）+ middleware logger 接入 + `webui/api.go` 3 个 JSON 端点（projects/config/history）+ adminEnabled 安全护栏 | `internal/recorder/` + `internal/webui/{mount,api}.go` + middleware/logger 改造 | — | v0.3 | ~500 行 | ✅ 已完成 (commit 64bdf5a..7b4b7b9) |
| **2** | SSE 实时推送 `/__fakeserver/events` + recorder.Subscribe 多订阅 + 心跳 15s + 慢客户端非阻塞丢包 | `internal/webui/sse.go` + recorder Subscribe/Unsubscribe | — | Phase 1 | ~300 行 | 待开始 |
| **3** | embed 静态资源 + 4 个 UI 页面（侧栏项目列表 / 路由 / 历史 / 配置）+ 极简 HTML/CSS/JS + 综合 E2E + 文档收尾 | `internal/webui/assets/` + page handlers + v0.4 milestone 闭环 | — | Phase 2 | ~600 行 | 待开始 |

总计：v0.4 ≈ 1400 行代码（含测试 + 静态资源），分 3 期落地。

> **关于依赖引入时机**：v0.4 **无新增第三方依赖**（design §14 硬约束）。embed 用标准库 `embed`；JSON 用 `encoding/json`；SSE 用 `net/http` + `http.Flusher`；UI 资源是手写 HTML/CSS/JS，不引外部 CDN、不用构建工具链。

---

## 3. 各 Phase 详述

### Phase 1 — recorder 包 + 3 个 JSON API + adminEnabled 护栏

**详细计划**：[v0.4/2026-05-20-fakeserver-v0.4-phase1-recorder-api.md](v0.4/2026-05-20-fakeserver-v0.4-phase1-recorder-api.md)

**目标**：让 `internal/recorder.Ring` 环形缓冲就绪并接入 `middleware.Logger`（每条请求日志同时写一份到 ring）；`internal/webui` 包初步提供 3 个 JSON 端点 (`/__fakeserver/api/projects`、`/__fakeserver/api/config`、`/__fakeserver/api/history`)；serve 启动期按 `cfg.Server.AdminEnabled` 装配（false → 全 `/__fakeserver/*` 不挂载）。

**范围 · 包含**：

- **`internal/recorder/recorder.go`** 新文件：
  - `type Entry struct` — TS / Method / Path / Status / DurationMs / ClientIP / RouteIndex / CaseIndex / ProxyTarget（design §11.4），全部为值类型 + `encoding/json` 友好
  - `type Ring struct` — 内部 `[]Entry` 环形 + head index + size + `sync.RWMutex` + subscribers slice（Phase 2 用，Phase 1 占位字段 + `Subscribe` 接口签名预留但未实现）
  - `New(size int) *Ring` — `size <= 0 → size = 200`（默认值，与 `cfg.Server.HistorySize` 默认对齐）
  - `(r *Ring) Append(e Entry)` — 满则覆盖最旧；Phase 1 仅 ring 写入，**不**做 broadcast（Phase 2 加）
  - `(r *Ring) Snapshot() []Entry` — 返回按时间顺序的副本（最新在最后）；并发安全
  - **不**包含：`Subscribe / Unsubscribe`（留 Phase 2）
- **`internal/middleware/logger.go` 改造**：
  - 新签名：`Logger(out io.Writer, quiet bool, ring *recorder.Ring) func(http.Handler) http.Handler`
  - **向后兼容**：`ring == nil` 时行为与 v0.3 一致（仅写 stdout）
  - 改造点：写日志后追加 `ring.Append(Entry{...})`——`Entry.RouteIndex / CaseIndex / ProxyTarget` 字段 Phase 1 留空（route 命中信息要 mock/proxy 包合作；留 v1.x 加，design §11.4 也明示 Phase 1 接口先到位、字段值补全留后续）
  - 调用方 `serve.go` 改一处签名传 ring
- **`internal/webui/mount.go`** 新文件：
  - `Mount(r *rux.Router, cfg *config.Config, regPath string, ring *recorder.Ring)` — 主入口
  - 按 `cfg.Server.AdminEnabled` 决定是否挂载（design §11.6）；false → no-op + 启动期 info 日志
  - 注册 3 个 GET endpoint：`/__fakeserver/api/projects`、`/api/config`、`/api/history`
- **`internal/webui/api.go`** 新文件：
  - `apiProjectsHandler(regPath string) rux.HandlerFunc` — 读 `~/.config/fakeserver/projects.json`（通过 `registry.Load`），输出全部 Project 列表 JSON。死进程 PID 文件**不**自动清理（与 list 子命令的副作用一致；UI 通过定期刷新看到状态）
  - `apiConfigHandler(cfg *config.Config) rux.HandlerFunc` — 输出当前 `cfg` JSON（脱敏：`osenvWhitelist` 保留 key 名；`server.fakerSeed` 显示，`env` 字段中以 token/secret/password 子串结尾的 key 值替换为 `"***"`）
  - `apiHistoryHandler(ring *recorder.Ring) rux.HandlerFunc` — 输出 `ring.Snapshot()`
- **`internal/cli/serve.go` 接入**：
  - 创建 `ring := recorder.New(cfg.Server.HistorySize)`（cfg=nil → 默认 200）
  - logger 中间件传入 ring
  - `webui.Mount(r, cfg, regPath, ring)` 在 `assembleHandler` 中（在 `admin.Mount` 之后）
  - 启动期检测 `cfg.Server.Host == "0.0.0.0" && cfg.Server.AdminEnabled` → stderr 打 WARNING 行（design §11.6）
- **新增测试**：
  - `recorder_test.go`：Append 满后覆盖最旧 + Snapshot 时序 + 并发 Append + Snapshot 不撕裂
  - `logger_test.go` 扩展：传 ring → 请求后 Snapshot 含一条记录 + Status/Path/Duration 正确
  - `webui/api_test.go`：3 个端点单元测试 + 脱敏断言 + adminEnabled=false 时返回 404
  - `cli/serve_v04_e2e_test.go`：webui mount 后请求 `/__fakeserver/api/history` 看到刚才打过的请求

**范围 · 不包含**：

- SSE `/events` 端点（留 Phase 2）
- recorder.Subscribe / Unsubscribe（留 Phase 2）
- 静态资源 embed（留 Phase 3）
- UI 页面 HTML/CSS/JS（留 Phase 3）
- 请求 body 录入 Entry（design §11.4 明确不录 body；debug capture 是 v1.x 功能）
- Entry 中 RouteIndex / CaseIndex / ProxyTarget 字段填充（结构占位，值填充留 v1.x 与 mock/proxy 包协作时）

**前置依赖**：v0.3（registry 包就绪 + serve 启动期 Upsert）。

**新增第三方依赖**：无。

**DoD**：

1. `internal/recorder/recorder.go` 全部公开函数（`New / Append / Snapshot`）有测试；并发 Append + Snapshot 无 race
2. `middleware.Logger` 接受 nil ring（向后兼容）+ 接受非 nil ring 时正确写入；既有 `logger_test.go` 不回归
3. `webui.Mount` 在 `adminEnabled: false` 下完全不注册端点（GET /__fakeserver/api/projects 404）
4. 3 个 JSON 端点输出结构正确（projects / config / history）；config 端点脱敏（token/secret/password 子串 key → `"***"`）
5. 0.0.0.0 + adminEnabled 组合启动期 stderr 有 WARNING（design §11.6）
6. `go build ./...` + `go test ./...` + `go vet ./...` 全绿
7. `internal/recorder` ≥ 80%；`internal/webui` ≥ 70%（端点逻辑多，首版阈值放低）
8. bd v0.4 Phase 1 epic 创建并关闭

**对 design 章节的映射**：§2.3（依赖图：middleware→recorder，webui→recorder/registry/config）/ §11.3（API 端点 4 个里 3 个 JSON）/ §11.4（请求历史数据来源 + Entry 字段）/ §11.6（安全护栏 adminEnabled + 0.0.0.0 警告）。

**实际落地偏差**：

- **`AdminEnabled` 从 `bool` 升级为 `*bool`**：v0.1 的 known limitation——用户写 `adminEnabled: false` 与漏写无法区分——被 v0.4 修复（design §11.6 要求 `adminEnabled: false` 真正生效）。schema.go 一处字段类型变更 + defaults.go 默认逻辑 + 3 处消费点改用 `*bool` 判断；测试构造点加 `boolPtr()` helper。这是 schema 改动，影响面 7 个文件，但 JSON 序列化语义不变（Go json 模块对 `*bool` 透明）。
- **`adminOn(cfg)` 在 cfg=nil 时返回 true**：保留 v0.1 起 "echo-only 模式下 `/__fakeserver/healthz` 仍可达"的契约（cli/serve_test.go:TestServe_Healthz）；仅当 cfg.Server.AdminEnabled 显式 `*false` 时禁用 admin/webui。
- **adminEnabled=false 下注册 catch-all 404**：echo 包注册 `/*path` 兜底路由会把 `/__fakeserver/api/*` 等吞掉返回 200。assembleHandler 在 adminOn=false 时显式注册 `r.Any("/__fakeserver/*path", 404)` 阻断 echo 兜底——这是 design §11.6 "UI 也不可达"的精确语义实现。
- **`-race` 测试本机环境不可用**：Windows 上 CGO 默认关闭 + 无 gcc 工具链，无法跑 `go test -race`。recorder 并发安全靠 `sync.RWMutex` 保护 + 普通并发测试（N=16 goroutine × 1000 写 + 100 读）通过验证；CI 环境若可用 cgo 可补 race 检测。
- **Entry.RouteIndex/CaseIndex/ProxyTarget 字段值留空**：design §11.4 提到这些字段，但需要 mock/proxy 包在 handler 内反向告知 logger 命中信息——本 Phase 未做这层耦合，字段结构占位但值为零值。延后到 v1.x mock/proxy 协作时填充。

**Phase 1 测试覆盖**：5 + 8 + 5 + 2 = 20 个新增用例；`internal/recorder` **100.0%**；`internal/webui` 81.4% (≥70%)；`internal/middleware` 88.3% (维持)；`internal/cli` 51.0% (v0.3 时 49.9%，本 Phase 提升 1.1)；`internal/config` 88.3% (维持)。

**Phase 1 commit 流水**：
- Task 1: `64bdf5a` (recorder.Ring + 测试)
- Task 2: `160e8b2` (middleware.Logger 加 ring 参数)
- Task 3: `428afd4` (webui.Mount + 3 JSON 端点 + 脱敏)
- Task 4: `7b4b7b9` (serve.go 接入 + AdminEnabled *bool + 0.0.0.0 警告 + catch-all 404)
- Task 5: 文档收尾（本次 commit）

---

### Phase 2 — SSE 实时推送 + 多订阅者管理

**详细计划**：（Phase 1 完成后展开）

**目标**：让 `/__fakeserver/events` 端点把 ring 收到的每条 Entry 实时推送给所有订阅者；恢复 design §11.3 例子中的 `event: request` / `event: reload` 两类事件；心跳 15s 防代理超时；慢客户端非阻塞 send（满 → 丢弃最旧 + 打点）。

**范围 · 包含**：

- **`internal/recorder/subscribe.go`**（或并入 recorder.go）：
  - `(r *Ring) Subscribe() (<-chan Entry, func())` — 返回事件 channel + unsubscribe 函数；channel 容量 32（非阻塞 send，满则丢弃最旧 + 累加 dropped 计数）
  - `(r *Ring) Append(e Entry)` 在 Phase 2 内部 broadcast 给所有 subscribers（保持 Phase 1 的写 ring 行为）
- **`internal/webui/sse.go`** 新文件：
  - `sseEventsHandler(ring *recorder.Ring) rux.HandlerFunc`
  - 响应头：`Content-Type: text/event-stream` + `Cache-Control: no-cache` + `Connection: keep-alive`
  - 用 `http.Flusher` 立即 flush；每收到一个 Entry → `event: request\ndata: <json>\n\n`
  - 心跳：每 15s 发 `: ping\n\n`（design §11.5）
  - 客户端断开（`r.Context().Done()`）→ unsubscribe + return
- **`internal/webui/mount.go` 扩展**：注册 `GET /__fakeserver/events`
- **重载事件**（design §11.3 例 2：`event: reload`）：
  - watcher 触发 holder.Swap 后调用 ring 提供的 `EmitReload(diff ReloadDiff)`，分发给所有订阅者
  - `ReloadDiff` 字段：`{Added: []string, Removed: []string, Changed: []string}`
- **测试**：
  - `recorder/subscribe_test.go`：多 Subscriber 收到 Append + 慢客户端不阻塞 + Unsubscribe 真生效
  - `webui/sse_test.go`：用 `httptest.NewRecorder` + `goroutine` 模拟 client，断言 event/data 行格式
  - `cli/serve_v04_sse_e2e_test.go`：真起 httptest server → curl-like client EventSource 接收 1 个请求 event + heartbeat

**范围 · 不包含**：

- `event: reload` 端到端连通（Phase 2 提供 EmitReload 接口，watcher 端调用留 Phase 3 一并接 UI 的"reload 提示"组件）
- 多 server 实例间共享 ring（v0.4 内 ring 是单进程内 in-mem；多实例 SSE 留 v1.x）

**DoD**：

1. `Subscribe` 返回的 channel 在 Unsubscribe 后被关闭；多次 Unsubscribe 不 panic
2. 慢客户端满 channel → 计数 `eventsDropped` 自增，正常客户端不受影响
3. SSE 端点心跳每 15s 发一次（测试用 100ms 周期注入）
4. SSE 客户端断开 → server 端 unsubscribe 干净（goroutine 不泄漏）
5. `go build ./...` + 全包绿 + `recorder` ≥ 85%
6. bd v0.4 Phase 2 epic 创建并关闭

**对 design 章节的映射**：§11.3（`event: request` / `event: reload`）/ §11.5（SSE 心跳 + 慢客户端非阻塞 + 客户端断开自动 unsubscribe）。

---

### Phase 3 — embed 静态资源 + 4 个 UI 页面 + 综合 E2E + v0.4 milestone 闭环

**详细计划**：（Phase 2 完成后展开）

**目标**：让浏览器访问 `http://host:port/__fakeserver/ui/` 看到完整 4 页 UI（项目列表 / 路由 / 历史 / 配置）；history 页通过 `EventSource('/__fakeserver/events')` 实时追加新请求；resources 通过 `go:embed` 编入二进制（无外部文件依赖）。

**范围 · 包含**：

- **`internal/webui/assets/`** 静态资源目录：
  - `index.html` — 框架页（侧栏 + 主区 placeholder + 4 个 tab）
  - `style.css` — 系统字体 + 暗色友好 + 简单网格 / 表格
  - `main.js` — `fetch` API 4 个端点 + `EventSource` 监听 + 简单路由（hashchange）+ 表格渲染
  - `routes.html` / `history.html` / `config.html` — 或全部走 SPA 风格在 main.js 切换主区内容（取决于 Phase 3 Task 1 选型）
- **`internal/webui/assets.go`** 新文件：`//go:embed assets/*` + `http.FileServer` 包装
- **`internal/webui/mount.go` 扩展**：注册 `GET /__fakeserver/ui/*` → 静态文件 handler
- **watcher → ring.EmitReload 接入**：Phase 2 留的 EmitReload 钩子在 serve.go 的 watcher onReload 回调中触发，向所有 SSE 订阅者推 reload 事件
- **综合 E2E**：
  - 启动 serve（小 cfg）→ HTTP GET `/__fakeserver/ui/` → 看到 HTML 含 `<title>fakeserver</title>` + 4 个 tab 链接
  - GET `/__fakeserver/ui/style.css` → text/css + 含 ".sidebar" 选择器
  - GET 一个 mock 路由 → GET `/api/history` → 含 1 条
  - 0.0.0.0 + adminEnabled 启动期日志含 WARNING
- **文档收尾**：
  - v0.4 overview 回写 Phase 1/2/3 commit 流水
  - design.md 修订记录追加 `v0.4-phase0.4.1-applied` ... `v0.4-phase0.4.3-applied`
  - design.md §13 追加 Phase 1/2/3 各 3 条事实
  - bd v0.4 milestone epic 关闭

**范围 · 不包含**：

- 用户写操作（编辑路由 / 修改配置 — design §11.1 明确"只读为主"）
- 暗色模式自动切换（v1.x backlog）
- 国际化（v1.x backlog）
- 持久化历史（design §11.4 末尾明确不做；v1.x debug capture 时再考虑）
- 鉴权（design §11.6：用户公网部署自行加反向代理）

**DoD**：

1. `http.GET /__fakeserver/ui/` 返回 HTML 含侧栏 + 4 个 tab
2. UI 的 JS 通过 EventSource 接收实时事件并追加 history 表格行
3. config 页显示 JSON5（可纯文本展示，不要求语法高亮——design §11.2 提到的"JSON5 语法高亮"由极简实现裁掉，列为 Phase 3 实际偏差）
4. adminEnabled: false 下 `/__fakeserver/ui/` 也返回 404
5. embed.FS 包内一切静态资源都在 `internal/webui/assets/` 下，不引外部 CDN
6. `go build ./...` + 全包绿 + `webui` 覆盖率不下降
7. v0.4 overview 三个状态行均 `✅ 已完成`
8. design.md §13 含 v0.4 Phase 1+2+3 阶段确认（每段 **严格 3 条事实**）
9. bd v0.4 总 milestone epic 关闭；`bd ready` 无 v0.4 相关 issue

**对 design 章节的映射**：§11.2（4 个页面）/ §11.3（剩余 `event: reload` 端到端） / §14 v0.4 行清空"待开始"。

---

## 4. 跨 Phase 追踪表

| design 章节 | Phase 1 | Phase 2 | Phase 3 |
|---|:-:|:-:|:-:|
| §2.3 依赖图（middleware→recorder，webui→recorder/registry/config） | ✓ | + Subscribe | — |
| §11.1 设计原则（极简 / 只读 / embed / 无登录） | adminEnabled 护栏 | — | embed 静态资源 + 只读 UI |
| §11.2 4 个 UI 页面 | — | — | ✓ |
| §11.3 API 端点（projects/config/history/events/ui） | 前 3 个 JSON | events SSE | ui/* 静态 + reload event |
| §11.4 请求历史数据来源 + Entry 字段 | ✓ | — | — |
| §11.5 SSE 心跳 + 慢客户端 + 自动 unsubscribe | — | ✓ | — |
| §11.6 安全护栏（adminEnabled + 0.0.0.0 警告） | ✓ | — | — |
| §14 v0.4 行 | — | — | 清空"待开始" |

---

## 5. 与 design / phase plan 的关系

- **本文档（overview）**：v0.4 内部拆分的 source of truth；维护 Phase 边界与依赖关系
- **`docs/fakeserver-design.md`**：所有 Phase 共享的设计契约；任何 Phase 落地发现 design 偏差时回写 §13 "已落地"段（**严格 3 条事实**，已在 v0.2 overview §5 固化）
- **`phase<N>-*.md`**：单个 Phase 的可执行 plan，由 `superpowers:writing-plans` 在 Phase 启动前展开

执行顺序（推荐，与 v0.1/v0.2/v0.3 节奏一致）：

```
overview → phase1 → 执行 → 回写 design §13 → overview 更新状态 → phase2 → ...
```

每个 Phase 完成后必须做的事：

1. 在本文档 §2 状态列写 "✅ 已完成 (commit <SHA range>)"
2. 在对应 phase 详述末尾写"实际落地偏差"（如有）
3. 如果偏差涉及未来 Phase 的设计假设，把它前置到对应 Phase 详述里
4. 回写 design 修订记录与 §13 "已落地"段——每 Phase 落地确认条目**严格 3 条事实**

---

## 6. 风险与已知不确定项

| 不确定项 | 受影响 Phase | 风险等级 | 处置 |
|---|---|---|---|
| rux v2 链路下 `http.Flusher` 是否对 SSE 真有效（loggingResponseWriter 已实现透传，但需 spike 验证） | Phase 2 | 中 | Phase 2 Task 1 先 spike 起一个最小 SSE handler 验证 flush 路径 |
| `embed.FS` 在 cmd/fakeserver 主二进制下是否正确编入（embed 路径必须相对包目录） | Phase 3 | 低 | Phase 3 Task 1 spike：写一个 hello.html 验证 embed FS 可用 |
| recorder 的 Append broadcast 在大量订阅者下的性能（每条请求遍历 N 个 channel） | Phase 2 | 低 | 单 fakeserver 实例 UI 订阅者通常 1–3 个；超出非典型场景 |
| `cfg.Server.AdminEnabled` 在 v0.1 默认 true 的语义对 v0.4 仍合适？ | Phase 1 | 低 | 是。design §11.6 用户公网部署需自行关；本地开发默认开是 v0.4 UX 设计意图 |
| watcher 提供的 reload diff 信息（added/removed/changed）当前是否可得？ | Phase 3 | 中 | Phase 3 Task 1 探查 watcher 回调，若缺则在 watcher 包加一个轻量 diff 计算（route slice 对比） |
| 0.0.0.0 + adminEnabled 警告会不会在 v0.3 用户运行 `fakeserver serve` 时产生噪音？ | Phase 1 | 低 | 默认 host 是 0.0.0.0 + adminEnabled 默认 true → 默认配置必然触发警告。文档说明：若希望关闭警告，显式改 host 为 127.0.0.1 或关 adminEnabled |

---

## 7. 自检

| 项 | 结果 |
|---|---|
| 每 Phase 末尾可 `go run ./cmd/fakeserver serve` 跑出对用户可用产物 | ✓（Phase 1 curl 验证；Phase 2 EventSource 验证；Phase 3 浏览器验证）|
| Phase 边界清晰（API 内核 / SSE / UI 资源）| ✓ |
| 每个 Phase 自带测试（单元 + 集成 + E2E）| ✓ |
| 跨 Phase 追踪表覆盖 design §11 全部小节 | ✓ |
| 无新增第三方依赖（硬约束）| ✓ |
| 失败策略明确（adminEnabled=false 时端点不挂载）| ✓ |
| 与 v0.3 overview 节奏 / 模板一致 | ✓ |
| 风险都有 spike 兜底 | ✓ |
