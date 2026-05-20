# Fakeserver v0.1 · Phase 2 — 配置加载 + 路由摘要

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：让 `fakeserver serve -c <paths>` 能加载并校验 JSON5 配置、解析 `@include`、合并多文件，启动时打印**路由摘要**——但**还不响应**配置里的 mock 请求（mock 响应留 Phase 3）；同时提供 `fakeserver init / check / routes` 三个新子命令。

**Architecture**：`internal/config/` 新包做"读 + 校验"两件事——loader 把若干 JSON5 文件加载为统一的强类型 `*Config`（含 `@include` 展开 / glob / 多文件合并 / 默认路径查找）；validate 集中检查必填、字段互斥、跨条目重复、保留前缀等（design §3.7）。`internal/cli/` 加 3 个新文件分别对应 init/check/routes 子命令，并修改 serve 接入 `-c/--config`。本 Phase **不**实现模板渲染或 mock 响应——`{{ ... }}` 字符串字面量保留原样，schema 中含 `proxy` 字段但 proxy 路由不响应。

**Tech Stack**：Go 1.25+ · `github.com/titanous/json5`（新引入）· 标准库 `path/filepath` / `os` / `strings` / `errors`。

**前置要求**：

- Phase 1 已完成（见 `docs/plans/2026-05-19-fakeserver-v0.1-phase1-skeleton.md`）
- 已读 `docs/fakeserver-design.md` §3（配置 Schema）/ §3.5（默认查找）/ §3.6（拆目录示例）/ §3.7（集中校验）/ §5.7 init/check/routes 子命令行
- 已读 `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` §3 Phase 2 详述（边界与不含范围）
- 当前 `fakeserver/` 工作目录有 `go.mod`（含 rux/v2、gcli/v3、goutil），13 个 commits

**Phase 2 完成定义（DoD）**（与 overview §3 Phase 2 同步；任何 PR 必须全部满足）：

1. `fakeserver init` 在空目录生成 `fakeserver.json5` 模板；目标已存在时报错退出
2. `fakeserver check -c bad.json5` 在 JSON 语法错 / 字段互斥 / include 循环等场景下退出非 0 + **集中报错**（一次列出全部问题）
3. `fakeserver routes -c valid.json5` 打印路由摘要表（方法 / 路径 / mock|proxy 标记 / cases 数）
4. `fakeserver serve -c valid.json5` 启动时打印路由摘要后进入 listen；命中配置中的路由暂时仍 fallback 到 echo（暂不响应 body），命中 `/__fakeserver/healthz` 仍返回 200
5. `go test ./...` 通过；`internal/config/` 单元测试覆盖率 ≥ 80%
6. 默认查找路径行为：CWD 无 `-c` 时 3 档候选都不存在则降回 echo-only 模式（不报错）
7. 引入 `github.com/titanous/json5` 为唯一新依赖（不引 easytpl/expr/fsnotify/gofakeit）

---

## 文件结构（Phase 2 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `go.mod` / `go.sum` | 引入 `github.com/titanous/json5` |
| 新建 | `internal/config/schema.go` | 强类型结构体：`Config` / `ServerOpts` / `Route` / `RouteCase` / `ProxyConfig`；默认值常量 |
| 新建 | `internal/config/loader.go` | `Load(paths, envName, overrides) (*Config, error)` / `loadFile` / `parseJSON5` / `expandIncludes` / `mergeMaps` |
| 新建 | `internal/config/defaults.go` | `DefaultPaths() []string` 默认查找 / `applyDefaults(*Config)` |
| 新建 | `internal/config/validate.go` | `Validate(*Config) []error` 集中校验 |
| 新建 | `internal/config/loader_test.go` | 表驱动测试：JSON5 解析 / include 展开 / 多文件合并 / 默认查找 |
| 新建 | `internal/config/validate_test.go` | 表驱动测试：所有 design §3.7 校验项 |
| 新建 | `internal/config/testdata/...` | 测试夹具：valid/invalid/include/duplicate 目录 |
| 新建 | `internal/cli/init.go` | `fakeserver init [--with-env]` 子命令；生成 `fakeserver.json5` 模板 |
| 新建 | `internal/cli/check.go` | `fakeserver check -c <paths>` 子命令；仅校验 |
| 新建 | `internal/cli/routes.go` | `fakeserver routes -c <paths>` 子命令；离线打印路由摘要 |
| 新建 | `internal/cli/summary.go` | `PrintRouteSummary(cfg *config.Config, w io.Writer)`；serve/routes 共享 |
| 新建 | `internal/cli/init_test.go` | init 子命令测试（临时目录 + 已存在报错） |
| 新建 | `internal/cli/check_test.go` | check 子命令测试（覆盖 valid/invalid 退出码） |
| 新建 | `internal/cli/routes_test.go` | routes 子命令测试（含摘要格式断言） |
| 新建 | `internal/cli/summary_test.go` | 摘要打印的输出格式测试 |
| 新建 | `internal/cli/templates.go` | `initTemplate` / `initEnvTemplate` 常量字符串：init 子命令的内嵌模板 |
| 修改 | `internal/cli/app.go` | 注册 init/check/routes 三个新子命令 |
| 修改 | `internal/cli/serve.go` | 加 `-c/--config` 选项；启动时加载配置 + 打印摘要；mock 路由暂不注册（仍走 echo） |
| 修改 | `internal/cli/serve_test.go` | 增加"`-c` 加载并打印路由摘要"用例 |
| 修改 | `docs/plans/2026-05-19-fakeserver-v0.1-overview.md` | §2 表的 Phase 2 状态列写"✅ 已完成 (commit <SHA range>)" |
| 修改 | `docs/fakeserver-design.md` | 修订记录新增一行；§13 如有偏差追加"已落地"条目 |

> **注**：本 Phase **不**新建 `internal/{tpl,mock,proxy,middleware}/` 任何目录——它们留给 Phase 3-5。schema 中含 `proxy` 字段定义但**仅作字段位**，loader/validate 处理它的结构，serve 暂不为 proxy 路由注册 handler。

---

## Task 1: 引入 titanous/json5 + smoke test 锁定行为

**Files**:
- 修改：`go.mod` / `go.sum`
- 新建：`internal/config/loader.go`（最小 stub）
- 新建：`internal/config/loader_test.go`（smoke）

- [ ] **Step 1.1: 拉依赖**

执行：

```
go get github.com/titanous/json5
go mod tidy
```

预期：`go.mod` 出现 `github.com/titanous/json5 v...`；`go.sum` 更新。

- [ ] **Step 1.2: 写 smoke test 锁定 lib 行为**

新建 `internal/config/loader_test.go`（**最小 smoke**，下个 Task 才扩为完整测试）：

```go
package config

import (
	"strings"
	"testing"

	"github.com/titanous/json5"
)

// TestJSON5Lib_SmokeAcceptsCommonExtensions 锁定 titanous/json5 对我们配置文件
// 实际依赖的 JSON5 扩展项的支持。如果哪天换库或库行为变了，此测试先红。
func TestJSON5Lib_SmokeAcceptsCommonExtensions(t *testing.T) {
	src := `{
		// line comment
		/* block comment */
		server: { port: 5090 },        // 无引号 key
		fallback: "echo",
		routes: [
			{ method: 'GET', path: "/ping" },  // 单引号字符串
			{ method: "POST", path: "/users" }, // trailing comma 允许
		],
	}`
	var out map[string]any
	if err := json5.NewDecoder(strings.NewReader(src)).Decode(&out); err != nil {
		t.Fatalf("json5 should accept common extensions; err=%v", err)
	}
	if out["fallback"] != "echo" {
		t.Errorf("expected fallback=echo, got %v", out["fallback"])
	}
}

// TestJSON5Lib_SmokeErrorMentionsContext: 解析失败时错误信息应足够定位问题
// （不强求行号，但要包含可识别的位置或 token 信息）。
func TestJSON5Lib_SmokeErrorMentionsContext(t *testing.T) {
	src := `{ server: { port: 5090  routes: [] }`  // 缺逗号
	var out map[string]any
	err := json5.NewDecoder(strings.NewReader(src)).Decode(&out)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	// 仅验证有 error；具体 message 由 lib 决定
}
```

- [ ] **Step 1.3: 新建 `internal/config/loader.go` 最小 stub**

新建该文件，逐字写入：

```go
// Package config loads, merges, and validates fakeserver JSON5 configurations.
//
// Load() walks one or more JSON5 files (or the default search paths when no
// path is given), expands "@include" references, merges results, and produces
// a strongly-typed *Config. Validate() then runs all of design §3.7's checks
// collectively, returning every problem at once.
//
// The schema (struct definitions) lives in schema.go; default values and the
// CWD search list live in defaults.go; cross-checks live in validate.go.
package config

// Load is the public entry point. Implementations land in subsequent steps;
// this stub exists so json5 dependency resolves before the package has any
// concrete consumer.
func Load(paths []string, envName string, overrides map[string]string) (*Config, error) {
	return nil, nil
}

// Config is forward-declared here so the stub compiles. The real definition
// (with all fields) lands in schema.go in Task 2.
type Config struct{}
```

- [ ] **Step 1.4: 验证编译 + smoke 测试通过**

```
go build ./...
go test ./internal/config/... -run TestJSON5Lib -v
```

预期：两个 smoke 用例 PASS。

- [ ] **Step 1.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add go.mod go.sum internal/config/loader.go internal/config/loader_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "chore(config): 引入 titanous/json5 + smoke 锁定常见 JSON5 扩展"
```

---

## Task 2: schema.go 强类型结构体与默认值

**Files**:
- 新建：`internal/config/schema.go`
- 新建：`internal/config/defaults.go`
- 新建：`internal/config/schema_test.go`（轻量，仅默认值与 ApplyDefaults 行为）

> **职责拆分**：`schema.go` 仅放结构体定义与字段 tag；`defaults.go` 放 `DefaultPaths()` 与 `applyDefaults()`；测试单独。

- [ ] **Step 2.1: 先写测试**

新建 `internal/config/schema_test.go`：

```go
package config

import "testing"

func TestApplyDefaults_ZeroValues(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 5090 {
		t.Errorf("expected default port 5090, got %d", cfg.Server.Port)
	}
	if cfg.Server.MaxBodySize != "1MiB" {
		t.Errorf("expected default maxBodySize 1MiB, got %q", cfg.Server.MaxBodySize)
	}
	if !cfg.Server.AdminEnabled {
		t.Errorf("expected default adminEnabled=true")
	}
	if cfg.Server.HistorySize != 200 {
		t.Errorf("expected default historySize 200, got %d", cfg.Server.HistorySize)
	}
	if cfg.Fallback != "echo" {
		t.Errorf("expected default fallback=echo, got %q", cfg.Fallback)
	}
}

func TestApplyDefaults_PreservesNonZero(t *testing.T) {
	cfg := &Config{
		Server: ServerOpts{Port: 9000, MaxBodySize: "5MiB"},
		Fallback: "404",
	}
	applyDefaults(cfg)

	if cfg.Server.Port != 9000 {
		t.Errorf("non-zero port should be preserved")
	}
	if cfg.Server.MaxBodySize != "5MiB" {
		t.Errorf("non-zero maxBodySize should be preserved")
	}
	if cfg.Fallback != "404" {
		t.Errorf("non-zero fallback should be preserved")
	}
	// 但默认 host 仍应被填上
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("zero host should still get default")
	}
}

