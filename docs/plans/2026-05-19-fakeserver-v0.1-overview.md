# Fakeserver v0.1 阶段规划总览

> 本文档把 v0.1 MVP（见 `../fakeserver-design.md` §1.3 / §14）按"独立可测可交付"原则拆成 5 个 Phase。每个 Phase 对应一份 `phase<N>-*.md` 详细计划，由 `superpowers:writing-plans` 在执行前展开。

## 修订记录

| 日期 | 版本 | 作者 | 变更说明 |
|---|---|---|---|
| 2026-05-19 | v0.1-overview | inhere | 初稿。固化 v0.1 内部的 5 Phase 拆分，Phase 1 已完成 |

后续修订：每完成一个 Phase 后在对应行回写 commit 摘要与实际偏差。

---

## 1. 拆分原则

**v0.1 MVP 是一次性范围**（design §1.3），但**一次性执行成本太高**——直接照搬 design 文档展成单一计划，估算超过 5090 行步骤、20+ 个模块改动，无法在 review checkpoint 之间清晰收口。

把 v0.1 切成 5 个 Phase 的依据：

1. **每个 Phase 末尾必须能 `go run ./cmd/fakeserver serve` 跑出一个对用户可用的产物**——而不仅是"能编译"。例如 Phase 2 完成后 `serve -c config.json5` 能加载并打印路由摘要，即便此时还没真正响应配置里的 mock；Phase 3 完成后单一响应配置已能 echo 出去。
2. **每个 Phase 只引入一组紧密相关的新依赖**——避免 go.sum 一次性大量变动让 review 失焦。
3. **每个 Phase 内部的模块依赖封闭**——前一个 Phase 没准备好的接口，本 Phase 不引用。模块依赖矩阵（design §2.3）按拓扑序展开。
4. **每个 Phase 自带完整测试**——单元 + E2E，不靠后续 Phase 兜底。

## 2. Phase 拆分总览

| Phase | 一句话目标 | 主要新增模块 / 子命令 | 新增第三方依赖 | 前置依赖 | 估计代码量 | 状态 |
|---|---|---|---|---|---|---|
| **1** | 项目骨架 + 零配置 echo | `cmd/fakeserver/`、`internal/cli/`、`internal/echo/`、`internal/admin/` + `serve` 子命令 | rux/v2、gcli/v3、goutil | — | ~400 行 | ✅ 已完成 |
| **2** | 配置加载 + 路由摘要 | `internal/config/`、`internal/cli/{init,check,routes}.go` | titanous/json5 | Phase 1 | ~800 行 | ✅ 已完成 (commit 4182891..b316e98) |
| **3** | 模板与单一响应 mock | `internal/tpl/`（含 faker）、`internal/mock/{router,responder}.go` | easytpl、gofakeit | Phase 2 | ~900 行 | ✅ 已完成 (commit 37839c8..732691e) |
| **4** | 多响应 + 条件分支 + bodyFile + proxy | `internal/mock/{selector,matcher}.go`、`internal/proxy/` | expr-lang/expr | Phase 3 | ~600 行 | ✅ 已完成 (commit 85e01d3..f5a454c) |
| **5** | 运行时与可观测性 | `internal/middleware/`、`internal/config/watcher.go`、admin `/routes` | fsnotify | Phase 4 | ~500 行 | ✅ 已完成 (commit 2f784d3..eb10a80) |

总计：v0.1 MVP ≈ 3200 行代码（含测试），分 5 期落地。

> **关于依赖引入时机**：design §2.4 列了 8 个第三方库；Phase 1 引入了 3 个（rux/v2、gcli/v3、goutil），剩余 5 个在 Phase 2-5 真正用到时按表逐期引入，避免 go.sum 提前膨胀。

---

## 3. 各 Phase 详述

### Phase 1 — 项目骨架 + 零配置 echo（✅ 已完成）

**详细计划**：[phase1-skeleton.md](2026-05-19-fakeserver-v0.1-phase1-skeleton.md)

**目标**：让 `fakeserver serve` 能跑起来——零配置时充当 httpbin 风格 echo server。

**范围 · 包含**：

- Go module 初始化 + `.gitignore`
- 极薄 `cmd/fakeserver/main.go`（≤ 15 行）
- `internal/cli/` 包：gcli App 构造 + serve 子命令骨架 + signal 退出
- `internal/echo/Mount(r)`：复用 rux/v2 的 `server.MountEchoRoutes`
- `internal/admin/Mount(r)`：`/__fakeserver/healthz`
- 全套单元 + E2E 测试

