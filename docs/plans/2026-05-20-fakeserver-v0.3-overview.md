# Fakeserver v0.3 阶段规划总览

> 本文档把 v0.3（全局项目注册 + `list/use` 子命令 + PID 文件）按"独立可测可交付"原则拆成 2 个 Phase。每个 Phase 对应一份 `phase<N>-*.md` 详细计划，由 `superpowers:writing-plans` 在执行前展开。
>
> **前置**：v0.2 已于 2026-05-20 完整闭环（见 [2026-05-20-fakeserver-v0.2-overview.md](2026-05-20-fakeserver-v0.2-overview.md)）。

## 修订记录

| 日期 | 版本 | 作者 | 变更说明 |
|---|---|---|---|
| 2026-05-20 | v0.3-overview | inhere | 初稿。固化 v0.3 内部的 2 Phase 拆分（registry/PID + list/use CLI）|
| 2026-05-20 | v0.3-overview-phase1-applied | inhere | Phase 1 落地：registry 包（store/lock/pid）+ serve 启动期 Upsert + PID 文件 |
| 2026-05-20 | v0.3-overview-phase2-applied | inhere | Phase 2 落地：list/use 子命令 + envs 提取 + 跨进程并发 E2E + v0.3 milestone 闭环 |

后续修订：每完成一个 Phase 后在对应行回写 commit 摘要与实际偏差。

---

## 1. v0.3 范围与拆分原则

**v0.3 目标**：把 design §10 列出的全局项目注册能力完整接入。

design §14 路线图明确 v0.3 范围**仅含项目注册 + list/use + PID 文件**——Web UI（§11）继续留 v0.4，WS/SSE / 录制回放 / json-server CRUD 留 v1.0+。

把 v0.3 切成 2 个 Phase 的依据（与 v0.1/v0.2 拆分原则一致）：

1. **每个 Phase 末尾必须能 `go run ./cmd/fakeserver serve` 跑出一个对用户可用的产物**——Phase 1 完成后 `serve` 启动期自动写入 `~/.config/fakeserver/projects.json` + `<cwd>/.fakeserver/run.pid`，退出时清理 PID（用户无感知，但通过文件 inspect 可见效果）；Phase 2 完成后 `fakeserver list / use` 子命令可用，能看到所有项目的 running/idle 状态。
2. **每个 Phase 内部模块依赖封闭**：Phase 1 全部新增代码都在 `internal/registry` 包内部 + `serve.go` 极小接入；Phase 2 仅新增 `internal/cli/list.go` `internal/cli/use.go` 与对 registry 包的只读消费。
3. **每个 Phase 自带完整测试**——单元 + 集成 + E2E，不靠后续 Phase 兜底。
4. **跨进程文件锁的选型在 Phase 1 Task 1 先做 spike**：是用 `github.com/gofrs/flock` 引入新依赖（跨平台库），还是 `build tag` 拆分自己写（POSIX `flock` syscall + Windows `LockFileEx`）。**这是 v0.3 唯一可能引入新依赖的点**，必须先 spike 比较后再选。

## 2. Phase 拆分总览

| Phase | 一句话目标 | 主要新增模块 / 子命令 | 新增第三方依赖 | 前置依赖 | 估计代码量 | 状态 |
|---|---|---|---|---|---|---|
| **1** | `internal/registry` 包（projects.json 读写 + 跨进程文件锁 + PID 文件）+ serve 启动期 Upsert | `internal/registry/{store,lock,pid}.go` + `internal/cli/serve.go` 接入 | `github.com/gofrs/flock v0.13.0`（Task 1 spike 决策）| v0.2 | ~600 行 | ✅ 已完成 (commit 9dba198..644b9e9) |
| **2** | `fakeserver list / use` 子命令 + envs 提取 + 进程探活 + 综合 E2E + docs 回写 | `internal/cli/{list,use}.go` + registry 探活接口 + envs 提取助手 | — | Phase 1 | ~500 行 | ✅ 已完成 (commit 0e7277e..7227a23) |

总计：v0.3 ≈ 1100 行代码（含测试），分 2 期落地。

