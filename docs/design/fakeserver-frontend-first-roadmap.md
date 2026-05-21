# Fakeserver Frontend-First Roadmap

> 状态：设计草案
> 日期：2026-05-21
> 目标读者：fakeserver 维护者、前端联调使用者、后续阶段实现者
> 优先场景：A. 前端日常联调

## 1. 背景

fakeserver v0.4 已具备配置驱动 mock 的主链路能力：JSON5 配置、include、env 文件、模板与 faker、cases、proxy、bodyFile、热加载、请求历史、SSE 与只读 Web UI。

下一阶段不应优先追求协议覆盖或大型导入能力，而应先解决“前端开发中是否顺手”的问题。前端日常联调的核心需求不是“能不能模拟所有后端”，而是：

1. 快速初始化一个能跑的 mock 项目。
2. 配置写错时能在启动前发现，或在运行时报出可操作错误。
3. 能清楚看到请求命中了哪条 route、哪个 case、返回了什么。
4. 能方便复现请求、复制 curl、重放请求。
5. 能稳定切换 empty/error/slow/auth-expired 等前端状态。
6. 常见 CRUD 页面不需要手写大量 route。

本文档把后续功能按 v0.5+ 拆成多个可独立交付的阶段，优先服务前端开发者的日常联调效率。

## 2. 设计原则

### 2.1 简单路径优先

简单 mock 不应要求用户理解完整 schema。后续新增能力要提供低门槛入口，例如：

- `fakeserver init --full`
- Web UI route tester
- resources 自动 CRUD
- scenario 快速切换

高级 JSON5 routes/cases/proxy/template 继续保留，作为复杂场景的精确控制层。

### 2.2 配置必须可解释

用户遇到错误时，错误信息必须尽量包含：

- 配置文件路径
- route index
- method/path
- 出错字段，例如 `body`、`headers.X-Trace-Id`、`cases[1].when`
- 原始错误
- 修复建议

例如：

```text
template error in .fakeserver/routes/health.json5 routes[1] GET /api/meta body:
  bad character '-' in ".request.headers.User-Agent"
hint:
  use {{ index .request.headers "User-Agent" }}
```

### 2.3 UI 是调试台，不是管理后台

Web UI 的主目标是辅助前端联调，不是做完整低代码后台。优先做：

- 请求详情
- route tester
- copy as curl
- replay
- scenario 切换
- 资源数据查看与 reset

不优先做：

- 完整配置编辑器
- 多用户权限
- 图表大屏
- 复杂项目管理

### 2.4 默认本地友好，公网部署显式负责

默认仍面向本地开发。涉及 request/response body capture、admin UI、scenario override 等能力时，应默认安全：

- 默认绑定 `127.0.0.1`
- 敏感字段脱敏
- body capture 有大小限制
- `0.0.0.0 + adminEnabled=true` 继续 warning
- 鉴权/TLS 放到后续阶段

## 3. 阶段路线图

| 阶段 | 主题 | 主要目标 |
|---|---|---|
| v0.5 | 开发体验与诊断 | 让用户快速初始化、提前发现配置错误、看懂错误 |
| v0.6 | Web UI 调试台 | 在 UI 中看请求详情、复制 curl、replay、测试 route |
| v0.7 | 场景与异常态控制 | 稳定切换 empty/error/slow/auth-expired 等前端状态 |
| v0.8 | 资源 CRUD | 以类似 json-server 的方式快速生成列表/详情/表单接口 |
| v0.9 | 导入与生成 | 从 OpenAPI/curl/Postman 生成 route 初稿 |
| v1.0 | 实时协议与高级模拟 | SSE/WS mock、静态目录、简单 auth/TLS |

推荐执行顺序：v0.5 -> v0.6 -> v0.7 -> v0.8 -> v0.9 -> v1.0。

理由：先降低使用门槛和排错成本，再增强调试体验，最后再引入有状态资源和协议能力。

## 4. v0.5：开发体验与诊断

### 4.1 目标

让新项目能快速起步，配置错误能尽量在启动前暴露，运行期错误能直接指向修复方式。

### 4.2 功能范围

#### `fakeserver init --full`

生成一套完整示例项目：

```text
fakeserver.json5
fakeserver.env.json5
.fakeserver/
  routes/
    health.json5
    users.json5
    orders.json5
    auth.json5
    files.json5
    proxy.json5
  fixtures/
    report.json
    readme.txt
```

示例应覆盖：

- 基础 mock
- path params
- query params
- POST JSON body
- first-match cases
- weighted cases
- env 文件
- faker
- bodyFile
- proxy
- Web UI 入口说明

命令参数：

```bash
fakeserver init --full
fakeserver init --full --force
fakeserver init --minimal
```

