package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inhere/fakeserver/internal/registry"
)

// TestListUse_FullWorkflow_v03_MilestoneClosure 是 v0.3 milestone 闭环 E2E。
//
// 模拟 serve 启动注册 → list 看到 running → 进程"死亡" → list 看到 idle 且 PID 文件
// 自动清理 → use 切换 lastActiveId → list 输出顺序变化。
//
// 不真起 serve（避免端口绑定 + 进程信号），而是按 v0.3 Phase 1 集成 E2E 的同样
// 风格手动模拟启动期 registry 写入路径，再调 runList / runUse 验证输出。
func TestListUse_FullWorkflow_v03_MilestoneClosure(t *testing.T) {
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	pidPath := filepath.Join(d, "run.pid")

	// ── 场景 1: 模拟 serve 启动 → 注册项目 + 写 PID（PID = 当前进程，IsAlive=true）──
	now := time.Now().UTC()
	proj := registry.Project{
		ID: "alpha-id-12", Name: "alpha-app",
		ConfigPath: "/abs/cfg.json5", CWD: d,
		Envs: []string{"dev", "staging"}, LastEnv: "dev",
		LastPort: 5090, LastRunAt: now, PIDFile: pidPath,
	}
	if err := registry.WithLock(regPath+".lock", func() error {
		reg, _ := registry.Load(regPath)
		registry.Upsert(reg, proj)
		return registry.Save(regPath, reg)
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.WritePIDFile(pidPath, os.Getpid(), 5090, now); err != nil {
		t.Fatal(err)
	}

	// 加一个 idle 的旧项目用于排序测试
	older := registry.Project{
		ID: "beta-id-34", Name: "beta-app",
		ConfigPath: "/abs/beta.json5", CWD: d,
		LastEnv: "prod", LastPort: 6060,
		LastRunAt: now.Add(-24 * time.Hour),
	}
	reg, _ := registry.Load(regPath)
	registry.Upsert(reg, older)
	reg.LastActiveId = "alpha-id-12" // 模拟 serve 之后 alpha 是 active
	_ = registry.Save(regPath, reg)

	// ── 场景 2: list 输出 running 状态 + envs 信息（envs 不在表格里但 projects.json 含）──
	var buf bytes.Buffer
	if err := runList(listOptions{regPath: regPath, out: &buf}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "running") {
		t.Errorf("running status missing from list:\n%s", out)
	}
	if !strings.Contains(out, "alpha-app") || !strings.Contains(out, "beta-app") {
		t.Errorf("both projects should appear:\n%s", out)
	}
	idxAlpha := strings.Index(out, "alpha-app")
	idxBeta := strings.Index(out, "beta-app")
	if idxAlpha > idxBeta {
		t.Errorf("alpha (active) should appear before beta (idle):\n%s", out)
	}

	// 验证 envs 字段已持久化到 projects.json
	loaded, _ := registry.Load(regPath)
	got, _ := registry.Get(loaded, "alpha-id-12")
	if len(got.Envs) != 2 || got.Envs[0] != "dev" {
		t.Errorf("Project.Envs not persisted; got %v", got.Envs)
	}

	// ── 场景 3: 模拟进程"死亡"——把 PID 文件改成不存在的 pid，再 list ──
	if err := registry.WritePIDFile(pidPath, 9999999, 5090, now); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := runList(listOptions{regPath: regPath, out: &buf}); err != nil {
		t.Fatal(err)
	}
	out = buf.String()
	if !strings.Contains(out, "idle") {
		t.Errorf("dead project should show idle:\n%s", out)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("dead PID file should be auto-removed; stat err=%v", err)
	}

	// ── 场景 4: use beta（前缀）→ lastActiveId 切到 beta ──
	buf.Reset()
	if err := runUse(useOptions{regPath: regPath, out: &buf, id: "beta"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "beta-id-34") {
		t.Errorf("use output should show full id; got %q", buf.String())
	}
	loaded, _ = registry.Load(regPath)
	if loaded.LastActiveId != "beta-id-34" {
		t.Errorf("LastActiveId=%q, want beta-id-34", loaded.LastActiveId)
	}

	// ── 场景 5: list 再次输出，beta 现在排在前面 ──
	buf.Reset()
	if err := runList(listOptions{regPath: regPath, out: &buf}); err != nil {
		t.Fatal(err)
	}
	out = buf.String()
	idxBeta = strings.Index(out, "beta-app")
	idxAlpha = strings.Index(out, "alpha-app")
	if idxBeta > idxAlpha {
		t.Errorf("beta (now active) should appear first:\n%s", out)
	}
}
