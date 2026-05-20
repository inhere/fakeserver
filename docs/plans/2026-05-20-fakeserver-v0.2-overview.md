# Fakeserver v0.2 阶段规划总览

> 本文档把 v0.2（环境配置文件 + osenv 白名单完整化）按"独立可测可交付"原则拆成 3 个 Phase。每个 Phase 对应一份 `phase<N>-*.md` 详细计划，由 `superpowers:writing-plans` 在执行前展开。
>
> **前置**：v0.1 MVP 已于 2026-05-20 完整闭环（见 [2026-05-19-fakeserver-v0.1-overview.md](2026-05-19-fakeserver-v0.1-overview.md)）。

## 修订记录

| 日期 | 版本 | 作者 | 变更说明 |
|---|---|---|---|
| 2026-05-20 | v0.2-overview | inhere | 初稿。固化 v0.2 内部的 3 Phase 拆分（env 文件 + osenv 白名单 + SourceFile bug 修复）|

后续修订：每完成一个 Phase 后在对应行回写 commit 摘要与实际偏差。

---

## 1. v0.2 范围与拆分原则

**v0.2 目标**：把 design §8 列出的环境配置文件能力完整接入，并修复 v0.1 留下的 `Route.SourceFile` 存量 bug（bd `lite-tools-gko`）。

design §14 路线图明确 v0.2 范围**仅含 env 相关功能**——项目注册（§10）+ Web UI（§11）继续留 v0.3 / v0.4。

把 v0.2 切成 3 个 Phase 的依据（与 v0.1 拆分原则一致）：

1. **每个 Phase 末尾必须能 `go run ./cmd/fakeserver serve` 跑出一个对用户可用的产物**——Phase 1 完成后 `internal/config/envfile.go` 能加载 `fakeserver.env.json5` 并填充 `cfg.Env`，但 CLI flag / `.env` 模板访问还未接入；Phase 2 完成后 `serve --env staging --var k=v` 真正影响响应；Phase 3 完成后 E2E 闭环。
2. **每个 Phase 内部模块依赖封闭**：env 文件加载是纯包内逻辑（Phase 1），CLI / tpl 整合在 Phase 2，E2E 收尾在 Phase 3。
3. **每个 Phase 自带完整测试**——单元 + 集成 + E2E，不靠后续 Phase 兜底。
4. **顺手修 v0.1 backlog**：`Route.SourceFile` bug 在 Phase 1 一并修复——env 文件路径解析与 bodyFile 相对路径解析共享同一 Source 字段，否则两者都会在 CWD ≠ config 目录时退化。

## 2. Phase 拆分总览

| Phase | 一句话目标 | 主要新增模块 / 子命令 | 新增第三方依赖 | 前置依赖 | 估计代码量 | 状态 |
|---|---|---|---|---|---|---|
| **1** | SourceFile bug 修复 + envfile.go 基础加载（$default 合并 + $active + 默认查找） | `internal/config/envfile.go` + loader Route.SourceFile 填充 | — | v0.1 | ~400 行 | ✅ 已完成 (commit 29a1eb9..7c5a0f9) |
| **2** | CLI 整合 + osenv 白名单完整化 + watcher 同步监听 env 文件 | `internal/cli/serve.go --env/--var` flag + `internal/tpl/funcs.go` osenv 收紧 + envfile 模板渲染 | — | Phase 1 | ~500 行 | 待开始 |
| **3** | v0.2 收尾 E2E + 综合场景 + 文档回写 | `internal/cli/serve_e2e_test.go` 扩展 + docs 回写 | — | Phase 2 | ~200 行 | 待开始 |

总计：v0.2 ≈ 1100 行代码（含测试），分 3 期落地。

> **关于依赖引入时机**：v0.2 **无新增第三方依赖**。env 文件复用 Phase 2 的 `titanous/json5` 解析；watcher 复用 Phase 5 的 `fsnotify`；模板渲染复用 Phase 3 的 `easytpl` / `text-template`。go.sum 在 v0.2 期间不应有第三方依赖增删。

---

## 3. 各 Phase 详述

### Phase 1 — SourceFile 修复 + envfile.go 基础加载

**详细计划**：[v0.2/phase1-envfile.md](v0.2/phase1-envfile.md)

