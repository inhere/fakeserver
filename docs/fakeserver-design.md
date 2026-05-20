# Fakeserver 设计文档

> 状态：草稿 v0.3（待评审）
> 适用版本：v0.x – v0.4

## 修订记录

| 日期 | 版本 | 作者 | 变更说明 |
|---|---|---|---|
| 2026-05-19 | v0.1-draft | inhere | 初稿。覆盖 MVP 范围（核心 mock + 模板），WS/SSE 留 v2 |
| 2026-05-19 | v0.2-draft | inhere | 补充：环境配置文件 / Proxy 路由 / 项目注册 / Web UI / Faker；MVP 纳入 faker + proxy；其余按 v0.2–v0.4 分期。`.env` 命名空间调整（文件 env），原 OS env 改名 `.osenv` |
| 2026-05-19 | v0.3-draft | inhere | 复审补全：跨条目去重报错、OPTIONS 顺序、Content-Type 推断表、admin CORS 豁免、proxy 模板边界、默认配置查找、init 产物、bodyLimit 适用对象、cases 兜底、启动摘要样例、CLI 长格式、proxy 错误 route 字段、前台运行说明、字段顺序进未决项、静态目录服务进 v1.x |
| 2026-05-19 | v0.3-phase1-applied | inhere | Phase 1 落地：项目骨架 + internal/cli + 极薄 cmd 入口 + echo（rux v2 MountEchoRoutes）+ admin /healthz + E2E。rux 实际为 v2.0.0（module path `github.com/gookit/rux/v2`），与原 design 假设接口名不同，详见 §13 已落地条目 |
| 2026-05-19 | v0.3-phase2-applied | inhere | Phase 2 落地：internal/config 包（schema/loader/include/merge/defaults/validate）+ cli init/check/routes + serve 接入 -c。mock 路由仅打印摘要，实际响应留 Phase 3 |
| 2026-05-19 | v0.3-phase3-applied | inhere | Phase 3 落地：internal/tpl（含 22+ 自有函数 + gofakeit 桥接 + 双渲染器）+ internal/mock（router/responder）+ serve 接入。单一响应模式 mock 真正生效；cases/proxy 留 Phase 4 |
| 2026-05-19 | v0.3-phase4-applied | inhere | Phase 4 落地：internal/mock 增 matcher（expr）+ selector（四 strategy）+ cases.go；internal/proxy 整包（ReverseProxy + 全量字段）；config.Validate 增 when 语法预检；新增 config.Warn 警告通道（first-match 无兜底、proxy.target 私网 info）；cli 装配 proxy.Mount 与 Warn 输出 |
| 2026-05-19 | v0.3-phase5-applied | inhere | Phase 5 落地：internal/middleware（recoverer/logger/cors/bodylimit/chain/holder）+ internal/config/watcher.go + admin /__fakeserver/routes + 启动 banner + serve --quiet/--no-cors/--no-watch flag。v0.1 MVP 完整闭环。 |

后续修订请按时间倒序追加。每次评审/落地变更必须更新本表，并在对应章节内打 `(v0.X 修订)` 锚点。

---

## 1. 背景概述

### 1.1 项目目标

fakeserver 是一个 **配置驱动的 HTTP Mock/Fake 服务器**，用于在前后端联调、依赖外部服务尚未就绪、自动化测试等场景下，快速搭建可控的接口响应。它解决四类问题：

1. **零配置可用**：不写一行配置也能启动一个 httpbin 风格的回显服务，便于排查请求是否打到了 server、各字段实际值如何。
2. **声明式 mock**：通过 JSON5 配置文件描述路由 → 响应的映射，支持延迟、状态码、响应头、响应体、响应体读自文件等常见维度。
3. **动态响应**：通过 Go 模板语法把请求上下文、随机值、时间戳、伪造数据等注入响应体，模拟更贴近真实业务的数据形态；支持同路径多响应（随机/轮询/权重）与条件分支（按请求字段走不同分支）模拟边界场景。
4. **渐进式真实化**：单条路由可声明为 proxy 类型，把请求转发到真实后端；联调期间"部分接口已就绪"的混合场景天然支持。

### 1.2 需求来源

- **PRD**：见同目录 `../prd.md`
- **设计讨论纪要**：本文档 § 2–§ 12 即为讨论结论的固化
- **关键决策**：
  - Web 框架使用 `github.com/gookit/rux/v2`（v2.0.0；module path 需显式带 `/v2` 后缀，否则会拉到 v1 分支）
  - 模板引擎使用 `github.com/gookit/easytpl`（讨论决定，复用其 `tplfunc.StdFuncMap` 基础函数）
  - 表达式求值使用 `github.com/expr-lang/expr`（轻量、活跃维护、替代已停维护的 govaluate）
  - JSON5 解析使用 `github.com/titanous/json5`
  - 数据伪造使用 `github.com/brianvoe/gofakeit/v7`

### 1.3 范围边界（分期）

**v0.1（MVP，本设计主链路）**：

- 单 HTTP server（rux）+ 默认 httpbin 风格 echo
- JSON5 配置加载、多文件合并、`@include` 子文件引入（含 glob、循环检测）
- 路由声明：方法/路径/状态码/响应头/响应体/`bodyFile`/延迟
- 同路径多响应：`random` / `round-robin` / `weighted` / `first-match` 四种 strategy
- 条件分支 `cases[].when`：基于请求字段的表达式匹配
- Go 模板渲染：请求上下文 `.request`、全局 `.now/.config`、内置函数集
- **Faker 数据生成**（双入口：常用函数 + `fake "<field>"` 通用）
- **Proxy 路由**（出现 `proxy` 字段即视为 proxy 路由）
- 运行时能力：热加载（fsnotify + 防抖 + 原子切换）、请求日志、CORS 默认开启
- CLI：子命令形态，`fakeserver serve -c <paths>` 为主入口；前台运行，Ctrl+C 退出
- 内置 admin 端点 `/__fakeserver/routes`、`/__fakeserver/healthz`

**v0.2**：

- **环境配置文件** `fakeserver.env.json5`（`$default` + 多 env 段）
- CLI `--env <name>` 与 `--var key=val` 单点覆盖
- 模板命名空间调整：`.env`（文件 env）、`.osenv`（OS env）

**v0.3**：

- **全局项目注册** `~/.config/fakeserver/projects.json`
- CLI 子命令 `fakeserver list` / `fakeserver use <id>`
- PID 文件 + 探活

**v0.4**：

- **轻量 Web UI**：项目列表、配置只读视图、路由表（含 mock/proxy 标识）、请求历史
- 内置 SSE `/__fakeserver/events` 实时请求流
- 请求历史环形缓冲（默认 200 条，可配）

**v1.0+（不在本文档范围）**：

- WebSocket / Server-Sent Events 协议 mock
- 录制-回放（capture & replay 真实流量）
- json-server 风格的自动 RESTful CRUD
- 静态目录服务（`static: "./public"`）
- 鉴权 / TLS / 多 server 实例

### 1.4 同类工具对比

| 工具 | 语言 | 配置形态 | 模板能力 | proxy | env 切换 | fakeserver 取舍 |
|---|---|---|---|---|---|---|
| json-server | Node | 资源 JSON | 无 | 部分 | 无 | 我们走声明式 mock，不做 CRUD（留 v1.x） |
| WireMock | Java | JSON/DSL | 有 | 有 | 有 | 体积重；我们更轻 |
| MockServer | Java | JSON/DSL | 部分 | 有 | 有 | 同上 |
| httpbin | Python | 内置端点 | 无 | 无 | 无 | 我们以其默认 echo 风格为零配置起点 |
| Mockoon | 桌面应用 | GUI JSON | 有 | 部分 | 有 | 我们是 CLI 工具，便于 CI/容器化；附带轻量 web UI |
| JetBrains HTTP Client | IDE 插件 | http 文件 | 有 | 文件 env | — | env 文件格式向其看齐 |

fakeserver 的差异化定位：**Go 单二进制 · 轻配置 · 模板+Faker 友好 · IDE 风格 env 切换 · 全局项目记录 · 自带轻量 web 面板**。

---

## 2. 架构总览与模块边界

### 2.1 一句话定位

fakeserver 把 JSON5 路由声明转成 rux 路由；mock / proxy 路由共存；无配置时直接启用 rux 内置 httpbin 风格 echo。

### 2.2 模块切分（按分期标注）

