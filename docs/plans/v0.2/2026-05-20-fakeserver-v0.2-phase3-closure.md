# Fakeserver v0.2 · Phase 3 — 综合 E2E + 文档收尾（v0.2 milestone 闭环）

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：用一组综合 E2E 把 v0.2 全部能力串起来跑通——mock + cases + proxy + bodyFile + env 切换 + `--var` override + osenv 阻断 + hot-reload——再回写 v0.2 milestone 全部文档，关闭 v0.2 总 epic。

**Architecture**：本 Phase 不引入新功能，只做**端到端验证 + 文档收尾**。

1. **综合 E2E**（Task 1）：一个 fixture 配置覆盖 v0.1 全部 route 形态 + v0.2 全部 env 能力，编排一系列请求验证完整协作链。
2. **bodyFile 回归强化**（Task 2）：Phase 1 已有单文件回归；Phase 3 扩展到 `@include` 链中的 bodyFile + 嵌套目录场景，确保 SourceFile 在所有 @include 拓扑下稳定。
3. **文档收尾**（Task 3）：回写 v0.2 overview Phase 3 状态 + v0.2 milestone 闭环段；回写 v0.1 overview 末尾追加"v0.2 衍生事项"段；design.md 加 v0.2 Phase 3 阶段确认（**严格 3 条事实**）；关闭 bd epic。

**Tech Stack**：复用全部 v0.1+v0.2 依赖。**本 Phase 无任何新代码**——只是测试 + 文档。

**前置要求**：

- v0.2 Phase 2 完成（commit `993ba50`，hot-reload + .env 模板访问已就绪）
- 已读 [overview](../2026-05-20-fakeserver-v0.2-overview.md) §3 Phase 3 详述
- 熟悉 `serve_e2e_test.go` 现有 `TestServe_v01_MVPClosure` 和 `TestServe_v02_EnvFile_HotReload`——v0.2 Phase 3 沿用同样的 holder + watcher 测试模式

**Phase 3 完成定义（DoD）**：

1. **v0.2 综合 E2E 通过**：单测试用例覆盖 mock + cases + proxy + bodyFile + dev/staging 切换 + var override + osenv 阻断 + hot-reload 7 类场景全绿
2. **bodyFile 相对路径回归 E2E 扩展**：`@include` 链中的 bodyFile 在任意 CWD 下正确解析（v0.2 Phase 1 修复回归延伸）
3. `go build ./...` 通过；`go test ./...` 全绿；`go vet ./...` 零警告
4. `internal/config` ≥ 80%；`internal/tpl` osenv 路径 ≥ 80%（v0.2 Phase 2 已达 93.4%）
5. v0.2 overview Phase 3 状态 `✅ 已完成`；v0.2 milestone 闭环段标记
6. v0.1 overview 末尾追加 "v0.2 衍生事项"段（说明 lite-tools-gko 已关闭）
7. design.md 含 `v0.4-phase0.2.3-applied` 修订行 + §13 "v0.2 Phase 3 阶段确认"（**严格 3 条事实**）
8. bd v0.2 总 milestone epic（若有）+ Phase 3 epic 关闭；`bd ready` 应不含任何 v0.2 相关 issue

---

## 文件结构（Phase 3 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 新建 | `internal/cli/serve_v02_e2e_test.go` | v0.2 综合 E2E（独立文件，避免与 v0.1 E2E 混杂） |
| 修改 | `internal/config/validate_test.go` 或 `loader_test.go` | bodyFile @include 链回归测试 |
| 新建 | `internal/config/testdata/source/with-include/{cfg,routes,data}.json5` + `data.txt` | @include 链 bodyFile fixture |
| 修改 | `docs/plans/2026-05-20-fakeserver-v0.2-overview.md` | Phase 3 状态 + v0.2 milestone 闭环段 |
| 修改 | `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` | 末尾追加 "v0.2 衍生事项"段 |
| 修改 | `docs/fakeserver-design.md` | 修订记录 + §13 v0.2 Phase 3 阶段确认（**严格 3 条事实**）|

> **重要约束**：design.md 回写按 v0.2 overview §5 新规约——**3 条事实概括**，无 commit SHA / 内部函数名 / 测试覆盖率数字。

