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

	pid, port, _, err := registry.ReadPIDFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	if pid != os.Getpid() || port != 5090 {
		t.Errorf("pid=%d port=%d", pid, port)
	}

	if err := registry.RemovePIDFile(pidPath); err != nil {
		t.Fatalf("RemovePIDFile: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("PID file should be removed; stat err=%v", err)
	}
}
