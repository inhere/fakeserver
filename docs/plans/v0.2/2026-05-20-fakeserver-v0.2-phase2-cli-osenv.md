# Fakeserver v0.2 · Phase 2 — CLI 整合 + osenv 白名单 + env 文件 hot-reload

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：让 `fakeserver serve --env <name> --var k=v` 真正影响渲染——`.env` 在 mock body 模板里可访问、`osenv` 白名单覆盖 env 文件中的调用、env 文件改动通过现有 watcher 触发 holder swap。

**Architecture**：

1. **CLI 层**（Task 1）：`serveOptions` 增 `EnvName / VarOverrides` 字段 + gcli 注册 `-e/--env` 和 `--var key=val`；`FAKESERVER_ENV` 环境变量作为 fallback。
2. **Load 层**（Task 2-4）：`config.Load(paths, envName, overrides)` 的 envName / overrides 参数从 v0.1 reserved 状态进入实际使用。loader.Load 调 LoadEnvFile 传 envName；LoadEnvFile 返回后用 tpl.RenderEnvValues 渲染值层级模板；最后 deepMerge --var 覆盖。
3. **tpl 层**（Task 5）：`BuildRenderCtx` 增加 env 参数，把 cfg.Env 接到 `.env` 键；osenv 函数复用 v0.1 Phase 3 已实现的白名单逻辑（**已就位**），env 文件值渲染时传入相同 osenvWhitelist。
4. **watcher 层**（Task 5）：loader.Load 把 cfg.EnvSource 追加到 cfg.SourcePaths，watcher 自动监听 env 文件。

**Tech Stack**：Go 1.26+ · 复用 v0.1 全部依赖（无新增第三方包）· `text/template` 渲染 env 值。

**前置要求**：

- v0.2 Phase 1 完成（commit `c4b4dbb`，`cfg.Env / cfg.EnvSource` 字段就位）
- 已读 design §8.3（优先级链）/ §8.2 末尾（env 文件值层级模板渲染） / §8.5（hot-reload） / §4.6（osenv 白名单）
- 已读 [overview](../2026-05-20-fakeserver-v0.2-overview.md) §3 Phase 2 详述

**Phase 2 完成定义（DoD）**：

1. `serve --env staging` → `.env` 反映 staging 段值
2. `FAKESERVER_ENV=staging serve`（无 `--env` flag）→ 等价 `--env staging`
3. `--env dev --var apiHost=overridden` → `.env.apiHost == "overridden"`
4. `--var a=1 --var b=2`（多次 flag）→ 两个 key 都生效
5. `--var a=1,b=2`（逗号分隔单次 flag）→ 两个 key 都生效
6. env 文件中 `"token": "{{ osenv \"X\" }}"` 在 `osenvWhitelist` 包含 X 时生效；不包含时输出 `""`
7. 主配置 route body 中 `{{ .env.token }}` 在 staging 段时输出 staging.token
8. 编辑 env 文件 → 300ms 防抖 → holder swap → 下一个请求看到新 env 值；env 文件被删除 → 保留旧 cfg + stderr warn
9. `go test ./...` 通过；`internal/tpl` osenv 路径覆盖率 ≥ 80%；`internal/cli` 覆盖率维持

---

## 文件结构（Phase 2 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `internal/cli/serve.go` | serveOptions + flag + FAKESERVER_ENV + --var 解析 + loadServeConfig 签名 |
| 修改 | `internal/cli/serve_test.go` | --env / --var / FAKESERVER_ENV 集成测试 |
| 修改 | `internal/config/loader.go` | Load 用 envName / overrides；调 RenderEnvValues；EnvSource 追加到 SourcePaths |
| 修改 | `internal/config/loader_test.go` | envName 集成 + --var 覆盖 + watcher SourcePaths |
| 新建 | `internal/tpl/envrender.go` | `RenderEnvValues(env, osenvWhitelist, globals) error` — env 值层级渲染 |
| 新建 | `internal/tpl/envrender_test.go` | osenv 白名单约束 + 嵌套 map / slice 字符串渲染 |
| 修改 | `internal/tpl/context.go` | `BuildRenderCtx` 增 `envMap` 参数；`.env` 键改取入参 |
| 修改 | `internal/tpl/context_test.go` | envMap 接入测试 |
| 修改 | `internal/mock/responder.go` | `BuildRenderCtx` 调用站点传 cfg.Env（route 闭包持 cfg 引用，需小重构）|
| 修改 | `internal/mock/cases.go` | 同上 |
| 修改 | `internal/proxy/proxy.go` | `buildProxyRenderCtx` 增 envMap 参数 |
| 修改 | `docs/plans/2026-05-20-fakeserver-v0.2-overview.md` | Phase 2 状态回写 + 实际落地偏差 |
| 修改 | `docs/fakeserver-design.md` | 修订记录 + §13 v0.2 Phase 2 阶段确认（**严格 3 条事实**）|

