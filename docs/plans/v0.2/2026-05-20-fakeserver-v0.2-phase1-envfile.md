# Fakeserver v0.2 · Phase 1 — SourceFile 修复 + envfile.go 基础加载

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：让 `internal/config/Load()` 在加载主配置之后自动查找并加载同目录 `fakeserver.env.json5`，把 `$default` + 选中段合并为 `cfg.Env map[string]any`；同时修复 v0.1 `Route.SourceFile` 始终为空的存量 bug（`bd lite-tools-gko`），让相对路径 `bodyFile` 和 env 文件路径正确解析。

**Architecture**：

1. **SourceFile 修复**：v0.1 loader 在 `mapToConfig()` 用 `json.Marshal → json.Unmarshal` 做 JSON round-trip，而 `Route.SourceFile string \`json:"-"\`` tag 让该字段在 round-trip 中被吞掉，导致**所有 route 的 SourceFile 始终为空**。Phase 1 Task 1 spike 比较两种修复路径并选定，Task 2 实施 + 回归测试。
2. **envfile 加载**：新增 `internal/config/envfile.go`，提供 `LoadEnvFile(path, envName string) (envMap map[string]any, active string, err error)`。复用 loader 的 `loadFile()` 做 JSON5 解析，本地合并 `$default` + 选中段，不接 `@include`、不渲染模板（值层级模板渲染在 Phase 2 做）。
3. **loader.Load 整合**：在 `cfg.SourcePaths` 填充之后、`applyDefaults` 之前调 `LoadEnvFile`（默认查找规则：主配置同目录 `fakeserver.env.json5`），把结果写入 `cfg.Env`，记录 `cfg.EnvSource` 供 Phase 2 watcher 用。

**Tech Stack**：Go 1.26+ · 复用 `titanous/json5`（Phase 2 已引入）· 标准库 `encoding/json` / `path/filepath` / `os`。**本 Phase 无新增第三方依赖**。

**前置要求**：

- v0.1 MVP 已完成（最新 commit `e279766`，所有测试 + vet 零告警）
- 已读 [`docs/fakeserver-design.md`](../../fakeserver-design.md) §8.1（env 文件查找）/ §8.2（`$default` 合并）/ §8.3 优先级链（仅 `$active` + 首段两条；CLI / 环境变量留 Phase 2） / §3.2 Route.SourceFile 字段定义
- 已读本 milestone overview [`../2026-05-20-fakeserver-v0.2-overview.md`](../2026-05-20-fakeserver-v0.2-overview.md) §3 Phase 1 详述
- bd issue `lite-tools-gko` 当前 open，本 Phase 末关闭

**Phase 1 完成定义（DoD，来自 overview §3 Phase 1）**：

1. `bd lite-tools-gko` 关闭：`Route.SourceFile` 在 `config.Load` 之后**非空**；相对 `bodyFile` 解析按 **config 文件所在目录**而非 CWD（回归 E2E：在 `<tmpA>/cfg.json5` 引用 `bodyFile: "data.txt"`，从 `<tmpB>` cwd 运行 fakeserver 仍能命中）
2. `LoadEnvFile("test.env.json5", "dev") → (map, "dev", nil)` 返回 `$default ∪ dev` 深合并结果
3. `envName == ""` 时优先取 `$active` 字段；`$active` 缺省 → 取第一个非 `$default` 段；都没有 → 返回 `({}, "", nil)`
4. env 文件不存在 → 返回 `({}, "", nil)` + nil err
5. env 文件 root 非 object → 报错（与主配置相同语义）
6. `@include` 在 env 文件中出现 → 报错（design §8.2 明确禁止）
7. `go test ./...` 通过；`internal/config` 覆盖率维持 ≥ 80%

---

## 文件结构（Phase 1 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `internal/config/loader.go` | SourceFile 修复（按 Task 1 spike 结果选方案）|
| 修改 | `internal/config/loader_test.go` | SourceFile 回归测试用例 |
| 修改 | `internal/config/schema.go` | `Config` 增加 `Env map[string]any` + `EnvSource string` 字段；Route 字段保持不变 |
| 新建 | `internal/config/envfile.go` | `LoadEnvFile(path, envName) (map, string, error)` |
| 新建 | `internal/config/envfile_test.go` | 5 类 env 文件用例 + 入参选段 + 错误分支 |
| 新建 | `internal/config/testdata/env/default-only.json5` | 仅 `$default` 段 |
| 新建 | `internal/config/testdata/env/multi-env.json5` | `$default + dev + staging` 三段 |
| 新建 | `internal/config/testdata/env/with-active.json5` | `$active: "staging"` + 三段 |
| 新建 | `internal/config/testdata/env/missing-default.json5` | 无 `$default`，仅 dev/staging |
| 新建 | `internal/config/testdata/env/has-include-error.json5` | 含 `@include` 应报错 |
| 新建 | `internal/config/testdata/source/cfg.json5` + `data.txt` | bodyFile 相对路径回归用 |
| 修改 | `docs/plans/2026-05-20-fakeserver-v0.2-overview.md` | Phase 1 状态列回写 + 实际落地偏差段 |
| 修改 | `docs/fakeserver-design.md` | **简洁回写**：修订记录 1 行 + §13 "已落地（v0.2 Phase 1 阶段确认）" **3 条事实**（按 overview §5 新规约：概括即可）|

> **注**：Phase 1 **不**修改 `internal/cli/*`（CLI flag 留 Phase 2）、**不**修改 `internal/tpl/*`（osenv 收紧 + `.env` 接入留 Phase 2）。

---

## Task 1: SourceFile 修复 spike — 比较两种方案

**Files**:
- 无 commit（探查 only）；产出在 Task 1 报告里

> **风险点**（来自 overview §6）：SourceFile 修复方案选择——(a) Route struct 加 UnmarshalJSON hook + sentinel；(b) loader 用并行 slice 记录 + zip 进 Route。Phase 1 Task 1 spike 两种方案，比较代码量与可读性后定。

- [ ] **Step 1.1: 阅读现有 loader 流程**

读这几段代码理解 round-trip 路径：

