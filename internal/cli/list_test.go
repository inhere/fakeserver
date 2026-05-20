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

func TestList_EmptyRegistry_PrintsHint(t *testing.T) {
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	var buf bytes.Buffer
	if err := runList(listOptions{regPath: regPath, out: &buf}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no projects registered yet") {
		t.Errorf("empty registry output should hint about serve; got %q", buf.String())
	}
}

func TestList_RunningProject_ShowsRunningStatus(t *testing.T) {
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	pidPath := filepath.Join(d, "run.pid")
	// 写一个指向当前进程的 PID 文件 → IsAlive=true → STATUS=running
	now := time.Now().UTC()
	if err := registry.WritePIDFile(pidPath, os.Getpid(), 5090, now); err != nil {
		t.Fatal(err)
	}
	reg := &registry.Registry{
		Version:      1,
		LastActiveId: "alive-id",
		Projects: []registry.Project{{
			ID: "alive-id", Name: "alive-app",
			ConfigPath: "/abs/cfg.json5", CWD: d,
			LastEnv: "dev", LastPort: 5090, LastRunAt: now, PIDFile: pidPath,
		}},
	}
	if err := registry.Save(regPath, reg); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runList(listOptions{regPath: regPath, out: &buf}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "LAST RUN") {
		t.Errorf("header missing; got: %q", out)
	}
	if !strings.Contains(out, "running") {
		t.Errorf("alive project should show STATUS=running; got:\n%s", out)
	}
	if !strings.Contains(out, "5090") {
		t.Errorf("port 5090 should be in output; got:\n%s", out)
	}
	if !strings.Contains(out, "alive-app") {
		t.Errorf("name should appear; got:\n%s", out)
	}
}

func TestList_DeadProject_ShowsIdleAndCleansPID(t *testing.T) {
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	pidPath := filepath.Join(d, "run.pid")
	// 写一个指向不存在 pid 的 PID 文件 → IsAlive=false → STATUS=idle
	now := time.Now().UTC()
	if err := registry.WritePIDFile(pidPath, 9999999, 5090, now); err != nil {
		t.Fatal(err)
	}
	reg := &registry.Registry{
		Version: 1,
		Projects: []registry.Project{{
			ID: "dead-id", Name: "dead-app",
			ConfigPath: "/abs/cfg.json5", CWD: d,
			LastEnv: "prod", LastPort: 5090, LastRunAt: now, PIDFile: pidPath,
		}},
	}
	if err := registry.Save(regPath, reg); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runList(listOptions{regPath: regPath, out: &buf}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "idle") {
		t.Errorf("dead process should show idle; got:\n%s", out)
	}
	// 死进程的 PID 文件应被 list 自动清理
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("dead PID file should be removed; stat err=%v", err)
	}
}

func TestList_SortsLastActiveFirst(t *testing.T) {
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	t0 := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC) // older
	t1 := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC) // newer
	reg := &registry.Registry{
		Version:      1,
		LastActiveId: "active-old",
		Projects: []registry.Project{
			{ID: "newer-not-active", Name: "newer", LastRunAt: t1},
			{ID: "active-old", Name: "old-but-active", LastRunAt: t0},
		},
	}
	if err := registry.Save(regPath, reg); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runList(listOptions{regPath: regPath, out: &buf}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	idxActive := strings.Index(out, "old-but-active")
	idxNewer := strings.Index(out, "newer")
	if idxActive < 0 || idxNewer < 0 {
		t.Fatalf("both names should appear; out=\n%s", out)
	}
	if idxActive > idxNewer {
		t.Errorf("LastActiveId project should appear first; got:\n%s", out)
	}
}