> **关于依赖引入时机**：v0.3 可能引入 `github.com/gofrs/flock`（跨平台文件锁）——这是 v0.2 overview "无新增第三方依赖"陈述在 v0.3 的明确松绑。Phase 1 Task 1 spike 后若选自写 syscall 路线则保持零新依赖；若选 `gofrs/flock` 则 go.mod / go.sum 在 Phase 1 增 1 个 direct dep。

---

## 3. 各 Phase 详述

### Phase 1 — registry 包 + 启动期接入 + PID 文件

**详细计划**：[v0.3/2026-05-20-fakeserver-v0.3-phase1-registry.md](v0.3/2026-05-20-fakeserver-v0.3-phase1-registry.md)

**目标**：让 `internal/registry.Upsert(project)` 把当前 fakeserver 进程的项目元数据落到 `~/.config/fakeserver/projects.json`，并在 `<cwd>/.fakeserver/run.pid` 写 `pid\nport\nstartedAt`；`serve` 启动末尾调用 + 退出时（含信号退出）清理 PID 文件。

**范围 · 包含**：

- **`internal/registry/store.go`** 新文件：
  - `type Project struct` — id / name / configPath / cwd / envs / lastEnv / lastPort / lastRunAt / pidFile，对应 design §10.2
  - `type Registry struct` — version / lastActiveId / projects
  - `Load() (*Registry, error)` — 读 `~/.config/fakeserver/projects.json`；不存在 → 返回空 Registry + nil err；解析失败 → 改名 `.bak-<unix-ts>` + 重建空 + warn 日志
  - `Save(*Registry)` — 原子写：`projects.json.tmp` → `fsync` → `rename` 原子替换
  - `Upsert(p Project)` — 按 id 替换或追加；同时更新 lastActiveId（仅 serve 启动期；Phase 2 use 子命令也会调）
  - `Get(id)` / `Remove(id)` — Phase 2 list/use 用
  - `ProjectID(configAbsPath string) string` — `sha1(configAbsPath)` 前 12 hex（design §10.2）
- **`internal/registry/lock.go`** 新文件：
  - **Task 1 spike** 决定实现路径（`gofrs/flock` vs 自写 syscall）
  - 接口：`func WithLock(path string, fn func() error) error` — 拿锁 → 调 fn → 释放锁
  - 上锁失败重试 5 次（指数退避 10ms~160ms），仍失败 → warn 不阻塞（design §10.3）
  - 配套测试：并发 `WithLock` 在不同 goroutine 中串行执行（不验证跨进程，留 Phase 2 E2E）
- **`internal/registry/pid.go`** 新文件：
  - `WritePIDFile(path string, pid int, port int, startedAt time.Time) error` — 自动创建父目录（0700）
  - `ReadPIDFile(path string) (pid, port int, startedAt time.Time, err error)`
  - `RemovePIDFile(path string)` — 调用时若文件不存在不报错
  - `IsAlive(pid int) bool` — `os.FindProcess` + `signal(0)`（design §10.4）
  - 退出清理：v0.2 已有信号处理钩子吗？查 `internal/cli/serve.go` 现有 graceful shutdown 实现；若没有则**本 Phase 加上**（含 SIGINT / SIGTERM + Windows ctrl+c）
- **`internal/cli/serve.go` 接入**：
  - 启动末尾：`projectID = registry.ProjectID(cfg.SourcePaths[0])`；构建 Project（cwd / port / envs—Phase 1 envs 暂传 nil，Phase 2 补全）→ `registry.WithLock(...)` → `Load` → `Upsert` → `Save`
  - 启动末尾：`WritePIDFile`
  - 退出钩子：`RemovePIDFile`（含信号退出路径）
  - **失败策略**：registry 写入或 PID 写入失败均仅 warn，不阻塞 serve 启动（mock 优先级高于注册）
