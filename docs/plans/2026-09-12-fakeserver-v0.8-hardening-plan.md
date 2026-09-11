# fakeserver v0.8 加固与增强计划

> 日期：2026-09-12 · 基线：`dad2f82`（tag v0.7.0 之后）· 实施：主机 codex 分三阶段执行，每阶段结束由 Claude 验收
> 来源：2026-09-12 只读代码审查（codex 出报告，Claude 对照源码抽查属实），用户从中选定本计划的全部条目。

## 进度

| 阶段 | 内容 | 状态 | job / 提交 |
|---|---|---|---|
| 1 | 小修四项（1.1-1.4） | 已完成 | `46a9bb5` / `4f59d3e` / `8b4ad66` / `665bf5c` |
| 2 | fallback 可配置 + admin 默认仅本机（2.1-2.2） | 待开始 | |
| 3 | 模板两项（3.1-3.2） | 待开始 | |

## 通用约定（每个阶段都适用）

- 只改本仓库；不 push；不 amend / rebase 已有提交；不改 go.mod 依赖版本。
- 每个条目一个提交，英文 conventional commits（如 `fix(recorder): ...`）。文档改动可随所属条目一起提交。
- 换行一律 LF（仓库已有 `.gitattributes`）。
- 每个阶段结束必须全部通过：`gofmt -l .` 为空、`go vet ./...`、`go test ./...`、`go build ./cmd/fakeserver`（产物不留在仓库）；`git status --short` 为空。
- 行为改动要同步 README 与 `docs/fakeserver-design.md` 对应段落。
- 最终回复：提交列表（hash + 标题）/ 每个条目的实现位置（文件:行）/ 验收命令与结果 / 未做或有风险的点。只汇报实际做过并看到结果的事。
- 完成后把上面进度表中本阶段的状态改为"已完成"，并随最后一个提交一起提交。

## 阶段 1：小修四项

### 1.1 recorder 广播与取消订阅竞态（会 panic）

现状：`internal/recorder/recorder.go` 的 `broadcast` 在 `subsMu` 内复制订阅 channel 列表后解锁，再在锁外发送；
`Subscribe` 返回的 `cancel` 与 `CloseSubscribers` 在锁内 `close(ch)`。二者并发时会向已关闭的 channel 发送，直接 panic
（SSE 客户端断开的同时有请求被记录即可触发，优雅退出时 `CloseSubscribers` 也会撞上）。

要求：发送与关闭走同一把锁。发送本身是 `select { case ch <- ev: default: }` 非阻塞，持锁发送不会拖住其他订阅者，
可直接在持锁期间完成发送；也可改用其他同样安全的协议，但不得再出现"锁外 send、锁内 close"。

测试：新增并发测试，多个 goroutine 循环执行 Subscribe/cancel、Append（触发 broadcast）、CloseSubscribers，
在 `go test -race ./internal/recorder ./internal/webui` 下稳定通过（主机若 `CGO_ENABLED=0` 无法跑 -race，说明即可，验收方会在 Linux 上跑）。

### 1.2 版本信息

现状：
- `internal/cli/serve.go` 的 `version()` 写死返回 `"v0.1.0"`，启动横幅永远显示 v0.1.0，绕过了构建注入的版本。
- `cmd/fakeserver/main.go` 默认 `Version = "0.1.0"`（最新 tag 已是 v0.7.0），注释写的是小写 `-X main.version=...`，与实际变量名不符。
- 不经 make（`go build` / `go install` / `go run`）时没有任何版本兜底。
- `Makefile` 的 `build`、`install` 硬依赖 `upx`；`install` 用 `$(GOPATH)/bin`，环境变量 GOPATH 未设置时路径错误。
- README 安装说明写的是 `go install ./cmd/fakeserver`，而正式安装方式是 `make install`（会注入版本信息）。

要求：
- `version()` 改为取 `buildinfo`（横幅显示版本，有提交号时一并显示短提交号）。
- `main.go` 默认值改为 `"dev"`，修正注释中的变量名。
- `internal/buildinfo` 增加兜底：未通过 ldflags 注入（Version 为空或 `dev`）时读 `runtime/debug.ReadBuildInfo()`：
  主模块版本不是 `(devel)` 时用它作 Version；`vcs.revision` 取前 7 位作 GitHash，`vcs.modified=true` 时追加 `-dirty`；`vcs.time` 作 BuildTime。
  兜底逻辑写成接收 `*debug.BuildInfo` 的纯函数，单测覆盖：已注入 / 未注入且有 vcs 信息 / 未注入且无 vcs 信息 / modified。
- `Makefile`：安装路径用 `$(shell go env GOPATH)`（若设置了 `GOBIN` 则优先）；`build` 与 `install` 中的 upx 改为"检测到才压缩，否则打印跳过提示"，不再是硬依赖。
- README 安装段改为推荐 `make install` 并说明原因（写入版本、提交号与构建时间；upx 可选）；`go install ./cmd/fakeserver` 保留为备选，注明此时版本信息来自 git 提交兜底。

### 1.3 质量门禁

现状：`.github/workflows/go.yml` 只有 staticcheck（`fail_level: 'none'`，查出问题也不失败）、`go mod tidy` 与 `go test`；没有 gofmt、go vet；Makefile 没有统一检查入口。

要求：
- CI（ubuntu + stable 那一格即可）增加：gofmt 检查（有不合规文件时列出并失败）、`go vet ./...`、`go test -race ./...`；staticcheck 的 `fail_level` 改为 `'any'`（保留 `filter_mode: added`）。Windows 那一格保持原有测试。
- Makefile 增加 `fmt-check`、`vet`、`test`、`check`（= fmt-check + vet + test）目标，并加入 `.PHONY`。