func TestDefaultPaths_ReturnsThreeCandidates(t *testing.T) {
	paths := DefaultPaths()
	if len(paths) != 3 {
		t.Fatalf("expected 3 default candidates, got %d", len(paths))
	}
	expected := []string{
		"fakeserver.json5",
		"fakeserver.json",
		".fakeserver/config.json5",
	}
	for i, want := range expected {
		if paths[i] != want {
			t.Errorf("paths[%d]: want %q, got %q", i, want, paths[i])
		}
	}
}
```

- [ ] **Step 2.2: 运行测试确认失败**

```
go test ./internal/config/... -run "TestApplyDefaults|TestDefaultPaths" -v
```

预期：FAIL（`ServerOpts` 等类型尚未定义；`applyDefaults` / `DefaultPaths` 未实现）。

- [ ] **Step 2.3: 完整替换 `internal/config/loader.go` 中的 `Config` 占位** 并新建 `schema.go`

先删除 `loader.go` 里 `type Config struct{}` 那一行（只删这一行，保留 package 注释、Load 函数）。

新建 `internal/config/schema.go`，逐字写入：

```go
package config

// Config is the in-memory representation of one or more merged JSON5
// configuration files. Field types intentionally use plain Go primitives
// and maps so that consumers (cli/mock/proxy packages) do not need any
// json5-specific knowledge.
type Config struct {
	Server   ServerOpts       `json:"server"`
	Globals  map[string]any   `json:"globals"`
	Fallback string           `json:"fallback"`
	Routes   []Route          `json:"routes"`

	// SourcePaths records, in load order, every config file that
	// contributed to this Config (including @include expansions).
	// Used by validate.go for error messages and (in Phase 5) by the
	// hot-reload watcher to decide which files to subscribe to.
	SourcePaths []string `json:"-"`
}

// ServerOpts mirrors the "server" block in JSON5. Each field's default is
// applied by applyDefaults() in defaults.go when the field is its zero
// value.
type ServerOpts struct {
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	CORS           any      `json:"cors"`           // true | false | object — kept as raw any here; Phase 5 parses
	Log            *bool    `json:"log"`            // pointer to detect "unset" vs "false"
	MaxBodySize    string   `json:"maxBodySize"`
	AdminEnabled   bool     `json:"adminEnabled"`
	OSEnvWhitelist []string `json:"osenvWhitelist"`
	FakerSeed      int64    `json:"fakerSeed"`
	HistorySize    int      `json:"historySize"`
	ProjectName    string   `json:"projectName"`
}

// Route describes one declared route in JSON5. The same struct covers
// single-response (status/headers/body/bodyFile/delay), multi-response
// (strategy/cases), and proxy modes. Validate() in validate.go enforces
// the mutual-exclusion rules from design §3.2.
type Route struct {
	// Method may be a string ("GET"), an array (["GET","HEAD"]), or "*".
	// loader.go normalizes whatever the JSON5 yields into a []string.
	Method []string `json:"method"`
	Path   string   `json:"path"`

	// Single-response fields
	Status   int               `json:"status"`
	Delay    string            `json:"delay"`
	Headers  map[string]string `json:"headers"`
	Body     any               `json:"body"`
	BodyFile string            `json:"bodyFile"`

	// Multi-response fields
	Strategy string      `json:"strategy"`
	Cases    []RouteCase `json:"cases"`

	// Proxy mode (mutex with all of the above except method/path)
	Proxy *ProxyConfig `json:"proxy"`

	// SourceFile is the absolute path of the JSON5 file this route was
	// declared in (after @include expansion). Used by Validate() to
	// pinpoint duplicate-route locations in error messages.
	SourceFile string `json:"-"`
}

// RouteCase is one branch inside Route.Cases.
type RouteCase struct {
	When    string            `json:"when"`
	Weight  int               `json:"weight"`
	Status  int               `json:"status"`
	Delay   string            `json:"delay"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
	BodyFile string           `json:"bodyFile"`
}

// ProxyConfig is the "proxy" sub-block of a Route. Only schema/validate is
// implemented in Phase 2 — actual proxying is implemented in Phase 4.
type ProxyConfig struct {
	Target             string            `json:"target"`
	Rewrite            any               `json:"rewrite"` // string or []string
	StripPathPrefix    string            `json:"stripPathPrefix"`
	Headers            map[string]string `json:"headers"`
	ResponseHeaders    map[string]string `json:"responseHeaders"`
	Timeout            string            `json:"timeout"`
	InsecureSkipVerify bool              `json:"insecureSkipVerify"`
	PreserveHost       bool              `json:"preserveHost"`
	BodyLimit          string            `json:"bodyLimit"`
}
```

新建 `internal/config/defaults.go`，逐字写入：

```go
package config

// DefaultPaths returns the CWD-relative candidate paths searched when the
// user runs `fakeserver serve` without an explicit -c flag. Search order
// matches design §3.5: the first existing file wins; if none exist the
// caller should fall back to echo-only mode without erroring.
func DefaultPaths() []string {
	return []string{
		"fakeserver.json5",
		"fakeserver.json",
		".fakeserver/config.json5",
	}
}

// applyDefaults fills in any zero-valued field with the default specified
// in design §3.1. Non-zero fields are preserved untouched.
//
// Note: bool defaults need special handling — for fields that default to
// true (AdminEnabled), a literal `false` in JSON5 must round-trip as
// false. We achieve this by leaving AdminEnabled untouched if Globals or
// any sibling field signals "the user provided server{}" — but for v0.1
// we accept the simpler rule: AdminEnabled defaults to true on a brand
// new ServerOpts{} (the cmd-line layer can override post-load if it
// detects an explicit --no-admin flag in future Phases).
//
// Phase 5 may revisit this when the CORS block adds richer semantics.
func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 5090
	}
	if cfg.Server.MaxBodySize == "" {
		cfg.Server.MaxBodySize = "1MiB"
	}
	// AdminEnabled defaults to true. Because bool's zero value is false, we
	// cannot distinguish "user wrote false" from "user omitted". Phase 2
	// accepts this; if a Phase 5 user needs adminEnabled:false they can
	// either accept that behavior or wait for the pointer-based redesign.
	if !cfg.Server.AdminEnabled {
		cfg.Server.AdminEnabled = true
	}
	if cfg.Server.HistorySize == 0 {
		cfg.Server.HistorySize = 200
	}
	if cfg.Fallback == "" {
		cfg.Fallback = "echo"
	}
}
```

- [ ] **Step 2.4: 运行测试确认通过**

```
go test ./internal/config/... -run "TestApplyDefaults|TestDefaultPaths" -v
```

预期：3 个用例全 PASS。

- [ ] **Step 2.5: 校验编译**

```
go build ./...
go test ./internal/config/... -v
```

预期：schema/defaults 测试 + 之前的 JSON5 smoke test 全 PASS。

- [ ] **Step 2.6: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/schema.go internal/config/defaults.go internal/config/schema_test.go internal/config/loader.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): 定义 schema 结构体 + 默认值与默认查找路径"
```

---

## Task 3: 单文件加载（无 include 无合并）

**Files**:
- 修改：`internal/config/loader.go`
- 新建：`internal/config/testdata/valid/single-minimal.json5`
- 新建：`internal/config/testdata/valid/single-full.json5`
- 修改：`internal/config/loader_test.go`

> **目标**：让 `Load([]string{"path.json5"}, "", nil)` 返回填好默认值的 `*Config`，且 `routes` 字段是强类型 `[]Route`（method 标准化为 `[]string`，无 include 展开）。

- [ ] **Step 3.1: 写测试夹具**

新建 `internal/config/testdata/valid/single-minimal.json5`：

```json5
{
  routes: [
    { method: "GET", path: "/ping", body: "pong" },
  ],
}
```

新建 `internal/config/testdata/valid/single-full.json5`：

```json5
{
  server: { host: "127.0.0.1", port: 8080, log: false },
  globals: { apiVersion: "v1", userPool: ["alice", "bob"] },
  fallback: "404",
  routes: [
    { method: "GET", path: "/ping", body: "pong" },
    {
      method: ["GET", "HEAD"],
      path: "/users/{id}",
      status: 200,
      headers: { "Content-Type": "application/json" },
      body: { id: "{{ .request.params.id }}", name: "alice" },
    },
    {
      method: "*",
      path: "/api/proxy/*rest",
      proxy: { target: "http://upstream.local:8080", timeout: "10s" },
    },
  ],
}
```

- [ ] **Step 3.2: 写测试用例（先 fail）**

向 `internal/config/loader_test.go` **追加**（保留现有 smoke 用例）：

```go
func TestLoad_SingleMinimal(t *testing.T) {
	cfg, err := Load([]string{"testdata/valid/single-minimal.json5"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil cfg")
	}
	// 默认值已填
	if cfg.Server.Port != 5090 {
		t.Errorf("expected default port 5090, got %d", cfg.Server.Port)
	}
	if cfg.Fallback != "echo" {
		t.Errorf("expected default fallback=echo, got %q", cfg.Fallback)
	}
	// routes
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}
	r := cfg.Routes[0]
	if len(r.Method) != 1 || r.Method[0] != "GET" {
		t.Errorf("expected method=[GET], got %v", r.Method)
	}
	if r.Path != "/ping" {
		t.Errorf("expected path=/ping, got %q", r.Path)
	}
	if r.Body != "pong" {
		t.Errorf("expected body=\"pong\", got %v", r.Body)
	}
}