- **新增 testdata**：测试用 fixture（小 projects.json 样本，含 1/0/损坏三种）
- **新增单元测试**：
  - `store_test.go`：Load 不存在 → 空；Load 损坏 → bak + 空 + 验证 .bak 文件创建；Save → Load 往返；Upsert 同 id 替换、不同 id 追加；ProjectID 同输入幂等
  - `lock_test.go`：单进程内 N goroutine 串行；重试退避验证（不强求精确时间，验证总耗时上限）
  - `pid_test.go`：写读往返；IsAlive 当前进程 true；IsAlive 极大 pid（如 999999） false；Remove 不存在文件不报错
  - `serve_v03_e2e_test.go`：serve 启动 → 期望 PID 文件存在 + projects.json 含一条记录；停服 → PID 文件被删除

**范围 · 不包含**：

- `fakeserver list / use` 子命令（留 Phase 2）
- envs 字段填充（留 Phase 2，需 env 文件段名提取助手）
- 死进程的 PID 文件自动清理（留 Phase 2 list 时按需清理）
- `server.projectName` 配置覆盖（留 Phase 2）
- lastActiveId 在 Phase 1 设为"当前启动项目"——Phase 2 use 子命令会改它

**前置依赖**：v0.2（需要 cfg.SourcePaths 含主配置绝对路径；v0.2 Phase 1 SourceFile 修复就是此条前置）。

**新增第三方依赖**：Task 1 spike 决定。

**DoD**：

1. `internal/registry/store.go` `Load/Save/Upsert/Get/Remove/ProjectID` 全部有测试覆盖
2. 损坏的 projects.json 文件自动恢复为 .bak + 重建空（含 .bak 文件名时间戳格式校验）
3. `internal/registry/lock.go` 单进程并发安全（goroutine 级测试通过）；跨进程留 Phase 2 E2E 验证
4. `internal/registry/pid.go` 三个核心函数 PASS；IsAlive 在当前进程 / 已死 pid 两种情况都正确
5. serve 启动期 `~/.config/fakeserver/projects.json` 出现当前项目条目；停服后 PID 文件被清理
6. `go build ./...` 通过；`go test ./...` 全绿；`go vet ./...` 零警告
7. `internal/registry` 覆盖率 ≥ 80%（新包，新阈值）；`internal/cli` 维持当前水平不下降
8. 若 Task 1 spike 选 `gofrs/flock`：`go.mod` 增 1 direct dep；`go.sum` 同步；overview 表"新增第三方依赖"列改写

**对 design 章节的映射**：§10.1（路径约定）/ §10.2（文件结构）/ §10.3（并发安全）/ §10.4（PID 文件）→ §10.5（CLI 行为）的"写入侧"前半段。

**实际落地偏差**：

- **gofrs/flock v0.13.0 选型确认**：Task 1 spike 在 Windows 上验证同进程双 flock 实例 TryLock 互斥成立（fd 级锁），可用于跨进程锁 + 同进程 goroutine 互斥。go.mod 增 1 direct dep；indirect 升级 `golang.org/x/sys v0.30.0 → v0.37.0`。
- **lock 测试拆为两类反映 design §10.3 真实契约**：低竞争（N=5）严格串行 + 高竞争（N=30，临界区 50ms）不阻塞 warn-fallback。原计划"统一一个 goroutine 串行测试"未反映 design "仍失败仅 warn 不阻塞"的语义——直接套大并发会和 fallback 路径冲突。修正测试名为 `TestWithLock_SerializesGoroutines_LowContention` + `TestWithLock_HighContention_DoesNotBlock`。
- **IsAlive 拆 build-tag**：POSIX `signal(0)` + EPERM 容忍，Windows `FindProcess` 成功即真。两实现死 pid 行为一致（false），EPERM 边界 POSIX 视作 alive、Windows 不区分（成功 = alive）。
- **集成 E2E 不真起 runServe**：runServe 绑端口 + 阻塞 select 不适合普通 unit test；本 Phase 在 t.TempDir 串联 store/lock/pid 验证三原语协作。真实 runServe 全链路（含信号退出 PID 清理）的 E2E 留 Phase 2 子进程测试。
- **registry 覆盖率冲到 80.9%**：达 DoD 阈值，但 `Save` 仅 46.4% / `ReadPIDFile` 68.8% 是因为成功路径已覆盖、错误路径主要为 disk-IO 失败/fsync 失败/rename 失败 等难触发场景；补了"父路径已为文件 → mkdir 失败"和"PID 文件字段格式错"用例。