### 1.4 两处小问题

- `runServe`（`internal/cli/serve.go`）`signal.Notify(stop, ...)` 之后没有 `signal.Stop`，补 `defer signal.Stop(stop)`。
- `internal/proxy/proxy.go`：请求头 / 响应头模板渲染失败时只 `log.Printf` 并丢掉该头，请求照样转发。
  改为 fail-closed：请求头渲染失败不转发，返回 502；响应头渲染失败让 `ModifyResponse` 返回错误走 ErrorHandler 返回 502。
  错误体沿用现有 proxy 错误响应格式，写明 route 与出错的头名。两种情况各一条测试。

## 阶段 2：fallback 可配置 + admin 默认仅本机

### 2.1 未命中路由的返回方式可配置

现状：顶层 `fallback` 只接受 `"echo"`（默认，回显请求、状态 200）或 `"404"`；校验报错却写成 `server.fallback`；
echo 返回 200，被当作内部服务替身时调用方容易误判为成功。

要求：
- `fallback` 保持兼容字符串 `"echo"` / `"404"`，新增对象形式：
  `fallback: { status: 404, headers: { ... }, body: ... }`（也可 `bodyFile`）。`status` 缺省 404；`headers`/`body` 支持模板，渲染语义与普通 mock 路由一致（尽量复用 responder 的渲染代码，不要另写一套）。
- 所有兜底响应（echo / 404 / 自定义）都带响应头 `X-Fakeserver-Fallback: echo|404|custom`，方便调用方和日志一眼识别"没配这条路由"。
- `/__fakeserver/*` 管理端点不受 fallback 影响。
- 校验报错改为 `fallback: ...`（它是顶层字段）；对象形式的非法 status、body 与 bodyFile 同时给出等情况要报错。
- 启动横幅 `fallback=` 显示 `echo` / `404` / `custom(<status>)`。
- 测试：两种字符串形式行为不变且带标识头；对象形式的 status / headers / body 模板（能引用 `.request.path` 等）；校验错误。
- 文档：README `fallback` 说明、设计文档对应章节，给一个"内部服务替身返回统一错误结构"的示例。

### 2.2 admin / Web UI 端点默认只接受本机访问

现状：admin 默认开启（仅 `server.adminEnabled: false` 才关闭），默认又监听 `0.0.0.0`；局域网内任何人都能读配置、请求历史，
还能通过 PUT/DELETE 修改场景。目前只有一条启动 WARNING。

要求：
- 新增 `server.adminAllowRemote`（bool，默认 false）。为 false 时，来源地址（`RemoteAddr`，不信任 X-Forwarded-For）不是回环地址（127.0.0.0/8、::1）
  的 `/__fakeserver/*` 请求一律返回 403，JSON 错误体说明"admin 端点仅限本机访问，如需远程访问设置 server.adminAllowRemote: true"。
- `GET /__fakeserver/healthz` 例外，保持任何来源可达（v0.1 起的探活契约，不含敏感数据）。
- 启动提示随之调整：监听非回环地址且 adminAllowRemote=true 时发 WARNING；监听非回环地址但未放开时，横幅 `ui:` 行注明仅限本机访问。
- `fakeserver doctor` 的相关检查与新语义一致。
- 测试：远程来源访问 admin API / UI / SSE → 403；本机来源 → 正常；远程访问 healthz → 200；adminAllowRemote=true 时远程 → 正常。
- 文档：README 安全提示、设计文档 §11.6。写明这是行为变更，并说明从别的机器或容器（经端口映射）打开 UI 需要设置 adminAllowRemote。

## 阶段 3：模板两项

### 3.1 cases 分支的模板读不到请求体

现状：`internal/mock/cases.go` 的已知限制注释——`BuildRenderCtx` 第一次读取并排空 `req.Body`，`Respond` 里第二次构造上下文时 body 为空；
所以 `when` 能用 `.request.body`，选中 case 的 body/headers 模板却拿不到。

要求：每个请求只构造一次渲染上下文，matcher（when）与 responder 共用；case 的 body/headers 模板可以正常引用 `.request.body` 与 `.request.bodyRaw`。
删掉那段已知限制注释并更新设计文档中相应说明。测试：case 模板回显请求体字段。

### 3.2 模板输出保留原始类型

现状：`internal/mock/responder.go` 对所有模板字符串调用 `Renderer.Render`，结果一律是字符串；请求体里的数字写成
`count: "{{ .request.body.count }}"` 回显出去就成了字符串，调用方按数字解析会失败。现有模板函数里已有 `toJson`、`fromJson`、`obj`。

要求：
- 先确认 `obj` / `fromJson` 等现有函数能否在 body 渲染中保留类型；已有可用机制就复用并补文档，不要重复造。
- 否则新增一个模板函数（建议名 `jsonValue`，先检查无重名）：当 body 中某个 JSON 字符串值**整体只由一个以该函数结尾的模板动作组成**时，
  该值替换为函数参数的原始类型（数字 / 布尔 / null / 对象 / 数组）；与其他文本混排时，按其 JSON 文本内联进字符串；在 headers 中同样输出 JSON 文本。
  取不到的字段（nil / 缺失）得到 `null`。
- 实现方式自定（例如渲染时输出带随机 nonce 的哨兵串，再在 body 递归渲染里识别替换），但必须：不改变现有模板的行为；用户字符串里恰好出现类似标记时不会被误识别。
- 适用于 route、case 和 2.1 的自定义 fallback 的 body。
- 测试：数字 / 布尔 / 对象 / 数组 / null 原样回显；混排文本；headers；case 内使用；fallback 内使用。
- 文档：README 模板章节与设计文档，补一个"把请求里的数字 ID 按数字回显"的示例。