行为约定：

- 默认不覆盖已有文件。
- `--force` 才允许覆盖。
- `.fakeserver/` 已存在时，仅补缺文件，除非 `--force`。
- 生成后打印下一步命令。

#### `fakeserver doctor`

诊断本地开发环境和当前项目配置。

检查项：

- 当前目录是否有 `fakeserver.json5`
- env 文件是否存在并能解析
- include 文件是否存在
- bodyFile 是否存在
- port 是否被占用
- proxy target 是否可解析 URL
- `host=0.0.0.0 + adminEnabled=true` 风险
- Web UI 路径是否正确
- 是否存在常见模板风险

输出示例：

```text
fakeserver doctor

OK   config: fakeserver.json5
OK   env: fakeserver.env.json5 active=dev
OK   includes: 6 files
WARN admin: host=0.0.0.0 with adminEnabled=true exposes /__fakeserver/*
FAIL bodyFile: .fakeserver/routes/files.json5 GET /api/report -> ../fixtures/report.json not found

Fix:
  create .fakeserver/fixtures/report.json or update bodyFile path.
```

#### `fakeserver check --strict`

在现有 `check` 基础上增加更严格的静态检查。

新增检查：

- 预编译所有 route `body` 模板。
- 预编译所有 route `headers` 模板。
- 预编译所有 case `body` / `headers` 模板。
- 检测 header key 访问风险，例如 `.request.headers.User-Agent`。
- 检测常见函数不存在。
- 检测 cases 中 request body 被 matcher 和 response template 重复读取的风险。

注意：strict 不要求证明所有运行期字段一定存在，只做高价值静态风险拦截。

#### 错误提示增强

运行期错误应带上下文：

```json
{
  "error": "template error (body)",
  "detail": "...",
  "route": "GET /api/meta",
  "source": ".fakeserver/routes/health.json5",
  "field": "body",
  "hint": "use {{ index .request.headers \"User-Agent\" }}"
}
```

#### 启动 banner 修正

启动输出应包含：

- config 路径
- env 文件路径
- active env
- route 统计
- Web UI 地址
- admin 暴露 warning

示例：

```text
╭─ fakeserver v0.5.0
│  listening on http://127.0.0.1:5090
│  ui:          http://127.0.0.1:5090/__fakeserver/ui/
│  config:      fakeserver.json5 (+6 includes)
│  env:         dev (fakeserver.env.json5)
│  routes:      8 mock, 4 cases, 1 proxy, fallback=echo
╰─
```

### 4.3 不包含

- UI 大改
- CRUD resources
- OpenAPI import
- WS/SSE mock
- 配置编辑器

### 4.4 DoD

- `init --full` 生成的项目可以 `check --strict` 通过。
- `doctor` 对至少 8 类问题有明确诊断。
- strict check 能提前发现模板语法错误。
- 运行期模板错误包含 source/route/field/hint。
- banner 正确显示 active env 和 UI 地址。

## 5. v0.6：Web UI 调试台

### 5.1 目标

把 Web UI 从“只读信息面板”升级为“前端联调控制台”。前端开发者可以直接在 UI 中看请求详情、复现请求、构造请求。

### 5.2 功能范围

#### History 详情抽屉

点击 history 行后展示：

- request method/path/query/headers/body
- response status/headers/body preview
- duration
- client IP
- matched route
- matched case
- proxy target
- route source file

body 展示策略：

- 默认只展示前 64KiB。
- JSON 尝试 pretty print。
- 二进制显示大小和 content type，不直接渲染。
- 敏感字段脱敏。

#### Request/Response capture

新增配置：

```json5
server: {
  capture: {
    enabled: true,
    maxBodySize: "64KiB",
    redactKeys: ["password", "token", "secret", "authorization"],
  },
}
```

建议默认：

- 本地默认关闭或只 capture metadata。
- `init --full` 可显式开启，便于演示。

#### Route 命中信息补全

recorder entry 增加：

```go
RouteIndex  int
CaseIndex   int
RouteMode   string // mock | cases | proxy | echo
RouteSource string
ProxyTarget string
```

mock/proxy handler 在响应前把命中信息写入 request context，logger/recorder 从 context 读取。

#### Copy as curl

History 详情中一键复制：

```bash
curl -X POST http://localhost:5090/api/users \
  -H "Content-Type: application/json" \
  -d '{"name":"alice"}'
```

规则：

- 默认脱敏 Authorization/Cookie。
- UI 提供“保留敏感 header”开关。

#### Replay request

从 history 中重放请求。

能力：

- 原样 replay。
- 修改 headers/body 后 replay。
- 显示 replay 响应。

#### Route tester

Routes 页面点一条 route 后打开测试面板：