**目标**：让 `internal/config/Load()` 在加载主配置之后自动查找并加载同目录 `fakeserver.env.json5`，把 `$default` + 选中段合并为 `cfg.Env map[string]any`；同时修复 v0.1 `Route.SourceFile` 始终为空的存量 bug，让相对路径 `bodyFile` 和 env 文件路径正确解析。

**范围 · 包含**：

- **`internal/config/loader.go` 修 SourceFile bug**：现在的 JSON round-trip 把 `json:"-"` tagged 字段吃掉。改成在 `expandIncludes` 阶段直接基于 raw map 操作时记录每条 route 的来源文件路径，绕过 round-trip 丢字段问题。两种候选实现：(a) Route struct 加 UnmarshalJSON hook + loader 在 JSON5 解析时塞入 SourceFile sentinel；(b) loader 在 JSON5 → Config 转换后用一个并行 slice 记录每个 route 的来源文件路径，最后 zip 进 Route.SourceFile。Phase 1 Task 1 做 spike 比较两种方案的代码量与可读性。
- **`internal/config/envfile.go`** 新文件，提供 `LoadEnvFile(path, envName string) (envMap map[string]any, active string, err error)`：
  - JSON5 解析（与主配置 loader 复用 `loadFile()`，避免再造一份）
  - 合并 `$default` 段（深合并）+ 选中 env 段
  - `$active` 元字段支持（若 envName 入参为空且文件含 `$active`，按其值选段）
  - **不**支持 `@include`（design §8.2 明确禁止；遇到 `@` 开头字符串值直接报错）
- **默认查找规则**（design §8.1）：与主配置同目录的 `fakeserver.env.json5`；找不到 → 返回 `({}, "", nil)` 不报错
- **`Config.Env map[string]any` 字段**（schema.go 新增）+ `loader.Load` 在主配置加载完成后自动调 `LoadEnvFile` 并填充
- **`Config.EnvSource string` 字段**：记录命中的 env 文件绝对路径（v0.2 后续阶段 watcher 监听用）
- **新增 testdata**：`internal/config/testdata/env/{default-only, multi-env, with-active, missing-default, has-include-error}.json5`
- **单元测试**：覆盖 §8.2 / §8.3 选择优先级（仅 `$active` + 首段两条；CLI / 环境变量留 Phase 2）/ §8.4 占位输出（`.env` 字段已就位但模板访问留 Phase 2）

**范围 · 不包含**：

- CLI flag `--env / --var / FAKESERVER_ENV`（留 Phase 2）
- env 文件**值层级**的模板字符串渲染（留 Phase 2，需 tpl 接入）
- env 文件 hot-reload（留 Phase 2，需 watcher SourcePaths 扩展）
- osenv 白名单**完整生效**——v0.1 `tpl.BuildRenderCtx` 把 `.osenv` 设为空 map 占位；Phase 1 仍保持占位，Phase 2 收紧
- `.env` 在 mock body 模板里可访问（留 Phase 2，需 `tpl.BuildRenderCtx` 把 `cfg.Env` 接进来）

**前置依赖**：v0.1（需要 Phase 2 loader 框架、Phase 3 tpl 包接口稳定）。

**新增第三方依赖**：无。

**DoD**：

1. `bd lite-tools-gko` 关闭：`Route.SourceFile` 在 `config.Load` 之后非空；相对 `bodyFile` 解析按 config 文件所在目录而非 CWD（写一个回归 E2E：在 `/tmp/x/cfg.json5` 引用 `bodyFile: "data.txt"`，从 `/tmp/y` cwd 运行 fakeserver 仍能命中）
2. `LoadEnvFile("test.env.json5", "dev") → (map, active, nil)` 返回 `$default ∪ dev` 深合并结果；`active == "dev"`
3. `envName == ""` 时优先取 `$active` 字段；`$active` 缺省 → 取第一个非 `$default` 段；都没有 → 返回 `({}, "", nil)`
4. env 文件不存在 → 返回 `({}, "", nil)` + nil err（与 design §8.1 末尾"没有 env 文件 → 不报错"一致）
5. env 文件 root 非 object → 报错（与主配置相同）
6. `@include` 在 env 文件中出现 → 报错（design §8.2 明确禁止）
7. `go test ./...` 通过；`internal/config` 覆盖率维持 ≥ 80%

**对 design 章节的映射**：§8.1 文件查找 / §8.2 `$default` 合并（不含值层级模板渲染）/ §8.3 优先级链中"`$active` + 首段"两条 / §3.2 Route 字段层面接入 SourceFile / v0.1 backlog `lite-tools-gko` 修复。