```
fakeserver/
├── cmd/fakeserver/main.go            # [v0.1] CLI 入口（gookit/gcli），仅做参数解析与子命令分发
├── internal/
│   ├── config/                       # [v0.1] 配置加载、JSON5 解析、@include、合并、热加载
│   │   ├── loader.go                 #         路径解析、include 展开（带循环检测）、默认查找路径
│   │   ├── schema.go                 #         配置结构体定义 + 默认值
│   │   ├── envfile.go                # [v0.2]  fakeserver.env.json5 加载与 env 选择
│   │   └── watcher.go                # [v0.1]  fsnotify 监听 + 防抖
│   ├── mock/                         # [v0.1] mock 路由 → rux handler 转换
│   │   ├── router.go                 #         注册到 rux（语义优先级由 radix tree 保证）
│   │   ├── responder.go              #         单次响应处理（状态码/头/延迟/body/bodyFile）
│   │   ├── selector.go               #         多响应选择策略（随机/轮询/权重/首匹配）
│   │   └── matcher.go                #         条件分支匹配（expr-lang/expr）
│   ├── proxy/                        # [v0.1] proxy 路由
│   │   └── proxy.go                  #         httputil.ReverseProxy 封装 + path rewrite
│   ├── tpl/                          # [v0.1] 模板渲染层
│   │   ├── render.go                 #         easytpl 包装；text/html 双渲染器，FuncMap 共享
│   │   ├── context.go                #         构建 .request / .now / .env / .osenv / .config
│   │   ├── funcs.go                  #         内置函数集成（tplfunc.StdFuncMap + fakeserver 自有）
│   │   └── faker.go                  # [v0.1]  gofakeit 桥接：常用函数 + 通用 fake "<field>"
│   ├── echo/                         # [v0.1] 默认 echo 模式
│   │   └── mount.go                  #         桥接 rux 自带 server.EchoHandlers
│   ├── middleware/                   # [v0.1] 横切关注点
│   │   ├── cors.go                   #         默认开启，可关；admin 路径前缀豁免
│   │   ├── logger.go                 #         请求日志
│   │   ├── recoverer.go              #         panic 捕获 → 500 JSON
│   │   └── bodylimit.go              #         body 体积上限
│   ├── registry/                     # [v0.3] 全局项目记录
│   │   ├── store.go                  #         读写 ~/.config/fakeserver/projects.json
│   │   ├── lock.go                   #         文件锁 + 原子写
│   │   └── pid.go                    #         PID 文件 + 探活
│   ├── recorder/                     # [v0.4] 请求历史环形缓冲
│   │   └── recorder.go               #         内存 ring buffer + SSE 订阅者管理
│   └── webui/                        # [v0.4] 轻量 web 界面
│       ├── server.go                 #         embed 静态资源；挂载到 /__fakeserver/ui/
│       ├── api.go                    #         /__fakeserver/api/* JSON API
│       └── assets/                   #         html/css/js（embed.FS）
└── docs/
    ├── fakeserver-design.md          # 本文档
    └── examples/                     # 示例配置（含 include 拆分、env 文件、proxy 示范）
```

### 2.3 模块依赖方向（单向、无环）

```
main      → config + mock + proxy + echo + middleware + registry + webui
config    → (envfile 子模块)
mock      → tpl + config(schema)
proxy     → config(schema)
tpl       → (无内部依赖；外部 easytpl + gofakeit + expr 标准库)
echo      → (rux/server)
registry  → (标准库 only：os / encoding/json / sync)
recorder  → (标准库 only)
webui     → config + mock + recorder + registry
middleware→ recorder（v0.4 起，logger 同时写一份到 recorder）
```

`webui` 只读，不反向修改其他模块状态。每个模块对外暴露最小接口：

```go
package config
func Load(paths []string, envName string, overrides map[string]string) (*Config, error)
func Validate(cfg *Config) []error
func DefaultPaths() []string   // CWD 默认查找顺序

package mock
func Mount(r *rux.Router, cfg *Config, renderer tpl.Renderer) error

package proxy
func Mount(r *rux.Router, route *config.Route) error

package tpl
func NewRenderer(globals map[string]any, osenvWhitelist []string, fakerSeed int64) Renderer

package registry
func Upsert(p Project) error
func List() ([]Project, error)
func MarkActive(id string) error

package recorder
func New(size int) *Ring
func (r *Ring) Append(e Entry)
func (r *Ring) Snapshot() []Entry
func (r *Ring) Subscribe() (<-chan Entry, func())
```

### 2.4 第三方依赖清单

| 用途 | 库 | 引入期 | 理由 |
|---|---|---|---|
| Web 框架 | `github.com/gookit/rux/v2` | v0.1 | PRD 指定，内置 httpbin echo；module path 显式带 `/v2`，避免拉到 v1 分支 |
| 模板引擎 | `github.com/gookit/easytpl` | v0.1 | 用户指定；`tplfunc.StdFuncMap` 提供基础函数 |
| CLI | `github.com/gookit/gcli/v3` | v0.1 | 与 lite-tools 同生态 |
| JSON5 | `github.com/titanous/json5` | v0.1 | 用户指定 |
| 文件监听 | `github.com/fsnotify/fsnotify` | v0.1 | 事实标准 |
| 表达式 | `github.com/expr-lang/expr` | v0.1 | 轻量、活跃、语法直观 |
| Faker | `github.com/brianvoe/gofakeit/v7` | v0.1 | 函数最丰富、活跃维护 |
| 工具集 | `github.com/gookit/goutil` | v0.1 | strutil/fsutil/maputil |

无新增第三方依赖跨期引入；v0.2–v0.4 全部用标准库实现（embed/httputil/encoding/json 等）。

---

## 3. 配置文件 Schema、include 与多响应

### 3.1 顶层结构

```json5
{
  // ── 服务器选项 ──
  server: {
    host: "0.0.0.0",        // 默认 0.0.0.0
    port: 5090,             // 默认 5090
    cors: true,             // true | false | 对象（见 § 5.4）
    log: true,              // 默认 true，--quiet 等价 false
    maxBodySize: "1MiB",    // 请求体上限，超限 413
    adminEnabled: true,     // 是否启用 /__fakeserver/* 端点
    osenvWhitelist: [],     // 模板 osenv 函数白名单；空表示放行所有
    fakerSeed: 0,           // 0 → 随机；非 0 → 固定 seed，结果可复现（便于测试）
    historySize: 200,       // [v0.4] 请求历史环形缓冲长度
    projectName: "",        // 显式覆盖 registry 中显示的项目名；空则取目录名
  },

  // ── 模板全局变量 ──（所有 route 模板里通过 .config 访问）
  globals: {
    apiVersion: "v1",
    userPool: ["alice", "bob", "carol"],
  },

  // ── 默认行为 ──
  fallback: "echo",         // "echo" | "404"  无路由匹配时的兜底，默认 echo

  // ── 路由列表 ──
  routes: [
    { method: "GET", path: "/ping", body: "pong" },
    "@routes/users.json5",
    "@routes/admin/*.json5",          // 支持 glob
  ],
}
```

### 3.2 单个 Route 的完整字段

```json5
{
  // 必填
  method: "GET",              // 或 ["GET","HEAD"]，* 表示任意
  path: "/users/{id}",        // rux 语法：{name} / *rest

  // ── 单一响应模式 ──
  status: 200,                // 默认 200
  delay: "120ms",             // duration；可固定或区间 "100ms~500ms"（均匀分布）
  headers: {
    "Content-Type": "application/json",
    "X-Trace-Id": "{{ uuid }}",
  },
  body: {                     // 对象 → 自动 JSON 序列化；字符串 → 原样输出
    id: "{{ .request.params.id }}",
    name: "{{ fakeName }}",
    fetchedAt: "{{ now \"2006-01-02T15:04:05Z\" }}",
  },
  // 或 —— 与 body 互斥
  bodyFile: "fixtures/avatar.png",   // 读文件字节作响应；Content-Type 按扩展名推断

  // ── 多响应/条件分支模式（与上方单一响应字段互斥） ──
  strategy: "random",         // "random"(默认) | "round-robin" | "weighted" | "first-match"
  cases: [
    {
      when: "request.query.fail == \"1\"",   // expr 表达式；缺省即总匹配
      weight: 1,                              // 仅 strategy=weighted 使用
      status: 500,
      body: { error: "simulated failure" },
    },
    {
      status: 200,
      body: { ok: true, id: "{{ .request.params.id }}" },
    },
  ],

  // ── Proxy 模式（与 mock 字段全部互斥）── 详见 § 9
  proxy: {
    target: "http://real-backend.local:8080",
    // ... 详见 §9.2
  },
}
```

**互斥规则（启动期校验）**：

- 同时出现 `body` 和 `bodyFile` → 报错
- 同时出现 `cases` 与顶层 `body/bodyFile` → 报错
- 出现 `proxy` 字段时，**`body`/`bodyFile`/`cases`/`status`/`headers`/`delay` 中任一存在都报错**（proxy 与 mock 不混用）
- `cases[i]` 可独立设 `status/headers/delay/body/bodyFile`，未设则继承外层默认

**路由优先级**：精确路径 > 命名参数路径 > 通配路径（由 rux radix tree 保证；配置文件出现顺序不影响优先级）。同一条 route 内的多个 case 按 `strategy` 选择。**跨条目重复的 `method+path` 在启动期报错**（见 §3.4）。mock 路由与 proxy 路由共用同一 router，可在精确 mock 路径上"截胡"被 proxy 通配的范围。