> **重要约束**：design.md 回写按 v0.2 overview §5 新规约（v0.1 教训）——**3 条事实概括**，无 commit SHA、无内部函数名、无测试覆盖率数字。

---

## Task 1: CLI flag 注册 + FAKESERVER_ENV + --var 解析骨架

**Files**:
- 修改：`internal/cli/serve.go`
- 修改：`internal/cli/serve_test.go`

### Step 1.1: serveOptions 扩展

修改 `serveOptions` 结构体（在已有字段后追加）：

```go
type serveOptions struct {
	Port       int
	Host       string
	ConfigFlag string
	Quiet      bool
	NoCORS     bool
	NoWatch    bool
	// v0.2 Phase 2 新增
	EnvName      string   // --env / -e <name>; "" → fallback FAKESERVER_ENV → file $active → first segment
	VarOverrides []string // --var key=val（多次出现累加，单次支持 "k=v,k=v" CSV）
}
```

### Step 1.2: flag 注册

在 `newServeCmd` 的 `Config` 闭包内追加：

```go
		cmd.StrOpt2(&opts.EnvName, "env,e", "Environment segment name (override env file $active and FAKESERVER_ENV)")
		cmd.StrsOpt2(&opts.VarOverrides, "var", "Variable override key=val (repeatable; comma-separated allowed)")
```

> **注**：`gcli/v3` 的 `StrsOpt2` 接受 `*[]string`，多次出现累加。若 gcli 实际 API 是 `StrOpt2` 多次出现覆盖，则改用切片字段 + Func 内手动解析 args；以实测为准。

### Step 1.3: 实施 FAKESERVER_ENV fallback

在 `runServe` 内、`loadServeConfig` 调用前，处理 envName fallback：

```go
	// FAKESERVER_ENV fallback (design §8.3 优先级 2)
	if opts.EnvName == "" {
		opts.EnvName = os.Getenv("FAKESERVER_ENV")
	}
```

### Step 1.4: --var 解析助手

在 `internal/cli/serve.go` 末尾新增辅助函数（**仅解析，不执行**——Task 3 才接入 cfg.Env）：

```go
// parseVarOverrides converts ["a=1,b=2", "c=3"] → map[string]string{"a":"1","b":"2","c":"3"}.
// Supports both repeated flag (each entry one key) AND comma-separated single
// flag (k1=v1,k2=v2,...). Empty entries and malformed (no "=") are skipped
// with a stderr warn.
func parseVarOverrides(raw []string) map[string]string {
	out := map[string]string{}
	for _, entry := range raw {
		for _, pair := range strings.Split(entry, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			idx := strings.Index(pair, "=")
			if idx < 0 {
				fmt.Fprintf(os.Stderr, "warn: --var %q: missing '=' separator; skipped\n", pair)
				continue
			}
			out[strings.TrimSpace(pair[:idx])] = strings.TrimSpace(pair[idx+1:])
		}
	}
	return out
}
```

### Step 1.5: 测试

把以下追加到 `internal/cli/serve_test.go`：

