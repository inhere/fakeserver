# fakeserver

[English](README.md) | 简体中文

`fakeserver` 是一个配置驱动的 HTTP mock/fake server，用于前端联调、接口异常态模拟、外部服务未就绪时的本地替身，以及简单的请求排查。

它的核心形态是一个 Go 单二进制：

- 无配置时可直接启动 httpbin 风格的 echo 服务。
- 使用 JSON5 文件声明 mock route、响应状态码、响应头、延迟、响应体和 `bodyFile`。
- 支持 Go template、内置随机/时间/UUID/Faker 函数、`.env` 环境变量段。
- 支持 `cases`、`strategy` 和 `scenarios` 模拟不同业务状态。
- 支持单 route 代理到真实后端，方便 mock 与真实接口混用。
- 支持声明式分页（`paginate`），对响应体里的列表按请求页码切片。
- 请求历史可落盘为 JSONL（`server.historyFile`），需要时可记录请求/响应体。
- 内置 Web UI，可查看 routes、history、config，并切换场景或测试路由。

## 安装

本地开发可直接运行：

```bash
go run ./cmd/fakeserver --help
```

推荐使用 Makefile 安装（会写入版本、提交号和构建时间；检测到 `upx` 才压缩）：

```bash
make install
```

也可直接安装到 Go bin：

```bash
go install ./cmd/fakeserver
```

此方式未经过 ldflags 时会从 git 提交兜底版本信息。

构建当前平台二进制：

```bash
go build -o fakeserver ./cmd/fakeserver
```

也可以使用 Makefile：

```bash
make build
```

`make build` 检测到 `upx` 才压缩，没有时会跳过并提示。

## 快速开始

在一个需要 mock 的项目目录里生成完整示例：

```bash
fakeserver init --full
```

该命令会生成：

```text
fakeserver.json5
fakeserver.env.json5
.fakeserver/
  routes/
  fixtures/
```

检查配置：

```bash
fakeserver doctor -c fakeserver.json5 --env dev
fakeserver check --strict -c fakeserver.json5
```

启动服务：

```bash
fakeserver serve -c fakeserver.json5 --env dev
```

默认端口是 `5090`。启动后访问：

```text
http://127.0.0.1:5090/__fakeserver/ui/
```

也可以直接请求示例接口：

```bash
curl http://127.0.0.1:5090/ping
curl http://127.0.0.1:5090/api/users
curl "http://127.0.0.1:5090/api/users?empty=1"
```

## 常用命令

```bash
fakeserver init [--with-env] [--full] [--force]
```

生成 `fakeserver.json5`。`--with-env` 同时生成 `fakeserver.env.json5`，`--full` 生成一套前端联调示例项目。

```bash
fakeserver serve -c fakeserver.json5 --env dev
```

启动 HTTP server。常用选项：

- `-p, --port`：监听端口，不传时取配置 `server.port`，都没有则 `5090`
- `--host`：监听地址，不传时取配置 `server.host`，都没有则 `0.0.0.0`（运行中改配置里的 host/port 不会切换监听地址，需要重启）
- `-c, --config`：配置文件路径，支持逗号分隔多个文件
- `-e, --env`：选择 `fakeserver.env.json5` 中的环境段
- `--var key=value`：覆盖 env 变量，可重复，也支持单个 flag 内逗号分隔
- `--scenario`：启动时默认场景
- `--no-watch`：关闭配置热加载（默认开启：fsnotify + 每秒轮询兜底，9p/drvfs/NFS 等挂载盘上也能生效）
- `--no-cors`：关闭 CORS 中间件
- `-q, --quiet`：关闭请求访问日志
- `--history-file <path>`：把每个请求追加写入该 JSONL 文件（覆盖配置 `server.historyFile`；启动时按 append 打开，不覆盖已有内容）
- `--history-body`：历史文件里额外记录请求体与响应体（覆盖配置 `server.historyBody`）

```bash
fakeserver version [--json]
```

输出与 `--version` 相同的文本；`--json` 输出 `{"version","commit","buildTime","goVersion"}` 单行 JSON。