### 3.3 `@include` 机制

**触发条件**：值是字符串且以 `@` 开头。**仅在以下位置生效**：

| 位置 | 引入后处理方式 |
|---|---|
| `routes` 数组元素 | 文件内容须为 route 对象或 route 数组；数组会拍平进父数组 |
| 任意嵌套对象/数组里的字符串值 | 当作 JSON5 解析并整段替换（**不合并**，避免歧义） |

**路径规则**：

- 相对路径 → **相对当前配置文件目录**（不是 CWD）
- 支持 glob：`@routes/*.json5`、`@routes/**/*.json5`
- **嵌套引入**允许；**循环引入** loader 主动检测并报错（列出引用链）
- 文件后缀必须是 `.json` 或 `.json5`，其它后缀报错（避免与 `bodyFile` 语义混淆）
- **仅本地路径**，不支持 `http://` / `https://` 等远程 URL（防止启动期网络依赖）

**字面量 `@` 转义**：极少见，但若业务确需返回字面量 `"@xxx"`，写成 `"\@xxx"`，loader 见到 `\@` 前缀剥掉反斜杠当字面量。

### 3.4 多文件合并语义与去重 *(v0.3 修订)*

CLI 形式：`fakeserver serve -c base.json5,routes.json5,overrides.json5`

- **server / globals / fallback**：后者覆盖前者（map 深合并）
- **routes**：按顺序拼接成一个大数组（保留所有条目）
- 任意文件的 `routes` 元素也可以是 `@include`，递归展开

**跨条目去重**：合并后扫描所有 route，**若两条 route 的 `method+path` 完全相同（同 method 集合、同 path 模板），启动期报错**。要求用户合并到单条 route 的 `cases` 数组中显式声明意图，避免"我为什么命中了另一个文件里的同名路由"这类隐式陷阱。

报错样例：
```
route conflict: GET /users
  - defined at routes/users.json5:3
  - defined at overrides/users.json5:7
hint: merge into a single route with `cases:` if multiple responses are intended.
```

### 3.5 默认配置查找路径 *(v0.3 新增)*

CLI 不带 `-c` 时，按下列顺序在 CWD 查找：

```
1. ./fakeserver.json5
2. ./fakeserver.json
3. ./.fakeserver/config.json5
```

均不存在 → 启动**纯 echo 模式**（不报错；符合 §1.1 零配置可用）。`-c` 显式指定后不再做查找。

env 文件（v0.2+）的默认查找：与命中的主配置文件**同目录**下的 `fakeserver.env.json5`；找不到 → `.env` 为空 map，不报错。

### 3.6 完整示例（目录拆分）

```
example/
├── fakeserver.json5
├── fakeserver.env.json5
└── routes/
    ├── users.json5
    ├── auth.json5
    └── admin/
        ├── orders.json5
        └── reports.json5
```

`fakeserver.json5`：

```json5
{
  server: { port: 5090 },
  globals: { apiVersion: "v1" },
  routes: [
    "@routes/users.json5",
    "@routes/auth.json5",
    "@routes/admin/*.json5",
  ],
}
```

`routes/users.json5`（导出 route 数组）：

```json5
[
  { method: "GET", path: "/users",      body: { items: [], total: 0 } },
  { method: "GET", path: "/users/{id}", body: {
      id:    "{{ .request.params.id }}",
      name:  "{{ fakeName }}",
      email: "{{ fakeEmail }}",
  }},
  {
    method: "POST", path: "/users",
    strategy: "weighted",
    cases: [
      { weight: 1, when: "len(request.body.name) == 0", status: 400, body: { error: "name required" } },
      { weight: 9, status: 201, headers: { "Location": "/users/{{ uuid }}" }, body: { id: "{{ uuid }}", name: "{{ .request.body.name }}" } },
    ],
  },
]
```

`routes/auth.json5`（演示 bodyFile）：

```json5
{
  method: "GET",
  path: "/auth/avatar/{uid}",
  headers: { "Cache-Control": "no-store" },
  bodyFile: "../fixtures/default-avatar.png",   // 相对当前文件目录
}
```

### 3.7 启动期校验（fail-fast，集中报错）

配置加载完成后、注册 rux 路由前一次性校验，错误集中报出：

- 必填字段缺失
- `body` / `bodyFile` / `cases` / `proxy` 互斥冲突
- `bodyFile` 指向不存在的文件
- include 路径 / glob 无匹配
- 循环 include
- **跨条目 `method+path` 重复**（详见 §3.4）
- 表达式 `when` 语法错误（expr 编译期检查）
- 模板语法错误（预编译失败）
- 用户路由覆盖保留前缀 `/__fakeserver/*` → 拒绝并报错
- proxy.target 解析失败（非 http/https scheme）→ 报错

**警告（不阻止启动）**：

- 同 `method+path` 在 `strategy=first-match` 下 **`cases` 数组所有元素都写了 `when`**（没有兜底 case） → 警告，运行期所有 case 都不匹配会回 500
- `proxy.target` Host 解析为 localhost / 私有网段 → info 提示

校验通过后，所有模板、表达式**预编译并缓存**，请求路径不再做编译开销。

---

## 4. 模板上下文、内置函数与渲染策略

### 4.1 模板上下文完整结构

```go
type RenderCtx struct {
    Request RequestCtx          // .request
    Now     time.Time           // .now（每次请求重新取值）
    Env     map[string]any      // .env    [v0.2] 文件 env 当前段（含 $default 合并后值）
    OSEnv   map[string]string   // .osenv  进程环境变量快照（受白名单影响）
    Config  map[string]any      // .config（即 globals 段）
}

type RequestCtx struct {
    Method  string                 // .request.method        e.g. "POST"
    Path    string                 // .request.path          e.g. "/users/42"
    Proto   string                 // .request.proto         e.g. "HTTP/1.1"
    Host    string                 // .request.host          e.g. "127.0.0.1:5090"
    IP      string                 // .request.ip            真实 IP（X-Forwarded-For 优先）
    Params  map[string]string      // .request.params.id     路径参数（rux 提供）
    Query   map[string]any         // .request.query.page    单值→string；多值→[]string
    Headers map[string]string      // .request.headers["X-..."]  键大小写不敏感，规范化为 Canonical
    Body    any                    // .request.body          见下表
    BodyRaw string                 // .request.bodyRaw       原始字节字符串（便于回显）
}
```

> **命名空间调整 (v0.2)**：`.env` 现在指**文件 env**（来自 `fakeserver.env.json5`），原本表示 OS 环境变量的位置改名 `.osenv`。在 v0.1 阶段 `.env` 为 nil（文件未加载），模板若使用会被视为零值（空 map），不报错。

**`.request.body` 解析规则**（按 `Content-Type` 自动）：

| Content-Type | `.request.body` 类型 | 说明 |
|---|---|---|
| `application/json`、`*+json` | `map[string]any` 或 `[]any` | 解析失败则降级为字符串 + warn 日志 |
| `application/x-www-form-urlencoded` | `map[string]any` | 多值字段为 `[]string` |
| `multipart/form-data` | `map[string]any` | 文件字段为 `{filename, size, contentType}`，不读字节进内存 |
| `text/*` | `string` | 原文 |
| 其他 / 空 | `string`（原始字节的 UTF-8 视图） | 与 `.request.bodyRaw` 相同 |

**Body 体积上限**：`server.maxBodySize`（默认 1MiB），超限请求由中间件返回 413，不进入模板。

### 4.2 内置函数集成策略

`easytpl/tplfunc` 包文档列出了一长串函数，但其源码 `stdFuncMap` 实际仅实现了少量基础函数（join/trim/upper/lower/ucFirst/loFirst/env/expandenv + path 类），其余仍是 TODO。fakeserver 的策略：

```go
// 启动期一次性合并，单一注册点
funcs := template.FuncMap{}
maps.Copy(funcs, tplfunc.StdFuncMap())   // 复用已实现的基础函数
delete(funcs, "env")                      // tplfunc 的 env=os.Getenv 与我们的命名空间冲突；剔除
delete(funcs, "expandenv")                // 同上理由
maps.Copy(funcs, fakeserverFuncs)        // 补齐 + mock 专用（同名时覆盖）
maps.Copy(funcs, fakerFuncs)             // 注入 gofakeit 桥接函数（详见 §12）
```

### 4.3 内置函数清单（fakeserver 自有）

命名对齐 tplfunc 的路线图，便于未来 upstream 收编后平滑移除。