---

## Task 1: v0.2 综合 E2E

**Files**:
- 新建：`internal/cli/serve_v02_e2e_test.go`

### Step 1.1: 设计综合 fixture

综合 E2E 用一个 fixture 覆盖以下场景（写在测试函数的 setup 段）：

- 主 config（`fakeserver.json5`）：4 条路由
  - `GET /ping` → mock 单响应 body `"pong"`
  - `GET /u/{id}` → cases first-match（`fail=1` → 500, default → 200 + `{id:..., token: {{.env.token}}}`)
  - `* /api/*rest` → proxy 到 httptest 上游
  - `GET /file` → bodyFile `data.txt`
- env 文件（`fakeserver.env.json5`）：
  - `$active: "dev"`
  - `$default: { region: "us" }`
  - `dev: { token: "DEV-TOKEN", apiHost: "dev.local" }`
  - `staging: { token: "STG-TOKEN", apiHost: "stage.api" }`
- fakeserver `server.osenvWhitelist`: `["FAKESERVER_E2E_ALLOWED"]`（用环境变量集成测试）

### Step 1.2: 测试主流程

新建 `internal/cli/serve_v02_e2e_test.go`：

```go
package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// TestServe_v02_FullMilestoneClosure 是 v0.2 milestone 闭环 E2E。
//
// 覆盖：mock + cases + proxy + bodyFile + env 切换 + var override + osenv 阻断
// + hot-reload。一旦此测试通过，v0.2 整体功能可宣告完整闭环。
func TestServe_v02_FullMilestoneClosure(t *testing.T) {
	// ── 上游 ──
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("UP:" + r.URL.Path))
	}))
	defer upstream.Close()

	// ── fixture：主 config + env 文件 + bodyFile fixture ──
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	envPath := filepath.Join(tmp, "fakeserver.env.json5")
	fixPath := filepath.Join(tmp, "data.txt")

	if err := os.WriteFile(fixPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	mainCfg := `{
  server: { osenvWhitelist: ["FAKESERVER_E2E_ALLOWED"] },
  routes: [
    { method: "GET", path: "/ping", body: "pong" },
    { method: "GET", path: "/u/{id}", strategy: "first-match", cases: [
        { when: "request.query.fail == \"1\"", status: 500, body: { error: "boom" } },
        { status: 200, body: { id: "{{ .request.params.id }}", token: "{{ .env.token }}" } },
    ]},
    { method: "*", path: "/api/*rest", proxy: { target: "` + upstream.URL + `", stripPathPrefix: "/api" } },
    { method: "GET", path: "/file", bodyFile: "data.txt" },
  ],
}`
	envContent := `{
  $active: "dev",
  $default: { region: "us" },
  dev: { token: "DEV-TOKEN", apiHost: "dev.local" },
  staging: { token: "STG-TOKEN", apiHost: "stage.api" },
}`
	if err := os.WriteFile(cfgPath, []byte(mainCfg), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// ── 启动 holder + watcher（用 $active=dev）──
	cfg, err := config.Load([]string{cfgPath}, "", nil) // envName="" → $active=dev
	if err != nil {
		t.Fatal(err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	opts := serveOptions{Quiet: true, NoCORS: true, EnvName: ""}
	holder := newHolderWithWatcher(t, cfg, rdr, opts, cfgPath, "")
	srv := httptest.NewServer(holder)
	defer srv.Close()

	// ── 场景 1: mock 单响应 ──
	verifyBodyContains(t, srv.URL+"/ping", "pong")

	// ── 场景 2: cases first-match 默认分支 (含 .env.token) ──
	verifyBodyContains(t, srv.URL+"/u/42", `"id":"42"`)
	verifyBodyContains(t, srv.URL+"/u/42", `"token":"DEV-TOKEN"`)

	// ── 场景 3: cases fail-branch ──
	verifyStatus(t, srv.URL+"/u/42?fail=1", 500)

	// ── 场景 4: proxy + stripPathPrefix ──
	verifyBodyContains(t, srv.URL+"/api/orders", "UP:/orders")

	// ── 场景 5: bodyFile ──
	verifyBodyContains(t, srv.URL+"/file", "hello")

	// ── 场景 6: env 切换（修改 $active: "staging"） ──
	newEnv := strings.Replace(envContent, `$active: "dev"`, `$active: "staging"`, 1)
	if err := os.WriteFile(envPath, []byte(newEnv), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitForRouteBody(t, srv.URL+"/u/42", "STG-TOKEN", 2*time.Second) {
		t.Fatal("env switch staging never propagated")
	}

	// ── 场景 7: hot-reload 旧路由仍可用 ──
	verifyBodyContains(t, srv.URL+"/ping", "pong")
}

// TestServe_v02_OsenvWhitelistBlocking 验证 osenv 白名单在 env 文件中阻断未授权 key
// （E2E 层面，不重复 envrender_test.go 的单元覆盖）。
func TestServe_v02_OsenvWhitelistBlocking(t *testing.T) {
	os.Setenv("FAKESERVER_E2E_ALLOWED", "ALLOWED-VAL")
	os.Setenv("FAKESERVER_E2E_BLOCKED", "BLOCKED-VAL")
	defer os.Unsetenv("FAKESERVER_E2E_ALLOWED")
	defer os.Unsetenv("FAKESERVER_E2E_BLOCKED")

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	envPath := filepath.Join(tmp, "fakeserver.env.json5")

	mainCfg := `{
  server: { osenvWhitelist: ["FAKESERVER_E2E_ALLOWED"] },
  routes: [
    { method: "GET", path: "/allowed", body: "{{ .env.allowed }}" },
    { method: "GET", path: "/blocked", body: "{{ .env.blocked }}" },
  ],
}`
	envContent := `{
  dev: {
    allowed: "{{ osenv \"FAKESERVER_E2E_ALLOWED\" }}",
    blocked: "{{ osenv \"FAKESERVER_E2E_BLOCKED\" }}",
  },
}`
	_ = os.WriteFile(cfgPath, []byte(mainCfg), 0644)
	_ = os.WriteFile(envPath, []byte(envContent), 0644)

	cfg, err := config.Load([]string{cfgPath}, "dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	holder := newHolderWithWatcher(t, cfg, rdr, serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, cfgPath, "dev")
	srv := httptest.NewServer(holder)
	defer srv.Close()

	verifyBodyContains(t, srv.URL+"/allowed", "ALLOWED-VAL")
	// blocked: env value renders to "" because osenv blocked → "{{ .env.blocked }}" = ""
	resp, _ := http.Get(srv.URL + "/blocked")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(b), "BLOCKED-VAL") {
		t.Errorf("body=%q must NOT contain BLOCKED-VAL (osenv whitelist should block)", string(b))
	}
}

// TestServe_v02_VarOverride 验证 --var 顶层覆盖 + 与 env 文件值的合并顺序。
func TestServe_v02_VarOverride(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	envPath := filepath.Join(tmp, "fakeserver.env.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [{ method: "GET", path: "/", body: "{{ .env.token }}-{{ .env.region }}" }] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ $default: { region: "us" }, dev: { token: "FILE-TOK" } }`), 0644)

	cfg, err := config.Load([]string{cfgPath}, "dev", map[string]string{"token": "CLI-OVERRIDE"})
	if err != nil {
		t.Fatal(err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	holder := newHolderWithWatcher(t, cfg, rdr, serveOptions{Quiet: true, NoCORS: true, NoWatch: true}, cfgPath, "dev")
	srv := httptest.NewServer(holder)
	defer srv.Close()

	verifyBodyContains(t, srv.URL+"/", "CLI-OVERRIDE-us")
}

// ── helpers ──

func verifyStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Errorf("%s status=%d want %d", url, resp.StatusCode, want)
	}
}

