# rux/server 子包导出符号探测

rux 版本: v1.4.1
探测日期: 2026-05-19

## 关心的符号

- 处理 /anything 风格回显的 handler: NewEchoServer()
- 注册批量端点的入口: NewEchoServer()
- 函数签名（含参数/返回值）:
  ```
  func NewEchoServer() *Server
  ```

## 实现细节

`NewEchoServer()` 返回一个预配置的 `*Server`（`Server` 是 `*rux.Router` 的包装），
其中已注册了一条通配符路由：
```go
s.Any("/{all}", func(c *rux.Context) {
    data := testutil.BuildEchoReply(c.Req)
    c.Respond(200, data, render.NewJSONIndented())
})
```

该路由的处理器使用 `github.com/gookit/goutil/testutil.BuildEchoReply()` 
将 HTTP 请求自动转换为包含以下信息的 JSON 回显：
- origin: 客户端 IP
- url: 请求 URL
- method: HTTP 方法
- query: 查询参数（如有）
- headers: 请求头（如有）
- form: 表单数据（如有）
- body: 原始请求体
- json: JSON 体（Content-Type: application/json 时）
- files: 上传文件（如有）

返回 HTTP 200 和 JSON Indented 格式的数据。

## 落地结论

rux v1.4.1 **提供内置 echo handlers**（路径 A），不需自己实现通用 echo 回显。

在 internal/echo/mount.go 中：
1. 导入 `github.com/gookit/rux/server` 包
2. 调用 `server.NewEchoServer()` 获得预配置 server
3. 从该 server 提取 `*rux.Router`（因 `Server` 嵌入了 `*rux.Router`）
4. 用该 router 作为基础继续注册其他需要的端点
   - `/headers` GET → 可选（`NewEchoServer()` 的 /{all} 已回显 headers）
   - `/ip` GET → 可选（`NewEchoServer()` 的 /{all} 已回显 origin）
   - `/status/{code}` GET → 需自实现
   - `/delay/{seconds}` GET → 需自实现
   - `/__fakeserver/healthz` → admin 包负责

核心回显功能（/anything 及 headers/ip 信息）复用 rux/server 子包的 NewEchoServer()，
额外路由（/status, /delay）在此基础上扩展。