**Phase 1 测试覆盖**：12 个新增用例（store 7 + lock 3 + pid 7 + e2e 1）；`internal/registry` 80.9% (≥80%)；既有 cli/config/tpl 包覆盖率不下降。

**Phase 1 commit 流水**：
- Task 1: `9dba198` (gofrs/flock 选型 + go.mod)
- Task 2: `e7cbd46` (store.go)
- Task 3: `83c4768` (lock.go + 低/高竞争两类测试)
- Task 4: `42cb84a` (pid.go + IsAlive 跨平台)
- Task 5: `3ac36fe` (serve.go 接入)
- Task 6: `644b9e9` (集成 E2E + docs 回写 + 覆盖率补测)

---

### Phase 2 — `list` / `use` 子命令 + envs 提取 + 进程探活 + 综合 E2E

**详细计划**：（Phase 1 完成后展开）

**目标**：让 `fakeserver list` 表格输出全部已注册项目的 running/idle/port/env/last-run；`fakeserver use <id>` 更新 `lastActiveId`（不启动 server，仅做"下次 web UI 默认聚焦"准备，design §10.5）。

**范围 · 包含**：

- **`internal/cli/list.go`** 新文件：
  - 子命令注册（gcli）：`fakeserver list`
  - 输出格式按 design §10.5 表格（ID / NAME / STATUS / PORT / ENV / LAST RUN）
  - 探活：每个 project 读 PID 文件 → `registry.IsAlive(pid)`；死进程清理 PID 文件并标记 `idle`，活进程标记 `running` + port
  - 排序：lastActiveId 项目优先 → 其余按 lastRunAt 倒序
  - 空注册表：友好提示"无已注册项目，运行 `fakeserver serve` 自动注册当前项目"
- **`internal/cli/use.go`** 新文件：
  - 子命令注册：`fakeserver use <id>`
  - 校验 id 存在 → 更新 lastActiveId → Save
  - 支持前缀匹配（`fakeserver use a1b2`，唯一前缀即可）
- **envs 提取助手**（`internal/registry/envs.go` 或加到 store.go）：
  - `ExtractEnvNames(envFilePath string) []string` — 读 env 文件，返回所有非 `$default` / `$active` 段名，按字母序
  - serve 启动期把结果塞进 Project.envs
- **`server.projectName` 配置覆盖**（v0.2 schema 已有？需 grep；若无则在 schema.go 加 string 字段；默认空 → registry 回退到 cwd dir basename）
- **`registry.WithLock` 跨进程测试**：在 E2E 中起 2 个子进程同时 Upsert，验证 projects.json 不丢条目
- **综合 E2E** (`internal/cli/list_e2e_test.go`)：
  - 启动 serve（写注册 + PID）→ 跑 `fakeserver list` → 验证 STATUS 是 running、PORT 正确
  - 杀进程 → 跑 `fakeserver list` → 验证 STATUS 是 idle 且 PID 文件被清理
  - `fakeserver use <id>` → 验证 lastActiveId 更新
- **文档收尾**：
  - v0.3 overview 回写 Phase 1/2 commit 流水
  - design.md 修订记录追加 `v0.4-phase0.3.1-applied` + `v0.4-phase0.3.2-applied`
  - design.md §13 追加 Phase 1/2 各 3 条事实
  - bd v0.3 epic 关闭

**范围 · 不包含**：

- Web UI（留 v0.4）
- `fakeserver stop <id>` / 跨项目控制（design 未列；留 v1.x）
- 历史项目自动清理策略（如 90 天未运行的自动删除；留未来 milestone）
- 网络可达性探测（list 只通过 PID + signal(0) 探活，不发 HTTP probe）

**前置依赖**：Phase 1（registry 包就绪 + serve 启动期已注册）。

**新增第三方依赖**：无。

**DoD**：