```bash
fakeserver check --strict -c fakeserver.json5
```

加载并校验配置，不启动服务。`--strict` 会额外检查常见模板风险。

```bash
fakeserver routes -c fakeserver.json5
```

打印路由摘要。

```bash
fakeserver doctor -c fakeserver.json5 --env dev
```

诊断本地运行风险，包括配置、env 文件、include、`bodyFile`、端口、Web UI 暴露风险、proxy 目标等。

```bash
fakeserver list
fakeserver use <project-id>
```

查看已注册项目，或把某个项目标记为最近使用项目。项目注册信息保存在 `~/.config/fakeserver/projects.json`。

## 配置示例

最小配置：

```json5
{
  server: {
    host: "127.0.0.1",
    port: 5090,
    cors: true,
  },

  routes: [
    { method: "GET", path: "/ping", body: "pong" },
    {
      method: "GET",
      path: "/users/{id}",
      headers: { "Content-Type": "application/json; charset=utf-8" },
      body: {
        id: "{{ .request.params.id }}",
        name: "{{ fakeName }}",
        requestId: "{{ uuid }}",
      },
    },
  ],
}
```

启动：

```bash
fakeserver serve -c fakeserver.json5
```

请求：

```bash
curl http://127.0.0.1:5090/ping
curl http://127.0.0.1:5090/users/1001
```

## 配置结构

`fakeserver.json5` 常用顶层字段：

- `server`：监听地址、端口、CORS、日志、请求体上限、Web UI、history（`historySize` / `historyFile` / `historyBody` / `historyBodyMaxSize`）、capture、默认 scenario 等。
- `globals`：模板中可访问的全局变量。
- `fallback`：无路由命中时的行为，支持 `echo`、`404`，或自定义对象（`status`、`headers`、`body`/`bodyFile`，默认状态码 404）。所有兜底响应带 `X-Fakeserver-Fallback` 标识；内部服务替身建议使用 `404` 获得统一错误结构。

内部服务替身可使用自定义对象返回统一错误结构：

```yaml
fallback:
  status: 404
  body:
    data: null
    status: 404
    code: 404
    message: "fakeserver: no route for {{ .request.method }} {{ .request.path }}"
```

`echo`、`404` 和自定义兜底响应都会带 `X-Fakeserver-Fallback` 响应头。
- `routes`：路由声明数组，也可以使用 `@path/to/file.json5` 引入其他 route 文件。
- `scenarios`：命名场景，用于为多条 route 选择指定 case。

`routes` 支持直接声明 route，也支持 include：

```json5
{
  routes: [
    "@.fakeserver/routes/health.json5",
    "@.fakeserver/routes/users.json5",
    "@.fakeserver/routes/admin/*.json5",
  ],
}
```

include 路径相对当前配置文件所在目录，不是进程工作目录。

## Mock 响应

单响应 route：

```json5
{
  method: "POST",
  path: "/api/login",
  status: 200,
  delay: "120ms",
  headers: { "Content-Type": "application/json; charset=utf-8" },
  body: {
    token: "demo.{{ randString 16 \"base62\" }}",
    user: "{{ .request.body.username }}",
  },
}
```

从文件读取响应体：

```json5
{
  method: "GET",
  path: "/api/report",
  headers: { "Content-Type": "application/json; charset=utf-8" },
  bodyFile: "../fixtures/report.json",
}
```

`bodyFile` 相对当前 route 文件，而不是启动命令所在目录。

`check --strict` 会同时校验路由级与 case 级 `bodyFile` 是否可读，`doctor` 同样会报出缺失的 case 级 `bodyFile`。

## 声明式分页

`paginate` 让 fakeserver 按请求里的页码切片响应体中的列表，适合"分页查询"类接口：

```json5
{
  method: "POST",
  path: "/mes-order/device-task",
  paginate: {
    pageField: "current",   // 请求体/查询参数中的页码字段（默认 current）
    sizeField: "size",      // 请求体/查询参数中的页大小字段（默认 size）
    listPath: "data.list",  // 响应体里列表的点路径（必填）
    totalPath: "data.total" // 可选：写入切片前的总数
  },
  body: {
    data: { current: 1, size: 50, total: 0, list: [/* 全量 */] },
    status: 200,
    code: 0,
  },
}
```