```go
func TestParseVarOverrides_SingleFlag(t *testing.T) {
	got := parseVarOverrides([]string{"a=1"})
	if got["a"] != "1" {
		t.Errorf("a=%q want 1", got["a"])
	}
}

func TestParseVarOverrides_CommaSeparated(t *testing.T) {
	got := parseVarOverrides([]string{"a=1,b=2"})
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %v", got)
	}
}

func TestParseVarOverrides_MultipleFlag(t *testing.T) {
	got := parseVarOverrides([]string{"a=1", "b=2"})
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %v", got)
	}
}

func TestParseVarOverrides_MixedCSVAndMultiple(t *testing.T) {
	got := parseVarOverrides([]string{"a=1,b=2", "c=3"})
	if got["a"] != "1" || got["b"] != "2" || got["c"] != "3" {
		t.Errorf("got %v", got)
	}
}

func TestParseVarOverrides_MalformedSkipped(t *testing.T) {
	got := parseVarOverrides([]string{"a=1", "no-equals", "b=2"})
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("expected a/b only; got %v", got)
	}
	if _, has := got["no-equals"]; has {
		t.Error("malformed entry should be skipped")
	}
}
```

### Step 1.6: 跑测试 + Commit

```
cd D:/work/aidev/lite-tools/fakeserver
go build ./...
go test ./internal/cli/... -v -run "TestParseVarOverrides"
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve.go internal/cli/serve_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): --env/--var flag + FAKESERVER_ENV + parseVarOverrides 解析"
```

---

## Task 2: loader.Load 接入 envName + watcher 监听 env 文件

**Files**:
- 修改：`internal/config/loader.go`
- 修改：`internal/config/loader_test.go`
- 修改：`internal/cli/serve.go`（loadServeConfig 传 envName）

### Step 2.1: loader.Load 用 envName 参数

`config.Load(paths, envName, overrides)` 的签名 v0.1 已保留 envName 参数但未使用。改 Phase 1 集成段：

```go
	// v0.2 Phase 1: load env file (Phase 2 wires envName + overrides)
	if len(absSources) > 0 {
		envPath := filepath.Join(filepath.Dir(absSources[0]), DefaultEnvFileName)
		envMap, _, eerr := LoadEnvFile(envPath, envName) // ← envName 替代 ""
		if eerr != nil {
			return nil, fmt.Errorf("env file: %w", eerr)
		}
		cfg.Env = envMap
		if _, sterr := os.Stat(envPath); sterr == nil {
			cfg.EnvSource = envPath
			cfg.SourcePaths = append(cfg.SourcePaths, envPath) // ← 加入 watcher 监听列表
		}
	}
```

### Step 2.2: cli/serve.go loadServeConfig 传 envName

```go
func loadServeConfig(opts serveOptions) (*config.Config, error) {
	paths := splitConfigPaths(opts.ConfigFlag)
	if len(paths) > 0 {
		cfg, err := config.Load(paths, opts.EnvName, nil) // ← opts.EnvName
		// ...同 v0.1
	}
	// ...
	cfg, err := config.LoadDefault(wd, opts.EnvName) // ← 同样传递；需调整 LoadDefault 签名
	// ...
}
```

**注**：`LoadDefault` 当前签名 `LoadDefault(cwd) (*Config, error)`，需扩展为 `LoadDefault(cwd, envName) (*Config, error)`。

### Step 2.3: 集成测试

追加到 `internal/config/loader_test.go`：

```go
func TestLoad_EnvName_SelectsSegment(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	envPath := filepath.Join(tmp, DefaultEnvFileName)
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ $default: { host: "d" }, dev: { token: "DEV" }, staging: { token: "STG" } }`), 0644)

	cfg, err := Load([]string{cfgPath}, "staging", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env["token"] != "STG" {
		t.Errorf("env.token=%v want STG", cfg.Env["token"])
	}
	if cfg.Env["host"] != "d" {
		t.Errorf("env.host=%v want d (inherited from $default)", cfg.Env["host"])
	}
}

func TestLoad_EnvFile_AddedToSourcePaths(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	envPath := filepath.Join(tmp, DefaultEnvFileName)
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ dev: { x: 1 } }`), 0644)

	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range cfg.SourcePaths {
		if filepath.Clean(p) == filepath.Clean(envPath) {
			found = true
		}
	}
	if !found {
		t.Errorf("env path not in SourcePaths: %v", cfg.SourcePaths)
	}
}
```