- path params 表单
- query params 表单
- headers 表单
- body 编辑区
- Send 按钮
- 响应展示

### 5.3 API 设计草案

新增 admin API：

```text
GET  /__fakeserver/api/history/{id}
POST /__fakeserver/api/replay/{id}
POST /__fakeserver/api/request
```

`POST /api/request` 用于 UI route tester，body 示例：

```json
{
  "method": "POST",
  "path": "/api/users",
  "headers": {"Content-Type": "application/json"},
  "body": "{\"name\":\"alice\"}"
}
```

### 5.4 不包含

- 配置文件编辑
- 多用户权限
- 持久化历史
- 复杂 HAR 导出

### 5.5 DoD

- UI 中能打开请求详情。
- 能复制 curl。
- 能 replay 请求。
- 能从 route 页面发测试请求。
- recorder 能显示 route/case/proxy 命中信息。

## 6. v0.7：场景与异常态控制

### 6.1 目标

让前端稳定切换常见状态：成功、空列表、表单校验失败、401、403、404、409、500、慢请求、超时。

### 6.2 功能范围

#### Case 命名

cases 支持 `name`：

```json5
{
  method: "GET",
  path: "/api/users",
  strategy: "first-match",
  cases: [
    { name: "success", status: 200, body: { items: [{ id: "1" }] } },
    { name: "empty", status: 200, body: { items: [] } },
    { name: "server-error", status: 500, body: { error: "failed" } },
  ],
}
```

#### Scenario 配置

```json5
scenarios: {
  normal: {},
  emptyUsers: {
    routes: {
      "GET /api/users": "empty",
    },
  },
  authExpired: {
    routes: {
      "GET /api/profile": "unauthorized",
      "POST /api/orders": "unauthorized",
    },
  },
}
```

#### Scenario 选择优先级

建议优先级：

1. 请求 header：`X-Fakeserver-Scenario`
2. UI 当前选择
3. CLI `--scenario`
4. config 默认 scenario
5. normal

#### 强制下一次响应

UI 支持对某条 route 设置：

- 下一次返回 case X
- 接下来 N 次返回 case X
- 持续返回 case X，直到取消

后端需要一个 runtime override store。

#### 延迟和故障注入

全局控制：

```json5
faults: {
  latency: "300ms",
  errorRate: 0.1,
}
```

UI 控制：

- global latency
- route latency
- error rate
- timeout simulation

### 6.3 不包含

- 持久化 scenario runtime 状态
- 多用户隔离 scenario
- 复杂状态机

### 6.4 DoD

- UI 能切换 scenario。
- Header 能覆盖 scenario。
- 单条 route 能强制返回指定 case。
- 前端能稳定复现常见异常态。

## 7. v0.8：资源 CRUD

### 7.1 目标

让前端开发常见列表、详情、表单页面时，不需要手写 CRUD routes。

### 7.2 配置草案

```json5
resources: {
  users: {
    path: "/api/users",
    count: 20,
    id: "id",
    schema: {
      id: "uuid",
      name: "name",
      email: "email",
      role: ["admin", "editor", "viewer"],
      active: "bool",
      createdAt: "datetime",
    },
  },
}
```

### 7.3 自动生成路由

每个 resource 自动生成：

```text
GET    /api/users
GET    /api/users/{id}
POST   /api/users
PATCH  /api/users/{id}
DELETE /api/users/{id}
```

手写 routes 优先级高于 resources，便于覆盖特殊接口。

### 7.4 查询能力

第一版只做常用能力：

- `_page`
- `_limit`
- `_sort`
- `_order`
- `q`
- 字段精确过滤：`role=admin`

不做复杂 SQL 风格查询。

### 7.5 数据存储

默认 in-memory。

可选持久化：

```json5
resources: {
  store: ".fakeserver/data.json",
}
```

支持 reset：

```text
POST /__fakeserver/api/resources/reset
```

### 7.6 UI Resource 页面

功能：

- 查看 resource 列表
- 搜索
- 新增
- 编辑
- 删除
- reset seed data
- 导出当前数据

### 7.7 不包含

- 复杂关联关系
- transaction
- 权限模型
- 多实例共享 store

### 7.8 DoD

- 一个 resources 配置能支撑常见 CRUD 页面。
- 支持分页、排序、搜索、字段过滤。
- UI 能查看和 reset 数据。
- 手写 routes 能覆盖 resources。

## 8. v0.9：导入与生成

### 8.1 目标

减少从现有接口文档到 fakeserver 配置的成本。

### 8.2 OpenAPI 导入

命令：

```bash
fakeserver import openapi openapi.yaml
```

输出：

```text
.fakeserver/routes/generated/openapi.json5
```

生成策略：