**实际落地偏差**：

- **SourceFile 修复选用方案 B**（loader 内部 `__source_file__` sentinel + zip）：方案 A（Route UnmarshalJSON hook）代码量相近但侵入 Route struct；方案 B 完全限制在 loader 内部，对 Route 类型零侵入。详见 Phase 1 Task 1 spike 报告。
- **annotateRoutesWithSource 总是覆写 sentinel**：原计划"只在未设置时注入"的 guard 实际不必要（每条 route 仅在 loadFile 边界注解一次），且会让用户在 JSON5 写的 `__source_file__` 字段污染路径解析。修复后总是用 loader 的真实路径覆写——见 commit 7e8ca13。
- **JSON5 解析后键序不保证**：`titanous/json5` 把对象解码成 Go `map[string]any`，"first non-$default segment" 在多段情况下非确定。`TestLoadEnvFile_MultiEnv_FirstSegmentByDefault` 容忍 dev/staging 任一作为 active。若 v0.2 后续阶段发现用户依赖键序，需切换到 LinkedHashMap 风格的 decoder。
- **`$default` / `$active` 类型严格校验**：原 plan 用 `_, _ = .(type)` 静默忽略类型错；review 反馈后改为显式 present + type check，类型不对直接报错——见 commit 65e6d52。
- **envfile.go 不渲染值层级模板**（Phase 1 边界）：env 文件中 `"token": "{{ osenv \"X\" }}"` 在 Phase 1 直接作为字符串保留，{{ }} 表达式不展开。Phase 2 加渲染步骤。
- **`__source_file__` sentinel key 命名约定**：双下划线前后缀降低用户字段碰撞概率，但用户若真写了同名字段，loader 会无条件覆写（不是丢失数据，仍能写入路径，但用户自己的字段值被覆盖）。可接受。

**Phase 1 测试覆盖**：~22 个新增用例（loader SourceFile 3 + envfile 16 + loader 集成 2 + bodyFile 回归 1）；`internal/config` 覆盖率 88.5%（运行 `go test -cover ./internal/config/...` 后填入）。

**Phase 1 commit 流水**（8 个 commit）：
- Task 2: `29a1eb9` (SourceFile 修复) + `7e8ca13` (sentinel 覆写安全修复)
- Task 3: `4e1936c` (schema 字段 + envfile 骨架)
- Task 4: `4c38538` (LoadEnvFile 核心) + `65e6d52` (类型严格校验)
- Task 5: `abacf29` (@include 拒绝) + `105f29f` (\@xxx 转义正向测试)
- Task 6: `7c5a0f9` (loader 集成) + `8e88411` (docs 回写)

---

### Phase 2 — CLI 整合 + osenv 白名单 + env 文件 hot-reload

**详细计划**：`phase2-cli-osenv.md`（待生成）

**目标**：让 `fakeserver serve --env <name> --var k=v` 真正影响渲染——`.env` 在 mock body 模板里可访问、`osenv` 受白名单约束、env 文件改动通过现有 watcher 触发 holder swap。

**范围 · 包含**：

- **`serveOptions` 新增字段**：`EnvName string` / `VarOverrides []string`
- **gcli 注册新 flag**：`-e/--env <name>` 选段；`--var key=val`（支持多次出现，累加进 `[]string`）
- **env 选择优先级链**（design §8.3 完整）：
  ```
  1. CLI --env <name>
  2. 环境变量 FAKESERVER_ENV
  3. env 文件中字段 "$active": "dev"
  4. env 文件中第一个非 $default 段
  5. 没有 env 文件 → .env = {}（空），不报错
  ```
  Phase 1 仅实现 (3)+(4)+(5)，Phase 2 把 (1)+(2) 串到 loader 入口