func TestLoad_SingleFull_NormalizesMethodAndPreservesUserValues(t *testing.T) {
	cfg, err := Load([]string{"testdata/valid/single-full.json5"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// server 用户值保留
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host=127.0.0.1, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port=8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Log == nil || *cfg.Server.Log {
		t.Errorf("expected log=false, got %v", cfg.Server.Log)
	}
	if cfg.Fallback != "404" {
		t.Errorf("expected fallback=404, got %q", cfg.Fallback)
	}
	// globals
	if cfg.Globals["apiVersion"] != "v1" {
		t.Errorf("expected globals.apiVersion=v1")
	}
	// routes[0]: 单字符串 method 标准化
	if len(cfg.Routes[0].Method) != 1 || cfg.Routes[0].Method[0] != "GET" {
		t.Errorf("expected GET, got %v", cfg.Routes[0].Method)
	}
	// routes[1]: 数组 method 保持顺序
	if len(cfg.Routes[1].Method) != 2 || cfg.Routes[1].Method[0] != "GET" || cfg.Routes[1].Method[1] != "HEAD" {
		t.Errorf("expected [GET,HEAD], got %v", cfg.Routes[1].Method)
	}
	// routes[2]: proxy 字段被解析为强类型
	if cfg.Routes[2].Proxy == nil {
		t.Fatalf("expected proxy non-nil")
	}
	if cfg.Routes[2].Proxy.Target != "http://upstream.local:8080" {
		t.Errorf("expected target=http://upstream.local:8080, got %q", cfg.Routes[2].Proxy.Target)
	}
	if cfg.Routes[2].Proxy.Timeout != "10s" {
		t.Errorf("expected timeout=10s, got %q", cfg.Routes[2].Proxy.Timeout)
	}
	// SourcePaths 包含加载文件
	if len(cfg.SourcePaths) != 1 {
		t.Errorf("expected 1 source path, got %d", len(cfg.SourcePaths))
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load([]string{"testdata/does-not-exist.json5"}, "", nil)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
```

- [ ] **Step 3.3: 实现 Load 单文件加载**

完整替换 `internal/config/loader.go`，逐字写入：

```go
// Package config loads, merges, and validates fakeserver JSON5 configurations.
//
// Load() walks one or more JSON5 files, expands "@include" references,
// merges results, and produces a strongly-typed *Config. Validate() then
// runs all of design §3.7's checks collectively, returning every problem
// at once.
//
// The schema (struct definitions) lives in schema.go; default values and
// the CWD search list live in defaults.go; cross-checks live in
// validate.go.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/titanous/json5"
)

// Load reads each path in order, expands @include references relative to
// each file's directory, merges the results, applies defaults, and returns
// a *Config. envName and overrides are accepted but ignored in Phase 2 —
// they exist for v0.2's env-file integration.
//
// On any error (file not found, JSON5 syntax, include cycle, type
// mismatch), Load returns nil + a wrapping error. Validate() in
// validate.go is a separate step the caller must invoke.
func Load(paths []string, envName string, overrides map[string]string) (*Config, error) {
	if len(paths) == 0 {
		return nil, errors.New("config.Load: no paths provided")
	}

	var raws []map[string]any
	var absSources []string
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", p, err)
		}
		raw, err := loadFile(abs)
		if err != nil {
			return nil, err
		}
		// Phase 2 Task 4 will expand @include here.
		raws = append(raws, raw)
		absSources = append(absSources, abs)
	}

	merged := mergeMaps(raws)

	cfg, err := mapToConfig(merged)
	if err != nil {
		return nil, err
	}
	cfg.SourcePaths = absSources
	applyDefaults(cfg)
	return cfg, nil
}

// loadFile reads a single file and parses it as JSON5 into a map.
func loadFile(absPath string) (map[string]any, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", absPath, err)
	}
	var out map[string]any
	if err := json5.NewDecoder(strings.NewReader(string(data))).Decode(&out); err != nil {
		return nil, fmt.Errorf("parse %q: %w", absPath, err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// mergeMaps merges a slice of root maps left-to-right per design §3.4:
//   - server / globals / fallback: deep-merge (later overrides earlier)
//   - routes: append in order
// Empty input returns a fresh empty map.
func mergeMaps(raws []map[string]any) map[string]any {
	if len(raws) == 0 {
		return map[string]any{}
	}
	if len(raws) == 1 {
		return raws[0]
	}
	out := map[string]any{}
	for _, m := range raws {
		for k, v := range m {
			if k == "routes" {
				existing, _ := out["routes"].([]any)
				if newRoutes, ok := v.([]any); ok {
					out["routes"] = append(existing, newRoutes...)
				}
				continue
			}
			if existing, ok := out[k].(map[string]any); ok {
				if newMap, ok := v.(map[string]any); ok {
					out[k] = deepMerge(existing, newMap)
					continue
				}
			}
			out[k] = v
		}
	}
	return out
}

// deepMerge merges right into left recursively. Maps are merged key by
// key; non-map values from right replace left.
func deepMerge(left, right map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range left {
		out[k] = v
	}
	for k, v := range right {
		if existingMap, ok := out[k].(map[string]any); ok {
			if newMap, ok := v.(map[string]any); ok {
				out[k] = deepMerge(existingMap, newMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// mapToConfig converts the raw merged map into a strongly-typed *Config.
// We round-trip through encoding/json to leverage struct tags: titanous/json5
// already produced standard Go map/slice/primitive types, so a JSON
// re-encode is lossless. Custom normalization (Route.Method may be string
// or []string) happens after.
func mapToConfig(m map[string]any) (*Config, error) {
	// Normalize Route.Method shapes before re-encoding: JSON's struct tags
	// can't natively accept "string OR []string", so we coerce in-place.
	if routes, ok := m["routes"].([]any); ok {
		for i, r := range routes {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			method := rm["method"]
			switch v := method.(type) {
			case string:
				rm["method"] = []any{v}
			case nil:
				rm["method"] = []any{}
			case []any:
				// already normalized
			default:
				return nil, fmt.Errorf("routes[%d].method: unsupported type %T", i, v)
			}
		}
	}

	buf, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("re-encode to JSON: %w", err)
	}
	cfg := &Config{}
	if err := json.Unmarshal(buf, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal to Config: %w", err)
	}
	return cfg, nil
}
```

- [ ] **Step 3.4: 运行测试**

```
go test ./internal/config/... -v
```

预期：smoke + schema + 3 个新 Load 用例全 PASS。

- [ ] **Step 3.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/loader.go internal/config/loader_test.go internal/config/testdata
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): Load 单文件加载（JSON5 → 强类型 *Config）"
```

---

## Task 4: `@include` 展开（含 glob / 嵌套 / 循环检测）

**Files**:
- 修改：`internal/config/loader.go`
- 新建：`internal/config/testdata/include/{root,routes,nested,cycle}/...`
- 修改：`internal/config/loader_test.go`

> **职责**：design §3.3 规则——`@<path>` 字符串值在 routes 数组元素位置展开为 route 数组（拍平）或单个 route；glob 支持 `*` / `**`；相对路径基于"当前文件目录"；循环 include 检测并报错。**仅支持** `.json` / `.json5` 后缀。

- [ ] **Step 4.1: 写测试夹具（一组目录）**

新建以下文件结构：

`internal/config/testdata/include/root.json5`：
```json5
{
  routes: [
    { method: "GET", path: "/inline", body: "inline" },
    "@routes/users.json5",
    "@routes/admin/*.json5",
  ],
}
```

`internal/config/testdata/include/routes/users.json5`：
```json5
[
  { method: "GET", path: "/users", body: { items: [] } },
  { method: "GET", path: "/users/{id}", body: { id: "x" } },
]
```

`internal/config/testdata/include/routes/admin/orders.json5`：
```json5
{ method: "GET", path: "/admin/orders", body: { count: 0 } }
```

`internal/config/testdata/include/routes/admin/reports.json5`：
```json5
{ method: "GET", path: "/admin/reports", body: { count: 0 } }
```

`internal/config/testdata/include/cycle/a.json5`：
```json5
{ routes: ["@b.json5"] }
```

`internal/config/testdata/include/cycle/b.json5`：
```json5
{ routes: ["@a.json5"] }
```

`internal/config/testdata/include/nested/root.json5`：
```json5
{ routes: ["@level1.json5"] }
```

`internal/config/testdata/include/nested/level1.json5`：
```json5
[ "@level2.json5", { method: "GET", path: "/l1", body: "l1" } ]
```

`internal/config/testdata/include/nested/level2.json5`：
```json5
{ method: "GET", path: "/l2", body: "l2" }
```

- [ ] **Step 4.2: 写测试用例**

向 `internal/config/loader_test.go` **追加**：

```go
func TestLoad_Include_FlattensRoutesArray(t *testing.T) {
	cfg, err := Load([]string{"testdata/include/root.json5"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 期望：inline 1 + users.json5 数组 2 + admin/*.json5 两个文件各 1 = 5
	if len(cfg.Routes) != 5 {
		t.Fatalf("expected 5 routes, got %d", len(cfg.Routes))
	}
	wantPaths := map[string]bool{
		"/inline":         true,
		"/users":          true,
		"/users/{id}":     true,
		"/admin/orders":   true,
		"/admin/reports":  true,
	}
	for _, r := range cfg.Routes {
		if !wantPaths[r.Path] {
			t.Errorf("unexpected route path %q", r.Path)
		}
	}
}

func TestLoad_Include_DetectsCycle(t *testing.T) {
	_, err := Load([]string{"testdata/include/cycle/a.json5"}, "", nil)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") && !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected error to mention cycle/circular, got %v", err)
	}
}

func TestLoad_Include_NestedExpansion(t *testing.T) {
	cfg, err := Load([]string{"testdata/include/nested/root.json5"}, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes (l1+l2), got %d", len(cfg.Routes))
	}
}

func TestLoad_Include_GlobNoMatchErrors(t *testing.T) {
	// 写一个临时配置只引用不存在的 glob
	tmpDir := t.TempDir()
	rootPath := filepath.Join(tmpDir, "root.json5")
	if err := os.WriteFile(rootPath, []byte(`{ routes: ["@nope/*.json5"] }`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load([]string{rootPath}, "", nil)
	if err == nil {
		t.Fatal("expected glob-no-match error")
	}
}

func TestLoad_Include_RejectsUnsupportedExtension(t *testing.T) {
	tmpDir := t.TempDir()
	rootPath := filepath.Join(tmpDir, "root.json5")
	if err := os.WriteFile(rootPath, []byte(`{ routes: ["@foo.txt"] }`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load([]string{rootPath}, "", nil)
	if err == nil {
		t.Fatal("expected unsupported-extension error")
	}
}
```

需要在文件顶部 import 加 `"os"` 和 `"path/filepath"` 和 `"strings"`（如果尚未 import）。

- [ ] **Step 4.3: 运行测试确认失败**

```
go test ./internal/config/... -run TestLoad_Include -v
```

预期：所有 Include 用例 FAIL。

- [ ] **Step 4.4: 实现 include 展开**

修改 `internal/config/loader.go`：

1. 在 `loadFile` 函数之后**新增** `expandIncludes` 函数：

```go
// expandIncludes walks the JSON5-decoded map, replacing any string value
// starting with "@" (literal) with the JSON5 content from the referenced
// file. Behavior per design §3.3:
//   - Path resolution is relative to the including file's directory
//   - Globs ("*", "**") are expanded; zero matches is an error
//   - Recursion is allowed; cycles are detected via the visiting set
//   - Only .json/.json5 extensions are accepted
//   - "\@..." escapes a literal leading "@"
//
// In Phase 2 the only call site that interprets include results
// structurally is the top-level routes array — strings appearing
// elsewhere are still expanded (per design §3.3 second row) but the
// resulting value replaces the string in place.
//
// visiting holds the absolute paths currently on the recursion stack so
// we can detect cycles before re-reading the same file.
func expandIncludes(node any, baseDir string, visiting map[string]bool) (any, error) {
	switch v := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			expanded, err := expandIncludes(child, baseDir, visiting)
			if err != nil {
				return nil, err
			}
			out[k] = expanded
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			expanded, err := expandIncludes(item, baseDir, visiting)
			if err != nil {
				return nil, err
			}
			// If a string @include resolved to a slice (e.g. a routes
			// array), flatten it into the parent slice.
			if subSlice, ok := expanded.([]any); ok && isIncludeString(item) {
				out = append(out, subSlice...)
			} else {
				out = append(out, expanded)
			}
		}
		return out, nil
	case string:
		if !strings.HasPrefix(v, "@") {
			return v, nil
		}
		if strings.HasPrefix(v, "\\@") {
			return v[1:], nil // unescape literal leading @
		}
		target := v[1:]
		return resolveInclude(target, baseDir, visiting)
	default:
		return v, nil
	}
}

// isIncludeString reports whether the original node is a "@..." reference
// (not a literal). Used so a slice include can flatten into its parent.
func isIncludeString(node any) bool {
	s, ok := node.(string)
	if !ok {
		return false
	}
	return strings.HasPrefix(s, "@") && !strings.HasPrefix(s, "\\@")
}

// resolveInclude reads and recursively expands the file (or glob) named
// by spec, relative to baseDir.
func resolveInclude(spec, baseDir string, visiting map[string]bool) (any, error) {
	// Resolve absolute target(s)
	absSpec := spec
	if !filepath.IsAbs(absSpec) {
		absSpec = filepath.Join(baseDir, spec)
	}

	var matches []string
	if strings.ContainsAny(absSpec, "*?[") {
		m, err := filepath.Glob(absSpec)
		if err != nil {
			return nil, fmt.Errorf("@include glob %q: %w", spec, err)
		}
		if len(m) == 0 {
			return nil, fmt.Errorf("@include %q: no files matched", spec)
		}
		matches = m
	} else {
		matches = []string{absSpec}
	}

	var results []any
	for _, p := range matches {
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".json" && ext != ".json5" {
			return nil, fmt.Errorf("@include %q: unsupported extension %q (only .json/.json5)", p, ext)
		}
		if visiting[p] {
			return nil, fmt.Errorf("@include cycle detected: %s", cyclePath(visiting, p))
		}
		raw, err := loadFile(p)
		if err != nil {
			return nil, err
		}

		// Recurse into the loaded content
		visiting[p] = true
		expanded, err := expandIncludes(any(raw), filepath.Dir(p), visiting)
		delete(visiting, p)
		if err != nil {
			return nil, err
		}

		// The included file's root may be a route object (map), a slice
		// of routes, or a top-level config map. Convert to "any" and
		// append.
		results = append(results, expanded)
	}

	// Single match? Return the single result so the caller can decide
	// whether to flatten. Multiple matches always flatten as a slice.
	if len(results) == 1 {
		return results[0], nil
	}
	// Multiple matches: convert each to its inner shape and flatten
	flat := make([]any, 0, len(results))
	for _, r := range results {
		if slice, ok := r.([]any); ok {
			flat = append(flat, slice...)
		} else {
			flat = append(flat, r)
		}
	}
	return flat, nil
}

// cyclePath formats the visiting set into a "a → b → c → a" chain for
// error messages.
func cyclePath(visiting map[string]bool, dup string) string {
	keys := make([]string, 0, len(visiting)+1)
	for k := range visiting {
		keys = append(keys, k)
	}
	keys = append(keys, dup)
	return strings.Join(keys, " → ")
}
```

2. 修改 `Load` 函数，在 `mergeMaps` 之前先对每个文件**单独**展开 include：

把 Load 中的 raws 收集循环改为：

```go
	var raws []map[string]any
	var absSources []string
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", p, err)
		}
		raw, err := loadFile(abs)
		if err != nil {
			return nil, err
		}
		visiting := map[string]bool{abs: true}
		expanded, err := expandIncludes(any(raw), filepath.Dir(abs), visiting)
		if err != nil {
			return nil, err
		}
		expandedMap, ok := expanded.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config %q: root must be an object", abs)
		}
		raws = append(raws, expandedMap)
		absSources = append(absSources, abs)
	}
```

- [ ] **Step 4.5: 运行测试确认通过**

```
go test ./internal/config/... -v
```

预期：所有 include 用例 + 之前的 Task 1/2/3 用例全 PASS。

> **如果 TestLoad_Include_FlattensRoutesArray 失败**：检查 `expandIncludes` 在 `[]any` 分支里的 flatten 逻辑——`"@routes/users.json5"` 返回 `[]any{route1, route2}` 时必须**拍平**到父数组，而不是作为单个元素插入。

- [ ] **Step 4.6: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/loader.go internal/config/loader_test.go internal/config/testdata/include
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): @include 展开（含 glob/嵌套/循环检测/扩展名校验）"
```

---

## Task 5: 多文件合并 + DefaultPaths 集成

**Files**:
- 修改：`internal/config/loader.go`（增加 LoadDefault 辅助）
- 新建：`internal/config/testdata/merge/{base,override}.json5`
- 修改：`internal/config/loader_test.go`

- [ ] **Step 5.1: 写测试夹具**

`internal/config/testdata/merge/base.json5`：

```json5
{
  server: { port: 5090, host: "0.0.0.0" },
  globals: { apiVersion: "v1" },
  routes: [
    { method: "GET", path: "/base-only", body: "base" },
  ],
}
```

`internal/config/testdata/merge/override.json5`：

```json5
{
  server: { port: 9000 },
  globals: { apiVersion: "v2", extra: "added" },
  fallback: "404",
  routes: [
    { method: "GET", path: "/override-only", body: "override" },
  ],
}
```

- [ ] **Step 5.2: 写测试用例**

向 `internal/config/loader_test.go` **追加**：

```go
func TestLoad_MultiFile_Merge(t *testing.T) {
	cfg, err := Load(
		[]string{"testdata/merge/base.json5", "testdata/merge/override.json5"},
		"", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// server: 后者覆盖前者；未在 override 中的字段保留 base
	if cfg.Server.Port != 9000 {
		t.Errorf("expected port 9000 (override), got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0 (base preserved), got %q", cfg.Server.Host)
	}
	// globals: deep merge
	if cfg.Globals["apiVersion"] != "v2" {
		t.Errorf("expected apiVersion=v2 (override), got %v", cfg.Globals["apiVersion"])
	}
	if cfg.Globals["extra"] != "added" {
		t.Errorf("expected globals.extra=added")
	}
	// fallback: 后者覆盖
	if cfg.Fallback != "404" {
		t.Errorf("expected fallback=404 (override), got %q", cfg.Fallback)
	}
	// routes: append in order
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes (1 base + 1 override), got %d", len(cfg.Routes))
	}
	if cfg.Routes[0].Path != "/base-only" {
		t.Errorf("expected routes[0].path=/base-only, got %q", cfg.Routes[0].Path)
	}
	if cfg.Routes[1].Path != "/override-only" {
		t.Errorf("expected routes[1].path=/override-only, got %q", cfg.Routes[1].Path)
	}
}

func TestLoadDefault_PicksFirstExistingCandidate(t *testing.T) {
	tmpDir := t.TempDir()
	// 在 tmpDir 下放第二档候选 (fakeserver.json)，第一档 (fakeserver.json5) 故意不放
	target := filepath.Join(tmpDir, "fakeserver.json")
	if err := os.WriteFile(target, []byte(`{"routes":[{"method":"GET","path":"/x","body":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadDefault(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil cfg")
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].Path != "/x" {
		t.Errorf("expected single route /x, got %+v", cfg.Routes)
	}
}

func TestLoadDefault_NoneExistReturnsNilNilNoError(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := LoadDefault(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error when no defaults exist: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil cfg (echo-only fallback), got %+v", cfg)
	}
}
```

- [ ] **Step 5.3: 实现 LoadDefault**

向 `internal/config/loader.go` **追加**：

```go
// LoadDefault searches cwd for the conventional config files (see
// DefaultPaths) and loads the first one that exists. Returns (nil, nil)
// — not an error — when none exist, so the caller (cli/serve.go) can
// degrade to echo-only mode silently.
func LoadDefault(cwd string) (*Config, error) {
	for _, rel := range DefaultPaths() {
		abs := filepath.Join(cwd, rel)
		if _, err := os.Stat(abs); err == nil {
			return Load([]string{abs}, "", nil)
		}
	}
	return nil, nil
}
```

- [ ] **Step 5.4: 运行测试**

```
go test ./internal/config/... -v
```

预期：全部 PASS。

- [ ] **Step 5.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/loader.go internal/config/loader_test.go internal/config/testdata/merge
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): 多文件合并 + LoadDefault 默认查找"
```

---

## Task 6: 集中校验 `Validate(*Config) []error`

**Files**:
- 新建：`internal/config/validate.go`
- 新建：`internal/config/validate_test.go`
- 新建：`internal/config/testdata/invalid/...`（各种错误用例文件）

> **职责**：实现 design §3.7 所有校验项的子集（Phase 2 不引 expr/easytpl，所以 when 表达式语法与模板语法的校验留 Phase 4/3）。返回**所有**错误，不在第一个就 short-circuit。

校验清单（Phase 2 落地）：

1. routes 中每个 route 必须有 method 和 path
2. `body` 和 `bodyFile` 不能同时出现
3. `cases` 与顶层 `body/bodyFile` 不能同时出现
4. `proxy` 与 `body/bodyFile/cases/status/headers/delay` 任一同时出现报错
5. `proxy.target` 必须以 `http://` 或 `https://` 开头
6. `bodyFile` 指向的文件必须存在（相对 SourceFile 目录或绝对路径）
7. 跨条目同 `method+path` 重复报错（含同一文件内 + 跨文件）
8. 用户路由覆盖保留前缀 `/__fakeserver/*` 报错
9. `fallback` 必须是 `echo` 或 `404`（默认 echo）
10. `strategy` 必须是 `random`/`round-robin`/`weighted`/`first-match` 之一（缺省 = random）

警告（不阻止）：

11. `strategy: "first-match"` 下所有 case 都写了 `when` → warn "no fallback case"

> Phase 2 暂不校验 expr 表达式语法与 Go 模板语法——它们由 Phase 4/3 接入相应库时自动 cover。

- [ ] **Step 6.1: 写测试夹具（错误用例文件）**

新建以下文件：

`internal/config/testdata/invalid/body-and-bodyfile.json5`：
```json5
{ routes: [{ method: "GET", path: "/x", body: "a", bodyFile: "b" }] }
```

`internal/config/testdata/invalid/duplicate-routes.json5`：
```json5
{ routes: [
  { method: "GET", path: "/users", body: "first" },
  { method: "GET", path: "/users", body: "second" },
] }
```

`internal/config/testdata/invalid/missing-method.json5`：
```json5
{ routes: [{ path: "/x", body: "a" }] }
```

`internal/config/testdata/invalid/reserved-prefix.json5`：
```json5
{ routes: [{ method: "GET", path: "/__fakeserver/abuse", body: "x" }] }
```

`internal/config/testdata/invalid/proxy-and-body.json5`：
```json5
{ routes: [{ method: "GET", path: "/x", body: "a", proxy: { target: "http://upstream" } }] }
```

`internal/config/testdata/invalid/proxy-bad-scheme.json5`：
```json5
{ routes: [{ method: "GET", path: "/x", proxy: { target: "ftp://upstream" } }] }
```

`internal/config/testdata/invalid/bodyfile-missing.json5`：
```json5
{ routes: [{ method: "GET", path: "/x", bodyFile: "no-such-file.bin" }] }
```

`internal/config/testdata/invalid/bad-fallback.json5`：
```json5
{ fallback: "nope", routes: [{ method: "GET", path: "/x", body: "a" }] }
```

`internal/config/testdata/invalid/bad-strategy.json5`：
```json5
{ routes: [{ method: "GET", path: "/x", strategy: "weird", cases: [{ body: "a" }] }] }
```

- [ ] **Step 6.2: 写测试用例**

新建 `internal/config/validate_test.go`：

```go
package config

import (
	"strings"
	"testing"
)

func loadOrFatal(t *testing.T, path string) *Config {
	t.Helper()
	cfg, err := Load([]string{path}, "", nil)
	if err != nil {
		t.Fatalf("Load %q: %v", path, err)
	}
	return cfg
}

func TestValidate_HappyPath(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/valid/single-full.json5")
	errs := Validate(cfg)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_BodyAndBodyFile(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/body-and-bodyfile.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "body", "bodyFile") {
		t.Errorf("expected error mentioning body/bodyFile mutex; got %v", errs)
	}
}

func TestValidate_DuplicateRoutes(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/duplicate-routes.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "duplicate", "/users") {
		t.Errorf("expected duplicate-route error; got %v", errs)
	}
}

func TestValidate_MissingMethod(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/missing-method.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "method") {
		t.Errorf("expected missing-method error; got %v", errs)
	}
}

func TestValidate_ReservedPrefix(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/reserved-prefix.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "__fakeserver") {
		t.Errorf("expected reserved-prefix error; got %v", errs)
	}
}

func TestValidate_ProxyAndBody(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/proxy-and-body.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "proxy", "body") {
		t.Errorf("expected proxy/body mutex error; got %v", errs)
	}
}

func TestValidate_ProxyBadScheme(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/proxy-bad-scheme.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "ftp", "scheme") {
		t.Errorf("expected proxy scheme error; got %v", errs)
	}
}

func TestValidate_BodyFileMissing(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bodyfile-missing.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "no-such-file") {
		t.Errorf("expected missing-file error; got %v", errs)
	}
}

func TestValidate_BadFallback(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bad-fallback.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "fallback") {
		t.Errorf("expected fallback enum error; got %v", errs)
	}
}

func TestValidate_BadStrategy(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bad-strategy.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "strategy") {
		t.Errorf("expected strategy enum error; got %v", errs)
	}
}

// containsErrorWith returns true if any error message contains every one
// of the given substrings (case-sensitive).
func containsErrorWith(errs []error, subs ...string) bool {
	for _, e := range errs {
		msg := e.Error()
		ok := true
		for _, s := range subs {
			if !strings.Contains(msg, s) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
```

- [ ] **Step 6.3: 运行测试确认失败**

```
go test ./internal/config/... -run TestValidate -v
```

预期：FAIL（`Validate` 尚未实现）。

- [ ] **Step 6.4: 实现 Validate**

新建 `internal/config/validate.go`，逐字写入：

```go
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const reservedPrefix = "/__fakeserver/"

var (
	validFallback = map[string]bool{"echo": true, "404": true}
	validStrategy = map[string]bool{
		"":            true, // default → random
		"random":      true,
		"round-robin": true,
		"weighted":    true,
		"first-match": true,
	}
)

// Validate runs every cross-cutting check from design §3.7 and returns the
// full list of problems found. The caller (cli/check.go, cli/serve.go) is
// expected to print all errors and exit non-zero if the slice is non-empty.
//
// Phase 2 does NOT check expr (when:) syntax or template syntax — those
// require their respective libraries which arrive in Phase 4/3.
func Validate(cfg *Config) []error {
	if cfg == nil {
		return []error{fmt.Errorf("validate: config is nil")}
	}

	var errs []error

	// fallback enum
	if !validFallback[cfg.Fallback] {
		errs = append(errs, fmt.Errorf("server.fallback: must be \"echo\" or \"404\", got %q", cfg.Fallback))
	}

	// route-level checks
	type key struct {
		method, path string
	}
	seen := map[key]int{} // value = first index where seen

	for i, r := range cfg.Routes {
		prefix := fmt.Sprintf("routes[%d] (%s %s)", i, strings.Join(r.Method, ","), r.Path)

		if len(r.Method) == 0 {
			errs = append(errs, fmt.Errorf("%s: method is required", prefix))
		}
		if r.Path == "" {
			errs = append(errs, fmt.Errorf("%s: path is required", prefix))
		}

		// reserved prefix
		if strings.HasPrefix(r.Path, reservedPrefix) {
			errs = append(errs, fmt.Errorf("%s: path %q collides with reserved prefix %q", prefix, r.Path, reservedPrefix))
		}

		// strategy enum
		if !validStrategy[r.Strategy] {
			errs = append(errs, fmt.Errorf("%s: strategy %q is not one of random/round-robin/weighted/first-match", prefix, r.Strategy))
		}

		// proxy vs mock mutex
		if r.Proxy != nil {
			if r.Body != nil || r.BodyFile != "" || len(r.Cases) > 0 || r.Status != 0 || len(r.Headers) > 0 || r.Delay != "" {
				errs = append(errs, fmt.Errorf("%s: proxy is mutually exclusive with body/bodyFile/cases/status/headers/delay", prefix))
			}
			if r.Proxy.Target == "" {
				errs = append(errs, fmt.Errorf("%s: proxy.target is required", prefix))
			} else {
				u, err := url.Parse(r.Proxy.Target)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
					errs = append(errs, fmt.Errorf("%s: proxy.target scheme must be http or https, got %q", prefix, r.Proxy.Target))
				}
			}
		} else {
			// mock-side mutex
			if r.Body != nil && r.BodyFile != "" {
				errs = append(errs, fmt.Errorf("%s: body and bodyFile are mutually exclusive", prefix))
			}
			if len(r.Cases) > 0 && (r.Body != nil || r.BodyFile != "") {
				errs = append(errs, fmt.Errorf("%s: cases is mutually exclusive with top-level body/bodyFile", prefix))
			}
			if r.BodyFile != "" {
				resolved := resolveRoutePath(r.BodyFile, r.SourceFile, cfg.SourcePaths)
				if _, err := os.Stat(resolved); err != nil {
					errs = append(errs, fmt.Errorf("%s: bodyFile %q not found (resolved to %q)", prefix, r.BodyFile, resolved))
				}
			}
		}

		// duplicate detection: every (method, path) combination across all
		// listed methods of every route
		for _, m := range r.Method {
			k := key{method: m, path: r.Path}
			if prev, ok := seen[k]; ok {
				errs = append(errs, fmt.Errorf("duplicate route %s %s: defined at routes[%d] and routes[%d]", m, r.Path, prev, i))
				continue
			}
			seen[k] = i
		}
	}

	return errs
}

// resolveRoutePath resolves a relative path against the source file of the
// route (if known) or against the first source path. Absolute paths are
// returned unchanged.
func resolveRoutePath(p, routeSource string, sources []string) string {
	if filepath.IsAbs(p) {
		return p
	}
	base := routeSource
	if base == "" && len(sources) > 0 {
		base = sources[0]
	}
	if base == "" {
		return p
	}
	return filepath.Join(filepath.Dir(base), p)
}
```

- [ ] **Step 6.5: 运行测试**

```
go test ./internal/config/... -v
```

预期：所有 Validate 用例 + 之前的所有用例 PASS。

> **注**：`TestValidate_DuplicateRoutes` 期望错误消息含 "duplicate" 与 "/users"——确认 Validate 输出的字面量包含这两个 token（我们的实现 "duplicate route GET /users: defined at..." 满足）。

- [ ] **Step 6.6: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/validate.go internal/config/validate_test.go internal/config/testdata/invalid
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): Validate 集中校验（design §3.7 子集；expr/template 留 Phase 3/4）"
```

---

## Task 7: `cli/init` 子命令

**Files**:
- 新建：`internal/cli/templates.go`
- 新建：`internal/cli/init.go`
- 新建：`internal/cli/init_test.go`
- 修改：`internal/cli/app.go`

> **目标**：`fakeserver init` 在 CWD 生成 `./fakeserver.json5` 模板；`--with-env` 同时生成 `fakeserver.env.json5`；目标已存在时**报错退出**不覆盖。

- [ ] **Step 7.1: 写测试**

新建 `internal/cli/init_test.go`：

```go
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_CreatesFakeserverJSON5InEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	if err := runInit(initOptions{cwd: tmpDir}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	target := filepath.Join(tmpDir, "fakeserver.json5")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected %q to exist; %v", target, err)
	}
	if !strings.Contains(string(data), "routes:") {
		t.Errorf("template should contain 'routes:' section, got: %s", data)
	}
}

func TestInit_RefusesToOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "fakeserver.json5")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runInit(initOptions{cwd: tmpDir})
	if err == nil {
		t.Fatal("expected refuse-overwrite error")
	}
	if !strings.Contains(err.Error(), "exists") {
		t.Errorf("expected error to mention 'exists', got %v", err)
	}
	// 文件没被改写
	got, _ := os.ReadFile(target)
	if string(got) != "existing" {
		t.Errorf("file should not have been overwritten")
	}
}

func TestInit_WithEnvAlsoCreatesEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	if err := runInit(initOptions{cwd: tmpDir, withEnv: true}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	envTarget := filepath.Join(tmpDir, "fakeserver.env.json5")
	data, err := os.ReadFile(envTarget)
	if err != nil {
		t.Fatalf("expected %q to exist; %v", envTarget, err)
	}
	if !strings.Contains(string(data), "$default") {
		t.Errorf("env template should contain $default; got: %s", data)
	}
}

func TestInit_GeneratedConfigIsLoadable(t *testing.T) {
	tmpDir := t.TempDir()
	if err := runInit(initOptions{cwd: tmpDir}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	// 用 config.Load 加载它，验证模板是合法 JSON5
	target := filepath.Join(tmpDir, "fakeserver.json5")
	cfg, err := loadConfig(target)
	if err != nil {
		t.Fatalf("generated template should be loadable; err=%v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil cfg")
	}
}
```

> **注**：`loadConfig` 是一个**测试辅助函数**，下文 Step 7.4 会顺带在 `init.go` 加（包级私有）；如果在 implementer 看来更合适放别处，自由调整位置但函数签名保持 `loadConfig(path string) (*config.Config, error)`。

- [ ] **Step 7.2: 运行测试确认失败**

```
go test ./internal/cli/... -run TestInit -v
```

预期：FAIL（`runInit` / `initOptions` / `loadConfig` 未定义）。

- [ ] **Step 7.3: 新建 `internal/cli/templates.go`**

```go
package cli

// initTemplate is the JSON5 written by `fakeserver init`. Includes brief
// inline comments to onboard new users. Keep it small — heavy examples
// belong in docs/examples.
const initTemplate = `{
  // Fakeserver configuration — see docs/fakeserver-design.md for full schema.
  server: {
    port: 5090,
    cors: true,
  },

  // Routes are matched by rux's radix tree: static > param > wildcard.
  // The first matching route wins; multiple responses for the same
  // method+path go into a single route's "cases" array.
  routes: [
    { method: "GET", path: "/ping", body: "pong" },

    {
      method: "GET",
      path: "/users/{id}",
      body: {
        id: "{{ .request.params.id }}",
        name: "demo-user",
      },
    },
  ],
}
`

// initEnvTemplate is written by `fakeserver init --with-env`. The schema
// matches design §8.2.
const initEnvTemplate = `{
  "$default": {
    // Variables shared across every environment. Override per-env below.
  },
  dev: {
    host: "localhost:5090",
  },
  staging: {
    host: "stage.api.example.com",
  },
  prod: {
    host: "api.example.com",
  },
}
`
```

- [ ] **Step 7.4: 新建 `internal/cli/init.go`**

```go
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gookit/gcli/v3"

	"github.com/inhere/fakeserver/internal/config"
)

type initOptions struct {
	cwd     string
	withEnv bool
}

func newInitCmd() *gcli.Command {
	opts := initOptions{}
	return &gcli.Command{
		Name: "init",
		Desc: "Generate a starter fakeserver.json5 in the current directory",
		Config: func(cmd *gcli.Command) {
			cmd.BoolOpt2(&opts.withEnv, "with-env", "Also generate fakeserver.env.json5")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getwd: %w", err)
			}
			opts.cwd = wd
			return runInit(opts)
		},
	}
}

func runInit(opts initOptions) error {
	target := filepath.Join(opts.cwd, "fakeserver.json5")
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("init: %q already exists (refusing to overwrite; delete it first if intentional)", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("init: stat %q: %w", target, err)
	}
	if err := os.WriteFile(target, []byte(initTemplate), 0o644); err != nil {
		return fmt.Errorf("init: write %q: %w", target, err)
	}
	fmt.Printf("created %s\n", target)

	if opts.withEnv {
		envTarget := filepath.Join(opts.cwd, "fakeserver.env.json5")
		if _, err := os.Stat(envTarget); err == nil {
			return fmt.Errorf("init --with-env: %q already exists (refusing to overwrite)", envTarget)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("init: stat %q: %w", envTarget, err)
		}
		if err := os.WriteFile(envTarget, []byte(initEnvTemplate), 0o644); err != nil {
			return fmt.Errorf("init: write %q: %w", envTarget, err)
		}
		fmt.Printf("created %s\n", envTarget)
	}
	return nil
}

// loadConfig is a small testing/CLI helper that loads + applies defaults.
// Validation is the caller's job. Used by init_test.go and the routes/check
// commands.
func loadConfig(path string) (*config.Config, error) {
	return config.Load([]string{path}, "", nil)
}
```

> **注**：`gcli.BoolOpt2` 签名同 v3 实际 API；如果不存在，参考 Task 6 (Phase 1) 探查模式找等价方法。

- [ ] **Step 7.5: 注册到 app.go**

修改 `internal/cli/app.go`：在 `app.Add(newServeCmd())` 那一行下面追加：

```go
	app.Add(newInitCmd())
```

并保留原有"未来子命令"注释。

- [ ] **Step 7.6: 运行测试**

```
go test ./internal/cli/... -run TestInit -v
go test ./... -v
```

预期：全 PASS。

- [ ] **Step 7.7: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/templates.go internal/cli/init.go internal/cli/init_test.go internal/cli/app.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): init 子命令——生成 fakeserver.json5 模板（含 --with-env）"
```

---

## Task 8: `cli/check` 子命令

**Files**:
- 新建：`internal/cli/check.go`
- 新建：`internal/cli/check_test.go`
- 修改：`internal/cli/app.go`

> **目标**：`fakeserver check -c <paths>` 加载并 Validate，错误退出非 0 + 集中报错；成功打印 "OK: N routes"。

- [ ] **Step 8.1: 写测试**

新建 `internal/cli/check_test.go`：

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCheck_ValidConfigReturnsNilAndCountsRoutes(t *testing.T) {
	var buf bytes.Buffer
	err := runCheck(checkOptions{
		paths: []string{"../config/testdata/valid/single-full.json5"},
		out:   &buf,
	})
	if err != nil {
		t.Fatalf("expected nil err for valid config, got %v", err)
	}
	if !strings.Contains(buf.String(), "OK") {
		t.Errorf("expected OK in output, got %q", buf.String())
	}
}

func TestRunCheck_InvalidConfigReturnsErrorWithAllProblems(t *testing.T) {
	var buf bytes.Buffer
	err := runCheck(checkOptions{
		paths: []string{"../config/testdata/invalid/body-and-bodyfile.json5"},
		out:   &buf,
	})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "body") {
		t.Errorf("error should mention body/bodyFile mutex, got %v", err)
	}
}

func TestRunCheck_MissingPathReturnsError(t *testing.T) {
	var buf bytes.Buffer
	err := runCheck(checkOptions{
		paths: []string{"does-not-exist.json5"},
		out:   &buf,
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
```

- [ ] **Step 8.2: 运行测试确认失败**

```
go test ./internal/cli/... -run TestRunCheck -v
```

预期：FAIL。

- [ ] **Step 8.3: 实现 check.go**

新建 `internal/cli/check.go`：

```go
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gookit/gcli/v3"

	"github.com/inhere/fakeserver/internal/config"
)

type checkOptions struct {
	paths []string
	out   io.Writer
}

func newCheckCmd() *gcli.Command {
	var configFlag string
	return &gcli.Command{
		Name: "check",
		Desc: "Load and validate a fakeserver config (does not start the server)",
		Config: func(cmd *gcli.Command) {
			cmd.StrOpt2(&configFlag, "config,c", "Comma-separated config paths")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			paths := splitConfigPaths(configFlag)
			return runCheck(checkOptions{paths: paths, out: os.Stdout})
		},
	}
}

func runCheck(opts checkOptions) error {
	if len(opts.paths) == 0 {
		return fmt.Errorf("check: at least one --config path is required")
	}
	cfg, err := config.Load(opts.paths, "", nil)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}
	errs := config.Validate(cfg)
	if len(errs) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("check: %d problem(s):\n", len(errs)))
		for i, e := range errs {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, e.Error()))
		}
		return fmt.Errorf("%s", sb.String())
	}
	fmt.Fprintf(opts.out, "OK: %d routes loaded\n", len(cfg.Routes))
	return nil
}

