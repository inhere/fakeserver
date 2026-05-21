# Fakeserver v0.5 Development Experience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 面向前端日常联调场景，补齐 `init --full`、`check --strict`、`doctor`、运行期错误上下文与启动 banner，让 fakeserver 更容易初始化、更早发现配置问题、更容易定位错误。

**Architecture:** v0.5 不改变 mock/proxy 的核心响应模型，主要在 CLI 与诊断层补强。新增 `internal/cli/doctor.go` 与 `internal/config/strict.go`，扩展 `init.go/check.go/banner.go`；运行期错误上下文通过 mock responder 的统一错误出口增强，尽量复用 `config.Route.SourceFile`、现有 `Validate/Warn`、`tpl.Renderer` 和 registry/webui 能力。

**Tech Stack:** Go 标准库、现有 `gcli`、现有 `config/tpl/mock/cli` 包；不新增第三方依赖。

---

## 1. v0.5 范围

### 1.1 包含

- `fakeserver init --full`
  - 生成完整前端联调示例。
  - 包含 `fakeserver.json5`、`fakeserver.env.json5`、`.fakeserver/routes/*.json5`、`.fakeserver/fixtures/*`。
  - 支持 `--force` 覆盖已有文件。
- `fakeserver check --strict`
  - 在现有 schema 校验基础上预检查模板。
  - 输出 source/route/field/hint。
- `fakeserver doctor`
  - 环境与项目诊断。
  - 输出 OK/WARN/FAIL 与修复建议。
- 启动 banner 修正
  - 显示 UI URL。
  - 显示 env file 与 active env。
  - 保留 route 统计。
- 运行期错误上下文增强
  - mock 模板错误、bodyFile 错误等响应 JSON 增加 source/field/hint。
- 文档补充
  - 前端联调工作流文档。
  - v0.5 design 落地说明。

### 1.2 不包含

- Web UI history 详情抽屉。
- Copy as curl / Replay。
- Route tester。
- Scenario / case override。
- Resource CRUD。
- OpenAPI/curl/Postman import。
- SSE/WebSocket 协议 mock。
- 配置文件 Web 编辑器。

---

## 2. 文件结构

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `internal/cli/app.go` | 注册新 `doctor` 子命令 |
| 修改 | `internal/cli/init.go` | 支持 `--full` / `--force` |
| 修改 | `internal/cli/templates.go` | 保留 minimal 模板；新增 full 模板常量或引用 helper |
| 新建 | `internal/cli/full_templates.go` | 完整示例配置模板集中存放 |
| 修改 | `internal/cli/init_test.go` | 覆盖 `init --full`、不覆盖、`--force` |
| 修改 | `internal/cli/check.go` | 支持 `--strict` 并调用 strict checker |
| 新建 | `internal/config/strict.go` | strict check 结果结构、模板预编译、hint 生成 |
| 新建 | `internal/config/strict_test.go` | strict check 单测 |
| 新建 | `internal/cli/doctor.go` | doctor 命令与诊断输出 |
| 新建 | `internal/cli/doctor_test.go` | doctor 诊断覆盖 |
| 修改 | `internal/cli/banner.go` | UI URL、env active 显示 |
| 修改 | `internal/cli/banner_test.go` | banner 回归 |
| 修改 | `internal/mock/responder.go` | 运行期错误响应增加上下文与 hint |
| 修改 | `internal/mock/responder_test.go` | 错误响应上下文测试 |
| 修改 | `internal/mock/cases.go` | case route 错误字段传递，必要时增加 helper |
| 新建 | `docs/usage/frontend-workflow.md` | 前端日常联调用法 |
| 修改 | `docs/design/fakeserver-frontend-first-roadmap.md` | 保持 roadmap 不含 v0.5 具体拆分 |
| 修改 | `docs/fakeserver-design.md` | v0.5 完成后追加修订记录与严格 3 条事实 |

---

## 3. Task 1: `init --full` 完整示例初始化

**Files:**
- Modify: `internal/cli/init.go`
- Modify: `internal/cli/templates.go`
- Create: `internal/cli/full_templates.go`
- Modify: `internal/cli/init_test.go`

