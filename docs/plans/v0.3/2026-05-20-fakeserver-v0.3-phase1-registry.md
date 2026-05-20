# Fakeserver v0.3 · Phase 1 — registry 包骨架 + 启动期 Upsert + PID 文件

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。复选框 `- [ ]` 用于跟踪执行进度。建议使用 `superpowers:subagent-driven-development` 或 `superpowers:executing-plans` 来逐任务执行。

**Goal**：让 `internal/registry` 包从零落地——projects.json 读写 + 跨进程文件锁 + 损坏文件自动 bak + PID 文件读写 + 进程探活；并把 `serve` 子命令的启动末尾改造为"注册当前项目 + 写 PID 文件"、退出路径补上"清理 PID 文件"。

**Architecture**：

1. **`internal/registry`** 新包，三文件：
   - `store.go`：纯数据 + JSON 序列化 + 原子写
   - `lock.go`：跨进程文件锁（Task 1 spike 决定实现路径）
   - `pid.go`：PID 文件 IO + `IsAlive(pid)` 探活
2. **`internal/cli/serve.go` 接入**：runServe 启动尾段调一次 `registry.Upsert + WritePIDFile`；defer / shutdown 路径调 `RemovePIDFile`。
3. **失败策略**：registry / PID 写入失败均仅 warn，不阻塞 serve 启动（mock 路径优先；注册是 v0.3 新增辅助功能，不能让它打断已有功能）。

**Tech Stack**：

- 标准库：`crypto/sha1`、`encoding/json`、`os`、`os/signal`、`path/filepath`、`time`
- **新增第三方依赖（Task 1 决定）**：候选 `github.com/gofrs/flock`（跨平台文件锁）；备选自写 build-tag 拆分

**前置要求**：

- v0.2 Phase 3 完成（commit `1d4a368`，v0.2 milestone 闭环）
- 已读 [overview](../2026-05-20-fakeserver-v0.3-overview.md) §3 Phase 1 详述
- 已读 design.md §10 (10.1–10.5) — 项目注册全章
- 熟悉 `serve_e2e_test.go` 现有 `TestServe_v01_MVPClosure` —— Phase 1 的集成 E2E 沿用其 holder/watcher 模式 + 启动到端口可达后做文件级断言

**Phase 1 完成定义（DoD）**：

1. `internal/registry/store.go` 全部公开函数（Load / Save / Upsert / Get / Remove / ProjectID）有单元测试
2. 损坏的 projects.json 自动恢复（重命名 `.bak-<unix-ts>` + 重建空 + warn）；有专门测试覆盖
3. `internal/registry/lock.go` 单进程内 N goroutine 串行测试通过；上锁重试退避符合 design §10.3（10ms~160ms）
4. `internal/registry/pid.go` 三函数测试通过；`IsAlive(os.Getpid())==true`；`IsAlive(999999) == false`
5. serve 启动后，`~/.config/fakeserver/projects.json` 含一条当前项目记录（含正确 id/configPath/cwd/port/lastRunAt）
6. serve 启动后，`<cwd>/.fakeserver/run.pid` 含 `pid\nport\nstartedAt`；shutdown 后 PID 文件被删除
7. `go build ./...` + `go test ./...` + `go vet ./...` 全绿；`internal/registry` 覆盖率 ≥ 80%
8. 若 Task 1 选 `gofrs/flock`：go.mod / go.sum 增 1 direct dep；overview 新依赖列回填
9. bd v0.3 Phase 1 epic 创建并关闭（流程一致）

---

## 文件结构（Phase 1 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 新建 | `internal/registry/store.go` | Project / Registry struct + Load / Save / Upsert / Get / Remove / ProjectID |
| 新建 | `internal/registry/store_test.go` | store 单元测试 |
| 新建 | `internal/registry/lock.go` | WithLock 跨进程锁封装 |
| 新建 | `internal/registry/lock_test.go` | 单进程内并发 + 重试测试 |
| 新建 | `internal/registry/pid.go` | WritePIDFile / ReadPIDFile / RemovePIDFile / IsAlive |
| 新建 | `internal/registry/pid_test.go` | PID 三函数 + IsAlive 当前/不存在 pid |
| 新建 | `internal/cli/serve_v03_e2e_test.go` | serve 启动 → 文件级断言 → shutdown → 清理断言 |
| 修改 | `internal/cli/serve.go` | runServe 接入 Upsert + WritePIDFile + 退出清理 |
| 修改 | `go.mod` / `go.sum` | （若 Task 1 选 gofrs/flock）增 1 direct dep |

---

## Task 1: 文件锁选型 spike + 决策

**Files**:
- 临时探查：写一段 ~30 行的 main.go 测两个候选方案（可放 `tmp/spike-lock/`，本任务结束后删除）

**约束**：design §10.3 要求"上锁失败重试 5 次（指数退避 10ms~160ms），仍失败仅 warn 不阻塞"。

### Step 1.1: 候选方案对比（不写代码，只看资料）