**范围 · 不包含**：任何配置文件、模板、mock、proxy、中间件、热加载——它们留给 Phase 2+。

**前置依赖**：无。

**DoD**：

1. `go build ./...` 通过
2. `go test ./...` 全部通过（11 个用例）
3. `fakeserver serve` 在 :5090 启动
4. `curl /__fakeserver/healthz` → `{"status":"ok"}`
5. `curl /anything` → httpbin 风格 JSON
6. `Ctrl+C` 优雅退出
7. `cmd/fakeserver/main.go` ≤ 15 行

**对 design 章节的映射**：§2.2 模块切分的 cli/echo/admin 行、§5.5 默认 echo 接入、§5.6 admin /healthz、§5.1 启动流程步骤 1/8/11 子集。

**实际落地偏差**（已回写 design §13 "已落地"段）：
- rux module path 必须显式 `/v2`（v2.0.0），不是简单的 `rux`
- echo 通过 `server.MountEchoRoutes(r)` 一行接入；v2 端点比 design 描述更丰富
- v2 行为：`/ip` 返回 `origin` 字段、`/status/{非法code}` fallback 到 200

---

### Phase 2 — 配置加载 + 路由摘要

**详细计划**：[phase2-config.md](2026-05-19-fakeserver-v0.1-phase2-config.md)

**目标**：让 `fakeserver serve -c <paths>` 能加载并校验 JSON5 配置、解析 `@include`、合并多文件，并在启动时打印**路由摘要**（但**还不响应**配置中的 mock 请求——那是 Phase 3 的事）。

**范围 · 包含**：

- `internal/config/loader.go`：JSON5 解析、相对路径 include 展开、glob、循环检测
- `internal/config/schema.go`：Config / Route / Cases / Proxy 结构体 + 默认值
- `internal/config/` 的集中校验（design §3.7）：必填检查、字段互斥、跨条目同 method+path 重复、include 链一致性、保留路径 `/__fakeserver/*` 占用
- `internal/cli/init.go`：生成 `./fakeserver.json5` 模板（`--with-env` 可选生成 env 文件占位）
- `internal/cli/check.go`：仅做校验不启动 server（CI 友好）
- `internal/cli/routes.go`：离线打印路由摘要
- `serve` 子命令加 `-c/--config`：加载配置 + 打印摘要 + 继续走 echo 兜底（mock 路由暂不响应，预留 Phase 3 接入）
- 默认配置查找（design §3.5）：`./fakeserver.json5` → `./fakeserver.json` → `./.fakeserver/config.json5`

**范围 · 不包含**：

- 模板渲染（`{{ ... }}` 字符串字面量保留，不展开）
- mock 响应实际生效（仅打印摘要）
- env 文件（design §8，留 v0.2）
- 热加载（留 Phase 5）
- proxy（留 Phase 4）

**前置依赖**：Phase 1（需要 cli 包的子命令注册位置、admin/echo 包稳定）。

**新增第三方依赖**：`github.com/titanous/json5`。

**DoD**：

1. `fakeserver init` 在空目录生成 `fakeserver.json5` 模板；目标文件已存在时报错退出
2. `fakeserver check -c bad.json5` 在 JSON 语法错 / 字段互斥 / include 循环等场景下退出非 0 + 集中报错（一次列出全部问题）
3. `fakeserver routes -c valid.json5` 打印路由摘要表（方法 / 路径 / mock/proxy 标记 / cases 数）
4. `fakeserver serve -c valid.json5` 启动时打印路由摘要后进入 listen；命中配置中的路由暂时仍 fallback 到 echo（暂不响应 body），命中 `/__fakeserver/healthz` 仍返回 200
5. `go test ./...` 通过；`internal/config/` 单元测试覆盖 JSON5 解析、include 展开（glob/嵌套/循环）、合并、Validate 错误集中收集 ≥ 80%
6. 默认查找路径行为验证：在 CWD 无 `-c` 时，3 档候选都不存在则降回 echo-only 模式（不报错）

**对 design 章节的映射**：§3 全章（顶层结构 / Route 字段 / include / 合并 / 默认查找 / 校验）、§5.7 CLI 子命令（init/check/routes 三行）、§7 测试策略 `internal/config/*_test.go` 段。

**实际落地偏差**：