### 3.1 Step 1: 写 failing tests

新增测试：

- `TestRunInitFull_CreatesCompleteExample`
  - temp dir 执行 `runInit(initOptions{cwd: tmp, full: true})`
  - 断言生成：
    - `fakeserver.json5`
    - `fakeserver.env.json5`
    - `.fakeserver/routes/health.json5`
    - `.fakeserver/routes/users.json5`
    - `.fakeserver/routes/orders.json5`
    - `.fakeserver/routes/auth.json5`
    - `.fakeserver/routes/files.json5`
    - `.fakeserver/routes/proxy.json5`
    - `.fakeserver/fixtures/report.json`
    - `.fakeserver/fixtures/readme.txt`
  - 调用 `config.Load([]string{cfgPath}, "dev", nil)` + `config.Validate`，期望无错误。
- `TestRunInitFull_RefusesOverwriteByDefault`
  - 预先创建 `fakeserver.json5`
  - 执行 full init，期望返回 exit code 1 error。
- `TestRunInitFull_ForceOverwrites`
  - 预先创建旧配置
  - 执行 `force: true`
  - 断言配置被替换且可加载。
- `TestRunInit_MinimalStillWorks`
  - 锁定现有默认 init 行为不回归。

Run:

```bash
go test ./internal/cli -run 'TestRunInit' -count=1
```

Expected:

- 新测试因 `initOptions.full/force` 字段不存在或 full 文件未生成而失败。

### 3.2 Step 2: 扩展 initOptions 与 CLI flags

改造：

```go
type initOptions struct {
    cwd     string
    withEnv bool
    full    bool
    force   bool
}
```

`newInitCmd` 增加：

```go
cmd.BoolOpt2(&opts.full, "full", "Generate a complete frontend-friendly example project")
cmd.BoolOpt2(&opts.force, "force", "Overwrite existing generated config files")
```

行为：

- `--full` 隐含生成 env 文件，不需要再传 `--with-env`。
- 非 `--full` 保持当前 minimal 模板。
- 非 `--force` 时遇到任何目标文件已存在都拒绝。
- `--force` 时覆盖 full 目标文件；minimal 也允许覆盖 `fakeserver.json5` / env 文件。

### 3.3 Step 3: 新增 full 模板

建议在 `internal/cli/full_templates.go` 放：

```go
type generatedFile struct {
    Path string
    Body string
}

func fullInitFiles() []generatedFile
```

模板内容以最近本地验证过的完整示例为基础，但要避免运行期坑：

- header 中带 `-` 的 key 使用 `index`。
- cases 成功响应不依赖已被 matcher 消费的 `.request.body`。
- 所有 `bodyFile` 路径相对 include 文件目录有效。
- proxy 目标用 `https://httpbin.org`，doctor 可以 warn 网络不可达但 check 不失败。

### 3.4 Step 4: 实现文件写入 helper

新增 helper：

```go
func writeGeneratedFile(root string, file generatedFile, force bool) error
```

要求：

- 自动 `os.MkdirAll(filepath.Dir(target), 0o755)`。
- 非 force 且文件存在时返回 `errorx.Failf(1, ...)`。
- 写入权限 `0o644`。

### 3.5 Step 5: 验证并提交

Run:

```bash
go test ./internal/cli -run 'TestRunInit' -count=1
go test ./internal/config -run 'TestLoad|TestValidate' -count=1
```

Commit:

```bash
git add internal/cli/init.go internal/cli/templates.go internal/cli/full_templates.go internal/cli/init_test.go
git commit -m "feat(cli): add init --full example project"
```

---

## 4. Task 2: `check --strict` 模板预检查

**Files:**
- Modify: `internal/cli/check.go`
- Create: `internal/config/strict.go`
- Create: `internal/config/strict_test.go`
- Modify: `internal/cli/check_test.go`

### 4.1 Step 1: 写 failing tests

`internal/config/strict_test.go`：

- `TestStrictValidate_CatchesBodyTemplateSyntax`
  - route body 包含 `{{ .unclosed`
  - 期望返回问题，包含 `field=body`、method/path。