候选 A — `github.com/gofrs/flock`：
- 跨平台（POSIX flock + Windows LockFileEx 内置）
- API：`fl := flock.New(path); ok, err := fl.TryLock()` 或 `fl.Lock()` 阻塞版
- 引入 1 个 direct dep；该库 indirect 依赖 0 个新包（纯 std）
- 维护活跃；issue 数低

候选 B — 自写 build-tag 拆分：
- `lock_unix.go` 用 `syscall.Flock`；`lock_windows.go` 用 `golang.org/x/sys/windows.LockFileEx`
- 零新 direct dep（但 windows 实现要引 `golang.org/x/sys`，可能 v0.1 已经 indirect 引入了）
- 代码量 ~150 行 + 双平台测试更繁

### Step 1.2: 快速 spike 验证候选 A 是否真够用

新建 `tmp/spike-lock/main.go`（**任务结束后删除整个 tmp/ 目录**）：

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

func main() {
	tmp, _ := os.MkdirTemp("", "fl-")
	defer os.RemoveAll(tmp)
	p := filepath.Join(tmp, "lock")

	fl := flock.New(p)
	t0 := time.Now()
	locked, err := fl.TryLock()
	if err != nil {
		panic(err)
	}
	fmt.Printf("got lock=%v in %v\n", locked, time.Since(t0))

	// 再起一个 flock 实例模拟另一进程
	fl2 := flock.New(p)
	ok, _ := fl2.TryLock()
	fmt.Printf("second TryLock ok=%v (期望 false)\n", ok)

	_ = fl.Unlock()
	ok, _ = fl2.TryLock()
	fmt.Printf("after unlock, second TryLock ok=%v (期望 true)\n", ok)
	_ = fl2.Unlock()
}
```

```
mkdir -p D:/work/aidev/lite-tools/fakeserver/tmp/spike-lock
# 把 main.go 写进去
cd D:/work/aidev/lite-tools/fakeserver
go get github.com/gofrs/flock
go run ./tmp/spike-lock
```

预期：
```
got lock=true in <1ms
second TryLock ok=false (期望 false)
after unlock, second TryLock ok=true (期望 true)
```

### Step 1.3: 决策 + 清理

- 若 spike 通过 → **选 A（gofrs/flock）**，理由：跨平台测试覆盖由库提供，代码量节省 ~100 行；维护方向上 v0.1 已有 `easytpl`/`fsnotify`/`expr-lang/expr` 等 6 个 direct dep，新增 1 个增量很小
- 若 spike 失败 → 切换 B 路线（自写 build-tag）；本 plan Task 3 改为按 B 编码
- 清理 spike 痕迹：

```
rm -rf D:/work/aidev/lite-tools/fakeserver/tmp/spike-lock
# 但 go.mod / go.sum 保留 gofrs/flock 给后续 Task 3 用（如果选 A）
```

### Step 1.4: 在 overview 回写选型决定

修改 `docs/plans/2026-05-20-fakeserver-v0.3-overview.md` §2 表的 Phase 1 行"新增第三方依赖"列：

- 选 A → 写 `github.com/gofrs/flock v0.x.y`
- 选 B → 写 `—`

**Commit**：

```
git -C D:/work/aidev/lite-tools/fakeserver add go.mod go.sum docs/plans/2026-05-20-fakeserver-v0.3-overview.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "deps(v0.3): 引入 gofrs/flock（Phase 1 Task 1 spike 决策）"
# 若选 B 路线则跳过本 commit，直接进 Task 2
```

---

## Task 2: registry/store.go — Project / Registry struct + Load / Save / ProjectID

**Files**:
- 新建：`internal/registry/store.go`
- 新建：`internal/registry/store_test.go`

### Step 2.1: 先写测试（TDD）—— 最小 Load/Save 往返

新建 `internal/registry/store_test.go`：

```go
package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tmpRegFile(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	return filepath.Join(d, "projects.json")
}

