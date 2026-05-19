# rux/server 子包导出符号探测（v2 对齐版）

rux 版本: v2.0.0（module path: `github.com/gookit/rux/v2`）
探测日期: 2026-05-19

## 关心的符号

- 处理 /anything 风格回显的 handler: `server.MountEchoRoutes(r *rux.Router)`（批量挂载，含 /anything）
- 注册批量端点的入口: `server.MountEchoRoutes(r *rux.Router)`（一次性挂载完整 httpbin 端点集）
- 函数签名:
  ```go
  func MountEchoRoutes(r *rux.Router)
  func NewEchoServer() *Server   // 备用，会绑死整个 Server，本项目不用
  ```

## v2 端点全集（由 MountEchoRoutes 注册）

- `GET /`                           — HTML 首页索引
- `ANY /anything`, `ANY /anything/*path` — 完整请求 JSON 回显
- `ANY /get|/post|/put|/patch|/delete` — method-locked 端点（错方法返回 405）
- `GET /headers`                    — 仅 headers
- `GET /ip`                         — `{"origin": "<ip>"}`（注意：不是 `ip` 字段）
- `GET /user-agent`                 — User-Agent
- `ANY /status/{code}`              — 任意状态码；非法值 fallback 到 200
- `GET /delay/{seconds}`            — sleep（cap 10s）
- `GET /redirect/{n}`               — 倒数重定向
- `GET /cookies`, `GET /cookies/set/{name}/{value}` — cookie 操作
- `GET /basic-auth/{user}/{passwd}` — Basic Auth
- `GET /bytes/{n}`                  — 随机字节
- `GET /uuid`                       — UUID v4
- `GET /download/{filename}`        — 合成下载
- `POST /upload`                    — multipart 上传 echo
- `ANY /*path`                      — 最后兜底（任何未匹配路径都会回显）

## 落地结论

在 `internal/echo/mount.go` 中以一行调用接入：

```go
func Mount(r *rux.Router) {
    server.MountEchoRoutes(r)
}
```

由于 v2 自带 `/*path` 兜底，**不需要** 调用 `r.NotFound(...)`——echo 的 catch-all 已覆盖任意 path。

## 与 design §1.4 / §5.5 的偏差

design 旧描述假设 rux v2 提供"散装 handler 可 NotFound 接入 + RegisterEchoEndpoints"。实际 v2 提供更优雅的 `MountEchoRoutes` 单点入口，且端点种类比 design 描述的更丰富（多了 /uuid /redirect /cookies/* /basic-auth/* /bytes/{n} /download /upload）。Task 9 会把这些事实回写到 design。