1. `fakeserver list` 输出表格符合 design §10.5 样例
2. `fakeserver use <id>` / 前缀匹配 PASS
3. envs 字段在 serve 启动期被填充（projects.json 含 envs 数组）
4. 跨进程并发 Upsert E2E 通过（projects.json 不丢条目）
5. 死进程探活 + PID 文件清理在 list 时自动完成
6. `go vet ./...` 零警告；全包绿；overview 三个状态行均 `✅ 已完成`
7. design.md §13 含 Phase 1 + Phase 2 阶段确认（每段 3 条事实）
8. bd v0.3 总 epic 关闭；`bd ready` 不含任何 v0.3 相关 issue

**对 design 章节的映射**：§10.5（CLI 行为）/ §10.3 跨进程文件锁的 E2E 验证 / §10.4 探活完整实现 / §14 v0.3 行清空"待开始"。

**实际落地偏差**：

- **`ExtractEnvNames` 放到 `internal/config` 包而非 `internal/registry`**：原 plan 设想 `registry/envs.go`，但 envs 提取需要复用 `config.loadFile`（包内函数），把它放到 `config/envfile.go` 与 `LoadEnvFile` 并排更干净；避免 registry → config 反向依赖。serve.go 调用方改为 `config.ExtractEnvNames`。
- **`server.projectName` 配置覆盖未实现**：原计划包括"server.projectName 显式覆盖 name 字段"，实际落地 name 字段直接取 `filepath.Base(filepath.Dir(mainCfg))`。配置层加 string 字段属于 schema 改动，与 Phase 2 的 CLI 子命令目标不强相关；推迟到未来视用户反馈再加。design §10.2 字段描述保持"用户可在 server.projectName 显式覆盖"的设计意图，但本 Phase 不消费。
- **list 死进程 PID 清理为 in-process 删除文件，不修改 projects.json**：design §10.4 "死进程清理 PID 文件" 已实现；但 Project.PIDFile 字段保留指向同一路径——下次 serve 重新启动会复用此 PIDFile 值并写新内容。简化了 list 的逻辑（不需要 Save 回 projects.json），且不影响 design §10.5 契约。
- **跨进程 E2E 用 `TestMain` helper 模式**：通过环境变量 `FAKESERVER_REG_HELPER=1` 把测试二进制双用为 helper 子进程，避免另起一个独立 main 包。N=4 个子进程同时 Upsert 不同 id 验证文件锁正确性。

**Phase 2 测试覆盖**：~16 个新增用例（envs 5 + list 4 + use 7+ + cross-process 1 + 综合 E2E 1）；`internal/registry` 80.9%；`internal/cli` 49.9%（v0.2 时 42.3%，本 Phase 提升 7.6 个百分点）；`internal/config` 88.3%（不下降）。

**Phase 2 commit 流水**：
- Task 1: `0e7277e` (ExtractEnvNames + serve 接入)
- Task 2: `c243280` (list 子命令)
- Task 3: `0db3225` (use 子命令)
- Task 4: `95c3448` (跨进程并发 E2E)
- Task 5: `7227a23` (综合 E2E)
- Task 6: 文档收尾（本次 commit）

---

## v0.3 Milestone 闭环

**v0.3 = design §10 (全局项目注册 + list/use + PID 文件) 完整落地**。

| Phase | 提交范围 | 主要交付 |
|---|---|---|
| Phase 1 | `9dba198..af19a16` | registry 包（store/lock/pid）+ serve 启动期 Upsert + PID 文件 |
| Phase 2 | `0e7277e..7227a23` | envs 提取 + list/use 子命令 + 跨进程并发 E2E + 综合 E2E |

新增第三方依赖：`github.com/gofrs/flock v0.13.0`（跨平台文件锁）。

v0.4 入口已就绪（design §11 Web UI + §12 请求历史 ring buffer + SSE 推送）。

---

## 4. 跨 Phase 追踪表

| design 章节 | Phase 1 | Phase 2 |
|---|:-:|:-:|
| §10.1 路径约定（`~/.config/fakeserver/projects.json`） | ✓ | — |
| §10.2 文件结构（Project / Registry struct） | ✓ | + envs 字段填充 |
| §10.3 并发安全（原子写 + 文件锁 + 损坏恢复） | ✓ | + 跨进程 E2E |
| §10.4 PID 文件 + 探活 | 写入 + IsAlive 函数 | list 时清理死进程 PID |
| §10.5 CLI 行为（list / use） | — | ✓ |
| §14 v0.3 行 | — | 清空"待开始" |