- `loadFile()` 返回类型从 plan 假设的 `map[string]any` 调整为 `any`——因为被 @include 的文件根可能是数组（如 routes/users.json5 是 `[ {...}, {...} ]` 而非对象）。`Load` 主流程对根做了 map 类型断言以保证主配置仍是对象
- gcli v3.3.1 默认行为：CLI Func 返回普通 `fmt.Errorf` 时进程退出码仍是 0（仅 stderr 打印 ERROR）。Phase 2 收尾时修复——`internal/cli/{init,check,routes}.go` 改用 `errorx.Failf(1, ...)`（实现 `errorx.ErrorCoder` 接口）；`internal/cli/app.go` 改 `app.Run(nil)` 为 `os.Exit(app.Run(nil))`。修复 commit `b316e98`
- 其余实现与 plan 一致；无设计偏离

**Phase 2 测试覆盖**：49 个用例（admin 1 + cli 16 + config 26 + echo 6）；`internal/config` 覆盖率 88.5%

---

### Phase 3 — 模板与单一响应 mock

**详细计划**：[phase3-template-mock.md](2026-05-19-fakeserver-v0.1-phase3-template-mock.md)

**目标**：让 `fakeserver serve -c routes.json5` **真正响应** 配置里的 **单一响应模式** route（含模板渲染 + faker），同时支持 `bodyFile`。

**范围 · 包含**：

- `internal/tpl/render.go`：easytpl 包装；text/html 双渲染器；FuncMap 注入
- `internal/tpl/context.go`：构建 `.request` / `.now` / `.config`（design §4.1，注意 `.env` / `.osenv` 在 v0.2 才完整，本 Phase 留空 map 占位）
- `internal/tpl/funcs.go`：合并 `tplfunc.StdFuncMap()` + fakeserver 自有函数集（design §4.3 全集 22 个）
- `internal/tpl/faker.go`：gofakeit 桥接（design §12.2 A 段 20 个常用 + B 段通用入口）
- `internal/mock/router.go`：把 config 中**单一响应** route 注册到 rux router；路由优先级由 rux radix 树保证
- `internal/mock/responder.go`：单次响应处理（status / headers 渲染 / body 渲染 / bodyFile / delay / Content-Type 推断）
- `serve` 子命令接入 mock：路由摘要表里"mock"标识真正能响应了

**范围 · 不包含**：

- `cases` 数组与 strategy（多响应留 Phase 4）
- `when` 条件分支（留 Phase 4）
- proxy（留 Phase 4）
- 中间件 / 热加载（留 Phase 5）

**前置依赖**：Phase 2（需要 config schema、Route 结构体、Validate）。

**新增第三方依赖**：`github.com/gookit/easytpl`、`github.com/brianvoe/gofakeit/v7`。

**DoD**：

1. 编写一个测试用 config：`{ routes: [{ method:"GET", path:"/u/{id}", body: { id: "{{ .request.params.id }}", name: "{{ fakeName }}" }}] }`，`curl /u/42` 返回 200 + JSON，含 `"id":"42"` 与一个非空 name
2. `bodyFile: "fixtures/avatar.png"` route 返回文件字节，Content-Type 按 `.png` 推断
3. Content-Type 推断 4 种 case（design §4.5）全部覆盖测试
4. text 模式（默认）不做 HTML 转义；显式 `Content-Type: text/html` 进 html 模式
5. faker 函数：常用 20 个 + 通用 `fake "<name>"` 都可用；`server.fakerSeed: 123` 可让响应字段稳定重现
6. `delay` 区间 `"100ms~500ms"` 实际 sleep 落在范围内
7. `go test ./...` 通过；`internal/{tpl,mock}` 单元测试覆盖率 ≥ 80%

**对 design 章节的映射**：§4 全章（上下文 / 函数 / 双渲染器 / 渲染顺序）、§12 Faker、§3.2 单一响应字段（不含 cases/proxy）、§7 测试策略 `internal/{tpl,mock}/*_test.go` 段。

**实际落地偏差**：