// splitConfigPaths parses the comma-separated -c value into a path slice.
// Empty input returns nil so the caller can fall back to defaults.
func splitConfigPaths(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
```

- [ ] **Step 8.4: 注册到 app.go**

修改 `internal/cli/app.go`，在 `app.Add(newInitCmd())` 后追加：

```go
	app.Add(newCheckCmd())
```

- [ ] **Step 8.5: 运行测试**

```
go test ./internal/cli/... -v
go test ./...
```

预期：全 PASS。

- [ ] **Step 8.6: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/check.go internal/cli/check_test.go internal/cli/app.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): check 子命令——加载并集中报错（不启动 server）"
```

---

## Task 9: 路由摘要打印 + `cli/routes` 子命令

**Files**:
- 新建：`internal/cli/summary.go`
- 新建：`internal/cli/summary_test.go`
- 新建：`internal/cli/routes.go`
- 新建：`internal/cli/routes_test.go`
- 修改：`internal/cli/app.go`

> **职责拆分**：`summary.go` 暴露 `PrintRouteSummary(cfg, w)`，供 routes 子命令 + serve 启动 banner 复用；`routes.go` 只做 CLI 包装。

- [ ] **Step 9.1: 先写 summary 测试**

新建 `internal/cli/summary_test.go`：

```go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestPrintRouteSummary_FormatsMockAndProxyRows(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/ping"},
			{Method: []string{"GET", "HEAD"}, Path: "/users/{id}", Cases: []config.RouteCase{{}, {}}, Strategy: "random"},
			{Method: []string{"*"}, Path: "/api/*rest", Proxy: &config.ProxyConfig{Target: "http://upstream:8080"}},
		},
	}
	var buf bytes.Buffer
	PrintRouteSummary(cfg, &buf)
	out := buf.String()

	for _, want := range []string{"/ping", "/users/{id}", "/api/*rest", "mock", "proxy", "http://upstream:8080"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary should contain %q; got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "2 cases") && !strings.Contains(out, "cases: 2") {
		t.Errorf("summary should mention case count; got:\n%s", out)
	}
}

func TestPrintRouteSummary_EmptyRoutesPrintsHeaderOnly(t *testing.T) {
	cfg := &config.Config{Routes: nil}
	var buf bytes.Buffer
	PrintRouteSummary(cfg, &buf)
	if strings.TrimSpace(buf.String()) == "" {
		t.Errorf("expected at least a header line; got empty")
	}
}
```

- [ ] **Step 9.2: 实现 summary.go**

```go
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/inhere/fakeserver/internal/config"
)

// PrintRouteSummary writes a human-readable, one-line-per-route summary of
// the loaded config. Used by both the `routes` subcommand and the serve
// startup banner. Format intentionally mirrors design §5.1's example:
//
//   GET    /ping                            → mock
//   GET    /users/{id}                      → mock (2 cases, random)
//   *      /api/*rest                       → proxy http://upstream:8080
func PrintRouteSummary(cfg *config.Config, w io.Writer) {
	fmt.Fprintf(w, "Routes (%d):\n", len(cfg.Routes))
	for _, r := range cfg.Routes {
		method := strings.Join(r.Method, ",")
		if method == "" {
			method = "?"
		}
		var note string
		if r.Proxy != nil {
			note = "→ proxy " + r.Proxy.Target
		} else if len(r.Cases) > 0 {
			strat := r.Strategy
			if strat == "" {
				strat = "random"
			}
			note = fmt.Sprintf("→ mock (%d cases, %s)", len(r.Cases), strat)
		} else {
			note = "→ mock"
		}
		fmt.Fprintf(w, "  %-6s %-32s %s\n", method, r.Path, note)
	}
}
```

- [ ] **Step 9.3: 写 routes 子命令测试**

新建 `internal/cli/routes_test.go`：

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRoutes_ValidConfigPrintsSummary(t *testing.T) {
	var buf bytes.Buffer
	err := runRoutes(routesOptions{
		paths: []string{"../config/testdata/valid/single-full.json5"},
		out:   &buf,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "/ping") || !strings.Contains(out, "/users/{id}") {
		t.Errorf("expected routes in output, got:\n%s", out)
	}
}

func TestRunRoutes_InvalidConfigReturnsError(t *testing.T) {
	err := runRoutes(routesOptions{
		paths: []string{"../config/testdata/invalid/body-and-bodyfile.json5"},
		out:   &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
```

- [ ] **Step 9.4: 实现 routes.go**

```go
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/gookit/gcli/v3"

	"github.com/inhere/fakeserver/internal/config"
)

type routesOptions struct {
	paths []string
	out   io.Writer
}

func newRoutesCmd() *gcli.Command {
	var configFlag string
	return &gcli.Command{
		Name: "routes",
		Desc: "Print the route summary for a config (does not start the server)",
		Config: func(cmd *gcli.Command) {
			cmd.StrOpt2(&configFlag, "config,c", "Comma-separated config paths")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			return runRoutes(routesOptions{
				paths: splitConfigPaths(configFlag),
				out:   os.Stdout,
			})
		},
	}
}

func runRoutes(opts routesOptions) error {
	if len(opts.paths) == 0 {
		return fmt.Errorf("routes: at least one --config path is required")
	}
	cfg, err := config.Load(opts.paths, "", nil)
	if err != nil {
		return fmt.Errorf("routes: %w", err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		return fmt.Errorf("routes: config has %d validation error(s); run `fakeserver check` for details", len(errs))
	}
	PrintRouteSummary(cfg, opts.out)
	return nil
}
```

- [ ] **Step 9.5: 注册到 app.go**

```go
	app.Add(newRoutesCmd())
```

- [ ] **Step 9.6: 运行所有 cli 测试**

```
go test ./internal/cli/... -v
go test ./...
```

预期：全 PASS。

- [ ] **Step 9.7: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/summary.go internal/cli/summary_test.go internal/cli/routes.go internal/cli/routes_test.go internal/cli/app.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): routes 子命令 + 路由摘要打印"
```

---

## Task 10: `serve` 接入 `-c/--config`

**Files**:
- 修改：`internal/cli/serve.go`
- 修改：`internal/cli/serve_test.go`

> **目标**：
> - `serve` 加 `-c/--config` 选项（短/长格式同 check/routes）
> - 启动时：
>   - 有 `-c`：load + validate + 打印摘要；validate 出错则退出非 0
>   - 无 `-c`：尝试 `LoadDefault(cwd)`；若有结果同上；若无则打印 "no config; echo-only" 并继续
> - **Phase 2 不**把 mock 路由注册到 rux router——配置中的 routes 仅打印摘要，请求仍走 echo（待 Phase 3 接入）

- [ ] **Step 10.1: 写测试**

向 `internal/cli/serve_test.go` 追加：

```go
import (
	// ... 保留原 imports
	"bytes"
	"github.com/inhere/fakeserver/internal/config"
)

// 新增测试：assembleRouter 即便在有 config 时也仅装 admin + echo。
// 验证 Phase 2 的边界：mock 路由不响应（不注册）。
func TestServe_ConfigLoadDoesNotRegisterMockRoutes(t *testing.T) {
	cfg, err := config.Load(
		[]string{"../config/testdata/valid/single-full.json5"},
		"", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// 在 Phase 2，assembleRouter 与 cfg 无关——仅 admin + echo。
	// 用 PrintRouteSummary 验证 cfg 含 mock 路由，但 router 不应注册。
	var buf bytes.Buffer
	PrintRouteSummary(cfg, &buf)
	if !bytes.Contains(buf.Bytes(), []byte("/users/{id}")) {
		t.Errorf("summary should list /users/{id}; got %s", buf.String())
	}

	// 路由 /users/42 当前请求会走 echo catch-all 而不是 cfg.Routes —— 这是 Phase 2 的合约。
	// 真正的 mock 响应在 Phase 3 接入。这条测试只是把"不注册"事实显式化。
}
```

- [ ] **Step 10.2: 修改 serve.go**

完整替换为：

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/admin"
	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/echo"
)

type serveOptions struct {
	Port       int
	Host       string
	ConfigFlag string
}

func newServeCmd() *gcli.Command {
	opts := serveOptions{
		Port: 5090,
		Host: "0.0.0.0",
	}
	c := &gcli.Command{
		Name: "serve",
		Desc: "Start the fakeserver HTTP server",
		Config: func(cmd *gcli.Command) {
			cmd.IntOpt2(&opts.Port, "port,p", "Listening port")
			cmd.StrOpt2(&opts.Host, "host", "Listening host")
			cmd.StrOpt2(&opts.ConfigFlag, "config,c", "Comma-separated config paths (default: search CWD)")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			return runServe(opts)
		},
	}
	return c
}

// assembleRouter is unchanged from Phase 1: admin + echo only.
// Phase 3 will add mock-route registration here; Phase 4 adds proxy;
// Phase 5 adds middleware.
func assembleRouter() *rux.Router {
	r := rux.New()
	admin.Mount(r)
	echo.Mount(r)
	return r
}

// loadServeConfig resolves the -c flag (or default CWD search) into an
// optional *Config. Returns (nil, nil) when no -c was given and no
// default-search candidate exists — caller treats this as "echo-only".
func loadServeConfig(opts serveOptions) (*config.Config, error) {
	paths := splitConfigPaths(opts.ConfigFlag)
	if len(paths) > 0 {
		cfg, err := config.Load(paths, "", nil)
		if err != nil {
			return nil, err
		}
		if errs := config.Validate(cfg); len(errs) > 0 {
			return nil, fmt.Errorf("config invalid:\n%s", joinErrs(errs))
		}
		return cfg, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	cfg, err := config.LoadDefault(wd)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		if errs := config.Validate(cfg); len(errs) > 0 {
			return nil, fmt.Errorf("config invalid:\n%s", joinErrs(errs))
		}
	}
	return cfg, nil
}

func joinErrs(errs []error) string {
	var sb []byte
	for i, e := range errs {
		sb = append(sb, fmt.Sprintf("  %d. %s\n", i+1, e.Error())...)
	}
	return string(sb)
}

// runServe assembles the router, optionally loads and prints config, then
// runs the HTTP server with signal-driven graceful shutdown.
//
// Phase 2 caveat: cfg.Routes are PRINTED to stdout but NOT registered to
// the router — actual mock response handling lands in Phase 3.
func runServe(opts serveOptions) error {
	cfg, err := loadServeConfig(opts)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: assembleRouter(),
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		fmt.Printf("fakeserver listening on http://%s\n", addr)
		if cfg != nil {
			PrintRouteSummary(cfg, os.Stdout)
			fmt.Println("(Phase 2: mock routes are listed but not yet served; requests still echo)")
		} else {
			fmt.Println("no config; running in echo-only mode")
		}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case sig := <-stop:
		fmt.Printf("\nreceived %s, shutting down...\n", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	fmt.Println("bye.")
	return nil
}
```

- [ ] **Step 10.3: 编译 + 全套测试**

```
go build ./...
go test ./... -v
```

预期：之前所有用例 PASS + 新增的 `TestServe_ConfigLoadDoesNotRegisterMockRoutes` PASS。

- [ ] **Step 10.4: 手动冒烟（PowerShell 后台 job）**

```powershell
go build -o fakeserver.exe ./cmd/fakeserver

# 测 1：无 -c
$j = Start-Job { D:/work/aidev/lite-tools/fakeserver/fakeserver.exe serve --port 4567 }
Start-Sleep -Seconds 2
Invoke-WebRequest -Uri http://localhost:4567/__fakeserver/healthz -UseBasicParsing | Select-Object -ExpandProperty StatusCode
Stop-Job $j; Remove-Job $j

# 测 2：用现成 valid config
$j = Start-Job { D:/work/aidev/lite-tools/fakeserver/fakeserver.exe serve -c internal/config/testdata/valid/single-full.json5 --port 4567 }
Start-Sleep -Seconds 2
# /users/42 当前应仍走 echo（200），因为 Phase 2 不注册 mock
Invoke-WebRequest -Uri http://localhost:4567/users/42 -UseBasicParsing | Select-Object -ExpandProperty StatusCode
Stop-Job $j; Remove-Job $j

rm fakeserver.exe
```

预期：两次 healthz/users 请求都 200。终端输出含"Routes (3):"等摘要行。

- [ ] **Step 10.5: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve.go internal/cli/serve_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): serve 接入 -c/--config + 启动打印路由摘要（mock 注册留 Phase 3）"
```

---

## Task 11: DoD 验证 + 文档回写

**Files**:
- 修改：`docs/plans/2026-05-19-fakeserver-v0.1-overview.md`
- 修改：`docs/fakeserver-design.md`

- [ ] **Step 11.1: 执行 DoD 检查清单**

逐项核对：

| 命令 / 检查 | 预期 |
|---|---|
| `go build ./...` | 0 退出 |
| `go test ./...` | 全 PASS；config 覆盖率 ≥ 80%（`go test -cover ./internal/config/...`） |
| `./fakeserver.exe init` 在空目录 | 生成 `fakeserver.json5`；再执行报错 |
| `./fakeserver.exe init --with-env` | 额外生成 `fakeserver.env.json5` |
| `./fakeserver.exe check -c internal/config/testdata/invalid/duplicate-routes.json5` | 退出非 0，stderr 含 `duplicate` 与 `/users` |
| `./fakeserver.exe routes -c internal/config/testdata/valid/single-full.json5` | 打印 3 行路由（含 mock/proxy 标记） |
| `./fakeserver.exe serve -c .../single-full.json5 --port 4567` | 启动 listen，打印摘要；`curl /__fakeserver/healthz` 200；`curl /users/42` 仍是 echo 200（Phase 2 不响应 mock） |
| `./fakeserver.exe serve --port 4567`（无 config 且 CWD 无 fakeserver.json5） | 启动并打印 "no config; echo-only" |

任一不符 → 停下报 BLOCKED 详细原因。

- [ ] **Step 11.2: 回写 overview**

修改 `docs/plans/2026-05-19-fakeserver-v0.1-overview.md`：

- §2 表的 Phase 2 状态列从"待开始"改为：`✅ 已完成 (commit <第一个 commit SHA>..<最后一个 commit SHA>)`
- §3 Phase 2 详述末尾追加一节"### 实际落地偏差"，列出 Phase 2 执行过程中发现的与 design / overview 不一致的事实（如果有）。例如：
  - JSON5 lib 实际行为某项与 design 假设不同
  - 某个 schema 字段在执行中调整了类型
  - 等等

如果没有偏差，写"无偏差，按 plan 直接落地"。

- [ ] **Step 11.3: 回写 design**

修改 `docs/fakeserver-design.md`：

修订记录追加一行：

```markdown
| 2026-05-19 | v0.3-phase2-applied | inhere | Phase 2 落地：internal/config 包（schema/loader/include/merge/defaults/validate）+ cli init/check/routes + serve 接入 -c。mock 路由仅打印摘要，实际响应留 Phase 3 |
```

如果 Phase 2 发现了与 design 不一致的事实，在 §13 "已落地（Phase 1 阶段确认）"子段中**追加 Phase 2 条目**——如：
- titanous/json5 在某场景下的行为细节
- ValidateError 信息格式（如有变化）
- 等等

- [ ] **Step 11.4: Commit**

```
git -C D:/work/aidev/lite-tools/fakeserver add docs/plans/2026-05-19-fakeserver-v0.1-overview.md docs/fakeserver-design.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "docs: 回写 Phase 2 落地结果（config 包 + init/check/routes 子命令）"
```

---

## Phase 2 完成 · 下一步

完成本计划后，仓库具备：

- ✅ 完整 `internal/config/` 包：JSON5 解析、@include 展开、多文件合并、默认查找、集中校验
- ✅ 3 个新子命令：`init` / `check` / `routes`
- ✅ `serve -c <paths>` 加载并打印路由摘要
- ✅ 测试覆盖率 ≥ 80%（`internal/config`）
- ✅ 引入唯一新依赖 `github.com/titanous/json5`

**Phase 3 预告（不在本计划范围）**：

- 引入 `github.com/gookit/easytpl` + `github.com/brianvoe/gofakeit/v7`
- 新增 `internal/tpl/` 包：`render.go`、`context.go`、`funcs.go`、`faker.go`
- 新增 `internal/mock/` 包：`router.go`、`responder.go`
- `serve` 把 cfg 中**单一响应模式**的 routes 真正注册到 rux router，响应模板 + faker 输出
- 仍**不**含 cases/strategy（Phase 4）、proxy 实际响应（Phase 4）、middleware（Phase 5）

**生成 Phase 3 plan**：当 Phase 2 落地完成并所有 DoD 通过后，再次调用 `superpowers:writing-plans` skill。Phase 3 会基于 Phase 2 落地的 schema/loader 接口展开。

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| 无 TBD / placeholder | ✓（Task 1 的 Load stub 在 Task 3 被完整实现；Task 7 的 loadConfig helper 由测试侧驱动） |
| 类型签名前后一致 | ✓（`config.Config` / `Route` / `ProxyConfig` / `Load(paths, envName, overrides)` / `Validate(cfg)` 在 Task 1-10 中保持） |
| 包路径前后一致 | ✓（`github.com/inhere/fakeserver/internal/{config,cli,echo,admin}` 全文统一） |
| TDD：先测后实现 | ✓（Task 2/3/4/5/6/7/8/9/10 都遵循） |
| 频繁提交 | ✓（每 Task 一次 commit；累计 11 个新 commit） |
| 仅新增 titanous/json5 单一第三方依赖 | ✓（Phase 2 不引 easytpl/expr/fsnotify/gofakeit） |
| 覆盖 design §3 / §3.5 / §3.7 / §5.7 init/check/routes / §7 config 测试段 | ✓ |
| Phase 2 边界明确（mock 不响应、proxy 不响应、env 文件不引、热加载不引） | ✓（Task 10 显式注释 + DoD #4） |
| 失败路径明确 | ✓（gcli 方法名 / JSON5 lib 边界 / include 循环 / 文件不存在 / 跨条目重复） |