```
通用：
  uuid                            v4 UUID 字符串
  shortid()                       8 位 base62
  incr "counter-name"             进程内自增计数器（按名独立）
  default v fallback              {{ .request.query.page | default 1 }}
  coalesce v...                   返回首个非空值

时间：
  now [layout]                    不传 → 当前 time.Time；传 layout → 格式化字符串
  timestamp [unit]                unit ∈ "s"|"ms"|"us"|"ns"，默认 "s"
  addDate years months days       基于 .now 加减

环境：
  env "KEY" ["default"]           [v0.2] 取 .env 当前段的值；v0.1 阶段总返回 default
  osenv "KEY" ["default"]         取进程环境变量；受 server.osenvWhitelist 限制
  expandEnv "..."                 字符串里所有 $VAR / ${VAR} 用 osenv 替换

随机：
  randInt min max                 [min,max] 闭区间
  randFloat min max
  randString n [charset]          charset ∈ "alpha"|"alnum"|"hex"|"base62"，默认 "alnum"
  randChoice list...              从可变参数或切片中随机选一项
  shuffle list                    返回打乱后的新切片
  weighted (list (weight,value))  加权抽样

编码：
  b64enc / b64dec                 标准 base64
  urlenc / urldec
  jsonEscape v                    按 JSON 字符串规则转义

JSON：
  toJson v                        序列化为紧凑 JSON
  fromJson s                      反序列化为对象
  jsonPath obj "a.b[0]"           快速取嵌套字段，缺失返回空

字符串补充：
  title                           tplfunc 未提供
  split sep s                     tplfunc 未提供

控制 / 调试：
  fail msg                        立即令本次模板渲染失败，回落到错误响应
  print v                         stderr 打印（调试用，不影响输出）
```

> Faker 函数集另见 **§ 12**（按字段名常用函数 + 通用入口 `fake`）。

### 4.4 渲染器：text vs html 双模式

easytpl 默认基于 `html/template`，会把 `"`、`<`、`&` 等做 HTML 实体转义——对返回 JSON 是灾难性的。处理方案：

- **两套渲染器并存**，按响应 Content-Type 自动选：
  - `RendererText`（**默认**）：内部用 `text/template`，零转义。**所有 mock 路径默认走这套。**
  - `RendererHTML`：用 easytpl 原生 `html/template`。**仅当 `headers["Content-Type"]` 显式以 `text/html` 开头**时使用。

- 两套渲染器共享同一份合并后的 FuncMap，函数行为完全一致。

- 启动时把每条 route 的 `headers/body` 模板源**预编译**到对应渲染器的命名缓存里，key 为 `<routeIdx>:<field>`；请求路径只做 `Execute`，无解析开销。

### 4.5 渲染顺序、Content-Type 推断与错误语义 *(v0.3 修订)*

单次响应的渲染顺序（后字段可引用前字段）：

```
1. cases 选择（如有，先求 when 表达式；首匹配/无匹配处理见下）
2. 渲染 headers（每个 value 独立渲染）
3. 渲染 status（如配置为模板字符串，少见但允许）
4. 渲染 body：
   - 对象 → 递归遍历，对每个字符串叶子节点做模板渲染（结果保持为字符串）
   - 字符串 → 整体渲染一次
5. 处理 bodyFile（不渲染，直接拷贝；Content-Type 缺省按扩展名推断）
6. Content-Type 推断（headers 未显式指定时）
7. 应用 delay（在所有渲染完成后 sleep）
```

**Content-Type 推断规则**（仅当 `headers["Content-Type"]` 未显式设置时生效）：

| body 形态 | 推断结果 |
|---|---|
| 对象 / 数组（map / slice） | `application/json; charset=utf-8` |
| 字符串 | `text/plain; charset=utf-8` |
| `bodyFile` | `mime.TypeByExtension(ext(file))`；未知扩展名 → `application/octet-stream` |
| 无 body 也无 bodyFile（仅 status/headers） | 不主动设置 |

**cases 选择的错误处理**：

| 场景 | 行为 |
|---|---|
| `strategy=first-match`：遍历 cases，首个 `when` 求值为 true（或无 `when`）的 case 命中 | 命中那个 case |
| `first-match` 下所有 case 都没命中 | 500 + `{ "error": "no case matched", "route": "..." }`，warn 日志 |
| `random/round-robin/weighted`：先**过滤** `when` 求值为 true 的 case，再按策略从过滤后集合中选 | 与首匹配错误处理相同 |
| `when` 表达式求值出错（类型错/字段缺失） | 视为不匹配，继续判断下一 case；warn 日志 |

**其他错误**：

| 错误位置 | 行为 |
|---|---|
| 模板执行 err / `fail` / 超时 | 500 + `{ "error": "template error", "detail": "...", "route": "..." }`，stderr warn；**不杀进程** |
| 启动期预编译失败 | 启动报错并退出（与 § 3.7 集中报错合流） |
| bodyFile 读失败 | 500 + 路径写日志 |
| recoverer 捕获到 panic | 500 + 栈打到 stderr |

### 4.6 安全护栏

- **osenv 白名单**：`server.osenvWhitelist` 配置后，`osenv` 函数仅放行清单内 key，命中外返回空串 + warn。避免误把宿主敏感变量泄露。空清单等价于放行所有（开发友好）。**env 文件中的模板字符串调用 `osenv` 同样受此白名单约束**（env 文件不是逃逸通道）。
- **模板执行超时**：每个请求级 2s，防止无限循环模板拖死 server。超时按模板错误处理。
- **保留路径**：`/__fakeserver/*` 仅 fakeserver 内部端点使用，用户路由命中该前缀启动报错。

---

## 5. 运行机制

### 5.1 启动流程 *(v0.3 修订)*

```
1. 解析 CLI：fakeserver serve [-c PATHS] [-p|--port PORT] [-e|--env ENV] [--var k=v]...
                                [--quiet] [--no-cors] [--no-watch]
2. 配置路径解析：
   - -c 指定 → 用指定路径（多文件以逗号分隔）
   - 未指定 → 在 CWD 按 §3.5 顺序查找；都没有 → 启动纯 echo 模式
3. [v0.2] 加载 env 文件：config.envfile.Load(<configDir>/fakeserver.env.json5, envName)
4. 加载主配置：config.Load(paths) → 多文件合并 + @include 递归展开 + 循环检测
5. 应用 CLI 覆盖：--var key=val 写入 .env 段（运行时只读快照）
6. 集中校验：config.Validate() → 收集所有错误后一次性报出；任一致命错即退出
7. 预编译：模板源 + 表达式源全部 compile 成对象，缓存到内存索引
8. 构建 rux 路由：
   a. 注册全局中间件：requestId → recoverer → logger（→ recorder, v0.4）→ cors → bodyLimit
      （cors 中间件对 /__fakeserver/* 路径前缀豁免；OPTIONS 短路在路由匹配之后执行）
   b. 遍历 cfg.routes → mock.Mount() 或 proxy.Mount()（按 route 字段判断）
   c. 注册 admin 端点 /__fakeserver/routes、/healthz（若 adminEnabled）
   d. [v0.4] 注册 /__fakeserver/ui/、/__fakeserver/api/*、/__fakeserver/events
   e. 若 fallback == "echo" → 把 rux/server 的 EchoHandlers 挂到 NotFound
   f. 否则注册一个 404 JSON handler
9. [v0.3] 写 PID 文件 + 调用 registry.Upsert() 注册当前项目
10. 启动文件监听：watcher.Start()（--no-watch 跳过）
11. 启动 HTTP server（前台运行，Ctrl+C 退出），打印 banner 和路由摘要
```

**启动 banner + 路由摘要样例**：

```
fakeserver v0.1.0 on :5090  ·  5 routes (4 mock, 1 proxy)  ·  env=dev  ·  echo fallback

  GET    /ping                            → mock
  GET    /users                           → mock
  GET    /users/{id}                      → mock (faker)
  POST   /users                           → mock (2 cases, weighted)
  *      /api/users/*rest                 → proxy http://real-backend.local:8080
```

### 5.2 热加载（fsnotify + 防抖 + 原子切换）

```
监听流程：
- 监听所有已加载文件的所属目录（不监听单文件 inode；保存常用临时文件+rename）
- 收到事件 → 入 channel
- 单一 worker：drain channel，固定 300ms 防抖窗口
- 窗口结束后重新走 Load + Validate + 预编译
- 校验失败：保留旧路由表，stderr 红字打印错误，不切换（服务不中断）
- 校验通过：原子替换 router 指针（atomic.Pointer[Router]）
- 打印 diff 摘要：+ POST /users  - DELETE /old  ~ GET /me (cases 1→3)
```

请求路径始终经过间接层 `r := currentRouter.Load(); r.ServeHTTP(w, req)`，所以替换不影响在途请求，新请求看到新表。

**监听范围**：所有 include 链上展开过的文件 + 目录（glob 模式监听目录），**包括 env 文件**。env 文件变更同样触发热加载。

### 5.3 请求日志