---

## 5. 与 design / phase plan 的关系

- **本文档（overview）**：v0.3 内部拆分的 source of truth；维护 Phase 边界与依赖关系
- **`docs/fakeserver-design.md`**：所有 Phase 共享的设计契约；任何 Phase 落地发现 design 偏差时回写 §13 "已落地"段（**严格 3 条事实** —— v0.2 overview §5 已固化此规约）
- **`phase<N>-*.md`**：单个 Phase 的可执行 plan，由 `superpowers:writing-plans` 在 Phase 启动前展开（详细到每一步 2–5 分钟）

执行顺序（推荐，与 v0.1 / v0.2 节奏一致）：

```
overview → phase1 → 执行 → 回写 design §13 → overview 更新状态 → phase2 → ...
```

每个 Phase 完成后必须做的事：

1. 在本文档 §2 状态列写 "✅ 已完成 (commit <SHA range>)"
2. 在对应 phase 详述末尾写"实际落地偏差"（如有）
3. 如果偏差涉及未来 Phase 的设计假设，把它前置到对应 Phase 详述里
4. 回写 design 修订记录与 §13 "已落地"段——**注意：design 文档是设计契约，不是实施日志**。每 Phase 落地确认条目**严格 3 条事实**（v0.2 overview 已固化）；不要把 commit SHA、文件级实现细节、内部辅助函数名、测试覆盖率等堆进 design。这些落地细节的归宿是**本 overview 的"实际落地偏差"段** + **对应 phase plan**。

---

## 6. 风险与已知不确定项

| 不确定项 | 受影响 Phase | 风险等级 | 处置 |
|---|---|---|---|
| 跨平台文件锁选型（gofrs/flock vs 自写 syscall） | Phase 1 | 中 | Phase 1 Task 1 先 spike 比较后再选 |
| `~/.config` 在 Windows 是否真的可用（design §10.1 明确强制此路径） | Phase 1 | 低 | Task 1 同时验证 `os.UserHomeDir() / ".config" / "fakeserver"` 在 Windows / Linux / macOS 三平台 |
| `cwd` 与 `cfg.SourcePaths[0]` 目录在用户跨目录启动时不一致 | Phase 1 | 低 | PID 文件路径按 design 走 `<cwd>/.fakeserver/run.pid`；configPath 用绝对路径 |
| `signal(0)` 探活在 Windows 下 `os.FindProcess` 行为差异 | Phase 1 / 2 | 中 | Phase 1 Task 3 pid_test 中显式覆盖 Windows + Linux 路径；如不一致写 build tag |
| 损坏 projects.json 自动 `.bak` 文件累积问题（用户长期使用堆积） | Phase 1 | 低 | 暂不实现自动清理，留 v0.4+ 决定（如保留最近 3 个）|
| serve 启动期 registry / PID 写入失败时的用户体验 | Phase 1 | 低 | 失败仅 warn 日志，不阻塞 serve（mock 是主要价值；注册是辅助功能）|

每个不确定项在对应 Phase plan 的"前置探测"步骤里**先 spike 再实现**，避免实现到一半发现底层假设错（v0.1 rux v2 API 探测、v0.2 文件锁选型都是案例）。

---

## 7. 自检

| 项 | 结果 |
|---|---|
| 每 Phase 末尾可 `go run ./cmd/fakeserver serve` 跑出对用户可用产物 | ✓（Phase 1 写文件可见；Phase 2 list/use 命令可用）|
| Phase 边界清晰（registry 内核 vs CLI 子命令）| ✓ |
| 每个 Phase 自带测试（单元 + 集成 + E2E）| ✓ |
| 跨 Phase 追踪表覆盖 design §10 全部小节 | ✓ |
| 新依赖引入有 spike 兜底 | ✓（Phase 1 Task 1）|
| 失败策略明确（registry 失败仅 warn 不阻塞 serve）| ✓ |
| 与 v0.2 overview 节奏 / 模板一致 | ✓ |