- 页码优先取请求体字段，取不到再取查询参数；页大小缺省（或大于列表长度）时整表返回一页。
- 超出页返回空列表（`[]`），不会报错；`totalPath` 指向的位置存在时写入切片前的总数。
- 与 `cases` 组合时先按 `strategy` 选 case 再分页；case 未写 `paginate` 则继承 route 级配置，写了则覆盖。
- 列表既可以在 `body` 里，也可以来自 `bodyFile`。
- `listPath` 在响应体里解析不到、或响应体不是 JSON 时返回 500 + `paginate error`，不会静默返回未分页数据。

## Cases 与场景

同一路由下可以用 `cases` 模拟成功、空数据、错误等状态：

```json5
{
  method: "GET",
  path: "/api/users",
  strategy: "first-match",
  cases: [
    { name: "empty", when: "request.query.empty == \"1\"", status: 200, body: { items: [] } },
    { name: "server-error", when: "request.query.fail == \"1\"", status: 500, body: { error: "failed" } },
    { name: "success", status: 200, body: { items: [{ id: "u-1", name: "{{ fakeName }}" }] } },
  ],
}
```

场景配置：

```json5
{
  server: {
    scenario: "emptyUsers",
  },
  scenarios: {
    emptyUsers: {
      routes: {
        "GET /api/users": "empty",
      },
    },
    serverErrors: {
      routes: {
        "GET /api/users": "server-error",
      },
    },
  },
}
```

场景优先级：

```text
X-Fakeserver-Scenario > Web UI selected scenario > --scenario > server.scenario > route strategy
```

单次请求覆盖场景：

```bash
curl -H "X-Fakeserver-Scenario: serverErrors" http://127.0.0.1:5090/api/users
```

`when` 表达式说明：

- `check`（不加 `--strict` 也会）预编译所有 `when`，语法错误直接报错，错误信息带 `METHOD /path` 与 case 名。
- 字段缺失（取到 `nil`）属于正常不匹配，不算错误。
- 运行时求值出错（类型错误/求值异常）仍按"不匹配"降级，但不再静默：访问日志行追加 `when_error=<case>:<原因>`，history entry 记录 `whenError`（Web UI 请求详情可见），全部 case 都不匹配时的 500 响应体里用 `unmatched`（只列名字）与 `whenErrors`（带原因）列出各 case 的结果。

## 环境变量文件

`fakeserver.env.json5` 用于按环境切换模板变量：

```json5
{
  "$active": "dev",

  "$default": {
    tenant: "lite-tools",
    token: "{{ osenv \"FAKESERVER_DEMO_TOKEN\" \"demo-token-local\" }}",
  },

  dev: {
    baseUrl: "http://localhost:5090",
  },

  staging: {
    baseUrl: "https://staging.example.test",
  },
}
```

选择环境：

```bash
fakeserver serve -c fakeserver.json5 --env staging
```

也可以使用环境变量：

```bash
FAKESERVER_ENV=staging fakeserver serve -c fakeserver.json5
```

优先级：

```text
--env > FAKESERVER_ENV > fakeserver.env.json5 的 $active > 第一个非 $default 段
```

模板中通过 `.env` 访问：

```json5
{
  body: {
    baseUrl: "{{ .env.baseUrl }}",
    tenant: "{{ .env.tenant }}",
  },
}
```

## Proxy 路由

proxy route 可以把一部分请求转发到真实服务：

```json5
{
  method: ["GET", "POST"],
  path: "/proxy/httpbin/*rest",
  proxy: {
    target: "https://httpbin.org",
    stripPathPrefix: "/proxy/httpbin",
    timeout: "10s",
    headers: {
      "X-Fakeserver-Trace": "{{ uuid }}",
    },
    responseHeaders: {
      "X-Served-By": "fakeserver-proxy",
    },
  },
}
```