func TestLoad_NotExist_ReturnsEmptyRegistry(t *testing.T) {
	reg, err := Load(tmpRegFile(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.Version != 1 {
		t.Errorf("Version=%d, want 1", reg.Version)
	}
	if len(reg.Projects) != 0 {
		t.Errorf("Projects len=%d, want 0", len(reg.Projects))
	}
}

func TestSave_Then_Load_RoundTrip(t *testing.T) {
	p := tmpRegFile(t)
	reg := &Registry{
		Version:      1,
		LastActiveId: "abc123",
		Projects: []Project{{
			ID:         "abc123",
			Name:       "my-app",
			ConfigPath: "/abs/cfg.json5",
			CWD:        "/abs",
			Envs:       []string{"dev", "staging"},
			LastEnv:    "dev",
			LastPort:   5090,
			LastRunAt:  time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
			PIDFile:    "/abs/.fakeserver/run.pid",
		}},
	}
	if err := Save(p, reg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.LastActiveId != "abc123" {
		t.Errorf("LastActiveId=%q", got.LastActiveId)
	}
	if len(got.Projects) != 1 || got.Projects[0].Name != "my-app" {
		t.Errorf("Projects mismatch: %+v", got.Projects)
	}
}
```

跑：

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/registry/... -count=1
# 预期 FAIL（包还没建）
```

### Step 2.2: 写最小 store.go 让 Step 2.1 测试通过

新建 `internal/registry/store.go`：

```go
// Package registry 维护 fakeserver 跨进程的项目注册表与 PID 文件。
// 详见 design §10。
package registry

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Project 是 projects.json 中单条记录，对应 design §10.2 表结构。
type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ConfigPath string    `json:"configPath"`
	CWD        string    `json:"cwd"`
	Envs       []string  `json:"envs,omitempty"`
	LastEnv    string    `json:"lastEnv,omitempty"`
	LastPort   int       `json:"lastPort,omitempty"`
	LastRunAt  time.Time `json:"lastRunAt"`
	PIDFile    string    `json:"pidFile,omitempty"`
}

// Registry 是 projects.json 顶层结构。
type Registry struct {
	Version      int       `json:"version"`
	LastActiveId string    `json:"lastActiveId,omitempty"`
	Projects     []Project `json:"projects"`
}

// Load 读取 path 指向的 projects.json。
//
// 行为约定（design §10.3）：
//   - 文件不存在 → 返回空 Registry{Version:1} + nil err
//   - 解析失败 → 重命名为 path+".bak-<unix-ts>"，新建空文件，warn 日志，返回空 Registry + nil err
//   - IO 错误（非 NotExist） → 返回 nil + err
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyRegistry(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		// 损坏 → 备份 + 重建空
		bak := fmt.Sprintf("%s.bak-%d", path, time.Now().Unix())
		if renameErr := os.Rename(path, bak); renameErr != nil {
			fmt.Fprintf(os.Stderr, "warn: registry corrupt at %s and rename to bak failed: %v\n", path, renameErr)
		} else {
			fmt.Fprintf(os.Stderr, "warn: registry corrupt at %s; renamed to %s; using empty registry\n", path, bak)
		}
		return emptyRegistry(), nil
	}
	if reg.Version == 0 {
		reg.Version = 1
	}
	if reg.Projects == nil {
		reg.Projects = []Project{}
	}
	return &reg, nil
}

func emptyRegistry() *Registry {
	return &Registry{Version: 1, Projects: []Project{}}
}

// Save 原子写：tmp → fsync → rename（design §10.3）。
// 自动创建父目录（0700）。
func Save(path string, reg *Registry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if reg.Projects == nil {
		reg.Projects = []Project{}
	}
	if reg.Version == 0 {
		reg.Version = 1
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open tmp %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("fsync tmp: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp → %s: %w", path, err)
	}
	return nil
}

// ProjectID 由 sha1(configAbsPath) 前 12 hex 位组成（design §10.2）。
// caller 应传绝对路径；非绝对路径会被先 filepath.Abs 化（出错则按原值哈希）。
func ProjectID(configPath string) string {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		abs = configPath
	}
	sum := sha1.Sum([]byte(abs))
	return hex.EncodeToString(sum[:])[:12]
}

// Upsert 把 p 写入 reg：按 id 替换或追加，同时把 lastActiveId 更新为 p.ID。
// 不持久化；caller 在外层 Save。
func Upsert(reg *Registry, p Project) {
	for i := range reg.Projects {
		if reg.Projects[i].ID == p.ID {
			reg.Projects[i] = p
			reg.LastActiveId = p.ID
			return
		}
	}
	reg.Projects = append(reg.Projects, p)
	reg.LastActiveId = p.ID
}

// Get 返回 id 命中的 Project 副本与是否找到。
func Get(reg *Registry, id string) (Project, bool) {
	for _, p := range reg.Projects {
		if p.ID == id {
			return p, true
		}
	}
	return Project{}, false
}

// Remove 按 id 移除一条记录。不存在不报错；若 lastActiveId 命中删除项，自动清空。
func Remove(reg *Registry, id string) {
	out := reg.Projects[:0]
	for _, p := range reg.Projects {
		if p.ID != id {
			out = append(out, p)
		}
	}
	reg.Projects = out
	if reg.LastActiveId == id {
		reg.LastActiveId = ""
	}
}
```

跑：

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/registry/... -count=1
# 预期 PASS（两个测试）
```

### Step 2.3: 补充测试 — 损坏文件 / Upsert / Get / Remove / ProjectID 幂等

把以下追加到 `store_test.go`：

```go
func TestLoad_Corrupt_RenamesToBakAndReturnsEmpty(t *testing.T) {
	p := tmpRegFile(t)
	if err := os.WriteFile(p, []byte("{not valid json"), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reg.Projects) != 0 {
		t.Errorf("expected empty reg after corrupt; got %d", len(reg.Projects))
	}
	// 应存在一个 .bak-<ts> 文件
	dir := filepath.Dir(p)
	entries, _ := os.ReadDir(dir)
	var hasBak bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), filepath.Base(p)+".bak-") {
			hasBak = true
			break
		}
	}
	if !hasBak {
		t.Error("expected projects.json.bak-<ts> after corrupt; not found")
	}
}