- `tplfunc.StdFuncMap` 实际包含 ~110 个函数（Task 1 探查发现），远超 design §4.2 假设的"少数基础函数 + TODO"。但 BaseFuncMap 的合并顺序（先 copy tplfunc，再 copy fakeserver 自有覆盖）保证 §4.3 列出的语义在我们自己手里
- `gofakeit/v7` 实际 API：`Seed(int64)`（不是 plan 假设的 uint64）；`Generate(s string) (string, error)` 返回**二元组**（plan 假设单字符串返回）；`GetFuncs` **不存在**——通用 `fake "<name>"` 入口用 `Generate("{<name>}")` 实现。`Sentence/Paragraph` 在 v7 改为 variadic 参数，调用时不传参用默认值
- `rux v2 Context.Params` 是**方法**返回 `*Params` 而非字段；内部 `data [16]Param + n uint8` 全私有；**无 AddParam**。遍历用 `c.Params().Snapshot() []Param`。这迫使 mock 测试**不能**手动构造 *rux.Context，改用 `httptest.NewServer + rux.New() + Mount` 走真实路由匹配——结果反而更稳健（测试覆盖生产路径，不依赖 stub）
- `rux v2 responseWriter` 缓存 `WriteHeader` 状态码到首次 `Write` 才下发；零 body 响应需 `Write(nil)` 触发——已在 `Respond` 末尾处理
- 本 Phase 不调用 `easytpl.Renderer`，仅复用其 `tplfunc.StdFuncMap()` 作为基础函数集。easytpl 的 layout/partial 能力 v0.1 用不上，留待未来
- **Bug 修复（commit `4d1f542`）**：`BuildRenderCtx` 原本返回 `*RenderCtx` struct，Go `text/template` 按字段反射只识别大写字段名（`.Request.Params.id`），与 design §4.1 全篇小写访问（`.request.params.id`）冲突。修复：`BuildRenderCtx` 改为返回 `map[string]any`，键名按 design §4.1 小写命名；`Renderer.Render(src, ctx map[string]any)` 接口签名同步调整。`RenderCtx`/`RequestCtx` struct 保留作为 design §4.1 的类型契约说明，不再运行时使用
- **测试覆盖率补足（commit `732691e`）**：从 tpl 64.1% / mock 69.3% 补至 **tpl 95.3% / mock 92.1%**——超过 DoD ≥ 80% 阈值。新增 50 个 tpl 用例（faker wrapper + funcs 错误分支 + render 边界） + 13 个 mock 用例（模板渲染失败、bodyFile 不存在、delay 区间等错误路径）
- 其余实现与 plan 一致

**Phase 3 测试覆盖**：**107 个用例**（tpl 72 + mock 22 + cli 5 + 其余 admin/config/echo = 加总 ~120 PASS）；`internal/tpl` 覆盖率 **95.3%**；`internal/mock` 覆盖率 **92.1%**（DoD ≥ 80% 阈值已达）

---

### Phase 4 — 多响应 + 条件分支 + Proxy

**详细计划**：[phase4-advanced-proxy.md](2026-05-19-fakeserver-v0.1-phase4-advanced-proxy.md)

**目标**：mock 路由支持 `cases` 多响应（四种 strategy）与 `when` 条件分支；route 出现 `proxy` 字段时改走反向代理。

**范围 · 包含**：

- `internal/mock/selector.go`：四种 strategy（random / round-robin / weighted / first-match）；轮询计数器 per-route
- `internal/mock/matcher.go`：expr 表达式编译 + 求值（含错误降级——表达式求值错视为不匹配 + warn）
- `internal/proxy/proxy.go`：`httputil.ReverseProxy` 封装；path rewrite（stripPathPrefix + regex `=>` 替换，含 `$1` 捕获组语义）；header 注入（仅 header value 走模板）；bodyLimit 限上行；502 / 拨号失败错误体
- 启动期校验扩展：proxy 字段互斥（与 body/bodyFile/cases/status/headers/delay）、proxy.target scheme 校验
- 路由摘要表区分 mock / proxy（`→ proxy <target>` 标记）

**范围 · 不包含**：

- WS / SSE proxy（v1.0+）
- proxy 路由内的 cases/strategy（design 明确不支持）
- 中间件 / 热加载（留 Phase 5）

**前置依赖**：Phase 3（需要 tpl 渲染器、mock router 已能注册单一响应；本 Phase 在 router 注册时按"有 cases" / "有 proxy" 分支调度到 selector/proxy）。

**新增第三方依赖**：`github.com/expr-lang/expr`。

**DoD**：

1. `strategy: "weighted"` 实测分布：1000 次请求按权重比例（容差 ±5%）
2. `strategy: "first-match"` 命中首个 when 为 true 的 case；全不匹配返回 500 + `{"error":"no case matched"}`
3. 配置中所有 case 都写 `when` 时启动期发 warn（兜底缺失警告）
4. proxy route：用 `httptest.NewServer` 起假上游，验证 path rewrite / header 注入 / 拨号失败 502 / `target` 错误字段
5. proxy 路由能与精确 mock 路由共存（精确 mock 截胡通配 proxy）
6. proxy 字段与 mock 字段互斥的所有组合启动期报错
7. `go test ./...` 通过；`internal/{mock,proxy}` 单元测试覆盖率 ≥ 80%