proxy route 与 mock 字段互斥：出现 `proxy` 后，不应再配置 `body`、`bodyFile`、`cases`、`status`、`headers`、`delay`。

## Web UI 与调试接口

启用 `server.adminEnabled` 后，启动服务会挂载调试端点；默认仅接受本机来源。需要从别的机器或容器（经端口映射）打开 UI 时，显式设置 `server.adminAllowRemote: true`：

```text
/__fakeserver/ui/
/__fakeserver/healthz
/__fakeserver/routes
/__fakeserver/events
/__fakeserver/api/projects
/__fakeserver/api/config
/__fakeserver/api/history
/__fakeserver/api/history/{id}
/__fakeserver/api/scenario
```

Web UI 可用于：

- 查看项目、配置、路由和请求历史。
- 查看单次请求详情，包括 headers/body、命中的 route/case、proxy target、`when` 求值错误。
- 复制 curl、在浏览器里 replay 请求。
- 在 Routes 页面直接测试接口。
- 切换 selected scenario，或对单个 route 设置 case override。

安全注意：如果实际监听地址是 `0.0.0.0`（来自 `--host` 或配置 `server.host`）且 `adminEnabled: true`，调试端点会暴露给局域网。仅本地联调建议在配置里写：

```json5
server: {
  host: "127.0.0.1",
}
```

或者启动时加 `--host 127.0.0.1` 临时覆盖。

## 请求历史落盘

内存 history 之外，可把每个请求追加写入 JSONL 文件：

```json5
server: {
  historyFile: ".fakeserver/history.jsonl",
  historyBody: false,          // true 时额外记录请求体/响应体
  historyBodyMaxSize: "64KiB", // 每个 body 的截断上限，默认 64KiB
}
```

或启动时用 `--history-file <path>` / `--history-body` 覆盖（命令行优先）。文件按启动追加、不覆盖；每行是 JSON，字段与 Web UI 的 history entry 一致（时间、method、path、status、耗时、client、命中的 route/case、proxy target、`whenError`）。开启 `historyBody` 后每行还会带请求/响应体（超出上限时标 `truncated: true`）；`Authorization`、`Cookie`、`X-*-Key`、`X-*-Token` 头始终脱敏。写失败只打 warn，不影响服务。

> 运行中修改 `server.historyFile` 需要重启才能生效。

## 模板上下文

JSON body 中使用 `jsonValue` 可保留请求字段的原始类型，例如 `{{ jsonValue .request.body.id }}` 会将数字 ID 按数字回显；混排文本仍按字符串渲染，响应头输出 JSON 文本。`toJson`/`fromJson` 在标准模板渲染中产生字符串，不能单独保留 body 字段类型。

常用上下文：

- `.request.method`
- `.request.path`
- `.request.params`
- `.request.query`
- `.request.headers`
- `.request.body`
- `.env`
- `.osenv`
- `.config`

Header key 含横线时，使用 `index`：

```gotemplate
{{ index .request.headers "User-Agent" }}
```

常用函数示例：

```gotemplate
{{ uuid }}
{{ shortid }}
{{ now "2006-01-02T15:04:05Z07:00" }}
{{ randInt 1 10 }}
{{ randString 16 "base62" }}
{{ fakeName }}
{{ fakeEmail }}
{{ fakeCity }}
{{ fake "color" }}
```

## 开发

运行测试：

```bash
go test ./...
```

构建：

```bash
go build -o fakeserver ./cmd/fakeserver
```

查看 CLI：

```bash
go run ./cmd/fakeserver --help
go run ./cmd/fakeserver serve --help
```

主要目录：

```text
cmd/fakeserver/       CLI 入口
internal/cli/         子命令与运行装配
internal/config/      JSON5/env/include/validate/watch
internal/mock/        mock route、cases、selector
internal/proxy/       reverse proxy
internal/tpl/         模板上下文和函数
internal/middleware/  CORS、日志、recover、body limit
internal/recorder/    请求历史
internal/webui/       内置 Web UI 和 API
docs/                 设计文档、分期计划、使用说明
```

更多前端联调流程见 `docs/usage/frontend-workflow.md`。