- `internal/config/loader.go:31-72`（`Load` 主流程：loadFile → expandIncludes → mergeMaps → mapToConfig → applyDefaults）
- `internal/config/loader.go:148-180`（`mapToConfig`：JSON marshal → unmarshal）
- `internal/config/loader.go:198-238`（`expandIncludes`：递归 walk + 展开 `@xxx`）
- `internal/config/schema.go:40-64`（Route 字段定义，`SourceFile string \`json:"-"\``）

确认 bug 根因：mapToConfig 的 `json.Marshal(m) → json.Unmarshal(buf, cfg)` 经由 `json:"-"` 必然把 SourceFile 丢失；无任何代码路径在 Unmarshal 后给 Route.SourceFile 赋值。

- [ ] **Step 1.2: 评估方案 A — Route 加 UnmarshalJSON hook + sentinel key**

思路：

1. expandIncludes 阶段把每个 route 的 raw map 注入一个 sentinel key（如 `_source_file`），值是当前 baseDir 对应的绝对文件路径
2. Route 实现 `UnmarshalJSON([]byte) error`：先正常解码到一个 alias type，再读 sentinel key 写到 SourceFile
3. mapToConfig 的 round-trip 保留 sentinel key（不再 `json:"-"`），Unmarshal 阶段 hook 把它转到 SourceFile 字段

成本评估：
- 代码量：UnmarshalJSON ~25 行 + expandIncludes 注入 sentinel ~10 行 = ~35 行
- 副作用：Route struct 多出一个 alias type；sentinel key 出现在中间 JSON buffer 里（不会进入最终对外的 cfg）
- 可读性：Unmarshal hook 是 Go 标准模式，但读者需要理解"sentinel key 是 loader 内部约定"

- [ ] **Step 1.3: 评估方案 B — loader 用并行 slice 记录 + zip 进 Route**

思路：

1. expandIncludes 改签名为 `(node any, baseDir string, visiting map, routeSources *[]string) (any, error)`
2. 遇到 `routes` 数组的元素时，根据当前 baseDir append `routeSources`
3. mapToConfig 不变（保留现有 round-trip）
4. Load 在 mapToConfig 之后，遍历 `cfg.Routes` 与 `routeSources` 同位 zip 写入 `cfg.Routes[i].SourceFile`

成本评估：
- 代码量：expandIncludes 签名 + 调用点改造 ~15 行 + Load 末尾 zip ~10 行 = ~25 行
- 副作用：expandIncludes 签名变化（仅 loader 内部使用，无外部 API 风险）
- 可读性：纯 loader 内部状态，对 Route struct 无侵入；新读者更容易理解

> **关键判断点**：方案 A 把 SourceFile 的来源做成"Route 自描述"，方案 B 把它做成"loader 内部记账"。fakeserver 的 loader 已经是单一入口（`Load`），且 `Route.SourceFile` 仅用于路径解析（不参与 JSON 序列化），方案 B 的"内部记账"语义更贴近实际用途。

- [ ] **Step 1.4: 选定方案 + 报告**

**默认选 B**（除非 spike 发现 expandIncludes 重构成本超预期）。

报告里写：
1. 选 A 还是 B + 一句话理由
2. 估计 Task 2 实际代码改动量
3. 是否需要在 Task 2 之外另起一个 Task（如果方案 A 引入了 alias type，可能需要单独 commit）

**Task 1 不出代码、不出 commit**。报告即交付。

---

## Task 2: 实施 SourceFile 修复 + 回归测试

**Files**:
- 修改：`internal/config/loader.go`（按 Task 1 选定方案）
- 修改：`internal/config/loader_test.go`
- 新建：`internal/config/testdata/source/cfg.json5`
- 新建：`internal/config/testdata/source/data.txt`

> **本 Task 假设选定方案 B**（loader 并行 slice + zip）。若 Task 1 spike 选 A，按 A 调整 Step 2.3 的实现代码。

### Step 2.1: 写回归测试

把以下用例追加到 `internal/config/loader_test.go`（保留 Phase 2 现有用例不动）：

```go
func TestLoad_RouteSourceFile_Populated(t *testing.T) {
	// /tmp/<X>/cfg.json5 with one route → cfg.Routes[0].SourceFile
	// must be the absolute path to cfg.json5
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	body := `{
		routes: [
			{ method: "GET", path: "/x", body: "ok" },
		],
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}
	want := cfgPath
	got := cfg.Routes[0].SourceFile
	// Compare via filepath.Clean to neutralize separator differences
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Errorf("SourceFile = %q, want %q", got, want)
	}
}