- `TestStrictValidate_CatchesHeaderTemplateSyntax`
  - header value 包含坏模板。
- `TestStrictValidate_HintsHeaderKeyWithDash`
  - body 包含 `{{ .request.headers.User-Agent }}`
  - 期望 hint 包含 `index .request.headers "User-Agent"`。
- `TestStrictValidate_ValidFullInitExample`
  - 使用 Task 1 full 模板生成到 temp dir 后加载，strict 无错误。

`internal/cli/check_test.go`：

- `TestRunCheck_StrictReportsTemplateProblems`
  - `runCheck(checkOptions{strict:true})`
  - 期望 exit code 1，输出包含 `strict` 问题。

Run:

```bash
go test ./internal/config -run TestStrictValidate -count=1
go test ./internal/cli -run TestRunCheck_Strict -count=1
```

Expected:

- build fail 或测试失败，因为 strict checker 还不存在。

### 4.2 Step 2: 定义 strict 问题结构

在 `internal/config/strict.go`：

```go
type StrictProblem struct {
    Source string
    RouteIndex int
    CaseIndex int
    Method string
    Path string
    Field string
    Message string
    Hint string
}

func (p StrictProblem) Error() string
```

格式建议：

```text
.fakeserver/routes/health.json5 routes[1] GET /api/meta body: template: ...
hint: use {{ index .request.headers "User-Agent" }}
```

### 4.3 Step 3: 实现 StrictValidate

函数：

```go
func StrictValidate(cfg *Config) []error
```

检查：

- route headers values。
- route body string leaves。
- case headers values。
- case body string leaves。

需要递归遍历 `body any` 中的 string leaf：

```go
func walkTemplateStrings(field string, node any, visit func(field string, src string))
```

字段命名示例：

- `body`
- `body.items[0].name`
- `headers.X-Trace-Id`
- `cases[1].body.message`

模板预编译：

```go
template.New("strict").Funcs(tpl.BaseFuncMap(cfg.Server.OSEnvWhitelist)).Parse(src)
```

注意：

- 仅 parse，不 execute，避免需要 request 上下文。
- 不要引入 mock 包，避免 config -> mock 依赖反向。
- 可从 `internal/tpl` 引入 `BaseFuncMap`，当前 config 已依赖 expr/json5/fsnotify 等，新增内部 tpl 依赖需确认无环：`tpl` 不依赖 config，因此可行。

### 4.4 Step 4: hint 生成

第一版只做高价值 hint：

- 如果模板源码匹配 `.request.headers.<token-with-dash>`：

```text
use {{ index .request.headers "User-Agent" }}
```

- 如果 error 包含 `function "xxx" not defined`：

```text
check function name or docs/fakeserver-design.md template functions
```

- 如果 error 包含 `unexpected EOF`：

```text
check closing braces: {{ ... }}
```

### 4.5 Step 5: 接入 `check --strict`

`checkOptions` 增加：

```go
strict bool
```

`newCheckCmd` 增加：

```go
cmd.BoolOpt2(&strict, "strict", "Also pre-parse route templates and report common runtime risks")
```

`runCheck`：

- 先 `config.Validate`。
- 再 `config.StrictValidate`。
- 合并输出。
- 成功时：

```text
OK: 11 routes loaded
OK: strict template checks passed
```

### 4.6 Step 6: 验证并提交

Run:

```bash
go test ./internal/config -run TestStrictValidate -count=1
go test ./internal/cli -run 'TestRunCheck' -count=1
go test ./... -count=1
```

Commit:

```bash
git add internal/config/strict.go internal/config/strict_test.go internal/cli/check.go internal/cli/check_test.go
git commit -m "feat(config): add strict template validation"
```

---

## 5. Task 3: `doctor` 环境与项目诊断

**Files:**
- Modify: `internal/cli/app.go`
- Create: `internal/cli/doctor.go`
- Create: `internal/cli/doctor_test.go`

### 5.1 Step 1: 写 failing tests

新增测试：