**对 design 章节的映射**：§3.2 cases/strategy 段、§4.5 cases 选择错误处理表、§9 全章（Proxy 路由）、§6 错误处理 cases-no-match / proxy 上游错误两行。

**实际落地偏差**：

- **expr v1.17.8 API 与 plan 假设零偏差**：`Compile(src, AsBool(), Env(stubEnv))` 返回 `(*vm.Program, error)`；`Run(prog, env any) (any, error)`；`AsBool()` 是 `Option`；`Env(map[string]any)` 可声明类型上下文。所有签名与 plan 假设吻合。
- **expr 运行期"字段缺失" vs "嵌套 nil"行为差异**：`request.query.nope`（query 是空 map）→ `(nil, nil)` 静默返回；而 `request.query.nope`（query 不存在）→ `(nil, error "cannot fetch nope from <nil>")`。两条路径殊途同归——`Matcher.Evaluate` 都返回 `ok=false`，调用方 warn 跳过。
- **`tpl.BuildRenderCtx` 会 drain req.Body**：在 proxy Director 阶段调用会消耗要转发的请求体。落地解决方案是 `internal/proxy/proxy.go` 新增 `buildProxyRenderCtx(req)` helper，只从 method/path/host/headers/query/ip 字段构建 ctx，**不读 body**。已知限制：proxy 模板访问 `.request.body/bodyRaw/params` 会得到空——design §9 已经只承诺 headers/responseHeaders value 的模板渲染。
- **MaxBytesReader 不适用于 ReverseProxy 的 bodyLimit 强制**：MaxBytesReader 的错误经由 ReverseProxy 的 body 拷贝在 ErrorHandler 之外冒出，无法干净返回 413。落地方案：在 rux handler 入口用 `readUpTo(buf of size limit+1)` 前置读取，超限直接 413+JSON，body 未达上游。
- **`parseByteSize` 内联实现**：不引入 `humanize` 或 `goutil/byteutil`。仅支持大写后缀（B/KB/MB/GB/TB + KiB/MiB/GiB/TiB），无单位 fallback 到纯字节。`"16b"`/`"16XB"` 等非法形态显式报错。
- **timeout vs dial-fail 区分**：ErrorHandler 通过 `errors.Is(perr, context.DeadlineExceeded || context.Canceled)` 加字符串兜底 `strings.Contains(err, "timeout"|"deadline")` 双层检测，分别映射 504/502。
- **`config.Warn(cfg) []string` 警告通道与 `Validate` 并列**：警告只写 stderr、不影响 exit code。serve / check 子命令在 Validate 通过后调用 Warn，每条以 `warn: ` 前缀输出。
- **Phase 4 commit 流水（11 个 commit）**：
  - Task 1: 85e01d3 (引入 expr + smoke) + a4ea93c (smoke 防御断言)
  - Task 2: 14e6878 (Matcher 实现) + 271cf6d (nil-safe doc + log 缺字段 err)
  - Task 3: b34b47f (selector 四种 strategy) + 56555a2 (分布测试 N=5090)
  - Task 4: 334f318 (RespondCases) + 1c79add (headers 合并测试 + 抑制 log 噪声)
  - Task 5: 5244fdf (router cases 分支 + Validate when 预检 + Warn) + efaf9a8 (错误前缀对齐 + 测试覆盖)
  - Task 6: a2222e9 (rewrite 编译) + 020bb3e (空 pattern 拒绝 + 文档分隔语义)
  - Task 7: 48fd323 (proxy 基础透传 + 502)
  - Task 8: 006ad08 (全量字段) + 71b318d (ip ctx + 严格 parseByteSize + 模板 err log)
  - Task 9: 793fbb8 (cli 装配 + E2E) + f5a454c (assembleRouter 注释)

**Phase 4 测试覆盖**：
- `internal/mock`: 91.3% 覆盖（≥ 80% DoD）
- `internal/proxy`: 85.7% 覆盖（≥ 80% DoD）
- `internal/config`: 既有覆盖维持
- 全包 `go test ./...` 全绿；新增 ~50 个测试用例（含 cases / selector / matcher / rewrite / proxy 全量字段 + E2E）

---

### Phase 5 — 运行时与可观测性