### Step 2.4: 跑测试 + Commit

```
go test ./internal/config/... -v -run "TestLoad_EnvName_SelectsSegment|TestLoad_EnvFile_AddedToSourcePaths"
go test ./... -count=1
git add internal/config/loader.go internal/config/loader_test.go internal/cli/serve.go
git commit -m "feat(config,cli): loader 接入 envName + EnvSource 加入 SourcePaths"
```

---

## Task 3: --var deep merge 进 cfg.Env

**Files**:
- 修改：`internal/config/loader.go`
- 修改：`internal/cli/serve.go`
- 修改：`internal/config/loader_test.go`

### Step 3.1: loader.Load overrides 参数接入

```go
	// 在 cfg.Env 赋值后、applyDefaults 前：
	if len(overrides) > 0 {
		for k, v := range overrides {
			cfg.Env[k] = v // 顶层覆盖
		}
	}
```

> **设计决策**：`--var key=val` 只覆盖顶层 key（不支持 dotted path 如 `a.b.c=val`）。简化语义，未来 v0.3 可扩展。

### Step 3.2: cli/serve.go 把 VarOverrides 传给 Load

```go
func loadServeConfig(opts serveOptions) (*config.Config, error) {
	overrides := parseVarOverrides(opts.VarOverrides)
	// ...
	cfg, err := config.Load(paths, opts.EnvName, overrides)
```

### Step 3.3: 测试

