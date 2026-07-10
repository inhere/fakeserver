# fakeserver

`fakeserver` 是一个配置驱动的 HTTP mock/fake server，用于前端联调、接口异常态模拟、外部服务未就绪时的本地替身，以及简单的请求排查。

它的核心形态是一个 Go 单二进制：

- 无配置时可直接启动 httpbin 风格的 echo 服务。
- 使用 JSON5 文件声明 mock route、响应状态码、响应头、延迟、响应体和 `bodyFile`。
- 支持 Go template、内置随机/时间/UUID/Faker 函数、`.env` 环境变量段。
- 支持 `cases`、`strategy` 和 `scenarios` 模拟不同业务状态。
- 支持单 route 代理到真实后端，方便 mock 与真实接口混用。
- 内置 Web UI，可查看 routes、history、config，并切换场景或测试路由。

## 安装

本地开发可直接运行：

```bash
go run ./cmd/fakeserver --help
```

安装到 Go bin：

```bash
go install ./cmd/fakeserver
```

构建当前平台二进制：

```bash
go build -o fakeserver ./cmd/fakeserver
```

也可以使用 Makefile：

```bash
make build
```

`make build` 会调用 `upx` 压缩二进制；如果本机没有 `upx`，使用上面的 `go build` 即可。

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

- `-p, --port`：监听端口，默认 `5090`
- `--host`：监听地址，默认 `0.0.0.0`
- `-c, --config`：配置文件路径，支持逗号分隔多个文件
- `-e, --env`：选择 `fakeserver.env.json5` 中的环境段
- `--var key=value`：覆盖 env 变量，可重复，也支持单个 flag 内逗号分隔
- `--scenario`：启动时默认场景
- `--no-watch`：关闭配置热加载
- `--no-cors`：关闭 CORS 中间件
- `-q, --quiet`：关闭请求访问日志

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

- `server`：监听地址、端口、CORS、日志、请求体上限、Web UI、history、capture、默认 scenario 等。
- `globals`：模板中可访问的全局变量。
- `fallback`：无路由命中时的行为，支持 `echo` 或 `404`。
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

启用 `server.adminEnabled` 后，启动服务会挂载调试端点：

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
- 查看单次请求详情，包括 headers/body、命中的 route/case、proxy target。
- 复制 curl、在浏览器里 replay 请求。
- 在 Routes 页面直接测试接口。
- 切换 selected scenario，或对单个 route 设置 case override。

安全注意：如果 `server.host` 是 `0.0.0.0` 且 `adminEnabled: true`，调试端点会暴露给局域网。仅本地联调建议使用：

```json5
server: {
  host: "127.0.0.1",
}
```

## 模板上下文

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