**详细计划**：[phase5-runtime.md](2026-05-19-fakeserver-v0.1-phase5-runtime.md)

**目标**：补上 v0.1 剩下的运行时能力——中间件全套、热加载、admin `/routes`、CORS、请求日志、保留路径校验。**Phase 5 完成 = v0.1 MVP 闭环**。

**范围 · 包含**：

- `internal/middleware/`：
  - `recoverer.go`：panic → 500 JSON + 栈打到 stderr
  - `logger.go`：彩色请求日志（time / method / path 模板 / status / duration / case 命中标记）
  - `cors.go`：默认开启；OPTIONS 在路由匹配**之后**才走 204 短路；`/__fakeserver/*` 路径前缀豁免 CORS
  - `bodylimit.go`：超 `server.maxBodySize` 返回 413
- `internal/config/watcher.go`：fsnotify 监听 + 300ms 防抖 + 原子 router 切换（保留旧表兜底，校验失败不切换）
- `internal/admin/handlers.go` 扩展：`/__fakeserver/routes` 返回当前路由 JSON 列表（mock + proxy 区分）
- 启动 banner（design §5.1 末尾样例）：版本 / 端口 / 路由计数 / fallback / env
- `serve` 子命令支持 `--quiet` / `--no-cors` / `--no-watch`

**范围 · 不包含**：

- env 文件（design §8，留 v0.2）
- 项目注册 `~/.config/fakeserver/projects.json`（留 v0.3）
- Web UI（留 v0.4）

**前置依赖**：Phase 4（中间件挂载点要在 mock/proxy 路由全部就位后；热加载要能 swap 包含 mock+proxy 的完整 router）。

**新增第三方依赖**：`github.com/fsnotify/fsnotify`。

**DoD**：

1. panic 被 recoverer 捕获 → 500 + JSON 错误体；server 不退出
2. CORS：默认 reflect Origin、自定义 `origins/methods/headers` 生效；`/__fakeserver/*` 无 CORS 头；OPTIONS 在用户路由命中后走用户 handler（不被中间件吃掉）
3. 编辑 config 文件 → 300ms 后 fsnotify 触发 → 重新加载校验通过 → router 原子切换 → 下个请求走新路由（在途请求走旧路由）；校验失败保留旧表 + stderr warn
4. `curl /__fakeserver/routes` 返回路由列表 JSON
5. `--quiet` 抑制请求日志；`--no-cors` 关闭 CORS 中间件；`--no-watch` 不启动 watcher
6. 启动 banner 含路由计数 + mock/proxy 分类
7. `go test ./...` 通过；`internal/middleware/*_test.go` 覆盖 cors 路径豁免、OPTIONS 顺序、recoverer 捕获
8. **v0.1 MVP 闭环 E2E**：写一个综合 config（含 mock 单响应 + cases + proxy + bodyFile），跑完整测试链路：启动 → 请求各种 route → 编辑 config 热加载 → 再次请求

**对 design 章节的映射**：§5.2 热加载、§5.3 请求日志、§5.4 CORS、§5.6 admin `/routes` + banner、§6 错误处理完整表、§7 测试策略 `internal/middleware/*_test.go` 与 cmd e2e 段。

**实际落地偏差**：