func TestProjectID_IsDeterministic_AndShort(t *testing.T) {
	id1 := ProjectID("/abs/a/b/cfg.json5")
	id2 := ProjectID("/abs/a/b/cfg.json5")
	id3 := ProjectID("/abs/a/c/cfg.json5")
	if id1 != id2 {
		t.Errorf("ID not deterministic: %s vs %s", id1, id2)
	}
	if id1 == id3 {
		t.Errorf("different paths collide: %s == %s", id1, id3)
	}
	if len(id1) != 12 {
		t.Errorf("ID length %d, want 12", len(id1))
	}
}

func TestUpsert_AddsNewAndReplacesExisting(t *testing.T) {
	reg := emptyRegistry()
	Upsert(reg, Project{ID: "a", Name: "Alpha"})
	Upsert(reg, Project{ID: "b", Name: "Beta"})
	if len(reg.Projects) != 2 {
		t.Fatalf("len=%d, want 2", len(reg.Projects))
	}
	Upsert(reg, Project{ID: "a", Name: "Alpha-v2"})
	if len(reg.Projects) != 2 {
		t.Fatalf("len=%d, want 2 after upsert-existing", len(reg.Projects))
	}
	got, ok := Get(reg, "a")
	if !ok || got.Name != "Alpha-v2" {
		t.Errorf("after upsert: %+v ok=%v", got, ok)
	}
	if reg.LastActiveId != "a" {
		t.Errorf("LastActiveId=%q, want a", reg.LastActiveId)
	}
}

func TestRemove_ClearsLastActiveWhenMatched(t *testing.T) {
	reg := emptyRegistry()
	Upsert(reg, Project{ID: "a"})
	Upsert(reg, Project{ID: "b"})
	reg.LastActiveId = "a"
	Remove(reg, "a")
	if _, ok := Get(reg, "a"); ok {
		t.Error("a should be removed")
	}
	if reg.LastActiveId != "" {
		t.Errorf("LastActiveId not cleared: %q", reg.LastActiveId)
	}
	Remove(reg, "nonexistent") // 不应 panic
}
```

跑：

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/registry/... -v -count=1
# 预期全 PASS（5+ 用例）
```

### Step 2.4: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/registry/store.go internal/registry/store_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(registry): store.go — Project/Registry struct + Load/Save/Upsert/Get/Remove/ProjectID + 损坏恢复"
```

---

## Task 3: registry/lock.go — WithLock 跨进程文件锁

**前置**：Task 1 已选 A（gofrs/flock）。若选 B，请把本 Task 的实现替换为 build-tag 拆分。

**Files**:
- 新建：`internal/registry/lock.go`
- 新建：`internal/registry/lock_test.go`

### Step 3.1: 先写并发测试

新建 `internal/registry/lock_test.go`：

```go
package registry

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithLock_SerializesGoroutines(t *testing.T) {
	d := t.TempDir()
	lockPath := filepath.Join(d, "lock")

	var (
		inside int32
		maxIn  int32
	)
	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			err := WithLock(lockPath, func() error {
				cur := atomic.AddInt32(&inside, 1)
				defer atomic.AddInt32(&inside, -1)
				// 跟踪最大同时持锁数
				for {
					m := atomic.LoadInt32(&maxIn)
					if cur <= m || atomic.CompareAndSwapInt32(&maxIn, m, cur) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				return nil
			})
			if err != nil {
				t.Errorf("WithLock: %v", err)
			}
		}()
	}
	wg.Wait()
	if maxIn > 1 {
		t.Errorf("max concurrent inside lock = %d, want 1", maxIn)
	}
}

// TestWithLock_PropagatesFnError 验证 fn 返回的错误透传，且锁仍被正确释放。
func TestWithLock_PropagatesFnError(t *testing.T) {
	d := t.TempDir()
	lockPath := filepath.Join(d, "lock")

	want := errSentinel{}
	got := WithLock(lockPath, func() error { return want })
	if got != want {
		t.Errorf("WithLock err=%v, want %v", got, want)
	}
	// 再上一次应成功（锁已释放）
	if err := WithLock(lockPath, func() error { return nil }); err != nil {
		t.Errorf("second WithLock should succeed: %v", err)
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "sentinel" }
```

跑：

```
go test ./internal/registry/... -count=1 -run "TestWithLock_"
# 预期 FAIL（lock.go 还没建）
```

### Step 3.2: 写 lock.go 让测试过

新建 `internal/registry/lock.go`：

```go
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// WithLock 拿到 path 指向的文件锁后调用 fn，最后释放锁。
// 上锁重试 5 次，指数退避 10ms~160ms（design §10.3）。
// 即便最终拿不到锁，仍然调用 fn（warn 日志），保证 registry 写入失败不阻塞 serve。
func WithLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir lock dir: %w", err)
	}
	fl := flock.New(path)
	backoff := 10 * time.Millisecond
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		ok, err := fl.TryLock()
		if err != nil {
			return fmt.Errorf("flock TryLock attempt %d: %w", attempt+1, err)
		}
		if ok {
			defer func() { _ = fl.Unlock() }()
			return fn()
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	// 5 次仍未拿到 → warn 但不阻塞
	fmt.Fprintf(os.Stderr, "warn: registry lock contention at %s; proceeding without lock\n", path)
	return fn()
}
```

跑：

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/registry/... -v -count=1 -run "TestWithLock_"
# 预期 PASS
```