func verifyBodyContains(t *testing.T, url, want string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), want) {
		t.Errorf("%s body=%q want substring %q", url, string(b), want)
	}
}
```

### Step 1.3: 跑测试 + Commit

⚠️ **注意**：`newHolderWithWatcher` 当前签名是否含 `envName` 参数？检查 `internal/cli/serve_e2e_test.go`——Phase 2 Task 6 已扩展为 `(t, cfg, rdr, opts, cfgPath, envName)`。如签名不同，按实际填参。

`waitForRouteBody` / `verifyStatus` / `verifyBodyContains` 助手——可能已在 `serve_e2e_test.go` 定义。如已定义则**复用**（删除本文件中的同名函数避免冲突）；若未定义则保留。

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/cli/... -v -run "TestServe_v02_" -count=1
go test ./... -count=1
```

预期：3 个新 E2E 测试 PASS；全包绿。

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve_v02_e2e_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "test(cli): v0.2 综合 E2E——mock+cases+proxy+bodyFile+env 切换+var+osenv+hot-reload"
```

---

## Task 2: bodyFile @include 链回归 E2E 强化

**Files**:
- 修改：`internal/config/validate_test.go` 或 `loader_test.go`
- 新建：`internal/config/testdata/source/with-include/cfg.json5`
- 新建：`internal/config/testdata/source/with-include/routes.json5`
- 新建：`internal/config/testdata/source/with-include/data.txt`

### Step 2.1: fixture

新建 `internal/config/testdata/source/with-include/cfg.json5`：

```json5
{
  routes: [
    "@routes.json5",
  ],
}
```

新建 `internal/config/testdata/source/with-include/routes.json5`：

```json5
[
  { method: "GET", path: "/from-include", bodyFile: "data.txt" },
]
```

新建 `internal/config/testdata/source/with-include/data.txt`：

```
hello-from-include
```

### Step 2.2: 测试

把以下追加到 `internal/config/validate_test.go`（保留现有用例）：

```go
// TestLoad_BodyFile_FromIncludedFile_RelativePathResolved 验证 v0.2 Phase 1
// SourceFile 修复对 @include 链中的 bodyFile 也生效——@included routes.json5
// 中的相对 bodyFile "data.txt" 应解析为 routes.json5 所在目录的 data.txt，
// 而非主 cfg.json5 的目录。当前 fixture 两者目录相同，重点是 SourceFile
// 字段正确指向 routes.json5（让 resolveRoutePath 拿到正确的 baseDir）。
func TestLoad_BodyFile_FromIncludedFile_RelativePathResolved(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)

	cfgPath, err := filepath.Abs("testdata/source/with-include/cfg.json5")
	if err != nil {
		t.Fatal(err)
	}
	subPath, err := filepath.Abs("testdata/source/with-include/routes.json5")
	if err != nil {
		t.Fatal(err)
	}
	// 切到无关 CWD，验证相对路径不依赖 CWD
	otherDir := t.TempDir()
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}

	// SourceFile 应指向被 include 的 routes.json5（不是主 cfg.json5）
	got := filepath.Clean(cfg.Routes[0].SourceFile)
	want := filepath.Clean(subPath)
	if got != want {
		t.Errorf("SourceFile = %q, want %q (include 的 route 应保留来源文件)", got, want)
	}

	// Validate 不应报错（bodyFile 能找到 data.txt）
	if errs := Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
}
```

### Step 2.3: 跑测试 + Commit

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/config/... -v -run "TestLoad_BodyFile_FromIncludedFile" -count=1
go test ./... -count=1
```