- **fsnotify v1.10.1 Windows 编辑器原子保存事件序列锁定**（Task 1 smoke）：编辑器"写 tmp → rename"操作在 Windows 触发 5 事件序列：`CREATE.tmp` → `WRITE.tmp` → `REMOVE` → `RENAME.tmp` → `CREATE`。300ms 防抖窗口足以聚合。
- **recoverer 增加 `stack` JSON 字段对齐 DoD #3**（Task 2 review-fix）：plan 任务代码原只输出 `{error, route, panic}`，DoD 明确要求 `{error, route, panic, stack}`。修复后 `debug.Stack()` 仅调用一次，stderr 日志与响应体使用同一份 stack。
- **logger 实现 `http.Flusher` / `http.Hijacker` 透传**（Task 3 review-fix）：原 `loggingResponseWriter` 仅嵌入 `http.ResponseWriter`，会遮蔽 Flusher 接口——影响 proxy 路由的 chunked 响应。修复后显式实现 Flush/Hijack 方法，条件转发到底层 writer。
- **CORS 默认行为完整化**（Task 4 review-fix）：CORSOpts 文档增加 reflect-mode + AllowCredentials 的 CSRF 警告；OPTIONS-handled-by-route 分支用 `Del+Add` 避免下游 Content-Type 重复；preflight 短路加 `Access-Control-Max-Age: 600`。
- **`readUpTo` off-by-one bug 修复**（Task 5 review-fix）：原实现在 body 恰好为 `max+1` 字节时返回 `(max+1, nil)`，绕过 BodyLimit 1 字节。同时影响 `internal/middleware/bodylimit.go` 与 `internal/proxy/proxy.go`，两处同步修复。
- **Holder 用 `atomic.Pointer[http.Handler]` 实现热替换**（Task 6）：泛型原子指针保证 Swap/ServeHTTP 并发安全；初始 nil 时返回 503 + JSON `{error: "server not ready"}`。
- **Watcher 订阅父目录而非单文件**（Task 7）：单文件 inotify 订阅在 rename-in-place 后失效；改为订阅 `filepath.Dir(path)`（去重）+ 事件回调里用 wanted map 过滤路径。
- **Watcher Stop 后 timer 仍可触发一次**（Task 7 review-doc）：`time.Timer.Stop()` 不等待已 expired 的 AfterFunc 回调；文档明确此 caveat，调用方需在 onChange 内部检查 stopped 标志。
- **admin.Mount 签名升级为 `(r, cfg)`**（Task 8）：让 `/__fakeserver/routes` handler 闭包绑定 cfg；Phase 1 healthz 测试改 `Mount(r, nil)`。
- **`cors: false` 配置生效**（Task 9 review-fix）：原 `corsOptsFromCfg` 在 `bool(false)` 时返回 reflect-mode（错误），修正为 `(opts, enabled)` 双返回；assembleHandler 据此决定是否添加 CORS 中间件。
- **`proxy.parseByteSize → ParseByteSize` 导出**（Task 9）：cli 包 `parseMaxBodySize` 复用，避免 5 行代码两处重复。
- **`Route.SourceFile` 始终为空**（Phase 2 存量 bug，Task 9 E2E review 发现）：`loader.go` 用 JSON round-trip 构造 Config，`json:"-"` 字段在 round-trip 中丢失。E2E 测试用绝对路径绕开。已记入 bd issue `lite-tools-gko`，留 v0.2 修复。
- **lite-tools-5an vet 警告清理**：Phase 4 留下的 8 处 `using resp before checking for errors` 警告全部修复，`go vet ./...` 零告警。

**Phase 5 测试覆盖**：
- `internal/middleware`: 88.5% 覆盖（≥ 80% DoD）
- `internal/config`: 测试覆盖维持
- 全包 `go test ./...` 全绿；v0.1 MVP 闭环 E2E（mock + cases + proxy + bodyFile + 热加载） PASS

**Phase 5 commit 流水（~18 个 commit，从 `2f784d3` 到 Task 10 收尾）**：
- Task 1: 2f784d3 (fsnotify smoke)
- Task 2: 661f566 (recoverer 初版) + b8b09a7 (加 stack 字段对齐 DoD)
- Task 3: c07678f (logger) + 0b6f7c5 (Flusher/Hijacker 透传)
- Task 4: 945739f (cors 初版) + 79af1e5 (CSRF 警告 + Max-Age)
- Task 5: d262c67 (bodylimit) + ece3ffc (readUpTo off-by-one 修复)
- Task 6: 6b42e0a (chain + holder)
- Task 7: 9497961 (watcher) + 6893a0b (Stop/armOrReset 文档)
- Task 8: 966c6c1 (admin /routes + banner)
- Task 9: 04e62c3 (serve 整合) + 4234df6 (cors:false + 测试 + polling)
- Task 10: eb10a80 (vet 清理) + 本次文档回写

**v0.1 MVP 完整闭环**：本 Phase 完成后，fakeserver v0.1 MVP 全部里程碑达成：
- ✅ 零配置 echo（Phase 1）
- ✅ JSON5 配置 + Validate + 子命令（Phase 2）
- ✅ 模板渲染 + 单一响应 mock + faker（Phase 3）
- ✅ 多响应 + 条件分支 + Proxy（Phase 4）
- ✅ 中间件全套 + 热加载 + admin /routes（Phase 5）

v0.2 路线图入口已就绪：env 文件 + osenv 完整 + 项目注册（design §8 / §10 / §11）；`Route.SourceFile` bug 修复（bd lite-tools-gko）。

---

## 4. 跨 Phase 追踪表