### Step 3.3: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/registry/lock.go internal/registry/lock_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(registry): lock.go — WithLock 跨进程文件锁 + 重试退避"
```

---

## Task 4: registry/pid.go — PID 文件 + 探活

**Files**:
- 新建：`internal/registry/pid.go`
- 新建：`internal/registry/pid_test.go`

### Step 4.1: 测试先行

新建 `internal/registry/pid_test.go`：

```go
package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWritePIDFile_ReadPIDFile_RoundTrip(t *testing.T) {
	d := t.TempDir()
	pidPath := filepath.Join(d, "nested", "run.pid")

	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	if err := WritePIDFile(pidPath, 1234, 5090, now); err != nil {
		t.Fatalf("Write: %v", err)
	}
	pid, port, started, err := ReadPIDFile(pidPath)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if pid != 1234 || port != 5090 {
		t.Errorf("pid=%d port=%d", pid, port)
	}
	if !started.Equal(now) {
		t.Errorf("startedAt=%v want %v", started, now)
	}
}

func TestRemovePIDFile_NotExist_NoError(t *testing.T) {
	d := t.TempDir()
	pidPath := filepath.Join(d, "absent.pid")
	if err := RemovePIDFile(pidPath); err != nil {
		t.Errorf("Remove non-existent: %v", err)
	}
}

func TestIsAlive_CurrentProcess_True(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Error("IsAlive(os.Getpid()) should be true")
	}
}

func TestIsAlive_NonExistentPID_False(t *testing.T) {
	// 用 999999 / 1e7 极大 pid。Windows 上不在 = false；Linux 上同理。
	if IsAlive(9999999) {
		t.Error("IsAlive(9999999) should be false")
	}
}
```

跑：

```
go test ./internal/registry/... -count=1 -run "PID|IsAlive"
# 预期 FAIL
```

### Step 4.2: 写 pid.go

新建 `internal/registry/pid.go`：

```go
package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WritePIDFile 写 pid + port + startedAt 到 path。
// 文件格式：每行一字段：
//
//	1234
//	5090
//	2026-05-20T10:00:00Z
//
// 父目录不存在自动创建（0700）。
func WritePIDFile(path string, pid int, port int, startedAt time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir pid dir: %w", err)
	}
	content := fmt.Sprintf("%d\n%d\n%s\n", pid, port, startedAt.UTC().Format(time.RFC3339))
	return os.WriteFile(path, []byte(content), 0600)
}

// ReadPIDFile 解析 path 指向的 PID 文件。空行容忍；多余行忽略。
func ReadPIDFile(path string) (pid int, port int, startedAt time.Time, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("read %s: %w", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 3 {
		return 0, 0, time.Time{}, fmt.Errorf("pid file %s: expected ≥3 lines, got %d", path, len(lines))
	}
	pid, err = strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("pid line: %w", err)
	}
	port, err = strconv.Atoi(strings.TrimSpace(lines[1]))
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("port line: %w", err)
	}
	startedAt, err = time.Parse(time.RFC3339, strings.TrimSpace(lines[2]))
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("startedAt line: %w", err)
	}
	return pid, port, startedAt, nil
}

// RemovePIDFile 删除 path 指向的 PID 文件；不存在不报错。
func RemovePIDFile(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove pid: %w", err)
	}
	return nil
}
```

`IsAlive` 因 Windows 与 POSIX 行为差异需要平台拆分。新建 `internal/registry/pid_alive_unix.go`：

```go
//go:build !windows

package registry

import (
	"errors"
	"os"
	"syscall"
)

// IsAlive 返回 pid 进程是否存活。
// POSIX：FindProcess 总是成功；用 signal(0) 探活——errno ESRCH/EPERM/0 各有含义。
func IsAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM = 进程存在但我们没权限发信号 → 仍算 alive
	return errors.Is(err, syscall.EPERM)
}
```

新建 `internal/registry/pid_alive_windows.go`：

```go
//go:build windows

package registry

import "os"

// IsAlive 在 Windows 上：os.FindProcess 失败即 dead；成功后 Release 然后返回 true。
// 注意：Windows 的 FindProcess 对死 pid 返回 err。
func IsAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = proc.Release()
	return true
}
```

跑：

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/registry/... -v -count=1 -run "PID|IsAlive"
# 预期 PASS
```