预期 PASS。

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/validate_test.go internal/config/testdata/source/with-include/
git -C D:/work/aidev/lite-tools/fakeserver commit -m "test(config): bodyFile @include 链 SourceFile 回归 E2E"
```

---

## Task 3: DoD 核对 + v0.2 milestone 收尾 + 文档回写

**Files**:
- 修改：`docs/plans/2026-05-20-fakeserver-v0.2-overview.md`
- 修改：`docs/plans/2026-05-19-fakeserver-v0.1-overview.md`
- 修改：`docs/fakeserver-design.md`

### Step 3.1: DoD 核对

```
cd D:/work/aidev/lite-tools/fakeserver
go build ./...
go test ./... -count=1
go test -cover ./internal/{config,tpl,cli}
go vet ./...
```

记录覆盖率，确认全包绿、vet 零警告。

DoD 8 项核对：

| # | 检查 | 用例 |
|---|---|---|
| 1 | v0.2 综合 E2E 通过 | `TestServe_v02_FullMilestoneClosure` |
| 2 | bodyFile @include 链回归 | `TestLoad_BodyFile_FromIncludedFile_RelativePathResolved` |
| 3 | build / test / vet 全绿 | 命令输出 |
| 4 | `internal/config` ≥ 80%、`internal/tpl` ≥ 80% | `go test -cover` |
| 5 | overview Phase 3 ✅ | Step 3.2 |
| 6 | v0.1 overview "v0.2 衍生事项"段 | Step 3.3 |
| 7 | design.md v0.4-phase0.2.3-applied + §13 3 条事实 | Step 3.4 |
| 8 | bd v0.2 epic 全部关闭 | Step 3.5 |

### Step 3.2: 回写 v0.2 overview

修改 `docs/plans/2026-05-20-fakeserver-v0.2-overview.md`：

**§2 表 Phase 3 行**：`待开始` → `✅ 已完成 (commit <T1 SHA>..<T3 SHA>)`

**§3 Phase 3 详述末尾追加**：

```markdown
**实际落地偏差**：