```go
func TestLoad_VarOverride_TopLevelKeys(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	envPath := filepath.Join(tmp, DefaultEnvFileName)
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ dev: { apiHost: "from-file", token: "T" } }`), 0644)

	cfg, err := Load([]string{cfgPath}, "dev", map[string]string{"apiHost": "OVERRIDE"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env["apiHost"] != "OVERRIDE" {
		t.Errorf("apiHost=%v want OVERRIDE (--var 应覆盖 env file)", cfg.Env["apiHost"])
	}
	if cfg.Env["token"] != "T" {
		t.Errorf("token=%v want T (未被 --var 覆盖)", cfg.Env["token"])
	}
}
```

### Step 3.4: Commit

```
git add internal/config/loader.go internal/config/loader_test.go internal/cli/serve.go
git commit -m "feat(config,cli): --var overrides deep merge 进 cfg.Env"
```

---

## Task 4: env 文件值层级模板渲染（osenv 等）

**Files**:
- 新建：`internal/tpl/envrender.go`
- 新建：`internal/tpl/envrender_test.go`
- 修改：`internal/config/loader.go`（调用 RenderEnvValues）

### Step 4.1: 测试驱动

新建 `internal/tpl/envrender_test.go`：

```go
package tpl

import (
	"os"
	"testing"
)

func TestRenderEnvValues_OsenvAllowed(t *testing.T) {
	os.Setenv("FAKESERVER_T_KEY", "secret")
	defer os.Unsetenv("FAKESERVER_T_KEY")

	env := map[string]any{
		"token": `{{ osenv "FAKESERVER_T_KEY" }}`,
	}
	if err := RenderEnvValues(env, []string{"FAKESERVER_T_KEY"}, nil); err != nil {
		t.Fatal(err)
	}
	if env["token"] != "secret" {
		t.Errorf("token=%v want 'secret' (osenv in whitelist)", env["token"])
	}
}

func TestRenderEnvValues_OsenvBlockedByWhitelist(t *testing.T) {
	os.Setenv("FAKESERVER_T_BLOCKED", "topsecret")
	defer os.Unsetenv("FAKESERVER_T_BLOCKED")

	env := map[string]any{
		"token": `{{ osenv "FAKESERVER_T_BLOCKED" }}`,
	}
	// whitelist 不含此 key
	if err := RenderEnvValues(env, []string{"OTHER_KEY"}, nil); err != nil {
		t.Fatal(err)
	}
	if env["token"] != "" {
		t.Errorf("token=%q want empty (osenv blocked by whitelist)", env["token"])
	}
}

func TestRenderEnvValues_NestedMapAndSlice(t *testing.T) {
	env := map[string]any{
		"plain":  "no-template",
		"templ":  `{{ "rendered" }}`,
		"nested": map[string]any{"k": `{{ "deep" }}`},
		"list":   []any{`{{ "first" }}`, "literal"},
	}
	if err := RenderEnvValues(env, nil, nil); err != nil {
		t.Fatal(err)
	}
	if env["templ"] != "rendered" {
		t.Errorf("templ=%v", env["templ"])
	}
	if nested := env["nested"].(map[string]any); nested["k"] != "deep" {
		t.Errorf("nested.k=%v", nested["k"])
	}
	if list := env["list"].([]any); list[0] != "first" || list[1] != "literal" {
		t.Errorf("list=%v", list)
	}
}

func TestRenderEnvValues_NoTemplateInNonString(t *testing.T) {
	env := map[string]any{
		"num":  42,
		"bool": true,
	}
	if err := RenderEnvValues(env, nil, nil); err != nil {
		t.Fatal(err)
	}
	if env["num"] != 42 || env["bool"] != true {
		t.Errorf("non-string values must pass through unchanged")
	}
}
```

### Step 4.2: 实现 envrender.go

```go
package tpl

import (
	"bytes"
	"fmt"
	"text/template"
)

// RenderEnvValues walks env (a map decoded from JSON5) and renders every
// string leaf as a text/template using fakeserver's BaseFuncMap. The same
// osenvWhitelist that gates {{ osenv }} in mock body templates also gates
// it here (design §4.6: env file is not an escape hatch).
//
// globals is exposed as ".config" inside env templates (matches BuildRenderCtx
// shape). nil is treated as empty map.
//
// Mutates env in place. Returns the first render error encountered.
func RenderEnvValues(env map[string]any, osenvWhitelist []string, globals map[string]any) error {
	funcs := BaseFuncMap(osenvWhitelist)
	ctx := map[string]any{
		"config": globals,
		// .env intentionally NOT exposed here — env values shouldn't
		// reference each other (Phase 2 v0.2 keeps simple; v0.3 may
		// allow cross-key references via topo-sort).
	}
	return renderEnvNode(env, funcs, ctx)
}

func renderEnvNode(node any, funcs template.FuncMap, ctx map[string]any) error {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if s, ok := child.(string); ok {
				rendered, err := renderString(s, funcs, ctx)
				if err != nil {
					return fmt.Errorf("env value at key %q: %w", k, err)
				}
				v[k] = rendered
			} else {
				if err := renderEnvNode(child, funcs, ctx); err != nil {
					return fmt.Errorf("env at key %q: %w", k, err)
				}
			}
		}
	case []any:
		for i, child := range v {
			if s, ok := child.(string); ok {
				rendered, err := renderString(s, funcs, ctx)
				if err != nil {
					return fmt.Errorf("env value at index %d: %w", i, err)
				}
				v[i] = rendered
			} else {
				if err := renderEnvNode(child, funcs, ctx); err != nil {
					return fmt.Errorf("env at index %d: %w", i, err)
				}
			}
		}
	}
	return nil
}

