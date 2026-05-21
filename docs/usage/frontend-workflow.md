# Frontend Workflow with fakeserver

本文档面向前端日常联调：快速初始化 mock、检查配置、启动服务、观察请求历史、切换 mock 响应。

## 1. 初始化

在项目根目录执行：

```bash
fakeserver init --full
```

该命令生成：

```text
fakeserver.json5
fakeserver.env.json5
.fakeserver/
  routes/
  fixtures/
```

如果目录里已有配置，默认不会覆盖。确实要重建示例时使用：

```bash
fakeserver init --full --force
```

## 2. 检查

先做环境诊断：

```bash
fakeserver doctor -c fakeserver.json5 --env dev
```

再做严格配置检查：

```bash
fakeserver check --strict -c fakeserver.json5
```

`doctor` 偏向运行环境和项目结构诊断，例如端口、env 文件、bodyFile、admin 暴露风险。`check --strict` 偏向配置内容，尤其是模板语法和常见模板写法问题。

## 3. 启动

```bash
fakeserver serve -c fakeserver.json5 --env dev
```

启动后打开：

```text
http://127.0.0.1:5090/__fakeserver/ui/
```

UI 可查看：

- Projects
- Routes
- History
- Config

`init --full` 默认启用 Web UI 调试台所需的受限 capture：

```json5
server: {
  capture: {
    enabled: true,
    maxBodySize: "64KiB",
    redactKeys: ["authorization", "cookie", "password", "token", "secret"],
  },
}
```

capture 只用于内存中的请求历史，超过 `maxBodySize` 会截断，二进制 body 不直接展示。命中 `redactKeys` 的 header 或 JSON 字段会显示为 `"***"`。

## 4. 常用调试

### 查看请求历史

打开 Web UI 的 History 页面，或直接访问：

```bash
curl http://127.0.0.1:5090/__fakeserver/api/history
```

点击 History 行可以打开详情抽屉，查看：

- 请求 headers/body
- 响应 headers/body
- 命中的 route mode、route index、case index、source file
- proxy target

详情 API：

```bash
curl http://127.0.0.1:5090/__fakeserver/api/history/1
```

### 复制 curl 与重放请求

History 详情里的 Copy curl 会生成同源请求命令，并默认跳过已经脱敏的敏感 header。

Replay 使用浏览器 `fetch` 发起，因此浏览器禁止设置的 header 不会被重放，例如：

- `Host`
- `Connection`
- `Content-Length`
- `Cookie`
- `Origin`
- `Referer`
- `Sec-*`

如果请求 body 没有被 capture，Replay 会不带 body 发送并在结果里提示。

### Routes 页面测试接口

Routes 页面每条 route 有 Test 按钮。测试面板支持：

- method
- path
- query，一行一个 `key=value`
- headers，一行一个 `Key: Value`
- body，按原文发送

发送后会展示响应 status、headers、body 和耗时，同时 History 会追加一条新记录。

### 修改配置热加载

修改 `fakeserver.json5` 或 `.fakeserver/routes/*.json5` 后，serve 默认会热加载。Web UI 会收到 reload 事件并刷新路由/配置数据。

### 模拟异常态

使用 `cases`：

```json5
{
  method: "GET",
  path: "/api/users",
  strategy: "first-match",
  cases: [
    { when: "request.query.empty == \"1\"", status: 200, body: { items: [] } },
    { when: "request.query.fail == \"1\"", status: 500, body: { error: "failed" } },
    { status: 200, body: { items: [{ id: "1", name: "alice" }] } },
  ],
}
```

前端可请求：

```text
/api/users
/api/users?empty=1
/api/users?fail=1
```

### 联调真实后端

使用 proxy route：

```json5
{
  method: ["GET", "POST"],
  path: "/proxy/httpbin/*rest",
  proxy: {
    target: "https://httpbin.org",
    stripPathPrefix: "/proxy/httpbin",
  },
}
```

## 5. 常见错误

### Header key 带横线

不要写：

```gotemplate
{{ .request.headers.User-Agent }}
```

应写：

```gotemplate
{{ index .request.headers "User-Agent" }}
```

`check --strict` 会提示这类问题。

### bodyFile 相对路径

`bodyFile` 相对当前 route 文件，而不是进程 CWD。

例如 `.fakeserver/routes/files.json5` 中：

```json5
bodyFile: "../fixtures/report.json"
```

实际指向：

```text
.fakeserver/fixtures/report.json
```

### admin 暴露风险

如果配置：

```json5
server: {
  host: "0.0.0.0",
  adminEnabled: true,
}
```

`/__fakeserver/*` 和 Web UI 会暴露给局域网。仅本地开发建议使用：

```json5
server: {
  host: "127.0.0.1",
}
```

### proxy 指向内网

proxy target 指向 localhost 或私网地址时，`doctor` 会提示确认。这不是错误，只是防止误代理到不该暴露的服务。