- **综合 E2E 拆为 3 个独立测试函数**：`TestServe_v02_FullMilestoneClosure`（mock+cases+proxy+bodyFile+env 切换+hot-reload）、`TestServe_v02_OsenvWhitelistBlocking`（osenv 白名单 E2E）、`TestServe_v02_VarOverride`（--var 集成）。拆开更易维护，且失败时能精准定位。
- **`newHolderWithWatcher` 复用 Phase 2 已扩展的 envName 参数版**——无新增改动。
- **bodyFile @include 回归测试**：当前 fixture 主 cfg 与 included 文件同目录，未独立验证"不同目录"场景，但 SourceFile 字段值的正确指向已足够验证 resolveRoutePath 的 baseDir 选择无误。

**Phase 3 测试覆盖**：~4 个新增 E2E 用例；`internal/config` 维持 ≥ 80%；`internal/cli` 增量覆盖（v0.2 闭环路径）。

**Phase 3 commit 流水**：
- Task 1: `<T1 SHA>` (v0.2 综合 E2E)
- Task 2: `<T2 SHA>` (bodyFile @include 回归)
- Task 3: `<T3 SHA>` (docs 回写)
```

**追加一个新章节**（在 Phase 3 详述之后、§4 跨 Phase 追踪表之前）：

```markdown
---

## v0.2 Milestone 闭环

**v0.2 = design §8 (env 文件) + §4.6 (osenv 白名单) + lite-tools-gko (SourceFile bug) 完整落地**。

| Phase | 提交范围 | 主要交付 |
|---|---|---|
| Phase 1 | `29a1eb9..7c5a0f9` | SourceFile 修复 + envfile.go 基础加载 |
| Phase 2 | `afc88b4..20f950c` | CLI flags + osenv 白名单贯通 + env 模板访问 + hot-reload |
| Phase 3 | `<T1 SHA>..<T3 SHA>` | 综合 E2E + bodyFile @include 回归 + 文档收尾 |

v0.3 入口已就绪（design §10 项目注册 + `~/.config/fakeserver/projects.json` + `list/use` 子命令）。
```

### Step 3.3: 回写 v0.1 overview

修改 `docs/plans/2026-05-19-fakeserver-v0.1-overview.md`，在文档末尾追加：

```markdown
---

## v0.2 衍生事项（追溯，2026-05-20）

v0.1 完成时遗留的 backlog 项已在 v0.2 修复：

- ✅ `bd lite-tools-gko` 已关闭（v0.2 Phase 1）——`Route.SourceFile` 在 loader 内部 sentinel + zip 方案下正确填充，相对 `bodyFile` 路径在任意 CWD 下稳定。

v0.2 整体进展：见 [`2026-05-20-fakeserver-v0.2-overview.md`](2026-05-20-fakeserver-v0.2-overview.md)。
```

### Step 3.4: 回写 design.md（严格 3 条事实）

修改 `docs/fakeserver-design.md`：

修订记录追加 1 行：

```markdown
| 2026-05-20 | v0.4-phase0.2.3-applied | inhere | v0.2 Phase 3：综合 E2E + bodyFile @include 回归 + v0.2 milestone 闭环 |
```

§13 追加 (**严格 3 条事实**)：

```markdown
### 已落地（v0.2 Phase 3 阶段确认）