- `TestRunDoctor_ValidProject`
  - temp dir 写 full init 文件。
  - 执行 `runDoctor`。
  - 输出包含：
    - `OK   config`
    - `OK   env`
    - `OK   includes`
    - `OK   bodyFile`
    - `OK   webui`
- `TestRunDoctor_MissingConfigFails`
  - 空目录执行。
  - 期望 FAIL config。
- `TestRunDoctor_MissingBodyFile`
  - route 指向不存在 bodyFile。
  - 期望 FAIL bodyFile + fix suggestion。
- `TestRunDoctor_AdminExposureWarn`
  - host=0.0.0.0 + adminEnabled=true。
  - 期望 WARN admin。
- `TestRunDoctor_PortOccupiedWarnOrFail`
  - 用 `net.Listen("tcp", "127.0.0.1:0")` 占一个端口。
  - 配置同端口。
  - 期望 WARN/FAIL port。

Run:

```bash
go test ./internal/cli -run TestRunDoctor -count=1
```

Expected:

- build fail，因为 doctor 不存在。

### 5.2 Step 2: 定义 doctor 输出结构

建议：

```go
type doctorOptions struct {
    cwd string
    configFlag string
    envName string
    out io.Writer
}

type doctorFinding struct {
    Level string // OK | WARN | FAIL
    Area string
    Message string
    Fix string
}
```

输出格式：

```text
OK   config   fakeserver.json5
WARN admin    host=0.0.0.0 with adminEnabled=true
     fix: set server.host to 127.0.0.1 or adminEnabled=false
```

### 5.3 Step 3: 实现诊断项

第一版覆盖：

1. config
   - 显式 `-c` 或默认路径。
   - 缺失时 FAIL。
2. load/validate
   - `config.Load` + `config.Validate`。
   - 有错误 FAIL，并打印所有问题。
3. env
   - `cfg.EnvSource != ""` OK。
   - 没有 env file WARN，不失败。
4. includes
   - `len(cfg.SourcePaths)-1`。
   - 大于等于 0 OK。
5. bodyFile
   - 可依赖 `config.Validate` 已检查。
   - doctor 输出摘要。
6. port
   - 读取 `cfg.Server.Host/Port`。
   - 尝试 listen 后立即 close。
   - 占用时 WARN/FAIL。
7. admin exposure
   - `0.0.0.0 + adminEnabled=true` WARN。
8. webui
   - adminEnabled true: OK webui `/__fakeserver/ui/ enabled`。
   - false: WARN webui disabled。
9. proxy
   - URL schema 检查由 Validate 完成。
   - private/localhost target 用 `config.Warn` 输出 WARN。

### 5.4 Step 4: 注册 CLI

`app.go`：

```go
app.Add(newDoctorCmd())
```

`doctor` flags：

```bash
fakeserver doctor
fakeserver doctor -c fakeserver.json5
fakeserver doctor --env dev
```

### 5.5 Step 5: 验证并提交

Run:

```bash
go test ./internal/cli -run TestRunDoctor -count=1
go test ./internal/cli -run 'TestRunCheck|TestRunInit' -count=1
```

Commit:

```bash
git add internal/cli/app.go internal/cli/doctor.go internal/cli/doctor_test.go
git commit -m "feat(cli): add doctor diagnostics"
```

---

## 6. Task 4: 启动 banner 修正

**Files:**
- Modify: `internal/cli/banner.go`
- Modify: `internal/cli/banner_test.go`
- Modify: `internal/config/schema.go` only if active env metadata is missing and must be stored
- Modify: `internal/config/loader.go` only if active env metadata must be populated

### 6.1 Step 1: 写 failing tests

新增/修改测试：

- `TestPrintBanner_ShowsUIURL`
  - addr=`127.0.0.1:5090`
  - cfg admin enabled。
  - 期望包含 `ui: http://127.0.0.1:5090/__fakeserver/ui/`。
- `TestPrintBanner_HidesUIWhenAdminDisabled`
  - adminEnabled=false。
  - 期望 `ui: (disabled)`。
- `TestPrintBanner_ShowsEnvSourceAndActive`
  - cfg 包含 env source/active。
  - 期望 `env: dev (fakeserver.env.json5)`。

