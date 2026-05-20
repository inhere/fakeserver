# Fakeserver v0.3 · Phase 2 — `list / use` 子命令 + envs 提取 + 综合 E2E + v0.3 milestone 闭环

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。

**Goal**：让 `fakeserver list` / `fakeserver use <id>` 子命令落地——表格输出全部已注册项目的 running/idle/port/env/last-run、`use` 更新 `lastActiveId`；同时把 serve 启动期 Project.envs 字段填齐；最后做跨进程并发 + 综合 E2E + v0.3 milestone 闭环（design §10 全章收尾）。

**前置**：v0.3 Phase 1 已完成（commit `9dba198..af19a16`，registry 包 + 启动期 Upsert + PID）。

**Phase 2 完成定义（DoD）**（与 overview §3 Phase 2 DoD 一致）：

1. `fakeserver list` 输出表格符合 design §10.5 样例（ID/NAME/STATUS/PORT/ENV/LAST RUN，6 列）
2. `fakeserver use <id>` / 前缀匹配 PASS
3. envs 字段在 serve 启动期被填充（projects.json 含 envs 数组）
4. 跨进程并发 Upsert E2E 通过（projects.json 不丢条目）
5. 死进程探活 + PID 文件清理在 list 时自动完成
6. `go vet ./...` 零警告；全包绿；overview 三个状态行均 `✅ 已完成`
7. design.md §13 含 v0.3 Phase 1 + Phase 2 阶段确认（每段 **严格 3 条事实**）
8. bd v0.3 总 epic 关闭；`bd ready` 不含任何 v0.3 相关 issue

---

## 文件结构（Phase 2 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 新建 | `internal/registry/envs.go` | `ExtractEnvNames(envFilePath string) []string` 助手 |
| 新建 | `internal/registry/envs_test.go` | envs 提取单元测试 |
| 修改 | `internal/cli/serve.go` | 启动期填充 Project.envs |
| 新建 | `internal/cli/list.go` | `fakeserver list` 子命令 + 探活 + PID 清理 |
| 新建 | `internal/cli/list_test.go` | list 单元 + 表格格式断言 |
| 新建 | `internal/cli/use.go` | `fakeserver use <id>` 子命令 + 前缀匹配 |
| 新建 | `internal/cli/use_test.go` | use 单元 + 前缀匹配测试 |
| 修改 | `internal/cli/app.go` | 注册 list/use 命令 |
| 新建 | `internal/registry/concurrent_e2e_test.go` | 跨进程并发 Upsert（起子进程） |
| 修改 | `docs/plans/2026-05-20-fakeserver-v0.3-overview.md` | Phase 2 状态 + v0.3 milestone 闭环段 |
| 修改 | `docs/fakeserver-design.md` | 修订记录 + §13 v0.3 Phase 2 阶段确认（**严格 3 条事实**） |

---

## Task 1 — envs 提取助手 + serve 启动期填充 Project.envs

**Files**: `internal/registry/envs.go` + `_test.go`，`internal/cli/serve.go`

### Step 1.1 测试先行（TDD）
覆盖：典型 env 文件（dev/staging/prod 三段）→ 返回 `[dev, prod, staging]`（字母序）；含 `$default` / `$active` → 已排除；空文件或 root 非 object → 返回 `[]`；文件不存在 → 返回 `[]` + nil。

### Step 1.2 实现
利用现有 `config.LoadEnvFile` 不行 —— 它只返回某一段。需要直接读 + JSON5 解析 + 列出顶层非 `$default`/`$active` 的 key。复用 `config.loadFile` 加 build constraint 不暴露 internal。或者：自己读 + 用 `titanous/json5` 解析。前者更省事。

### Step 1.3 serve.go 接入
找到 v0.3 Phase 1 添加的 `proj := registry.Project{...}` 段，在 `Envs: nil` 处改为：

```go
envFile := config.DefaultEnvFileName // "fakeserver.env.json5"
envFilePath := filepath.Join(filepath.Dir(mainCfg), envFile)
envs := registry.ExtractEnvNames(envFilePath)
// 再填入 proj.Envs = envs
```

### Step 1.4 跑测 + Commit
```
go test ./internal/registry/... -v -run "ExtractEnv" -count=1
go test ./... -count=1
git add ... && git commit -m "feat(registry): ExtractEnvNames + serve 启动期填充 Project.envs"
```