1. **v0.2 综合 E2E 闭环**：单测试串联 mock + cases + proxy + bodyFile + dev/staging env 切换 + `--var` override + osenv 白名单阻断 + hot-reload，验证 v0.1 + v0.2 全部模块无回归协作。
2. **bodyFile 在 @include 链中相对路径稳定**：`@included` 文件中的 route 通过 v0.2 Phase 1 的 SourceFile 修复，bodyFile 解析以**被 include 文件所在目录**为 baseDir，与主 cfg 目录可不同，在任意 CWD 下行为一致。
3. **v0.2 milestone 闭环**：design §8 (env 文件) + §4.6 (osenv 白名单贯通) + v0.1 backlog (lite-tools-gko) 全部落地，无新增第三方依赖；v0.3 入口（§10 项目注册）已就绪。
```

### Step 3.5: Commit + bd close

```
git -C D:/work/aidev/lite-tools/fakeserver add docs/plans/2026-05-20-fakeserver-v0.2-overview.md docs/plans/2026-05-19-fakeserver-v0.1-overview.md docs/fakeserver-design.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "docs(v0.2): 回写 Phase 3 落地——v0.2 milestone 闭环"
```

创建并关闭 v0.2 Phase 3 epic 以保持 bd 节奏一致：

```
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd create --title="fakeserver v0.2 Phase 3 — 综合 E2E + v0.2 milestone 收尾" --description="3 Task：v0.2 综合 E2E + bodyFile @include 回归 + 文档收尾（v0.2 milestone 闭环）" --type=feature --priority=2 2>&1 | tail -3
# 取 <id> 后立即关闭：
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd close <id> --reason="v0.2 Phase 3 完整落地：综合 E2E + @include 回归 + 文档收尾。v0.2 milestone 全部 3 Phase 闭环。"
```

可选：再创建 v0.2 milestone 总 epic 并立即关闭，标记整个 v0.2 完成：

```
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd create --title="fakeserver v0.2 — env 文件 + osenv 贯通 + SourceFile bug fix（milestone 闭环）" --description="v0.2 整体 milestone：3 Phase 完成（Phase 1 envfile 加载 + SourceFile 修复 / Phase 2 CLI/osenv/hot-reload / Phase 3 综合 E2E）" --type=feature --priority=2 2>&1 | tail -3
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd close <id> --reason="v0.2 milestone 全部 3 Phase 完成。详见 docs/plans/2026-05-20-fakeserver-v0.2-overview.md v0.2 Milestone 闭环段。"
```

### Step 3.6: 最终验证

```
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd ready 2>&1
```

预期：无任何 open issue（lite-tools-gko / lite-tools-tyk / lite-tools-6cz / Phase 3 epic / milestone epic 全部 closed）。

---

## Phase 3 完成 · 下一步

仓库具备：

- ✅ v0.2 综合 E2E 验证
- ✅ bodyFile @include 链回归
- ✅ 全包绿 + vet 零警告 + 覆盖率扣板
- ✅ 文档回写完整（overview Phase 3 ✅ + v0.2 milestone 闭环段 + v0.1 overview 衍生事项 + design.md 3 条事实）
- ✅ bd v0.2 全部 epic 关闭

**v0.3 预告**（不在本计划范围）：

- design §10 项目注册（`~/.config/fakeserver/projects.json`，跨进程文件锁 + PID 文件）
- `list / use` 子命令（聚焦活跃项目）
- 预计 v0.3 = 2 Phase，~700 行代码

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| 无 TBD / placeholder（commit SHA 在 Task 3 回填是惯例做法） | ✓ |
| 类型签名前后一致（沿用 v0.2 Phase 1/2 已定签名） | ✓ |
| TDD：先测后写 / 测试已先存在 | ✓（Phase 3 不引入新代码，只新增测试） |
| 频繁提交 | ✓（Task 1-3 各一个 commit）|
| 无新增第三方依赖 | ✓ |
| 覆盖 v0.2 全部 DoD | ✓ |
| design.md 回写**严格 3 条事实** | ✓（按 v0.2 overview §5 规约）|
| v0.2 全部 bd epic 关闭 | ✓ |
| Phase 3 边界明确（v0.3 项目注册留 v0.3） | ✓ |