1. 优先使用 OpenAPI examples。
2. 没有 example 时用 schema + faker 生成。
3. operationId 作为注释或 case name。
4. 保留可读 JSON5，而不是生成不可维护的大 blob。

第一版限制：

- 支持 OpenAPI 3.x 常用 schema。
- oneOf/allOf/anyOf 先做简单选择。
- auth 只转注释，不生成鉴权逻辑。

### 8.3 curl 导入

命令：

```bash
fakeserver import curl 'curl -X POST ...'
```

输出一个 route 初稿：

- method
- path
- headers
- request body 示例
- 默认 response body 占位

### 8.4 Postman 导入

命令：

```bash
fakeserver import postman collection.json
```

第一版只提取：

- method/path
- example response
- headers

### 8.5 UI 导入入口

Web UI 可提供：

- 粘贴 curl
- 上传 OpenAPI
- 预览生成 route

### 8.6 DoD

- 能从 OpenAPI 生成可运行 route 初稿。
- 能从 curl 生成单条 route。
- 生成结果可读、可手工维护。

## 9. v1.0：实时协议与高级模拟

### 9.1 目标

补齐实时页面开发需要的 SSE/WebSocket mock，并增加静态目录和基础保护能力。

### 9.2 SSE mock

配置草案：

```json5
{
  protocol: "sse",
  path: "/api/events",
  events: [
    { event: "ready", data: { ok: true }, at: "0ms" },
    { event: "message", data: { text: "{{ fakeSentence }}" }, every: "2s" },
  ],
}
```

### 9.3 WebSocket mock

配置草案：

```json5
{
  protocol: "websocket",
  path: "/ws/chat",
  onConnect: [
    { type: "send", data: { type: "welcome" } },
  ],
  onMessage: [
    {
      when: "message.type == \"ping\"",
      send: { type: "pong", at: "{{ now }}" },
    },
  ],
}
```

### 9.4 静态目录服务

```json5
{
  method: "GET",
  path: "/static/*rest",
  static: "./public",
}
```

### 9.5 简单保护

用于本地或局域网共享：

```json5
server: {
  adminAuth: {
    type: "basic",
    username: "dev",
    password: "{{ osenv \"FAKESERVER_ADMIN_PASSWORD\" }}",
  },
}
```

### 9.6 DoD

- SSE mock 能定时推送事件。
- WebSocket mock 能 onConnect/onMessage 响应。
- static 能服务目录文件。
- admin auth 能保护 UI 和 admin API。

## 10. 跨阶段依赖

### 10.1 v0.5 -> v0.6

v0.6 的 UI 调试台依赖 v0.5 的错误上下文与严格校验思想。尤其是 route source、field、hint 等元信息，应在 v0.5 先建立数据结构。

### 10.2 v0.6 -> v0.7

scenario 和 case override 需要 UI 能识别 route/case，并能展示 runtime 状态。v0.6 的 route tester 和 history detail 是 v0.7 的 UI 基础。

### 10.3 v0.7 -> v0.8

resources CRUD 会引入运行期状态。v0.7 的 runtime override store 可以为 resource reset、scenario 切换提供经验。

### 10.4 v0.8 -> v0.9

OpenAPI import 可以生成 routes，也可以生成 resources。先有 resources 模型再做 import，生成结果会更简洁。

## 11. 风险与取舍

### 11.1 UI 功能过重

风险：Web UI 变成复杂后台，拖慢 CLI 工具节奏。

取舍：UI 只服务调试闭环，不做完整配置管理。配置编辑仍以文件为 source of truth。

### 11.2 Capture 隐私与体积

风险：请求/响应 body 可能包含 token、密码、个人信息，也可能很大。

取舍：

- 默认不开启完整 body capture。
- 有 maxBodySize。
- 有 redactKeys。
- UI 明确标识截断。

### 11.3 Resources 状态复杂度

风险：CRUD 会引入持久化、并发、reset、多实例共享等问题。

取舍：

- 第一版只做单进程 in-memory。
- 可选 data file。
- 不做复杂关系和 transaction。

### 11.4 OpenAPI 导入复杂度

风险：OpenAPI schema 生态复杂，完整支持会拖慢进度。

取舍：

- 第一版只支持高频 schema。
- 生成可编辑初稿，不承诺 100% 契约模拟。

## 12. 建议优先实现清单

优先级从高到低：

1. `init --full`
2. `doctor`
3. `check --strict`
4. 运行期错误上下文增强
5. 启动 banner 修正 env/UI 信息
6. recorder 补 route/case/proxy 命中信息
7. History 详情抽屉
8. Copy as curl
9. Replay request
10. Route tester
11. Case name + scenario
12. UI 强制下一次响应
13. resources CRUD
14. OpenAPI import
15. SSE/WebSocket mock