```
中间件位置：global，紧跟 recoverer 之后
默认开启，--quiet 或 server.log=false 关闭

格式（彩色 by gookit/color，TTY 自动检测）：
  15:42:08.123  POST  /users          201  17.2ms   case=#1/3 (weighted)
  15:42:09.011  GET   /users/me       200   0.4ms
  15:42:09.508  GET   /users/{id}     500   2ms     ✗ template error: env "X" not allowed
  15:42:10.118  GET   /api/orders/42  200  120ms    → proxy http://real:8080
```

字段：时间 / 方法 / 路径**模板**（不展开参数，便于聚合）/ 状态 / 耗时 / 备注（命中的 case、proxy 标记、错误等）。

> [v0.4] 同一份记录同时写入 `recorder.Ring`，供 web UI 查看与 SSE 推送。

### 5.4 CORS *(v0.3 修订)*

默认开启，等价于：

```
Access-Control-Allow-Origin: <reflect req.Origin or *>
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS
Access-Control-Allow-Headers: <reflect req.Access-Control-Request-Headers or *>
Access-Control-Allow-Credentials: true   （仅当 Origin 被反射时）
Access-Control-Max-Age: 600
```

**OPTIONS 处理顺序**（修正先前的"直接短路"描述）：

```
1. 先尝试路由匹配（用户可能显式 mock 了一个 OPTIONS 端点）
2. 命中用户路由 → 走正常 mock/proxy 流程
3. 未命中 → CORS 中间件返回 204（preflight 兜底）
```

**Admin 路径豁免**：`/__fakeserver/*` 前缀的所有路由**不挂载** CORS 中间件，确保 admin UI 与 API 始终仅同源可达，即使用户把全局 cors 配成 `origins: ["*"]` 也不会泄露 admin 面板到第三方源。

可配置细化：

```json5
server: {
  cors: {
    origins: ["http://localhost:5173"],   // 字符串 / 数组 / "*"；默认反射 Origin
    methods: [...], headers: [...],
    credentials: true,
    maxAge: "10m",
  }
  // 或简写：cors: true | false
}
```

### 5.5 默认 Echo（rux/server 接入）

零配置启动时 fallback 走 echo。直接把 rux 内置 echo handler 装到 NotFound 与若干固定端点：

```go
// internal/echo/mount.go（伪代码）
func Mount(r *rux.Router) {
    r.NotFound(server.EchoNotFoundHandler())   // /anything 类回显
    server.RegisterEchoEndpoints(r)            // /status/{code}, /delay, /headers, /ip 等
}
```

**有路由配置时**：fallback 默认仍是 echo，便于"配置了 /users 但顺便 curl /anything 看请求被收到啥样"这类调试。显式 `fallback: "404"` 关闭。

> 启动时探测 rux/server 子包实际导出符号；若 API 名不一致，做薄适配层，不复制实现。

### 5.6 可观测性与运维

| 端点 | 期号 | 说明 |
|---|---|---|
| `GET /__fakeserver/routes` | v0.1 | 路由列表 JSON（mock + proxy） |
| `GET /__fakeserver/healthz` | v0.1 | 永远 200 |
| `GET /__fakeserver/ui/*` | v0.4 | Web UI 静态资源 |
| `GET /__fakeserver/api/projects` | v0.4 | 项目列表（来自 registry） |
| `GET /__fakeserver/api/config` | v0.4 | 当前项目配置（脱敏后） |
| `GET /__fakeserver/api/history` | v0.4 | 最近 N 条请求 |
| `GET /__fakeserver/events` | v0.4 | SSE 实时请求流 |

`server.adminEnabled: false` 关闭全部 `/__fakeserver/*` 端点（含 ui）。

退出信号（SIGINT/SIGTERM）：停止接受新连接，等待在途请求最多 5s 再退出；删除 PID 文件；更新 registry 的 `lastRunAt`。

### 5.7 CLI 子命令清单 *(v0.3 修订)*

| 子命令 | 期号 | 用法 / 行为 |
|---|---|---|
| `serve` | v0.1 | 启动 server（主命令），前台运行，Ctrl+C 退出。支持 `-c/--config`、`-p/--port`、`-e/--env`、`--var`、`--quiet`、`--no-cors`、`--no-watch` |
| `init` | v0.1 | 在 CWD 生成 `./fakeserver.json5` 模板（含 2–3 个示例 route 与注释）。**目标文件已存在则报错退出，不覆盖**。可选 `--with-env` 同时生成 `fakeserver.env.json5`（含 `$default` + `dev` 段） |
| `check` | v0.1 | 仅做配置校验，不启动 server（CI 友好）；与 `serve` 同样支持 `-c`、`-e` |
| `routes` | v0.1 | 离线打印路由摘要（不启动 server） |
| `list` | v0.3 | 列出 `~/.config/fakeserver/projects.json` 中所有项目 + running 状态 |
| `use <id>` | v0.3 | 切换 lastActiveId（不启动；与 `list` 配合，决定 web UI 默认聚焦） |

---

## 6. 错误处理总览 *(v0.3 修订)*

| 阶段 | 错误类型 | 处理 |
|---|---|---|
| 配置加载 | JSON5 语法 / include 缺失 / 循环 | 启动报错并退出，列出所有问题 |
| 配置校验 | 字段互斥 / bodyFile 不存在 / 表达式语法 / 模板语法 / proxy.target 无效 / 跨条目 method+path 重复 | 同上，集中报 |
| 热加载（运行时） | 任何上面错误 | 保留旧表 + stderr 警告，**不退出** |
| 请求处理 panic | 任意 | recoverer 中间件捕获 → 500 JSON + 栈打到 stderr |
| 模板执行 err | 函数返回 err / fail() / 超时 | 500 + `{error, detail, route}` |
| 表达式 when 求值 err | 类型错 / 字段缺失 | 视为不匹配，继续下个 case；warn 日志 |
| cases 全部不匹配 | first-match 无兜底 / random 等过滤后空集 | 500 + `{error: "no case matched", route}` |
| bodyFile 读失败 | 文件被删 / 权限 | 500 + 路径写日志 |
| 请求超限 | body 超 maxBodySize | 413 在中间件层返回，不进 handler |
| proxy 上游错误 | 拨号失败 / 超时 / 5xx | 默认透传上游响应；拨号失败返回 502 + `{error, route, target}` |
| registry 写入失败 | 文件权限 / 锁竞争 | warn 日志，不阻塞启动 |

**统一错误响应体**（仅当 Content-Type 缺省或为 application/json）：

```json
{ "error": "<short>", "detail": "<longer>", "route": "GET /users/{id}", "target": "<proxy target if any>" }
```

`target` 字段仅在 proxy 路由相关错误时出现（mock 错误不带）。若 route 上显式配了 `headers["Content-Type"]: text/plain`，错误用 text 形式回（仅含 `error` 简短信息）。

---

## 7. 测试策略

**分层一致原则**：能在哪一层测就在哪一层测，不滥用集成测试。

```
internal/config/*_test.go        # 纯函数，表驱动
  - JSON5 解析
  - @include 展开（含 glob、嵌套、循环、相对路径）
  - 多文件合并语义
  - 跨条目 method+path 重复检测
  - 默认配置查找（CWD 三档候选）
  - Validate 错误集中收集
  - [v0.2] env 文件 + $default 合并 + osenv 白名单

internal/tpl/*_test.go           # 纯函数
  - 所有自实现函数（uuid 看长度/charset、now/timestamp 看格式、rand* 看分布范围…）
  - 上下文构建（body 按 content-type 解析）
  - text vs html 渲染器的转义差异
  - Content-Type 推断（对象→json、字符串→text/plain、bodyFile→扩展名）
  - osenv 白名单
  - faker 桥接（含 seed 复现性）

internal/mock/*_test.go          # 单元 + httptest
  - selector 四种 strategy 概率/顺序
  - matcher 表达式求值（含错误降级）
  - cases 全部不匹配的 500 路径
  - responder：用 httptest.NewRecorder 验证 status/headers/body/delay

internal/proxy/*_test.go         # 单元 + httptest
  - 用 httptest.NewServer 起一个假上游，验证 path rewrite / header 注入 / 超时
  - 上游 502 / 拨号失败的错误分支
  - bodyLimit 拒绝过大上行 body
  - rewrite 中 $1 捕获组（不走模板）

internal/registry/*_test.go      # [v0.3] 单元 + 临时目录
  - 文件锁并发写入正确性
  - 损坏文件的恢复策略（rename 备份 + 重建）
  - 跨平台路径（用 t.Setenv HOME 在临时目录下验证）

internal/recorder/*_test.go      # [v0.4] 单元
  - 环形缓冲容量回绕
  - 多订阅者并发推送 + unsubscribe 不阻塞
  - 慢订阅者丢弃旧消息不影响其他订阅者

internal/middleware/*_test.go    # 单元 + httptest
  - cors 中间件对 /__fakeserver/* 路径前缀豁免
  - OPTIONS 在路由匹配后才走 204 短路

cmd/fakeserver/e2e_test.go       # 端到端
  - 真实启动 fakeserver（随机端口）+ http.Client 发请求验证
  - 覆盖：默认 echo、配置 mock、proxy 路由、热加载触发后新路由生效、env 切换
  - init 子命令生成的文件可直接 serve
```