### Step 4.3: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/registry/pid.go internal/registry/pid_alive_unix.go internal/registry/pid_alive_windows.go internal/registry/pid_test.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(registry): pid.go — WritePIDFile/ReadPIDFile/RemovePIDFile + IsAlive 跨平台"
```

---

## Task 5: serve.go 接入 — 启动期 Upsert + WritePIDFile + 退出清理

**Files**:
- 修改：`internal/cli/serve.go`

### Step 5.1: 决定接入点位置

`runServe` 当前结构（v0.2 后状态，见 `serve.go:213-301`）：

1. 解析 opts → loadServeConfig
2. 装配 holder + watcher
3. 起 srv + signal handler
4. select 等 signal / err
5. graceful shutdown

接入点：

- 写入 registry + 写 PID：在 **步骤 3 srv goroutine 启动之前**（"已经准备好 listen 但还没进入 select"）—— 此时 `addr/port` 已经确定，但服务还没起来。即便接入失败也能继续 listen。**实际启用的端口** 用 `opts.Port`；不去精解 `addr` 抢 :0 这种 case（v0.2 没引入 ephemeral port 形态）。
- 清理 PID：在 **步骤 5 graceful shutdown 之后**，无论成功失败。用 `defer` 注册在 runServe 函数尾部前——但 defer 在 `return` 前才执行，所以放在 select 之后的 cleanup 区即可。

### Step 5.2: 编辑 serve.go

修改 `internal/cli/serve.go` — 在 `runServe` 中插入以下逻辑。

定位"`addr := fmt.Sprintf(...)`"行之后（line ~228），插入：

```go
// v0.3：注册当前项目 + 写 PID 文件（失败不阻塞 serve）。
// 仅当有主配置文件时注册（echo-only 模式跳过）；end-to-end 路径见 design §10。
var pidPath string
if cfg != nil && len(cfg.SourcePaths) > 0 {
	mainCfg := cfg.SourcePaths[0]
	projID := registry.ProjectID(mainCfg)
	cwd, _ := os.Getwd()
	pidPath = filepath.Join(cwd, ".fakeserver", "run.pid")
	proj := registry.Project{
		ID:         projID,
		Name:       filepath.Base(filepath.Dir(mainCfg)),
		ConfigPath: mainCfg,
		CWD:        cwd,
		LastEnv:    opts.EnvName,
		LastPort:   opts.Port,
		LastRunAt:  time.Now().UTC(),
		PIDFile:    pidPath,
	}
	regPath := userRegistryPath()
	if rerr := registry.WithLock(regPath+".lock", func() error {
		reg, lerr := registry.Load(regPath)
		if lerr != nil {
			return lerr
		}
		registry.Upsert(reg, proj)
		return registry.Save(regPath, reg)
	}); rerr != nil {
		fmt.Fprintf(os.Stderr, "warn: registry write failed: %v\n", rerr)
	}
	if werr := registry.WritePIDFile(pidPath, os.Getpid(), opts.Port, proj.LastRunAt); werr != nil {
		fmt.Fprintf(os.Stderr, "warn: write pid file %s: %v\n", pidPath, werr)
	}
}
```

在 `runServe` 函数末尾（`return nil` 之前），增加 PID 清理：

```go
// v0.3：清理 PID 文件（不存在不报错）。
if pidPath != "" {
	if err := registry.RemovePIDFile(pidPath); err != nil {
		fmt.Fprintf(os.Stderr, "warn: remove pid file %s: %v\n", pidPath, err)
	}
}
```

在文件末尾添加 `userRegistryPath`（紧贴 `version()` 函数后）：

```go
// userRegistryPath 返回 ~/.config/fakeserver/projects.json 的绝对路径。
// design §10.1：跨平台统一用 ~/.config（Windows 不走 %APPDATA%）。
// HomeDir 解析失败时回退到当前目录下的 ".fakeserver/projects.json"，仅 warn。
func userRegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: UserHomeDir: %v; using cwd-local registry\n", err)
		return filepath.Join(".fakeserver", "projects.json")
	}
	return filepath.Join(home, ".config", "fakeserver", "projects.json")
}
```

在 `serve.go` 顶部 import 块添加：

```go
"github.com/inhere/fakeserver/internal/registry"
```

（若 `path/filepath` / `time` 未导入则一并补；当前 serve.go 应该都已有）

跑：

```
cd D:/work/aidev/lite-tools/fakeserver
go build ./... 2>&1 | tail -5
go test ./... -count=1 2>&1 | tail -15
# 预期 build 通过，全包测试绿（既有测试不应回归）
```

### Step 5.3: 手动验证（可选 spot check）

```
cd D:/tmp && mkdir -p fs-v03-check && cd fs-v03-check
cp D:/work/aidev/lite-tools/fakeserver/cmd/fakeserver/testdata/* . 2>&1 || echo "no testdata; 用手写最小 cfg"
cat > fakeserver.json5 <<'EOF'
{ routes: [{ method: "GET", path: "/p", body: "ok" }] }
EOF
D:/work/aidev/lite-tools/fakeserver/fakeserver.exe serve --quiet &
sleep 1
cat ~/.config/fakeserver/projects.json | head -20
cat .fakeserver/run.pid
kill %1
sleep 1
ls .fakeserver/ 2>&1   # 应该为空目录
```

> 这是可选的 manual sanity check；自动化覆盖留 Task 6。

### Step 5.4: Commit

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve.go
git -C D:/work/aidev/lite-tools/fakeserver commit -m "feat(cli): serve 启动期 Upsert projects.json + 写 PID + 退出清理（v0.3 Phase 1）"
```

---

## Task 6: serve_v03_e2e_test.go + DoD 核对 + 回写 design + bd close

**Files**:
- 新建：`internal/cli/serve_v03_e2e_test.go`
- 修改：`docs/plans/2026-05-20-fakeserver-v0.3-overview.md`
- 修改：`docs/fakeserver-design.md`

### Step 6.1: 集成 E2E

新建 `internal/cli/serve_v03_e2e_test.go`：

```go
package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/registry"
	"github.com/inhere/fakeserver/internal/tpl"
)

// TestRegistry_StartupWritesAndShutdownCleans 验证 Phase 1 DoD #5/#6：
// 启动 serve 后 ~/.config/fakeserver/projects.json 含当前项目 + PID 文件存在；
// shutdown 后 PID 文件被清理。
//
// 本测试**不真起 runServe**（runServe 会绑端口、阻塞 select），而是直接
// 在 t.TempDir() 模拟 serve 启动期与退出期的 registry 调用路径，验证
// store/lock/pid 三个原语在串联使用时表现正确。真实 runServe 全链路
// 留 Phase 2 综合 E2E。
func TestRegistry_StartupWritesAndShutdownCleans(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	if err := os.WriteFile(cfgPath, []byte(`{ routes: [{ method: "GET", path: "/p", body: "ok" }] }`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("%v", errs)
	}
	_ = tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)

	// 模拟 serve 启动期的 registry 接入路径
	regPath := filepath.Join(tmp, "projects.json")
	pidPath := filepath.Join(tmp, ".fakeserver", "run.pid")
	projID := registry.ProjectID(cfgPath)
	now := time.Now().UTC()
	proj := registry.Project{
		ID:         projID,
		Name:       "test-app",
		ConfigPath: cfgPath,
		CWD:        tmp,
		LastPort:   5090,
		LastRunAt:  now,
		PIDFile:    pidPath,
	}
	if err := registry.WithLock(regPath+".lock", func() error {
		reg, _ := registry.Load(regPath)
		registry.Upsert(reg, proj)
		return registry.Save(regPath, reg)
	}); err != nil {
		t.Fatalf("registry write: %v", err)
	}
	if err := registry.WritePIDFile(pidPath, os.Getpid(), 5090, now); err != nil {
		t.Fatalf("write pid: %v", err)
	}

	// 断言：registry 含 1 条记录、id 命中
	reg, err := registry.Load(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get(reg, projID); !ok {
		t.Errorf("registry should contain project %s; got %+v", projID, reg.Projects)
	}
	if reg.LastActiveId != projID {
		t.Errorf("LastActiveId=%q, want %q", reg.LastActiveId, projID)
	}

	// 断言：PID 文件可读 + 内容正确
	pid, port, _, err := registry.ReadPIDFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	if pid != os.Getpid() || port != 5090 {
		t.Errorf("pid=%d port=%d", pid, port)
	}

	// 模拟 shutdown 路径
	if err := registry.RemovePIDFile(pidPath); err != nil {
		t.Fatalf("RemovePIDFile: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("PID file should be removed; stat err=%v", err)
	}
}
```

跑：

```
cd D:/work/aidev/lite-tools/fakeserver
go test ./internal/cli/... -v -run "TestRegistry_" -count=1
# 预期 PASS
```

### Step 6.2: 完整 DoD 核对

```
cd D:/work/aidev/lite-tools/fakeserver
go build ./...
go test ./... -count=1
go test -cover ./internal/registry ./internal/config ./internal/tpl ./internal/cli
go vet ./...
```

记录覆盖率。DoD 9 项核对：

| # | 检查 | 用例 |
|---|---|---|
| 1 | store.go 公开函数全测试 | TestLoad_* / TestSave_* / TestUpsert_* / TestRemove_* / TestProjectID_* |
| 2 | 损坏 projects.json bak + 重建 | TestLoad_Corrupt_RenamesToBakAndReturnsEmpty |
| 3 | lock 单进程并发串行 | TestWithLock_SerializesGoroutines |
| 4 | pid 三函数 + IsAlive | TestWritePIDFile_* / TestIsAlive_* |
| 5/6 | serve 启动期 registry/PID 接入 | TestRegistry_StartupWritesAndShutdownCleans |
| 7 | build/test/vet 全绿 + 覆盖率 | 命令输出 |
| 8 | gofrs/flock 入 go.mod | Step 1.4 已 commit |
| 9 | bd Phase 1 epic 关闭 | Step 6.5 |

### Step 6.3: 回写 v0.3 overview

修改 `docs/plans/2026-05-20-fakeserver-v0.3-overview.md`：

**§2 表 Phase 1 行**：`待开始` → `✅ 已完成 (commit <T1 SHA>..<T6 SHA>)`

**§3 Phase 1 详述末尾追加**：

```markdown
**实际落地偏差**：

- **gofrs/flock 选型确认**：spike 通过；引入 1 direct dep；go.mod 同步。
- **IsAlive 跨平台 build-tag 拆分**：POSIX 用 signal(0) + EPERM 容忍，Windows 用 FindProcess 成功即真——两实现在死 pid 行为一致（false）但在 EPERM 边界含义不同。当前实现按 design §10.4 "活进程标记 running" 的最宽容义。
- **集成 E2E 不真起 runServe**：runServe 绑定端口 + 阻塞 select 不适合普通 unit test；本 Phase 直接在 t.TempDir 串联 store/lock/pid 验证三原语协作。runServe 真实启动路径的 E2E 留 Phase 2（含子进程启动）。
- **pidPath 取自 CWD 而非 ConfigPath dir**：若用户在 A 目录启动而配置在 B 目录，PID 文件落 A——按 design §10.4 明确写"`<cwd>/.fakeserver/run.pid`"。

**Phase 1 测试覆盖**：~12 个新增用例；`internal/registry` 覆盖率 ≥ 80%；既有 cli / config / tpl 包不下降。

**Phase 1 commit 流水**：
- Task 1: `<T1 SHA>` (gofrs/flock 选型 + go.mod)
- Task 2: `<T2 SHA>` (store.go)
- Task 3: `<T3 SHA>` (lock.go)
- Task 4: `<T4 SHA>` (pid.go + IsAlive 跨平台)
- Task 5: `<T5 SHA>` (serve.go 接入)
- Task 6: `<T6 SHA>` (集成 E2E + docs 回写)
```

修订记录追加：

```markdown
| 2026-05-XX | v0.3-overview-phase1-applied | inhere | Phase 1 落地：registry 包 + 启动期 Upsert + PID 文件 |
```

### Step 6.4: 回写 design.md（严格 3 条事实）

修改 `docs/fakeserver-design.md`：

修订记录追加 1 行：

```markdown
| 2026-05-XX | v0.4-phase0.3.1-applied | inhere | v0.3 Phase 1：registry 包（store/lock/pid）+ serve 启动期 Upsert + PID 写读 |
```

§13 追加：

```markdown
### 已落地（v0.3 Phase 1 阶段确认）

1. **registry 包零侵入接入 serve**：projects.json 读写、跨进程文件锁（gofrs/flock）、PID 文件 + IsAlive 探活全部在 `internal/registry` 内部封装；`internal/cli/serve.go` 仅在启动末尾与退出尾段调一次，不耦合具体实现细节。
2. **失败策略一致 warn-only**：registry / PID 写入失败均仅 stderr warn，不阻塞 serve 启动——mock 功能优先于注册元数据。
3. **PID 文件路径以 CWD 为锚，registry 文件路径以 HOME 为锚**：前者随当前进程工作目录走（不同 CWD 启动 PID 互不冲突）；后者跨进程统一，含跨进程文件锁与原子写。
```

### Step 6.5: Commit + bd close

```
git -C D:/work/aidev/lite-tools/fakeserver add internal/cli/serve_v03_e2e_test.go docs/plans/2026-05-20-fakeserver-v0.3-overview.md docs/fakeserver-design.md
git -C D:/work/aidev/lite-tools/fakeserver commit -m "docs(v0.3): 回写 Phase 1 落地 + 集成 E2E"
```

```
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd create --title="fakeserver v0.3 Phase 1 — registry 包 + 启动期 Upsert + PID" --description="6 Task：spike → store → lock → pid → serve 接入 → E2E + 回写" --type=feature --priority=2 2>&1 | tail -3
# 取 <id>：
BEADS_DIR=D:/work/aidev/lite-tools/.beads bd close <id> --reason="v0.3 Phase 1 完整落地：registry 包就绪 + serve 启动期 Upsert + PID 文件。"
```

---

## Phase 1 完成 · 下一步

仓库具备：

- ✅ `internal/registry/{store,lock,pid}.go` 完整 + 单测
- ✅ `serve` 启动写 projects.json + PID 文件，退出清理
- ✅ 全包绿 + vet 零警告 + registry ≥ 80% 覆盖率
- ✅ design.md §13 v0.3 Phase 1 阶段确认（严格 3 条）
- ✅ bd v0.3 Phase 1 epic 关闭

**Phase 2 预告**：

- `fakeserver list` 子命令（按 design §10.5 表格）
- `fakeserver use <id>` 子命令（含前缀匹配）
- envs 提取助手 + Project.envs 字段填充
- 跨进程并发 Upsert E2E（起 2 个子进程）
- 死进程 PID 文件清理（list 时自动）
- design.md §13 v0.3 Phase 2 阶段确认 + v0.3 milestone 闭环段

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每步 2–5 分钟、含具体命令/代码 | ✓ |
| TDD：先测后写 | ✓（Task 2/3/4 均测试先行）|
| 频繁提交 | ✓（Task 1-6 各一个 commit）|
| 新增依赖前 spike | ✓（Task 1 选型）|
| 失败策略明确（registry/PID 失败 warn-only）| ✓ |
| 覆盖 design §10.1–§10.4 写入侧 + IsAlive 函数 | ✓ |
| design.md 回写**严格 3 条事实** | ✓（按 v0.2 overview §5 规约）|
| Phase 1 / Phase 2 边界清晰（CLI 子命令留 Phase 2）| ✓ |
| 跨平台问题（Windows ~/.config / IsAlive 差异）已显式 spike 或 build-tag | ✓ |