| design 章节 | Phase 1 | Phase 2 | Phase 3 | Phase 4 | Phase 5 |
|---|:-:|:-:|:-:|:-:|:-:|
| §3 配置 Schema | — | ✓ | — | — | — |
| §3.5 默认查找 | — | ✓ | — | — | — |
| §3.6 示例（拆目录） | — | ✓ | — | — | — |
| §3.7 集中校验 | — | ✓ | proxy 互斥 | — | — |
| §4 模板（含 .env 部分占位） | — | — | ✓ | — | — |
| §4.5 Content-Type 推断 | — | — | ✓ | — | — |
| §4.6 osenv 白名单 | — | — | ✓ | — | — |
| §5.1 启动流程 | 子集 | + config | + mock | + proxy | + middleware/watcher |
| §5.2 热加载 | — | — | — | — | ✓ |
| §5.3 日志 | — | — | — | — | ✓ |
| §5.4 CORS | — | — | — | — | ✓ |
| §5.5 默认 echo | ✓ | — | — | — | — |
| §5.6 admin 端点 | /healthz | — | — | — | + /routes |
| §5.7 CLI 子命令 | serve | init/check/routes | — | — | — |
| §6 错误处理 | — | config | mock | proxy/cases | middleware |
| §7 测试策略 | echo/admin/cli | + config | + tpl/mock | + proxy | + middleware/e2e |
| §9 Proxy | — | schema 字段 | — | ✓ | — |
| §12 Faker | — | — | ✓ | — | — |

> "schema 字段"指 Phase 2 把 Route 的 proxy 字段定义放入 schema 但不实现逻辑；"+" 指本 Phase 在前序基础上追加。

---

## 5. 与 design / phase plan 的关系

- **本文档（overview）**：v0.1 内部拆分的 source of truth；维护 Phase 边界与依赖关系
- **`fakeserver-design.md`**：所有 Phase 共享的设计契约；任何 Phase 落地发现 design 偏差时回写 §13 "已落地"段
- **`phase<N>-*.md`**：单个 Phase 的可执行 plan，由 `superpowers:writing-plans` 在 Phase 启动前展开（详细到每一步 2–5 分钟）

执行顺序（推荐）：

```
overview → phase1 → 执行 → 回写 design § 13 → overview 更新状态 → phase2 → ...
```

每个 Phase 完成后必须做的事：

1. 在本文档 §2 状态列写 "✅ 已完成 (commit <SHA range>)"
2. 在对应 phase 详述末尾写"实际落地偏差"（如有）
3. 如果偏差涉及未来 Phase 的设计假设，把它前置到对应 Phase 详述里
4. 回写 design 修订记录与 §13 "已落地"段

---

## 6. 风险与已知不确定项

| 不确定项 | 受影响 Phase | 风险等级 | 处置 |
|---|---|---|---|
| `easytpl` html 模式与 text 模式切换实现细节（design §4.4 双渲染器） | Phase 3 | 中 | Phase 3 Task 1 先做 spike 验证 |
| `expr-lang/expr` 与 Go 模板上下文如何共享变量（design §3.2 when 表达式） | Phase 4 | 中 | Phase 4 Task 1 探查 expr.Env / VM 配置 |
| `fsnotify` 在 Windows 下编辑器原子保存的事件抖动 | Phase 5 | 低 | 防抖 300ms 已设计；如不足再调 |
| `gofakeit.GetFuncs()` 是否真存在（design §12.2 B 段通用入口实现依赖） | Phase 3 | 低 | Phase 3 Task 实现 faker 时先探一次 |
| rux v2 OPTIONS 与中间件的执行顺序 | Phase 5 | 中 | Phase 5 Task 中验证；如顺序不可控考虑自定义 mux 包装 |

每个不确定项在对应 Phase plan 的"前置探测"步骤里**先 spike 再实现**，避免实现到一半发现底层假设错（这次 Phase 1 rux v2 API 探测就是案例）。

---

## v0.2 衍生事项（追溯，2026-05-20）

v0.1 完成时遗留的 backlog 项已在 v0.2 修复：

- ✅ `bd lite-tools-gko` 已关闭（v0.2 Phase 1）——`Route.SourceFile` 在 loader 内部 sentinel + zip 方案下正确填充，相对 `bodyFile` 路径在任意 CWD 下稳定。v0.2 Phase 3 又补了一个 `@include` 链中 bodyFile 的回归用例 (`TestLoad_BodyFile_FromIncludedFile_RelativePathResolved`)。

v0.2 整体进展：见 [`2026-05-20-fakeserver-v0.2-overview.md`](2026-05-20-fakeserver-v0.2-overview.md)。