**目标覆盖率**：`internal/{config,tpl,mock,proxy,registry,recorder,middleware}` ≥ 80%；`cmd` 跑通主链路即可。

**测试夹具组织**：

```
testdata/
  configs/
    valid/ ...               # 各种合法配置
    invalid/ ...             # 各种错误用例（断言错误消息片段）
    include/                 # include 链测试，含一个 cycle 子目录
    env/                     # [v0.2] env 文件用例
    duplicate/               # 跨条目同 method+path 用例
  responses/                 # bodyFile 用的 fixtures
```

---

## 8. 环境配置文件（v0.2）

### 8.1 文件约定

| 路径 | 加载顺序 |
|---|---|
| 与主配置同目录的 `fakeserver.env.json5` | 默认自动加载 |
| `--env-file path/to/env.json5` 指定 | 覆盖默认查找；优先级最高 |

CLI 选项：`-e/--env <name>` 选择当前段（默认值见下文）；`--var key=val[,key=val]...` 单点覆盖（多次出现累加）。

### 8.2 文件结构

```json5
{
  "$default": {              // 所有 env 共享的基线；可省略
    apiHost: "localhost",
    timeout: "5s",
  },
  dev: {
    apiHost: "localhost:5090",
    token: "dev-xxx",
  },
  staging: {
    apiHost: "stage.api.com",
    token: "stg-yyy",
  },
  prod: {
    apiHost: "api.com",
    token: "{{ osenv \"PROD_TOKEN\" }}",   // env 文件值本身也是模板字符串
  },
}
```

合并规则（结果即模板 `.env`）：

```
.env = deepMerge($default, env[currentName])
.env = deepMerge(.env, parseKVPairs(--var))
```

模板表达式在 env 文件**值层级**支持（仅渲染字符串叶子，与 route body 同语义），可调用 `osenv` 引用 OS 环境变量（**受 §4.6 osenv 白名单约束**）、`uuid` 生成 stable seed 等。env 文件不支持 `@include`（避免和主配置 include 嵌套引起递归复杂度爆炸）。

### 8.3 默认 env 选择优先级

```
1. CLI --env <name>
2. 环境变量 FAKESERVER_ENV
3. env 文件中字段 "$active": "dev"（可选元字段）
4. env 文件中第一个非 $default 段
5. 没有 env 文件 → .env = {}（空），不报错
```

### 8.4 模板中使用

```json5
{
  method: "GET", path: "/me",
  body: {
    apiHost: "{{ .env.apiHost }}",
    token:   "{{ .env.token }}",
    fromOS:  "{{ osenv \"USER\" }}",
  }
}
```

### 8.5 热加载

env 文件被 watcher 监听；变更触发与主配置相同的"重新加载+原子切换"流程。切换 env（`fakeserver --env staging` 或修改 `$active`）需要重启或在 web UI（v0.4）触发软切换。

---

## 9. Proxy 路由（v0.1）

### 9.1 触发与互斥

**触发**：route 对象出现 `proxy` 字段即被视为 proxy 路由。**不引入 `type` 字段**。
**互斥**：proxy 路由不能与 `body / bodyFile / cases / status / headers / delay` 同存（启动期校验失败）。

### 9.2 字段定义

```json5
{
  method: "*",
  path: "/api/users/*rest",
  proxy: {
    target: "http://real-backend.local:8080",   // 必填，含 scheme + host[:port]；字面量，不渲染模板
    rewrite: "^/api/users => /v2/users",        // 可选：单条 string 或多条数组
                                                 //   语法：<go-regex> => <replacement>
                                                 //   $1/$2 引用正则捕获组（不是模板变量）
                                                 //   字面量，不走 template 渲染
                                                 //   匹配则替换；不匹配则继续下一条；全部不匹配则原样
    stripPathPrefix: "/api",                    // 可选：纯字符串前缀剥离（在 rewrite 前应用）；字面量
    headers: {                                  // 可选：注入到上游请求的 header（value 支持模板）
      "X-Forwarded-By": "fakeserver",
      "Authorization": "Bearer {{ .env.token }}",
    },
    responseHeaders: {                          // 可选：注入到回写客户端的响应 header（value 支持模板）
      "X-Mocked-By": "fakeserver-proxy",
    },
    timeout: "10s",                             // 单次请求超时（拨号 + 读写），默认 30s
    insecureSkipVerify: false,                  // HTTPS 自签证书时用
    preserveHost: false,                        // 是否保留客户端的 Host header；默认改为 target Host
    bodyLimit: "10MiB",                         // 上行请求 body 上限（透传客户端→上游），默认随 server.maxBodySize
  }
}
```

**模板渲染边界**（v0.3 明确）：proxy 块内**只有 `headers` 与 `responseHeaders` 的 value 字符串走 template 渲染**，其余字段（target / rewrite / stripPathPrefix / timeout 等）均为字面量。`rewrite` 中的 `$1`、`$2` 是 Go `regexp.ReplaceAllString` 的捕获组语法，与模板变量无关。

### 9.3 行为细节 *(v0.3 修订)*

- 使用标准库 `httputil.ReverseProxy`，每条 proxy route 持有一个独立实例（target/timeout 不同）
- **请求 body 默认透传**（标准库默认行为）；`proxy.bodyLimit` 限制**上行请求 body 的最大字节数**（防止用 mock server 当成大文件中继），不限制上游响应大小。未配置时继承 `server.maxBodySize`
- path 改写顺序：`stripPathPrefix` → 逐条 `rewrite` 尝试 → 用首个命中的结果
- header 注入在 `Director` 阶段执行；`X-Forwarded-For` / `X-Forwarded-Host` / `X-Forwarded-Proto` 自动追加
- 响应错误：拨号失败 → 502 + 标准错误体（含 `target` 字段）；上游 5xx 透传原响应
- 响应**默认不主动压缩 / 解压**：标准库 ReverseProxy 透传 `Content-Encoding`，客户端原样接收
- proxy 路由也参与请求日志（标记 `→ proxy <target>`）和请求历史
- 已知限制：proxy 路由内不进入模板渲染管线（除 header 值是模板字符串外）；不支持 cases/strategy。如需 conditional proxy，请在精确 mock 路由上"截胡"

### 9.4 安全护栏

- target 仅允许 `http://` 或 `https://`；其他 scheme 启动报错
- `proxy.target` 解析后的 Host 若是 `localhost` / 私有网段，启动日志打 info（提示用户确认）；不强制阻止

---

## 10. 全局项目注册（v0.3）

### 10.1 路径约定

**所有平台统一**：`~/.config/fakeserver/projects.json`

- Linux / macOS：`$HOME/.config/fakeserver/projects.json`
- Windows：`%USERPROFILE%\.config\fakeserver\projects.json`（**不使用 `%APPDATA%`**，方便跨平台查找与脚本处理）

目录不存在时启动期自动创建（0700 权限）。

### 10.2 文件结构

```json
{
  "version": 1,
  "lastActiveId": "a1b2c3d4e5f6",
  "projects": [
    {
      "id": "a1b2c3d4e5f6",
      "name": "my-app",
      "configPath": "/abs/path/fakeserver.json5",
      "cwd": "/abs/path",
      "envs": ["dev", "staging", "prod"],
      "lastEnv": "dev",
      "lastPort": 5090,
      "lastRunAt": "2026-05-19T10:23:11Z",
      "pidFile": "/abs/path/.fakeserver/run.pid"
    }
  ]
}
```

字段约定：

- `id`：`sha1(configAbsPath)` 取前 12 hex 位；同一配置文件路径在不同时间启动也是同一项目
- `name`：默认取配置文件所在目录名；用户可在 `server.projectName` 显式覆盖
- `envs`：从 env 文件提取的所有段名（剔除 `$default` / `$active`）

### 10.3 并发安全

- 写入：先写到 `projects.json.tmp` → `fsync` → `rename` 原子替换
- 锁：跨进程文件锁（Linux/macOS `flock`，Windows `LockFileEx`）；上锁失败重试 5 次（指数退避 10ms~160ms），仍失败仅 warn 不阻塞
- 损坏：解析失败 → 重命名为 `projects.json.bak-<timestamp>` → 重建空文件

### 10.4 PID 文件 + 探活

- 启动期写 `<cwd>/.fakeserver/run.pid`，含 `pid\nport\nstartedAt`
- `fakeserver list` 读 PID 文件 → `os.FindProcess` + signal(0) 探活；活进程标记 `running` + 端口；死进程清理 PID 文件
- 退出时（含信号退出）删除 PID 文件

### 10.5 CLI 行为