// TestLoad_RouteSourceFile_FromInclude verifies routes from @included
// files carry the included file's path, not the parent's.
func TestLoad_RouteSourceFile_FromInclude(t *testing.T) {
	tmp := t.TempDir()
	subDir := filepath.Join(tmp, "routes")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	subPath := filepath.Join(subDir, "users.json5")
	subBody := `[
		{ method: "GET", path: "/u", body: "user-route" },
	]`
	if err := os.WriteFile(subPath, []byte(subBody), 0644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tmp, "cfg.json5")
	cfgBody := `{
		routes: [
			{ method: "GET", path: "/main", body: "main-route" },
			"@routes/users.json5",
		],
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(cfg.Routes))
	}
	// /main came from main cfg
	if filepath.Clean(cfg.Routes[0].SourceFile) != filepath.Clean(cfgPath) {
		t.Errorf("Routes[0] (/main) SourceFile = %q, want %q",
			cfg.Routes[0].SourceFile, cfgPath)
	}
	// /u came from included file
	if filepath.Clean(cfg.Routes[1].SourceFile) != filepath.Clean(subPath) {
		t.Errorf("Routes[1] (/u) SourceFile = %q, want %q",
			cfg.Routes[1].SourceFile, subPath)
	}
}
```

需要的 import：`os`、`path/filepath`、`testing`（若 loader_test.go 已 import 则跳过）。

### Step 2.2: 跑测试确认 FAIL

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/config/... -v -run "TestLoad_RouteSourceFile"
```

预期：两个用例 FAIL（`cfg.Routes[i].SourceFile == ""`）。

### Step 2.3: 实现 SourceFile 修复（方案 B）

修改 `internal/config/loader.go`：

**a) 把 `expandIncludes` 签名扩展，增加 `routeSources *[]string` 参数**：

```go
func expandIncludes(node any, baseDir string, visiting map[string]bool, routeSources *[]string) (any, error) {
```

调用点同步改：
- `Load` 中：`expanded, err := expandIncludes(any(rawMap), filepath.Dir(abs), visiting, nil)`（顶层 Load 不收集——见下文）
- `resolveInclude` 内递归：`expanded, err := expandIncludes(any(raw), filepath.Dir(p), visiting, routeSources)`

**b) 在 `expandIncludes` 的 `[]any` 分支识别"这是 routes 数组的元素"，并把 baseDir 记录到 routeSources**：

实际写法是：把"识别 routes 数组"的逻辑放到 caller（Load）层，因为 expandIncludes 不知道当前层是不是 routes 数组。改造点是：

1. Load 的主流程在 `mergeMaps(raws)` 之后、`mapToConfig(merged)` 之前，对 `merged["routes"]` 做一次"建立 sourceFile 列表"的 walk
2. walk 时，对每个 route 元素，根据它来自哪个文件填一个 sourceFile

但是问题来了：`mergeMaps` 阶段已经把多个文件的 routes 拼成单个 array，丢失了 per-element 的 source 信息。要正确实现方案 B，必须在 **expandIncludes / mergeMaps 阶段**就把 source 信息附到 route map 上。

**实际推荐做法（细化方案 B）**：

- 在 expandIncludes 的 `[]any` 分支里，**只**给"看起来像 route 对象"的 map 元素注入一个 `__source_file__` 键，值为当前 `baseDir` + 文件路径
- mergeMaps 不动（map 拷贝时把 `__source_file__` 带过来）
- mapToConfig 的 normalize 阶段，**先**把每条 route 的 `__source_file__` 摘到一个并行 slice，**再**从 map 删除该键
- 标准 JSON marshal/unmarshal 不变
- Load 末尾用并行 slice zip 进 `cfg.Routes[i].SourceFile`

这样实现具体在 expandIncludes 加一段：

```go
case []any:
    out := make([]any, 0, len(v))
    for _, item := range v {
        expanded, err := expandIncludes(item, baseDir, visiting)
        if err != nil {
            return nil, err
        }
        // ... 现有 flatten 逻辑
    }
    return out, nil
```

替换为（注意，sourceFile 注入既要覆盖 **直接出现在 routes 数组中的对象**，也要覆盖 **@include 进来的对象 / 对象数组**）：

```go
case []any:
    out := make([]any, 0, len(v))
    for _, item := range v {
        expanded, err := expandIncludes(item, baseDir, visiting)
        if err != nil {
            return nil, err
        }
        if subSlice, ok := expanded.([]any); ok && isIncludeString(item) {
            // include 进来的数组：每个元素的来源是 include 的 target 文件
            // target 文件路径已被 resolveInclude 计算过——但当前层拿不到
            // 这里不注入；改用 resolveInclude 在递归前注入
            out = append(out, subSlice...)
        } else {
            out = append(out, expanded)
        }
    }
    return out, nil
```

**最简洁的实现**：让 **每一个 loadFile 调用点** 在拿到 raw 之后，遍历"如果是数组，给每个 map 元素注入 `__source_file__: <absPath>`"。两个调用点：

1. `Load` 主流程（顶层 cfg.json5）—— 这里 raw 必须是 map，所以是 `rawMap["routes"]` 的元素需要注入
2. `resolveInclude` 内 —— include 文件可能是单个 route 对象 / route 数组 / 顶层 cfg map（罕见）

引入辅助函数：

```go
// annotateRoutes 把 sourceFile 注入到 raw 中所有可见的 route map 元素。
// raw 可能是：
//   - map[string]any (顶层 cfg)：annotate(raw["routes"])
//   - []any (include 返回的 route 数组)：annotate each map element
//   - map[string]any (include 返回的单 route 对象)：annotate self
func annotateRoutes(node any, sourceFile string) {
    switch v := node.(type) {
    case map[string]any:
        if routes, ok := v["routes"].([]any); ok {
            for _, r := range routes {
                if rm, ok := r.(map[string]any); ok {
                    if _, set := rm["__source_file__"]; !set {
                        rm["__source_file__"] = sourceFile
                    }
                }
            }
        }
    case []any:
        for _, r := range v {
            if rm, ok := r.(map[string]any); ok {
                if _, set := rm["__source_file__"]; !set {
                    rm["__source_file__"] = sourceFile
                }
            }
        }
    }
}
```

调用点：

1. `Load` 在 `loadFile(abs)` 之后立即：`annotateRoutes(raw, abs)`（顶层 cfg 路径）
2. `resolveInclude` 在 `loadFile(p)` 之后立即：`annotateRoutes(raw, p)`（include 文件路径）

注意 `annotateRoutes` 用 `if _, set := rm["__source_file__"]; !set` 防止后注入覆盖先注入（保证"最近的来源文件胜出"）。

**c) `mapToConfig` 增加 sourceFile 摘除 + 同位 slice 构造**：

替换现有 `mapToConfig` 函数：

```go
func mapToConfig(m map[string]any) (*Config, []string, error) {
    var routeSources []string
    if routes, ok := m["routes"].([]any); ok {
        routeSources = make([]string, len(routes))
        for i, r := range routes {
            rm, ok := r.(map[string]any)
            if !ok {
                continue
            }
            if sf, ok := rm["__source_file__"].(string); ok {
                routeSources[i] = sf
                delete(rm, "__source_file__")
            }
            // Method 形态归一化（保留 Phase 2 旧逻辑）
            method := rm["method"]
            switch v := method.(type) {
            case string:
                rm["method"] = []any{v}
            case nil:
                rm["method"] = []any{}
            case []any:
                // already normalized
            default:
                return nil, nil, fmt.Errorf("routes[%d].method: unsupported type %T", i, v)
            }
        }
    }

    buf, err := json.Marshal(m)
    if err != nil {
        return nil, nil, fmt.Errorf("re-encode to JSON: %w", err)
    }
    cfg := &Config{}
    if err := json.Unmarshal(buf, cfg); err != nil {
        return nil, nil, fmt.Errorf("unmarshal to Config: %w", err)
    }
    return cfg, routeSources, nil
}
```

**d) Load 末尾 zip + 调用更新**：

```go
// 在 Load 函数里：
cfg, routeSources, err := mapToConfig(merged)
if err != nil {
    return nil, err
}
cfg.SourcePaths = absSources
// Zip SourceFile back into routes (v0.2 lite-tools-gko fix)
for i := range cfg.Routes {
    if i < len(routeSources) && routeSources[i] != "" {
        cfg.Routes[i].SourceFile = routeSources[i]
    }
}
applyDefaults(cfg)
return cfg, nil
```

### Step 2.4: 跑测试

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/config/... -v -run "TestLoad_RouteSourceFile" -count=1
```

预期：两个 SourceFile 用例 PASS。同时全包测试不应回归：

```
go test ./... -count=1
```

预期：全包绿。

> **可能的偏差**：`__source_file__` 是 loader 内部约定的 sentinel key。若 JSON5 主配置里用户**真的**写了一个名叫 `__source_file__` 的 route 字段，会被吞掉。这是约定俗成的 sentinel 命名风险——双下划线前后缀降低碰撞概率，可接受。报告中说明。

### Step 2.5: bodyFile 相对路径回归 E2E

新建 `internal/config/testdata/source/cfg.json5`：

```json5
{
  routes: [
    { method: "GET", path: "/file", bodyFile: "data.txt" },
  ],
}
```

新建 `internal/config/testdata/source/data.txt`（内容：`hello`）：

```
hello
```

把以下追加到 `internal/config/validate_test.go` 或 `loader_test.go`（**注意**：`bodyFile` 路径解析逻辑在 validate.go 的 `resolveRoutePath`，已实现的兜底逻辑用 `cfg.SourcePaths[0]`——SourceFile 修复后应优先用 route 自己的 SourceFile）：

```go
func TestLoad_BodyFileRelativePath_ResolvedAgainstConfigDir(t *testing.T) {
	// Repro for lite-tools-gko: bodyFile relative path should resolve to
	// the config file's directory, not the CWD. Test by loading from a
	// different CWD.
	wd, _ := os.Getwd()
	defer os.Chdir(wd)

	// Use the testdata fixture
	cfgPath, _ := filepath.Abs("testdata/source/cfg.json5")
	// Move CWD elsewhere to force the relative-path resolution to depend
	// on Route.SourceFile, not CWD.
	otherDir := t.TempDir()
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if errs := Validate(cfg); len(errs) > 0 {
		t.Fatalf("validate: %v", errs)
	}
	// If SourceFile is correctly set, Validate won't have errored on
	// bodyFile presence check (which uses resolveRoutePath with SourceFile).
	want := filepath.Clean(cfgPath)
	got := filepath.Clean(cfg.Routes[0].SourceFile)
	if want != got {
		t.Errorf("SourceFile = %q, want %q", got, want)
	}
}
```

跑：

```
go test ./internal/config/... -v -run "TestLoad_BodyFileRelativePath"
```

预期 PASS。`Validate` 在调用 `resolveRoutePath` 时会用 `Route.SourceFile` 优先解析，bodyFile 文件能被找到。

### Step 2.6: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/loader.go internal/config/loader_test.go internal/config/testdata/source/cfg.json5 internal/config/testdata/source/data.txt
git -C D:/work/aidev/lite-tools/fakeserver commit -m "fix(config): 填充 Route.SourceFile——修复 lite-tools-gko 相对 bodyFile 解析"
```

---

## Task 3: schema.go 加 Env/EnvSource + envfile.go 骨架

**Files**:
- 修改：`internal/config/schema.go`
- 新建：`internal/config/envfile.go`
- 新建：`internal/config/envfile_test.go`

### Step 3.1: schema.go 增字段

修改 `internal/config/schema.go` 中 `Config` 结构体定义，追加两个字段：

```go
type Config struct {
	Server   ServerOpts     `json:"server"`
	Globals  map[string]any `json:"globals"`
	Fallback string         `json:"fallback"`
	Routes   []Route        `json:"routes"`

	// SourcePaths records, in load order, every config file that
	// contributed to this Config (including @include expansions).
	SourcePaths []string `json:"-"`

	// Env is the resolved environment block (= $default ∪ chosen segment).
	// design §8.4: route templates access this via {{ .env.* }}.
	// v0.2 Phase 1 fills this from envfile.go; Phase 2 adds CLI/env-var
	// selection + template rendering of env values.
	Env map[string]any `json:"-"`

	// EnvSource is the absolute path of the env file that contributed
	// Env, or "" if no env file was found. v0.2 Phase 2 uses this to
	// extend watcher's SourcePaths so env edits trigger hot reload.
	EnvSource string `json:"-"`
}
```

### Step 3.2: envfile.go 骨架

新建 `internal/config/envfile.go`：

```go
// envfile.go implements design §8 environment-file loading. The file lives
// next to the main config (fakeserver.env.json5 by default) and provides
// per-environment value overrides accessible to mock templates as
// {{ .env.* }} (template wiring lands in v0.2 Phase 2; this Phase only
// produces cfg.Env / cfg.EnvSource).
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultEnvFileName is the basename loader.Load() searches next to the
// main config file. design §8.1.
const DefaultEnvFileName = "fakeserver.env.json5"

// LoadEnvFile parses path as a JSON5 env file and returns the merged
// environment block for envName.
//
// Behavior (design §8):
//   - File missing → ({}, "", nil). Not an error; v0.1 zero-config UX.
//   - Root non-object → error.
//   - Contains @include → error (design §8.2 末尾 explicitly禁止).
//   - $default segment is deep-merged into the chosen segment.
//   - Chosen segment selection (envName == ""):
//       1. file's "$active" field
//       2. first non-$default segment by iteration order
//       3. neither → ({}, "", nil)
//   - envName != "" → that exact segment is chosen; missing → error.
//
// active is the resolved segment name (informational; useful for banners
// and watcher feedback). Phase 1 doesn't render template strings inside
// env values — Phase 2 does.
func LoadEnvFile(path, envName string) (envMap map[string]any, active string, err error) {
	// Task 4-5 will fill this in; Task 3 just establishes the signature.
	return map[string]any{}, "", nil
}
```

### Step 3.3: envfile_test.go 编译性 smoke

新建 `internal/config/envfile_test.go`：

```go
package config

import "testing"

// Smoke: LoadEnvFile exists with the expected signature and returns
// (map[string]any, string, error). Task 4-5 add real behavior tests.
func TestLoadEnvFile_SignatureSmoke(t *testing.T) {
	env, active, err := LoadEnvFile("/non/existent/path.json5", "")
	if err != nil {
		t.Errorf("missing file should not error in stub; got %v", err)
	}
	if env == nil {
		t.Error("env should never be nil (use empty map instead)")
	}
	if active != "" {
		t.Errorf("missing file → active should be empty; got %q", active)
	}
}
```

### Step 3.4: 跑测试

```
cd D:/work/aidev/lite-tools/fakeserver
go build ./...
go test ./internal/config/... -v -run "TestLoadEnvFile_SignatureSmoke"
```

预期：编译通过 + smoke PASS。

### Step 3.5: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/schema.go internal/config/envfile.go internal/config/envfile_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): Config.Env/EnvSource 字段 + envfile.go 骨架"
```

---

## Task 4: LoadEnvFile $default 合并 + $active 选段 + envName 入参

**Files**:
- 修改：`internal/config/envfile.go`
- 修改：`internal/config/envfile_test.go`
- 新建：`internal/config/testdata/env/default-only.json5`
- 新建：`internal/config/testdata/env/multi-env.json5`
- 新建：`internal/config/testdata/env/with-active.json5`
- 新建：`internal/config/testdata/env/missing-default.json5`

### Step 4.1: 准备 testdata

新建 `internal/config/testdata/env/default-only.json5`：

```json5
{
  $default: {
    apiHost: "localhost",
    timeout: "5s",
  },
}
```

新建 `internal/config/testdata/env/multi-env.json5`：

```json5
{
  $default: {
    apiHost: "localhost",
    timeout: "5s",
  },
  dev: {
    apiHost: "localhost:5090",
    token: "dev-xxx",
  },
  staging: {
    apiHost: "stage.api.com",
    token: "stg-yyy",
  },
}
```

新建 `internal/config/testdata/env/with-active.json5`：

```json5
{
  $active: "staging",
  $default: {
    apiHost: "localhost",
  },
  dev: {
    apiHost: "dev.local",
    token: "dev-xxx",
  },
  staging: {
    apiHost: "stage.api.com",
    token: "stg-yyy",
  },
}
```

新建 `internal/config/testdata/env/missing-default.json5`：

```json5
{
  dev: {
    apiHost: "dev.local",
  },
  staging: {
    apiHost: "stage.api.com",
  },
}
```

### Step 4.2: 写测试

把以下替换到 `internal/config/envfile_test.go`（保留 Task 3 smoke）：

```go
package config

import (
	"testing"
)

func TestLoadEnvFile_SignatureSmoke(t *testing.T) {
	env, active, err := LoadEnvFile("/non/existent/path.json5", "")
	if err != nil {
		t.Errorf("missing file should not error; got %v", err)
	}
	if env == nil {
		t.Error("env should never be nil")
	}
	if active != "" {
		t.Errorf("active=%q, want empty", active)
	}
}

func TestLoadEnvFile_DefaultOnly(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/default-only.json5", "")
	if err != nil {
		t.Fatal(err)
	}
	// No non-$default segments → no segment chosen → empty result
	if len(env) != 0 {
		t.Errorf("default-only with no envName → empty map; got %v", env)
	}
	if active != "" {
		t.Errorf("active=%q want empty", active)
	}
}

func TestLoadEnvFile_MultiEnv_ChooseDevByName(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/multi-env.json5", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if active != "dev" {
		t.Errorf("active=%q want 'dev'", active)
	}
	if env["apiHost"] != "localhost:5090" {
		t.Errorf("apiHost=%v want 'localhost:5090' (dev overrides $default)", env["apiHost"])
	}
	if env["timeout"] != "5s" {
		t.Errorf("timeout=%v want '5s' (inherited from $default)", env["timeout"])
	}
	if env["token"] != "dev-xxx" {
		t.Errorf("token=%v want 'dev-xxx'", env["token"])
	}
}

func TestLoadEnvFile_MultiEnv_ChooseStagingByName(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/multi-env.json5", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if active != "staging" {
		t.Errorf("active=%q want 'staging'", active)
	}
	if env["token"] != "stg-yyy" {
		t.Errorf("token=%v want 'stg-yyy'", env["token"])
	}
}

func TestLoadEnvFile_MultiEnv_FirstSegmentByDefault(t *testing.T) {
	// envName="" + no $active → iteration order picks first non-$default
	env, active, err := LoadEnvFile("testdata/env/multi-env.json5", "")
	if err != nil {
		t.Fatal(err)
	}
	// Either "dev" or "staging" — Go map iteration order is randomized but
	// JSON5 preserves source order? Verify what titanous/json5 does. If
	// non-deterministic, the test should tolerate either.
	if active != "dev" && active != "staging" {
		t.Errorf("active=%q want 'dev' or 'staging' (first non-$default segment)", active)
	}
	if _, has := env["apiHost"]; !has {
		t.Errorf("no apiHost in env: %v", env)
	}
}

func TestLoadEnvFile_WithActiveField(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/with-active.json5", "")
	if err != nil {
		t.Fatal(err)
	}
	// $active: "staging" should win when envName is empty
	if active != "staging" {
		t.Errorf("active=%q want 'staging' ($active field should win)", active)
	}
	if env["apiHost"] != "stage.api.com" {
		t.Errorf("apiHost=%v want 'stage.api.com'", env["apiHost"])
	}
}

func TestLoadEnvFile_WithActive_EnvNameOverrides(t *testing.T) {
	// Explicit envName takes precedence over $active
	env, active, err := LoadEnvFile("testdata/env/with-active.json5", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if active != "dev" {
		t.Errorf("envName=dev should win over $active=staging; active=%q", active)
	}
	if env["token"] != "dev-xxx" {
		t.Errorf("token=%v want 'dev-xxx'", env["token"])
	}
}

func TestLoadEnvFile_MissingDefault(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/missing-default.json5", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if active != "dev" {
		t.Errorf("active=%q want 'dev'", active)
	}
	if env["apiHost"] != "dev.local" {
		t.Errorf("apiHost=%v want 'dev.local'", env["apiHost"])
	}
	if _, has := env["timeout"]; has {
		t.Errorf("no $default → no timeout key; got %v", env)
	}
}

func TestLoadEnvFile_EnvNameNotFound(t *testing.T) {
	_, _, err := LoadEnvFile("testdata/env/multi-env.json5", "production")
	if err == nil {
		t.Fatal("envName='production' not in file → expected error")
	}
}
```

### Step 4.3: 跑测试确认 FAIL

```
go test ./internal/config/... -v -run "TestLoadEnvFile_"
```

预期：除 SignatureSmoke 外其余 FAIL（stub 始终返回空 map）。

### Step 4.4: 实现 LoadEnvFile 核心

替换 `internal/config/envfile.go` 中 `LoadEnvFile` 函数（保留 const）：

```go
func LoadEnvFile(path, envName string) (envMap map[string]any, active string, err error) {
	envMap = map[string]any{}
	// File missing → ({}, "", nil)
	if _, err = os.Stat(path); os.IsNotExist(err) {
		return envMap, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("stat env file %q: %w", path, err)
	}

	raw, err := loadFile(path) // re-use loader.go's parser
	if err != nil {
		return nil, "", err
	}
	rootMap, ok := raw.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("env file %q: root must be an object", path)
	}

	// Reject @include in env files (design §8.2 末尾)
	if err := rejectIncludesInEnvFile(rootMap, path); err != nil {
		return nil, "", err
	}

	// Extract $default and meta $active; everything else is a candidate segment
	defaults, _ := rootMap["$default"].(map[string]any)
	activeMeta, _ := rootMap["$active"].(string)

	// Determine target segment
	target := envName
	if target == "" {
		target = activeMeta
	}
	if target == "" {
		// Find first non-$default segment by iteration order
		for k := range rootMap {
			if k == "$default" || k == "$active" {
				continue
			}
			target = k
			break
		}
	}
	if target == "" {
		// No segments at all → empty result, not an error
		return envMap, "", nil
	}

	chosen, ok := rootMap[target].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("env file %q: segment %q not found or not an object", path, target)
	}

	// Deep-merge $default into chosen segment (chosen wins on conflict)
	if defaults != nil {
		envMap = deepMerge(defaults, chosen)
	} else {
		envMap = deepMerge(map[string]any{}, chosen)
	}

	return envMap, target, nil
}

// rejectIncludesInEnvFile walks the root map and errors on any string
// value starting with "@" (design §8.2 末尾: env files do not support
// @include to keep recursion complexity bounded). Phase 1 only checks
// the surface — Phase 2's value-level template rendering does the deeper
// walk if needed.
func rejectIncludesInEnvFile(node any, path string) error {
	// Implemented in Task 5.
	return nil
}
```

> **注意**：`loadFile` 是 loader.go 已有的私有函数；envfile.go 同包内可直接调用。`deepMerge` 同样是 loader.go 私有函数；同包复用。

### Step 4.5: 跑测试

```
go test ./internal/config/... -v -run "TestLoadEnvFile_" -count=1
```

预期：除 `TestLoadEnvFile_HasInclude_Rejected`（Task 5 加）外，全部 PASS。

> **可能的偏差**：`TestLoadEnvFile_MultiEnv_FirstSegmentByDefault` 的"first segment"依赖 JSON5 解析后是否保留键序。titanous/json5 把对象解码成 `map[string]any`，Go map 无序。但测试只断言 `active in ["dev", "staging"]` 是 OK 的。如果用户希望"按 JSON5 源码顺序定首段"，需要切到 LinkedHashMap 风格——v0.2 不做，记入落地偏差。

### Step 4.6: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/envfile.go internal/config/envfile_test.go internal/config/testdata/env/default-only.json5 internal/config/testdata/env/multi-env.json5 internal/config/testdata/env/with-active.json5 internal/config/testdata/env/missing-default.json5
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): LoadEnvFile $default 合并 + $active + envName 选段"
```

---

## Task 5: 错误分支 — root 非 object / @include 拒绝 / 文件不存在

**Files**:
- 修改：`internal/config/envfile.go`
- 修改：`internal/config/envfile_test.go`
- 新建：`internal/config/testdata/env/has-include-error.json5`
- 新建：`internal/config/testdata/env/root-array.json5`

### Step 5.1: testdata

新建 `internal/config/testdata/env/has-include-error.json5`：

```json5
{
  $default: {
    apiHost: "@shared/host.json5",
  },
  dev: {
    token: "dev-xxx",
  },
}
```

新建 `internal/config/testdata/env/root-array.json5`：

```json5
[
  { dev: { apiHost: "x" } },
]
```

### Step 5.2: 测试

把以下追加到 `internal/config/envfile_test.go`：

```go
func TestLoadEnvFile_RootArray_Rejected(t *testing.T) {
	_, _, err := LoadEnvFile("testdata/env/root-array.json5", "")
	if err == nil {
		t.Fatal("root array → expected error")
	}
	if !strings.Contains(err.Error(), "root must be an object") {
		t.Errorf("error should mention 'root must be an object'; got %v", err)
	}
}

func TestLoadEnvFile_HasInclude_Rejected(t *testing.T) {
	_, _, err := LoadEnvFile("testdata/env/has-include-error.json5", "dev")
	if err == nil {
		t.Fatal("env file with @include → expected error")
	}
	if !strings.Contains(err.Error(), "@include") {
		t.Errorf("error should mention '@include'; got %v", err)
	}
}

func TestLoadEnvFile_FileNotExist(t *testing.T) {
	env, active, err := LoadEnvFile("/totally/does/not/exist.json5", "")
	if err != nil {
		t.Errorf("missing file should NOT error; got %v", err)
	}
	if len(env) != 0 {
		t.Errorf("missing file → empty map; got %v", env)
	}
	if active != "" {
		t.Errorf("missing file → empty active; got %q", active)
	}
}
```

需要 `import "strings"` 如果尚未引入。

### Step 5.3: 跑测试确认 FAIL（仅 @include 那个）

```
go test ./internal/config/... -v -run "TestLoadEnvFile_HasInclude_Rejected"
```

预期：FAIL（rejectIncludesInEnvFile 当前是空 stub）。其他两个用例应该已经 PASS（root array 在 Task 4 的 `rootMap, ok := raw.(map[string]any)` 已被拒；file-not-exist 在 Task 4 的 `os.IsNotExist` 已处理）。

### Step 5.4: 实现 rejectIncludesInEnvFile

替换 `internal/config/envfile.go` 中 `rejectIncludesInEnvFile` 函数：

```go
// rejectIncludesInEnvFile walks the root map and errors on any string
// value starting with "@" (and not escaped with "\@"). design §8.2 末尾:
// env files do not support @include — keeps recursion complexity bounded
// and avoids env-vs-route include semantics ambiguity.
func rejectIncludesInEnvFile(node any, path string) error {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if err := rejectIncludesInEnvFile(child, path); err != nil {
				return fmt.Errorf("env file %q at key %q: %w", path, k, err)
			}
		}
	case []any:
		for i, child := range v {
			if err := rejectIncludesInEnvFile(child, path); err != nil {
				return fmt.Errorf("env file %q at index %d: %w", path, i, err)
			}
		}
	case string:
		if strings.HasPrefix(v, "@") && !strings.HasPrefix(v, "\\@") {
			return fmt.Errorf("@include is not supported in env files (use main config @include instead): %q", v)
		}
	}
	return nil
}
```

需要 `import "strings"` 到 envfile.go。

### Step 5.5: 跑测试

```
go test ./internal/config/... -v -run "TestLoadEnvFile_" -count=1
```

预期：全部 11 个 LoadEnvFile 用例 PASS。

### Step 5.6: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/envfile.go internal/config/envfile_test.go internal/config/testdata/env/has-include-error.json5 internal/config/testdata/env/root-array.json5
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): envfile 拒绝 @include + 错误分支补全"
```