func renderString(src string, funcs template.FuncMap, ctx map[string]any) (string, error) {
	tpl, err := template.New("env").Funcs(funcs).Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
```

### Step 4.3: loader.Load 调用 RenderEnvValues

在 `loader.go` 中 `cfg.Env = envMap` 之后、`overrides` 处理之前插入：

```go
		// v0.2 Phase 2: render env values (osenv etc.) before overrides
		if rerr := tpl.RenderEnvValues(cfg.Env, cfg.Server.OSEnvWhitelist, cfg.Globals); rerr != nil {
			return nil, fmt.Errorf("env file: render values: %w", rerr)
		}
```

> **注**：此处 `cfg.Server.OSEnvWhitelist` 必须在 `applyDefaults` 之前可读——schema 默认值已让 nil whitelist 表示"放行所有"，无需调整。

需要在 loader.go 加 import `"github.com/inhere/fakeserver/internal/tpl"`。

### Step 4.4: 跑测试 + Commit

```
go test ./internal/tpl/... -v -run "TestRenderEnvValues_"
go test ./... -count=1
git add internal/tpl/envrender.go internal/tpl/envrender_test.go internal/config/loader.go
git commit -m "feat(tpl,config): env 文件值层级渲染 + osenv 白名单贯通"
```

---

## Task 5: tpl.BuildRenderCtx 接 cfg.Env → `.env`

**Files**:
- 修改：`internal/tpl/context.go`
- 修改：`internal/tpl/context_test.go`
- 修改：`internal/mock/responder.go`
- 修改：`internal/mock/cases.go`
- 修改：`internal/proxy/proxy.go`

### Step 5.1: 改 BuildRenderCtx 签名

```go
// BuildRenderCtx now accepts envMap (v0.2 Phase 2). v0.1 callers passed nil
// for env via a placeholder; v0.2 wires cfg.Env through.
func BuildRenderCtx(req *http.Request, params map[string]string, globals, envMap map[string]any) map[string]any {
	// ... 现有逻辑
	return map[string]any{
		"request": requestMap,
		"now":     time.Now(),
		"env":     envMap,            // ← v0.1 是 map[string]any{}，v0.2 改用入参
		"osenv":   map[string]string{},
		"config":  globals,
	}
}
```

如 envMap 为 nil，仍写空 map：

```go
	if envMap == nil {
		envMap = map[string]any{}
	}
```

### Step 5.2: 更新所有调用站点

- `internal/mock/responder.go`：`Respond(c, route, renderer)` 内部 `tpl.BuildRenderCtx(req, params, nil)` → `BuildRenderCtx(req, params, route.<cfg ref>?, route.<env ref>?)`。

  **问题**：route 不持 cfg/env 引用。最小改法：让 `Respond` 多收一个 envMap 参数，由调用方（router.go 内的 Mount 闭包）传入。同理 `RespondCases`。Mount 已有 cfg 引用，可在闭包内捕获 cfg.Env。

  改 `mock.Mount` 闭包：
  ```go
  handler = func(c *rux.Context) {
      Respond(c, route, renderer, cfg.Env) // 多传一参
  }
  ```

  和 cases handler 同样：
  ```go
  handler = func(c *rux.Context) {
      RespondCases(c, route, matchers, selector, renderer, cfg.Env)
  }
  ```

  `Respond` / `RespondCases` 函数签名扩展，内部 `tpl.BuildRenderCtx` 传 envMap。

- `internal/proxy/proxy.go`：`buildProxyRenderCtx(req, envMap)` 增 envMap 参数；`Build(route, renderer, envMap)` 也增；`Mount` 闭包传 cfg.Env。

### Step 5.3: 测试

`internal/tpl/context_test.go` 现有 `TestBuildRenderCtx_BasicFields` 调用 `BuildRenderCtx(req, params, globals)` 三参。改成四参（最后一个 envMap）。新增测试：

```go
func TestBuildRenderCtx_EnvAccessible(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	ctx := BuildRenderCtx(req, nil, nil, map[string]any{"token": "T"})
	env, ok := ctx["env"].(map[string]any)
	if !ok {
		t.Fatalf(".env not a map: %T", ctx["env"])
	}
	if env["token"] != "T" {
		t.Errorf(".env.token=%v want T", env["token"])
	}
}

func TestBuildRenderCtx_NilEnvBecomesEmptyMap(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	ctx := BuildRenderCtx(req, nil, nil, nil)
	env, ok := ctx["env"].(map[string]any)
	if !ok || env == nil {
		t.Errorf(".env should be non-nil empty map; got %v", ctx["env"])
	}
}
```

新增 mock 集成测试（`internal/mock/cases_test.go` 或 `responder_test.go`），用 cfg.Env 写一个 route body 模板 `{{ .env.token }}` 验证返回。

### Step 5.4: 跑全套测试 + Commit

```
go test ./... -count=1
git add internal/tpl/context.go internal/tpl/context_test.go internal/mock/responder.go internal/mock/cases.go internal/mock/router.go internal/proxy/proxy.go
git commit -m "feat(tpl,mock,proxy): BuildRenderCtx 接入 cfg.Env → 模板 .env 可访问"
```

---

## Task 6: Phase 2 收尾 — DoD 核对 + 文档回写

**Files**:
- 修改：`docs/plans/2026-05-20-fakeserver-v0.2-overview.md`
- 修改：`docs/fakeserver-design.md`

### Step 6.1: 综合 hot-reload 集成测试

`internal/cli/serve_e2e_test.go` 追加：

```go
func TestServe_v02_EnvFile_HotReload(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	envPath := filepath.Join(tmp, "fakeserver.env.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ routes: [{ method: "GET", path: "/t", body: "{{ .env.token }}" }] }`), 0644)
	_ = os.WriteFile(envPath, []byte(`{ dev: { token: "DEV" } }`), 0644)

	cfg, err := config.Load([]string{cfgPath}, "dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("%v", errs)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	holder := newHolderWithWatcher(t, cfg, rdr, serveOptions{Quiet: true, NoCORS: true}, cfgPath)
	srv := httptest.NewServer(holder)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/t")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "DEV" {
		t.Errorf("initial body=%q want DEV", string(b))
	}

	// 修改 env 文件，触发 reload
	_ = os.WriteFile(envPath, []byte(`{ dev: { token: "DEV2" } }`), 0644)
	if !waitForRouteBody(t, srv.URL+"/t", "DEV2", 2*time.Second) {
		t.Fatal("new env value never appeared after edit")
	}
}
```

> **可能的偏差**：`newHolderWithWatcher` 当前在 cli 包内是测试辅助函数，签名可能需要扩展为接收 envName / overrides。按实际情况调整。

### Step 6.2: 全套测试 + DoD 核对

```
go build ./...
go test ./... -count=1
go test -cover ./internal/{tpl,cli,config}/...
go vet ./...
```

DoD 9 项核对（按 overview §3 Phase 2）：

| # | 检查 | 测试用例 |
|---|---|---|
| 1 | `--env staging` → .env 反映 | `TestLoad_EnvName_SelectsSegment` |
| 2 | `FAKESERVER_ENV` 等价 | Task 1 集成测试 |
| 3 | `--env dev --var apiHost=X` | `TestLoad_VarOverride_TopLevelKeys` |
| 4 | 多次 `--var` 累加 | `TestParseVarOverrides_MultipleFlag` |
| 5 | csv `--var a=1,b=2` | `TestParseVarOverrides_CommaSeparated` |
| 6 | osenv 白名单覆盖 env 文件 | `TestRenderEnvValues_OsenvBlockedByWhitelist` |
| 7 | mock body `{{ .env.token }}` | Task 5 mock 集成 |
| 8 | env 文件 hot-reload | `TestServe_v02_EnvFile_HotReload` |
| 9 | 覆盖率 + 全包绿 | `go test -cover` 输出 |

### Step 6.3: 回写 overview

修改 `docs/plans/2026-05-20-fakeserver-v0.2-overview.md`：

§2 表 Phase 2 行状态列：`待开始` → `✅ 已完成 (commit <first SHA>..<last SHA>)`

§3 Phase 2 末尾追加：

```markdown
**实际落地偏差**：

- **`--var` 仅支持顶层 key 覆盖**：design §8.2 未明确 dotted path 语义；Phase 2 简化为顶层 deepMerge 替换。v0.3 可扩展。
- **env 值层级模板渲染时不暴露 `.env` 自引用**：避免循环引用复杂度。env 值只能引用 `osenv` / `now` / `uuid` 等，不能 `{{ .env.other_key }}`。design §8.4 未明确禁止但实际场景未要求；如未来用户需要再加 topo-sort 解析。
- **BuildRenderCtx 签名扩展为 4 参（添加 envMap）**：v0.1 占位的空 `.env` map 改由调用方传入；mock / proxy 包的 Mount 闭包同步扩展 handler 签名以传 cfg.Env。
- **env 文件路径加入 `cfg.SourcePaths` 而非独立字段**：watcher 直接读 SourcePaths，无需改 watcher API。

**Phase 2 测试覆盖**：<填实测>；`internal/tpl` osenv 路径覆盖率 <X>%；`internal/cli` 维持。

**Phase 2 commit 流水**：<填入 Task 1-6 实际 SHA>
```

### Step 6.4: 回写 design.md（严格 3 条事实）

修订记录追加：

```markdown
| 2026-05-20 | v0.4-phase0.2.2-applied | inhere | v0.2 Phase 2：env 选段 CLI/env-var 优先级链完整 + env 值模板渲染（osenv 白名单贯通）+ mock 模板 .env 接入 + env 文件 hot-reload |
```

§13 追加：

```markdown
### 已落地（v0.2 Phase 2 阶段确认）

1. **env 选段优先级链完整化**：`--env` CLI > `FAKESERVER_ENV` 环境变量 > 文件 `$active` > 首个非 `$default` 段 > 空。`--var key=val` 顶层 deepMerge 覆盖在最后。
2. **env 文件值层级模板渲染**：env 段的字符串叶子节点在 Load 阶段渲染（`{{ osenv "X" }}` / `{{ now }}` 等可用）；osenv 白名单在 env 文件中**同样生效**，env 文件不是逃逸通道。
3. **env 文件参与 hot-reload**：env 文件路径自动加入 watcher 监听列表，编辑后经 300ms 防抖触发 holder swap；mock body 模板 `{{ .env.* }}` 真正可访问。
```

### Step 6.5: Commit + bd close

```
git add docs/plans/2026-05-20-fakeserver-v0.2-overview.md docs/fakeserver-design.md
git commit -m "docs(v0.2): 回写 Phase 2 落地——CLI/env-var/--var + osenv 贯通 + hot-reload"

BEADS_DIR=D:/work/aidev/lite-tools/.beads bd create --title="v0.2 Phase 2 epic" --type=feature --priority=2 --description="..."
# 然后立即关闭：
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd close <id> --reason="Phase 2 落地完成，详见 overview §3 Phase 2"
```

> 也可在 Phase 2 启动时先建 epic、Task 6 关闭，与 v0.2 Phase 1 一致节奏。

---

## Phase 2 完成 · 下一步

仓库具备：

- ✅ `--env / --var / FAKESERVER_ENV` 完整优先级链
- ✅ env 文件值层级模板渲染（osenv 白名单贯通）
- ✅ mock body `{{ .env.* }}` 可访问
- ✅ env 文件 hot-reload（参与 watcher）

**Phase 3 预告**：

- v0.2 综合 E2E（多场景串联：dev/staging 切换 + var override + 热加载 + osenv 阻断）
- bodyFile 相对路径回归 E2E（沿用 Phase 1 的回归思路）
- 覆盖率扣板（含 cli 包）
- overview / design 终态回写 + 关闭 v0.2 总 epic

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| 无 TBD / placeholder | ✓（Task 6 待填的 commit SHA 是执行后回填，非 placeholder） |
| 类型签名前后一致 | ✓（`BuildRenderCtx(req, params, globals, envMap)` 四参在 Task 5 全链路调整；`RenderEnvValues(env, osenvWhitelist, globals)` 在 Task 4 引入、Task 5/6 复用） |
| 包路径前后一致 | ✓ |
| TDD：先测后写 | ✓ |
| 频繁提交 | ✓（Task 1-6 各一个 commit，共 6 个新 commit + Task 6 收尾 docs）|
| 无新增第三方依赖 | ✓ |
| 覆盖 design §8.3 完整优先级链 / §8.2 末尾值渲染 / §4.6 osenv 白名单贯通 / §8.5 hot-reload | ✓ |
| design.md 回写**严格 3 条事实**（按 overview §5 新规约） | ✓ |
| Phase 2 边界明确（v0.2 综合 E2E 留 Phase 3） | ✓ |
| DoD 9 项可被具体测试用例覆盖 | ✓ |