Run:

```bash
go test ./internal/cli -run TestPrintBanner -count=1
```

Expected:

- 当前 banner 不显示 UI/active env，因此失败。

### 6.2 Step 2: 补 active env 元数据

当前 `Config` 有：

```go
Env map[string]any
EnvSource string
```

如果没有 active env 字段，新增：

```go
EnvName string `json:"-"`
```

在 `loader.Load` 调 `LoadEnvFile` 后写入 active。

注意：

- 不改变 JSON API `/api/config` 输出，除非明确需要。
- `EnvName` 仅运行期 metadata。

### 6.3 Step 3: 修改 banner

输出顺序建议：

```text
╭─ fakeserver v0.5.0
│  listening on http://127.0.0.1:5090
│  ui:          http://127.0.0.1:5090/__fakeserver/ui/
│  config:      fakeserver.json5 (+6 includes)
│  env:         dev (fakeserver.env.json5)
│  routes:      7 mock, 3 cases, 1 proxy, fallback=echo
╰─
```

host 为 `0.0.0.0` 时 UI URL 可显示：

```text
http://127.0.0.1:<port>/__fakeserver/ui/
```

并保留 stderr warning。

### 6.4 Step 4: 验证并提交

Run:

```bash
go test ./internal/cli -run TestPrintBanner -count=1
go test ./internal/config -run 'TestLoad.*Env|TestLoad_Env' -count=1
```

Commit:

```bash
git add internal/cli/banner.go internal/cli/banner_test.go internal/config/schema.go internal/config/loader.go internal/config/loader_test.go
git commit -m "feat(cli): show ui and env in startup banner"
```

---

## 7. Task 5: 运行期错误上下文增强

**Files:**
- Modify: `internal/mock/responder.go`
- Modify: `internal/mock/responder_test.go`
- Modify: `internal/mock/cases.go`
- Modify: `internal/mock/cases_test.go` if case-specific field context is added

### 7.1 Step 1: 写 failing tests

新增测试：

- `TestRespond_TemplateErrorIncludesSourceFieldHint`
  - route SourceFile=`routes/health.json5`
  - body 包含 `{{ .request.headers.User-Agent }}`
  - 请求后返回 500 JSON。
  - 断言包含：
    - `error`
    - `detail`
    - `route`
    - `source`
    - `field`
    - `hint`
- `TestRespond_BodyFileErrorIncludesSource`
  - bodyFile 不存在。
  - 响应包含 source/field=`bodyFile`。
- `TestRespond_HeaderTemplateErrorIncludesField`
  - header 模板坏。
  - field=`headers.X-Trace-Id`。

Run:

```bash
go test ./internal/mock -run 'TestRespond_.*Error' -count=1
```

Expected:

- 当前错误响应没有 source/field/hint，因此失败。

### 7.2 Step 2: 定义错误上下文 helper

在 `responder.go` 内部新增：

```go
type responseErrorContext struct {
    Field string
    Hint string
}
```

或更简单：

```go
func writeErrorWithContext(w http.ResponseWriter, status int, short, detail string, route *config.Route, field string)
```

兼容现有 `writeError`：

```go
func writeError(...) {
    writeErrorWithContext(..., "")
}
```

JSON 字段：

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

空字段不输出或输出空字符串均可；推荐 `omitempty`。

### 7.3 Step 3: 复用 hint 逻辑

避免在 mock 包重复复杂逻辑。可以在 config 或 tpl 包提供小 helper：

```go
func TemplateHint(srcOrErr string) string
```

如果 Task 2 已在 `config/strict.go` 内实现 hint，可考虑移动到 `internal/tpl/hints.go`，避免 mock -> config 依赖。

建议依赖方向：

```text
config -> tpl
mock   -> tpl
tpl    -> stdlib only
```

### 7.4 Step 4: 标注字段位置

改造点：

- header render error:
  - field=`headers.<key>`
- body template error:
  - field=`body`
- bodyFile error:
  - field=`bodyFile`
- json marshal error:
  - field=`body`