---

## Task 6: loader.Load 整合 + Phase 1 收尾

**Files**:
- 修改：`internal/config/loader.go`
- 修改：`internal/config/loader_test.go`
- 修改：`docs/plans/2026-05-20-fakeserver-v0.2-overview.md`
- 修改：`docs/fakeserver-design.md`

### Step 6.1: loader.Load 接入 envfile

在 `internal/config/loader.go` 的 `Load` 函数末尾（`applyDefaults(cfg)` **之前**）插入：

```go
	// v0.2 Phase 1: auto-load fakeserver.env.json5 next to the primary
	// config file. envName="" so $active / first-segment selection
	// applies; Phase 2 wires CLI/env-var to pass real envName.
	if len(absSources) > 0 {
		envPath := filepath.Join(filepath.Dir(absSources[0]), DefaultEnvFileName)
		envMap, _, eerr := LoadEnvFile(envPath, "")
		if eerr != nil {
			return nil, fmt.Errorf("env file: %w", eerr)
		}
		cfg.Env = envMap
		// EnvSource is only set when the file existed (LoadEnvFile returns
		// empty map for missing files; we want EnvSource="" in that case).
		if _, sterr := os.Stat(envPath); sterr == nil {
			cfg.EnvSource = envPath
		}
	}

	applyDefaults(cfg)
```