```
fakeserver list
  ID            NAME       STATUS    PORT   ENV    LAST RUN
  a1b2c3d4e5f6  my-app     running   5090   dev    2026-05-19 10:23
  f7e6d5c4b3a2  other-app  idle      -      prod   2026-05-18 14:01

fakeserver use a1b2c3d4e5f6
  → lastActiveId 更新为 a1b2c3d4e5f6（不启动 server；下次打开 web UI 时默认聚焦此项目）
```

---

## 11. Web UI（v0.4）

### 11.1 设计原则

- **极简**：系统字体、原生 HTML/CSS/JS，无外部 CDN，无构建工具链
- **只读为主**：所有写操作（编辑路由、修改配置）暂不支持；调整请直接改文件，由热加载生效
- **嵌入静态资源**：`go:embed assets/*` 编入二进制，启动即可用
- **无登录**：默认仅绑定 `127.0.0.1`（继承 server.host）；公网部署需用户自行加反向代理鉴权（文档说明）

### 11.2 页面结构

```
/__fakeserver/ui/
├── /                       项目列表（侧栏）+ 当前项目概览（主区）
├── /routes                 当前项目路由表（mock + proxy 分组；支持搜索过滤）
├── /history                请求历史（虚拟滚动；可点开看单条详情）
└── /config                 当前项目配置只读视图（语法高亮 JSON5）
```

侧栏永久显示项目列表，主区随 URL 切换。

### 11.3 API 端点

详见 § 5.6 表格。所有 API 与 UI 静态资源**不挂载 CORS 中间件**（§5.4），仅同源访问。

`/__fakeserver/events`：

```
GET /__fakeserver/events
Accept: text/event-stream

event: request
data: {"ts":"...","method":"GET","path":"/users/42","status":200,"durationMs":3,"hit":"mock#1"}

event: reload
data: {"added":["GET /v2"], "removed":[], "changed":["GET /me"]}
```

### 11.4 请求历史的数据来源

请求日志中间件在写 stdout 的同时，调用 `recorder.Ring.Append(Entry)`，环形缓冲容量 = `server.historySize`（默认 200）。历史不持久化（重启清空）；持久化属于 v1.x 功能。

每个 `Entry` 含：时间戳、method、path、status、duration、client IP、命中 route 索引、命中 case 索引（如适用）、proxy target（如适用）。**不包含请求/响应 body**（隐私 + 体积）。如需 body 详情，路线图考虑增加 "debug capture" 开关（v1.x）。

### 11.5 SSE 实时推送

- 每个 SSE 连接通过 `recorder.Subscribe()` 拿一个独立 channel；客户端断开自动 unsubscribe
- 单个慢客户端不会拖累其他客户端：channel 非阻塞 send，满则丢弃该客户端最旧消息并打点（"events.dropped" 指标）
- 心跳：每 15s 发一个 `: ping` 注释行（保持连接活）

### 11.6 安全护栏

- `server.adminEnabled: false` → 所有 `/__fakeserver/*` 端点不挂载，UI 也不可达
- 强烈不推荐将 server.host 设为 0.0.0.0 同时打开 adminEnabled；启动时若检测到该组合，打 WARNING 日志

---

## 12. Faker 数据填充（v0.1）

### 12.1 引入

`github.com/brianvoe/gofakeit/v7`。启动期：

```go
seed := cfg.Server.FakerSeed
if seed == 0 {
    gofakeit.Seed(time.Now().UnixNano())
} else {
    gofakeit.Seed(seed)
}
```

测试场景设固定 seed，结果可复现。

### 12.2 函数表达：双入口

**A. 常用字段单独注册**（约 20 个，按使用频次精选）：

```
人物 / 联系：
  fakeName            fakeFirstName       fakeLastName
  fakeEmail           fakeUsername
  fakePhone

地理 / 地址：
  fakeCity            fakeCountry         fakeAddress
  fakeZip

网络 / 系统：
  fakeIPv4            fakeIPv6            fakeURL
  fakeUserAgent

业务 / 内容：
  fakeCompany         fakeJob
  fakeWord            fakeSentence        fakeParagraph

数值 / 时间：
  fakeIntRange a b              [a,b] 闭区间整数
  fakeFloatRange a b
  fakeDate                      随机日期（year ±5）
  fakePastDate                  过去 365 天内
  fakeFutureDate                未来 365 天内
```

**B. 通用入口 `fake "<name>"`**：

```
{{ fake "color" }}                 → gofakeit.Color()
{{ fake "creditcardnumber" }}      → gofakeit.CreditCardNumber(nil)
{{ fake "carmaker" }}              → gofakeit.CarMaker()
```

实现：维护一张 `name → func() string` 映射表，启动期通过 `gofakeit.GetFuncs()` 自动注册所有零参 `func() string` 函数；不存在的 name 在模板里返回空串 + warn（不报 500）。

### 12.3 模板示例

```json5
{
  method: "GET", path: "/users/{id}",
  body: {
    id:        "{{ .request.params.id }}",
    name:      "{{ fakeName }}",
    email:     "{{ fakeEmail }}",
    phone:     "{{ fakePhone }}",
    age:       "{{ fakeIntRange 18 80 }}",
    city:      "{{ fakeCity }}",
    bio:       "{{ fakeSentence }}",
    avatar:    "{{ fake \"imageURL\" }}",    // 通用入口兜底
    createdAt: "{{ fakePastDate }}",
  }
}
```

### 12.4 与现有内置函数的命名空间

- 所有 faker 函数以 `fake` 开头，不与 § 4.3 已注册的 `uuid` / `randInt` / 等冲突
- 通用入口 `fake`（单参版本）独占名字 `fake`，不可被覆盖

---

## 13. 未决项 / 待评审

### 已落地（Phase 1 阶段确认）

- **rux 真实 module path**：`github.com/gookit/rux/v2`（v2.0.0）。原 design §1.2 / §2.4 / §5.5 描述中提到的"rux v2"在 Go module 系统中需要显式 `/v2` 后缀；`github.com/gookit/rux`（不带 `/v2`）会拉到 v1.4.1，是另一个仓库分支。
- **rux/v2 server 子包真实 API**：Phase 1 实际接入的是 `server.MountEchoRoutes(r *rux.Router)`——一行调用即可挂上完整 httpbin 风格端点集（含 `/anything`、`/headers`、`/ip`、`/status/{code}`、`/delay/{seconds}`、`/uuid`、`/redirect/{n}`、`/cookies/*`、`/basic-auth/*`、`/bytes/{n}`、`/download/{filename}`、`/upload`，以及一个 HTML 首页 `/`）。design §5.5 原占位描述（"EchoNotFoundHandler / RegisterEchoEndpoints"）已过期，以本条为准；详见 `internal/echo/probe.md`。
- **CLI 编排实际位置**：CLI app 构造、子命令注册、serve 子命令的 run handler 全部位于 `internal/cli/`（`app.go` / `serve.go` / `serve_test.go`）；`cmd/fakeserver/main.go` 仅作极薄入口（5 行非空代码：package + import + var version + func main 调用 `cli.Run(version)`）。后续 Phase 子命令（init/check/routes/list/use）都在 `internal/cli` 内追加一个 .go 文件，不再回到 cmd/。
- **rux/v2 行为偏离 design 文档原假设**：
  - `/ip` 端点返回字段名是 `origin`（不是 design 与 plan 假设的 `ip`）
  - `/status/{非法 code}` fallback 到 200（不是 400）
  - `MountEchoRoutes` 末尾注册了 `/*path` catch-all，事实上代替了 `r.NotFound(...)`——不需要再单独挂 NotFound handler

### 已落地（Phase 2 阶段确认）

- **JSON5 解析库**：`github.com/titanous/json5 v1.0.0` 已接入；smoke 锁定其支持注释、无引号 key、单引号字符串、trailing comma 等扩展
- **`@include` 实际语义**：被引入文件的根可以是对象、数组或单个 route；string include 在 routes 数组里**自动拍平**，在其他位置**整体替换**；仅支持 `.json`/`.json5` 后缀；glob 无匹配视为错误
- **schema 中的 `Log` 字段用指针 `*bool`**：因为零值无法区分"未设置"与"显式 false"；Phase 5 接入 logger 中间件时按 `Log == nil` 视为默认开
- **Validate 范围**：Phase 2 校验所有非依赖 expr/template 的规则（互斥、必填、重复、保留前缀、enum 校验、bodyFile 存在性、proxy.target scheme）。`when` 表达式语法与 `body` 模板语法的校验留给 Phase 4/3 与对应库一并接入
- **Phase 2 边界**：`fakeserver serve -c <path>` 启动时**打印**路由摘要但**不注册** mock 路由 handler；配置中的 path 在 Phase 3 接入前仍走 echo `/*path` 兜底

### 已落地（Phase 3 阶段确认）