- no case matched:
  - field=`cases`

### 7.5 Step 5: 验证并提交

Run:

```bash
go test ./internal/mock -run 'TestRespond|TestCases' -count=1
go test ./internal/config -run TestStrictValidate -count=1
go test ./... -count=1
```

Commit:

```bash
git add internal/mock/responder.go internal/mock/responder_test.go internal/mock/cases.go internal/mock/cases_test.go internal/tpl/hints.go internal/tpl/hints_test.go internal/config/strict.go
git commit -m "feat(mock): include source field and hints in runtime errors"
```

---

## 8. Task 6: 前端联调工作流文档

**Files:**
- Create: `docs/usage/frontend-workflow.md`
- Modify: `docs/fakeserver-design.md`
- Modify: `docs/design/fakeserver-frontend-first-roadmap.md` only if final wording needs adjustment

### 8.1 Step 1: 新增 usage 文档

内容结构：

```markdown
# Frontend Workflow with fakeserver

## 1. 初始化
fakeserver init --full

## 2. 检查
fakeserver doctor
fakeserver check --strict -c fakeserver.json5

## 3. 启动
fakeserver serve -c fakeserver.json5 --env dev

## 4. 常用调试
- Web UI
- 请求历史
- 修改配置热加载
- cases 异常态
- proxy 联调真实后端

## 5. 常见错误
- header key with dash
- bodyFile relative path
- admin exposed on 0.0.0.0
- proxy private target warning
```

### 8.2 Step 2: 回写 design

v0.5 完成后追加：

- 修订记录 `v0.5-devex-applied`
- §13 或新增 v0.5 落地确认，严格 3 条事实。

注意：当前工作区可能存在用户未提交的 `docs/fakeserver-design.md` 变更。实现时必须先检查 diff，不要覆盖用户改动。

### 8.3 Step 3: 验证并提交

Run:

```bash
go test ./... -count=1
go vet ./...
go build ./...
```

Commit:

```bash
git add docs/usage/frontend-workflow.md docs/fakeserver-design.md docs/design/fakeserver-frontend-first-roadmap.md
git commit -m "docs: add frontend workflow for v0.5"
```

---

## 9. Final Quality Gates

在宣布 v0.5 完成前必须运行：

```bash
go test ./... -count=1
go vet ./...
go build ./...
go run ./cmd/fakeserver init --full
go run ./cmd/fakeserver check --strict -c fakeserver.json5
go run ./cmd/fakeserver doctor -c fakeserver.json5 --env dev
```

针对 `init --full` 的最后三个命令建议在 temp dir 或测试目录执行，避免覆盖本地配置。

覆盖率建议：

```bash
go test -cover ./internal/cli ./internal/config ./internal/mock
```

最低期望：

- `internal/cli` 不下降超过 2 个百分点。
- `internal/config` 仍 >= 85%。
- `internal/mock` 仍 >= 80%。

---

## 10. Beads / Git 收尾

每个 Task 完成后：

```bash
git status --short
git add <task files>
git commit -m "<task commit message>"
```

v0.5 全部完成后：

```bash
bd close <v0.5 issue id> --reason="fakeserver v0.5 development experience completed"
git status --short --branch
```

如果仓库没有 remote，最终说明无法 push；如果配置了 remote，按项目规则执行：

```bash
git pull --rebase
git push
git status
```

---

## 11. Acceptance Checklist

- [x] `fakeserver init --full` 生成完整示例并可 check。
- [x] `fakeserver init --full --force` 可覆盖生成文件。
- [x] `fakeserver check --strict` 能发现 body/header 模板语法错误。
- [x] strict check 对 header key dash 提供 `index` hint。
- [x] `fakeserver doctor` 覆盖 config/env/include/bodyFile/port/proxy/admin/UI。
- [x] banner 显示 UI URL、active env、env file。
- [x] mock 运行期错误 JSON 包含 source/field/hint。
- [x] 新增 `docs/usage/frontend-workflow.md`。
- [x] `go test ./... -count=1` 通过。
- [x] `go vet ./...` 通过。
- [x] `go build ./...` 通过。