### Step 6.2: loader 集成测试

追加到 `internal/config/loader_test.go`：

```go
func TestLoad_EnvFile_AutoLoaded(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	envPath := filepath.Join(tmp, DefaultEnvFileName)

	if err := os.WriteFile(cfgPath, []byte(`{ routes: [{ method: "GET", path: "/", body: "ok" }] }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte(`{ $default: { x: "y" }, dev: { token: "t" } }`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// envName="" + no $active → first non-$default segment ("dev")
	if cfg.Env["x"] != "y" {
		t.Errorf("Env.x=%v want 'y' (from $default)", cfg.Env["x"])
	}
	if cfg.Env["token"] != "t" {
		t.Errorf("Env.token=%v want 't' (from dev)", cfg.Env["token"])
	}
	if cfg.EnvSource != envPath {
		t.Errorf("EnvSource=%q want %q", cfg.EnvSource, envPath)
	}
}

func TestLoad_NoEnvFile_EmptyEnv(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(cfgPath, []byte(`{ routes: [] }`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env == nil {
		t.Error("cfg.Env should be non-nil empty map, not nil")
	}
	if len(cfg.Env) != 0 {
		t.Errorf("no env file → empty Env; got %v", cfg.Env)
	}
	if cfg.EnvSource != "" {
		t.Errorf("no env file → empty EnvSource; got %q", cfg.EnvSource)
	}
}
```

### Step 6.3: 跑全套测试 + DoD 核对

```
cd D:/work/aidev/lite-tools/fakeserver
go build ./...
go test ./... -count=1
go test -cover ./internal/config/...
go vet ./...
```

预期：

- 全包 PASS（含本 Phase 新增的 ~15 个用例）
- `internal/config` 覆盖率 ≥ 80%
- `go vet ./...` 零告警

DoD 7 项逐项核对：

| # | 检查 | 通过条件 |
|---|---|---|
| 1 | `bd lite-tools-gko` 关闭条件 | `TestLoad_RouteSourceFile_Populated` + `TestLoad_RouteSourceFile_FromInclude` + `TestLoad_BodyFileRelativePath_ResolvedAgainstConfigDir` 全 PASS |
| 2 | `LoadEnvFile("test.env.json5", "dev")` | `TestLoadEnvFile_MultiEnv_ChooseDevByName` PASS |
| 3 | envName == "" 选段优先级 | `TestLoadEnvFile_WithActiveField` + `TestLoadEnvFile_MultiEnv_FirstSegmentByDefault` PASS |
| 4 | env 文件不存在 → 空 | `TestLoadEnvFile_FileNotExist` + `TestLoad_NoEnvFile_EmptyEnv` PASS |
| 5 | env 文件 root 非 object → 报错 | `TestLoadEnvFile_RootArray_Rejected` PASS |
| 6 | @include 在 env 文件中 → 报错 | `TestLoadEnvFile_HasInclude_Rejected` PASS |
| 7 | 覆盖率 ≥ 80% | `go test -cover ./internal/config/...` 输出 |

### Step 6.4: Commit（loader 集成）

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/config/loader.go internal/config/loader_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(config): loader.Load 自动加载同目录 fakeserver.env.json5"
```

### Step 6.5: 回写 overview

修改 `docs/plans/2026-05-20-fakeserver-v0.2-overview.md`：

**§2 表 Phase 1 行**：状态列改为 `✅ 已完成 (commit <first SHA>..<last SHA>)`。`<first SHA>` 是 Task 2 的 commit；`<last SHA>` 是 Step 6.4 的 commit。

**§3 Phase 1 详述末尾追加**：

```markdown
**实际落地偏差**：

- **SourceFile 修复选用方案 B**（loader 内部 `__source_file__` sentinel + zip）：方案 A（Route UnmarshalJSON hook）代码量相近但侵入 Route struct；方案 B 完全限制在 loader 内部，对 Route 类型零侵入。详见 Phase 1 Task 1 spike 报告。
- **JSON5 解析后键序不保证**：`titanous/json5` 把对象解码成 Go `map[string]any`，"first non-$default segment" 在多段情况下不稳定。`TestLoadEnvFile_MultiEnv_FirstSegmentByDefault` 容忍 dev/staging 任一作为 active。若 v0.2 后续阶段发现用户依赖键序，需切换到 LinkedHashMap 风格的 decoder。
- **`__source_file__` sentinel key 碰撞风险**：用户若在主配置 route 中**真的**写一个键名为 `__source_file__` 的字段会被吞掉。双下划线前后缀的 sentinel 命名降低碰撞概率，可接受。
- **envfile.go 不渲染值层级模板**（Phase 1 边界）：env 文件中 `"token": "{{ osenv \"X\" }}"` 在 Phase 1 直接作为字符串保留，{{ }} 表达式不展开。Phase 2 加渲染步骤。

**Phase 1 测试覆盖**：~15 个新增用例（loader 2 + envfile 11 + loader 集成 2）；`internal/config` 覆盖率 <X>%（核对实测填）。

**Phase 1 commit 流水**：<填入 Task 2-6 实际 SHA 列表>
```

### Step 6.6: 回写 design.md

**严格按 overview §5 新规约**：design.md 仅写概括，不写 commit-level 细节。

修订记录追加 1 行：

```markdown
| 2026-05-20 | v0.4-phase0.2.1-applied | inhere | v0.2 Phase 1：env 文件加载（design §8.1/§8.2/§8.3 部分）+ Route.SourceFile 填充修复 |
```

§13 追加 **3 条事实**（不要超过）：

```markdown
### 已落地（v0.2 Phase 1 阶段确认）

1. **`fakeserver.env.json5` 自动加载**：loader.Load 在主配置加载后查找同目录 env 文件，合并 `$default` + 选中段写入 `cfg.Env`；env 文件路径写入 `cfg.EnvSource` 供后续 watcher 用。
2. **`Route.SourceFile` 填充**：loader 通过内部 sentinel key 在 expandIncludes 阶段记录 route 来源文件，绕过 JSON round-trip 对 `json:"-"` 字段的剥离。修复了 v0.1 遗留的 bd lite-tools-gko（相对 bodyFile 路径解析）。
3. **env 文件不支持 @include**：design §8.2 末尾约定的 "env 文件不接 include" 在 loader 层强制执行；遇 `@xxx` 字符串报错。
```

### Step 6.7: Commit（docs）

```
git -C D:/work/aidev/lite-tools/fakeserver add docs/plans/2026-05-20-fakeserver-v0.2-overview.md docs/fakeserver-design.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "docs(v0.2): 回写 Phase 1 落地——envfile 加载 + SourceFile 修复"
```

### Step 6.8: 关闭 bd 任务

```
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd close lite-tools-gko --reason="v0.2 Phase 1 修复 Route.SourceFile（loader 内部 sentinel key + zip 方案）；bodyFile 相对路径在 CWD ≠ config 目录时正确解析；新增回归 E2E TestLoad_BodyFileRelativePath_ResolvedAgainstConfigDir"
```

---

## Phase 1 完成 · 下一步

仓库具备：

- ✅ `internal/config/loader.go`：填充 `Route.SourceFile` + 自动加载 env 文件
- ✅ `internal/config/envfile.go`：`LoadEnvFile($default 合并 + $active + envName 选段)`
- ✅ `Config.Env` + `Config.EnvSource` 字段
- ✅ `bd lite-tools-gko` 关闭：相对 bodyFile 路径回归测试通过
- ✅ `internal/config/testdata/env/` 5 类 env 文件用例
- ✅ overview Phase 1 状态 ✅ + design.md 概括式回写（3 条事实）

**Phase 2 预告**（不在本计划范围）：

- `serveOptions` 增 `EnvName / VarOverrides` 字段
- gcli 注册 `-e/--env <name>` + `--var key=val`
- env 选择优先级链完整化（CLI → `FAKESERVER_ENV` → `$active` → 首段）
- env 文件**值层级**模板渲染（`"token": "{{ osenv \"X\" }}"` Load 阶段 render）
- `tpl.BuildRenderCtx` 把 `cfg.Env` 接到 `.env` key
- osenv 白名单收紧到 env 文件
- env 文件路径加入 `cfg.SourcePaths` → watcher 自动监听

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| 无 TBD / placeholder | ✓（Task 1 是探查无 commit，Task 3 的 stub 是受控的占位） |
| 类型签名前后一致 | ✓（`LoadEnvFile(path, envName) (map[string]any, string, error)` 在 Task 3 stub / Task 4-5 实现 / Task 6 调用点保持一致；`mapToConfig` 改名为返回三元组但只在 loader.go 内部使用） |
| 包路径前后一致 | ✓（`github.com/inhere/fakeserver/internal/config`） |
| TDD：先测后写 | ✓（每个 Task 都是写测试 → 跑 fail → 实现 → 跑 pass） |
| 频繁提交 | ✓（Task 2-6 各一个 commit，加 Task 6 的 docs commit = 共 6 个新 commit）|
| 无新增第三方依赖 | ✓（仅复用 titanous/json5 / 标准库）|
| 覆盖 design §8.1 + §8.2 + §8.3 部分（$active + 首段） + §3.2 SourceFile 字段 | ✓ |
| Phase 1 边界明确（CLI flag / 值层级模板渲染 / hot-reload / osenv 白名单收紧全部留 Phase 2） | ✓ |
| design.md 回写概括（3 条事实，无 commit SHA / 内部函数名） | ✓（按 overview §5 新规约）|
| 失败路径明确 | ✓（root 非 object / @include / envName 不存在 / 文件不存在四类）|
| DoD 7 项与 overview §3 Phase 1 DoD 7 项一致 | ✓ |