---

## Task 2 — `fakeserver list` 子命令

**Files**: `internal/cli/list.go` + `_test.go`，`internal/cli/app.go`

### Step 2.1 测试先行
- list 表头格式：`ID  NAME  STATUS  PORT  ENV  LAST RUN`（design §10.5 样例）
- 单个项目 running（PID alive）→ STATUS=running + PORT 显示
- 单个项目 idle（PID 文件不存在 / pid 已死）→ STATUS=idle + PORT=`-`
- 死进程的 PID 文件被自动清理（list 副作用）
- 空注册表 → 友好提示 `no projects registered yet; run 'fakeserver serve' to register the current project.`
- 排序：lastActiveId 项目优先 → 其余按 lastRunAt 倒序

### Step 2.2 实现
- 注册 gcli 命令 `list`
- 用 `text/tabwriter` 输出 6 列表格
- 探活：每个 project 若 `PIDFile != ""` → `ReadPIDFile` → `IsAlive` → 死则 `RemovePIDFile` 并标 idle；活则 running + 取 PIDFile 中 port

### Step 2.3 跑测 + Commit
```
go test ./internal/cli/... -v -run "TestList_" -count=1
git add ... && git commit -m "feat(cli): list 子命令——表格输出 + 探活 + PID 自动清理"
```

---

## Task 3 — `fakeserver use <id>` 子命令

**Files**: `internal/cli/use.go` + `_test.go`，`internal/cli/app.go`

### Step 3.1 测试先行
- 精确 id 命中：lastActiveId 更新
- 唯一前缀命中：解析到正确 id
- 多个项目共享前缀 → 报错 "ambiguous prefix"
- 不存在的 id → 报错 "no project matches"
- 空注册表 → 报错

### Step 3.2 实现
- 注册 gcli 命令 `use`，args=[id]
- resolveID(reg, prefix) 助手：精确匹配 → 返回；否则前缀扫描；多个匹配 → ambiguous

### Step 3.3 跑测 + Commit
```
go test ./internal/cli/... -v -run "TestUse_" -count=1
git add ... && git commit -m "feat(cli): use 子命令——更新 lastActiveId + 前缀匹配"
```

---

## Task 4 — 跨进程并发 Upsert E2E

**Files**: `internal/registry/concurrent_e2e_test.go`

测试场景：起 4 个子进程同时 WithLock + Upsert 不同 id，验证 projects.json 最终含 4 条记录、无丢失。子进程通过 `go test -run TestX` 的 `os.Args[0]` + 环境变量切换辅助 main 模式。Windows / POSIX 一致。

---

## Task 5 — 综合 E2E（serve → list → use 全链路）

**Files**: `internal/cli/list_e2e_test.go`

场景：t.TempDir 模拟 HOME 与 CWD → 装一份最小 cfg → 不真起 runServe，而是手动用 registry 写入一条带 PID 的项目 → 调 `runList()` 直接函数级断言输出含 `running`；kill PID（其实没启动，可以直接删 PIDFile 或者用 IsAlive 配合假 pid）→ 再调 runList → 断言 idle 且 PIDFile 被清理；`runUse(<id>)` → 断言 lastActiveId 更新。

---

## Task 6 — DoD 核对 + 文档收尾 + bd close + v0.3 milestone 闭环

```
go build ./... && go vet ./... && go test ./... -count=1
go test -cover ./internal/{registry,cli}
```

回写：
- v0.3 overview §2 Phase 2 行 `✅ 已完成 (commit <T1 SHA>..<T6 SHA>)`
- v0.3 overview Phase 2 详述末尾"实际落地偏差" + commit 流水
- v0.3 overview 追加 "v0.3 Milestone 闭环" 段
- design.md 修订记录追加 `v0.4-phase0.3.2-applied`
- design.md §13 追加"v0.3 Phase 2 阶段确认"（**严格 3 条事实**）
- bd Phase 2 epic + v0.3 milestone epic 创建并关闭

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每 Task 自带测试 | ✓ |
| 不污染 v0.1/v0.2 既有测试 | ✓ |
| 死进程 PID 文件自动清理 | ✓ (Task 2) |
| 前缀匹配 | ✓ (Task 3) |
| 跨进程并发 | ✓ (Task 4) |
| design.md 回写**严格 3 条事实** | ✓ |