- **`--var key=val,key=val` 解析**：单次 flag 支持逗号分隔；多次 flag 累加合并；最终深合并到 `.env` 顶层覆盖
- **env 文件值层级的模板渲染**（design §8.2 末尾）：env 文件中 `"token": "{{ osenv \"PROD_TOKEN\" }}"` 在 Load 阶段先 render 再合并到 `.env`。两阶段加载：先 parse + 静态合并（Phase 1），后 render（Phase 2）—— renderer 接受刚加载的 `cfg.Server.OSEnvWhitelist`
- **`tpl.BuildRenderCtx` 接入 `cfg.Env`**：v0.1 留的占位 `"env": map[string]any{}` 改为 `cfg.Env`（route body 模板里 `{{ .env.token }}` 真正取到值）
- **osenv 白名单收紧**（design §4.6）：
  - `tpl.osenv` 函数：若 `server.osenvWhitelist` 非空且 key 不在清单内 → 返回 `""` + warn 日志；空清单 → 放行所有（开发友好）
  - **env 文件中模板调用 `osenv` 同样受约束**——env 文件不是逃逸通道
  - 测试覆盖：清单空 / 非空命中 / 非空未命中 / env 文件中调用 3 种场景
- **env 文件路径加入 `cfg.SourcePaths`** → Phase 5 watcher 自动监听 → 文件变化触发 holder swap（无需新代码，仅修 SourcePaths 的填充点）
- **切换 env**：修改 env 文件中 `$active` 字段 → watcher reload → 新 cfg 含新 env 段 → swap

**范围 · 不包含**：

- 全局项目注册（`~/.config/fakeserver/projects.json`，留 v0.3）
- Web UI（留 v0.4）
- `list / use` 子命令（依赖项目注册，留 v0.3）
- 多文件 env（design §8 未提，保持单文件）

**前置依赖**：Phase 1（需要 envfile.go 已实现 + `cfg.Env` 字段就位 + `Route.SourceFile` 已修复）。

**新增第三方依赖**：无。

**DoD**：

1. `serve --env staging` 实测 `.env` 反映 staging 段值（route body 含 `{{ .env.apiHost }}` 输出 staging.apiHost）
2. `FAKESERVER_ENV=staging serve`（无 `--env` flag）等价 `--env staging`
3. `--env dev --var apiHost=overridden` → `.env.apiHost == "overridden"`
4. `--var a=1 --var b=2`（多次 flag）→ 两个 key 都生效
5. `--var a=1,b=2`（逗号分隔）→ 两个 key 都生效
6. env 文件中 `"token": "{{ osenv \"X\" }}"` 在 `osenvWhitelist` **包含** X 时生效；不包含时输出 `""` + stderr warn
7. 主配置 route body 中 `{{ .env.token }}` 在 staging 段时输出 staging 的 token
8. 编辑 env 文件 → 300ms 防抖 → holder swap → 下一个请求看到新 env 值；env 文件被删除 → 保留旧 cfg + stderr warn
9. `go test ./...` 通过；新增 env 相关用例覆盖；`internal/tpl` osenv 路径覆盖率 ≥ 80%

**对 design 章节的映射**：§8.3 完整优先级链 / §8.4 模板访问 / §8.5 热加载 / §4.6 osenv 白名单完整化。

---

### Phase 3 — v0.2 收尾 E2E + 文档回写

**详细计划**：`phase3-closure.md`（待生成）

**目标**：v0.2 完整闭环 E2E——综合 config + env 文件 + osenv + `--var` override + 热加载 env → 切换段位 → 请求验证。回写 v0.2 overview 状态 + design.md 修订记录与 §13 已落地段。

**范围 · 包含**：

- **综合 E2E**（`internal/cli/serve_e2e_test.go` 扩展或新建 `serve_e2e_env_test.go`）：
  - 写主配置 + env 文件（含 `$default + dev + staging` 三段）
  - 启动 `--env dev` → 验证 `.env.apiHost` 是 dev 值
  - 编辑 env 文件改 `$active: staging` → 等防抖 → 验证下一个请求看到 staging 值
  - 编辑 env 文件加新 key → 验证 `.env.<newkey>` 可在模板访问
  - `--var override.key=newval` → 验证覆盖优先级（CLI > env 文件 > $default）
  - osenv 白名单：env 文件用 osenv → 设白名单 → 看到值；移出白名单 → 看到 `""`