- **tplfunc.StdFuncMap 实际清单**：~110 个函数，覆盖 string/math/list/encoding/path/hash/other 多个分类（含 randInt/uuid/md5/b64enc/fromJson/toJson/default/coalesce 等）。design §4.2 原描述"TODO + 少量基础"已过期；fakeserver 自有 22+ 函数仍全部实现，BaseFuncMap 中后注册覆盖
- **gofakeit/v7 v7.15.0 实际 API**：
  - `Seed(int64)` —— 不是 uint64
  - `Generate(s string) (string, error)` —— 二元组返回，需 `out, _ := gofakeit.Generate(...)`
  - `GetFuncs` 不存在 —— 通用 `fake "<name>"` 入口用 `Generate("{<name>}")` 实现
  - `Sentence` / `Paragraph` 在 v7 为 variadic 参数，本 Phase 调用时不传参用默认行为
- **rux v2 Context Params 形态**：`c.Params()` 是方法返回 `*core.Params`；内部 `data [16]Param + n uint8` 全私有；**无 AddParam**。公开遍历用 `Snapshot() []Param`、单 key 查询用 `Get(name)`、或 `c.Param(name)` 快捷方式
- **rux v2 responseWriter 行为**：`WriteHeader(code)` 缓存状态码，到首次 `Write` 才真正发出。零 body 响应需 `Write(nil)` 触发 `ensureWriteHeader`——已在 `mock.Respond` 末尾处理
- **easytpl 接入范围**：仅复用 `tplfunc.StdFuncMap()` 作为基础 FuncMap；**不**使用 easytpl.Renderer 的 layout/partial 能力。fakeserver 的 text/html 双渲染器直接基于 stdlib `text/template` + `html/template`
- **Phase 3 边界**：mock router 跳过含 `cases` 或 `proxy` 字段的 route（Phase 4 处理）；未匹配请求仍走 echo `/*path` 兜底；模板里 `.env` Phase 3 为空 map（v0.2 才接 env 文件），`.osenv` 与 `osenv` 函数完整可用且受 osenvWhitelist 约束

### 已落地（Phase 4 阶段确认）

1. **expr-lang/expr v1.17.8 实际 API**：`Compile(src, AsBool(), Env(...))` / `Run(prog, env any) (any, error)`。字段访问平铺命名空间（`request.query.x`，无前置点号）。运行期错误（字段缺失、类型不匹配）按情形返回 `(nil, error)` 或 `(nil, nil)`——本工程的 `Matcher.Evaluate` 统一降级为 `(false, err)`，调用方 warn 跳过单条 case，不影响整条路由。
2. **ReverseProxy + Director 链路**：path 改写顺序锁定为 `stripPathPrefix` → `rewrite`（首匹配）→ host 切换；template 渲染**只在 `headers/responseHeaders` value 上**生效（design §9.2 明确）。其余字段 `target/rewrite/stripPathPrefix/timeout/...` 均为字面量。
3. **超时实现双层**：`Transport.ResponseHeaderTimeout` + 请求级 `context.WithTimeout`。ErrorHandler 通过 `errors.Is(perr, context.DeadlineExceeded || context.Canceled)` 加字符串兜底区分 504 vs 502。
4. **bodyLimit 在 rux handler 入口前置强制**：放弃 `http.MaxBytesReader`（其错误经 ReverseProxy 的 body 拷贝触发，路径不可控）。改为 `readUpTo(buf of size limit+1)` 读满探测，超限 → 413 + JSON 错误体，body 不打到上游。
5. **`config.Warn(cfg) []string` 与 `Validate(cfg) []error` 并列**：前者只产生 stderr advisories、不影响 exit code；后者产生终止性错误。`first-match` 所有 case 都带 `when`（无兜底）触发 warn；`proxy.target` 私网/localhost 触发 info 级 warn（不阻止启动）。
6. **proxy.headers 模板 ctx 缺 `body/bodyRaw/params`**：proxy 包独立的 `buildProxyRenderCtx(req)` 不读 req.Body（避免 drain），可用键为 method/path/host/headers/query/ip。需要 body 参与 header 模板的场景请改用 mock route 而非 proxy。
7. **Phase 4 边界**：未引入 `fsnotify`；未引入中间件；admin 端点不变；热加载、CORS、recoverer、bodylimit 中间件全部留 Phase 5。`config.Warn` 已就位但只挂在 serve / check 启动期 stderr——运行期警告路径（如 matcher 运行期 err）走 `log.Printf` 写 stderr。

### 已落地（Phase 5 阶段确认）

1. **fsnotify Windows rename 行为**：编辑器原子保存（写 tmp → rename）在 Windows 触发 5 事件序列（CREATE.tmp/WRITE.tmp/REMOVE/RENAME.tmp/CREATE）。300ms 防抖窗口充分聚合。
2. **CORS OPTIONS 后置策略**：用 `bufferedWriter` 在 middleware 层拦截路由响应；路由返回 404 时改写为 204 + preflight headers；否则透传仅追加 CORS header。无需在路由层注册 OPTIONS catch-all。
3. **Holder 原子 swap**：`atomic.Pointer[http.Handler]` 实现零拷贝热替换。在途请求继续走旧 handler 直至完成；新请求走新 handler。
4. **watcher 目录订阅**：单文件 fsnotify Add 在 rename-in-place 后失效；改为订阅每条 SourcePath 的所在目录（去重），事件回调里用 wanted map 过滤回 path 集合。
5. **admin.Mount 签名升级**：从 `Mount(r)` 到 `Mount(r, cfg)`，让 `/__fakeserver/routes` handler 闭包持有 cfg 引用。每次 watcher swap 调用一次 Mount（在 assembleHandler 内），/routes 总返回最新 cfg。
6. **`parseByteSize → ParseByteSize` 导出**：Phase 4 proxy 包私有函数提升为 `proxy.ParseByteSize`，被 cli 包 `parseMaxBodySize` 复用。
7. **`cors: false` 配置生效**：`corsOptsFromCfg(cfg) → (CORSOpts, bool)` 返回 enabled 标志；assembleHandler 据此决定是否添加 CORS 中间件，避免 default 分支误把 false 当 reflect-mode。
8. **v0.1 MVP 完整闭环 E2E 通过**：综合 config（mock + cases + proxy + bodyFile）启动 → 5 类请求验证 → 文件系统编辑 + 300ms 防抖 + holder swap → 新路由生效 → 旧路由仍工作。该测试覆盖 Phase 1-5 全部模块协作。
9. **存量 bug `Route.SourceFile`**（v0.2 修复，bd lite-tools-gko）：loader 用 JSON round-trip 构造 Config，`json:"-"` 字段被吞掉，导致 `bodyFile` 相对路径在 CWD ≠ config 目录时退化。v0.1 用绝对路径或 CWD 对齐 workaround。

### 待评审

- rux v2 `server/` 子包导出符号的具体名称需在实现时核对（设计中以 "EchoHandlers / RegisterEchoEndpoints" 占位）
- 退出 timeout（5s）、模板超时（2s）、热加载防抖（300ms）三个魔法数字是否需要做成配置项
- v1.0 阶段 WS / SSE 的 mock 描述形态（独立 schema 还是复用 routes + protocol 字段），本文档暂不涉及
- Web UI 是否需要 dark mode（v0.4 启动时若用户提到再加）
- **JSON 响应字段顺序稳定性**：当前 v0.1 用 Go `map[string]any` 默认顺序（不稳定）。若后续 e2e 测试体验受影响，再决定是否切换到 sorted keys 或保留 JSON5 解析时的字段原序（需调研 titanous/json5 是否暴露 token 顺序）

---

## 14. 分期路线图

| 期号 | 主要内容 | 新增模块 | 新增依赖 |
|---|---|---|---|
| **v0.1 MVP** | rux 接入 + 默认 echo + JSON5 + @include + 多响应/条件分支 + 模板 + Faker + Proxy + 热加载 + CORS + 日志 + admin/routes & healthz + 默认配置查找 + init 模板 | config, mock, proxy, tpl(含 faker), echo, middleware | rux, easytpl, gcli, titanous/json5, fsnotify, expr-lang/expr, gofakeit, goutil |
| **v0.2** | 环境配置文件 + `--env / --var` + 命名空间调整（`.env` / `.osenv`） | config/envfile | — |
| **v0.3** | 全局项目注册 + `fakeserver list/use` + PID 文件 | registry | — |
| **v0.4** | Web UI + 请求历史 ring buffer + SSE 推送 + admin/api/* | recorder, webui | — |
| **v1.0** | WS / SSE 协议 mock | wsmock, ssemock | gorilla/websocket（或同期推荐） |
| **v1.x** | 录制-回放、json-server 风格资源 CRUD、debug capture、Web UI dark mode/写操作、**静态目录服务**（`{ path:"/static/*rest", static:"./public" }`）、JSON 字段顺序稳定化、日志 JSON 格式 | — | — |

v0.2–v0.4 无新增第三方依赖，仅靠标准库实现。