- **`bodyFile` 相对路径回归 E2E**：在非 config 目录下运行 fakeserver，相对 `bodyFile` 仍能正确解析（验证 Phase 1 的 SourceFile 修复）
- **测试覆盖率扣板**：`internal/config` ≥ 80%；新增 `internal/tpl` osenv 路径覆盖；`internal/cli` ≥ 50%（cli 包大量 main flow 代码，目标低于其他包合理）
- **回写 [`2026-05-19-fakeserver-v0.1-overview.md`](2026-05-19-fakeserver-v0.1-overview.md)**：在文档末尾追加 "v0.2 衍生事项" 段，说明 v0.2 已修复的 v0.1 backlog（SourceFile bug）
- **回写本 overview** §2 表 Phase 1/2/3 状态列均 `✅ 已完成 (commit <SHA range>)`
- **回写 `docs/fakeserver-design.md`**：修订记录追加 v0.4-phase0.2-applied 行；§13 新增"已落地（v0.2 阶段确认）"子段（覆盖 SourceFile 修复方案、envfile 两阶段加载策略、osenv 白名单生效点、env 文件 hot-reload 行为等）

**范围 · 不包含**：

- v0.3 启动（项目注册、list/use）—— 单独的 milestone
- v0.2 期间未覆盖的 design 章节（无——v0.2 范围已穷尽 §8 + §4.6）

**前置依赖**：Phase 2（需要 CLI flag + osenv + env 热加载全部就绪）。

**新增第三方依赖**：无。

**DoD**：

1. v0.2 综合 E2E 测试通过：env 文件加载 + 切换 + var override + 热加载 + osenv 白名单 5 个场景全绿
2. `bodyFile` 相对路径回归 E2E 通过：CWD ≠ config 目录时仍能命中
3. `go vet ./...` 零警告（v0.2 没有引入新 vet 债）
4. `go build ./...` + `go test ./...` 全绿
5. `internal/config` 覆盖率 ≥ 80%；`internal/tpl` 维持 ≥ 80%
6. overview.md 三个 Phase 状态行均 `✅ 已完成 (commit <SHA range>)`
7. design.md 含 `v0.4-phase0.2-applied` 修订行 + Phase 1/2/3 阶段确认条目（每 Phase 至少 3 条事实）
8. v0.2 commit 流水完整记录在本 overview "实际落地偏差"段
9. bd `lite-tools-gko` 关闭（SourceFile）；v0.2 epic issue 关闭

**对 design 章节的映射**：§8 全章收尾 / §4.6 osenv 白名单完整 / §14 v0.2 行清空"待开始"标记。

---

## 4. 跨 Phase 追踪表

| design 章节 | Phase 1 | Phase 2 | Phase 3 |
|---|:-:|:-:|:-:|
| §3.2 Route.SourceFile（v0.1 backlog 修复） | ✓ | — | 回归 E2E |
| §4.6 osenv 白名单（完整） | — | ✓ | E2E |
| §5.2 热加载（env 文件加入 SourcePaths） | — | ✓ | E2E |
| §8.1 env 文件查找 | ✓ | — | — |
| §8.2 `$default` 合并 | ✓ | + 值层级模板渲染 | — |
| §8.3 env 选择优先级（5 条规则） | 部分（3/4/5） | 完整（含 1/2） | — |
| §8.4 模板 `.env` 访问 | 字段就位 | ✓ 接入 | E2E |
| §8.5 env 文件热加载 | — | ✓ | E2E |
| §14 v0.2 行 | — | — | 清空"待开始" |

> "部分"指 Phase 1 只实现"env 文件中 `$active` + 首段 + 无文件"分支，CLI `--env` / 环境变量 `FAKESERVER_ENV` 留 Phase 2。

---

## 5. 与 design / phase plan 的关系

- **本文档（overview）**：v0.2 内部拆分的 source of truth；维护 Phase 边界与依赖关系
- **`docs/fakeserver-design.md`**：所有 Phase 共享的设计契约；任何 Phase 落地发现 design 偏差时回写 §13 "已落地"段
- **`phase<N>-*.md`**：单个 Phase 的可执行 plan，由 `superpowers:writing-plans` 在 Phase 启动前展开（详细到每一步 2–5 分钟）

执行顺序（推荐，与 v0.1 节奏一致）：

```
overview → phase1 → 执行 → 回写 design § 13 → overview 更新状态 → phase2 → ...
```

每个 Phase 完成后必须做的事：

1. 在本文档 §2 状态列写 "✅ 已完成 (commit <SHA range>)"
2. 在对应 phase 详述末尾写"实际落地偏差"（如有）
3. 如果偏差涉及未来 Phase 的设计假设，把它前置到对应 Phase 详述里
4. 回写 design 修订记录与 §13 "已落地"段——**注意：design 文档是设计契约，不是实施日志**。回写要概括，每 Phase 落地确认条目 3–5 条事实即可，**关键决策点 + 与原设计假设的偏差**；不要把 commit SHA、文件级实现细节、内部辅助函数名、测试覆盖率等堆进 design。这些落地细节的归宿是 **本 overview 的"实际落地偏差"段** + **对应 phase plan**。v0.1 阶段往 design.md §13 写了过多 commit-level 细节（如"`debug.Stack()` 仅调用一次"、"`bufferedWriter` 拦截"等），后续 v0.2 收紧此规约

---

## 6. 风险与已知不确定项

| 不确定项 | 受影响 Phase | 风险等级 | 处置 |
|---|---|---|---|
| `Route.SourceFile` 修复方案选择：(a) Route struct 加 UnmarshalJSON hook + sentinel；(b) loader 用并行 slice 记录 + zip 进 Route | Phase 1 | 中 | Phase 1 Task 1 spike 两种方案，比较代码量与可读性后定 |
| env 文件**值层级**模板渲染时机：在 Load 阶段渲染需要先有 osenv whitelist；但 whitelist 在 `cfg.Server` 中——加载顺序耦合 | Phase 2 | 中 | Phase 2 Task 1 设计两阶段加载：先 parse + 静态合并（Phase 1 已完成），后 render（Phase 2，此时 `cfg.Server.OSEnvWhitelist` 已确定）|
| osenv 白名单约束如何应用到 env 文件里的 osenv 调用：在 `tpl.FuncMap` 重新生成时按 cfg 注入 whitelist，还是 envfile.go 独立持白名单 | Phase 2 | 低 | Phase 2 Task 实现时按"tpl 包统一 osenv 实现，envfile.go 调 tpl renderer"策略，避免逻辑分裂 |
| `--var key=val,key=val` 累加 vs 替换的语义：design §8 未明确，业界惯例是累加（多次出现合并） | Phase 2 | 低 | Phase 2 plan 明确按惯例做累加；用例覆盖"多次 flag"与"逗号分隔"两种语法 |
| watcher 监听 env 文件路径：env 文件被删除时如何处理（保留旧 cfg？报错？） | Phase 2 | 低 | 与主配置 Validate 失败时同策略：stderr warn + 保留旧 cfg/router；不让删除 env 文件破坏运行中的 server |
| env 文件循环引用：envfile 不支持 @include，但 env 段值是字符串，模板内可调 osenv——理论无循环，但若 `osenv "X"` 返回的字符串本身又是模板字符串怎么办（不应递归渲染） | Phase 2 | 低 | Phase 2 plan 明确：env 文件值层级**只渲染一次**（一遍 Execute），不做递归扩展 |

每个不确定项在对应 Phase plan 的"前置探测"步骤里**先 spike 再实现**，避免实现到一半发现底层假设错。

---

## 7. v0.2 → v0.3 衔接

v0.2 完成后，v0.3 入口已就绪：

- env 文件全部段名已可枚举（envfile.go 的副产物）→ design §10.2 `projects[].envs` 字段直接复用
- `cfg.Env` map 已有 → projects.json 记录 `lastEnv` 字段直接读
- Hot-reload 链路完整 → web UI（v0.4）的"软切换 env"可基于同一 holder swap 机制

v0.3 范围（design §10）：

- `internal/registry/` 包：`~/.config/fakeserver/projects.json` 读写 + 跨进程文件锁
- `list / use` 子命令
- PID 文件 + 探活

预计 v0.3 ≈ 2 Phase，~700 行代码。

---

## 自检（v0.2 overview 完成度）

| 检查项 | 结果 |
|---|---|
| 每 Phase 有"一句话目标 + 范围（含/不含）+ 前置依赖 + DoD + design 映射" | ✓ |
| 3 Phase DoD 加起来覆盖 v0.2 范围（design §8 + §4.6 + lite-tools-gko） | ✓ |
| Phase 边界明确（Phase 1 纯加载、Phase 2 整合、Phase 3 收尾） | ✓ |
| 跨 Phase 追踪表覆盖所有 design 章节 | ✓ |
| 风险与不确定项均映射到具体 Phase + 处置方案 | ✓ |
| 与 v0.1 overview 格式 / 拆分原则一致 | ✓ |
| 无新增第三方依赖（v0.2 完全复用 v0.1 deps） | ✓ |
| v0.3 衔接段落已说明 | ✓ |
