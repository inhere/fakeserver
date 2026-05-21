package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
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
	target := filepath.Join(tmpDir, "fakeserver.json5")
	cfg, err := loadConfig(target)
	if err != nil {
		t.Fatalf("generated template should be loadable; err=%v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil cfg")
	}
}

func TestRunInitFull_CreatesCompleteExample(t *testing.T) {
	tmpDir := t.TempDir()
	if err := runInit(initOptions{cwd: tmpDir, full: true}); err != nil {
		t.Fatalf("runInit full: %v", err)
	}

	wantFiles := []string{
		"fakeserver.json5",
		"fakeserver.env.json5",
		".fakeserver/routes/health.json5",
		".fakeserver/routes/users.json5",
		".fakeserver/routes/orders.json5",
		".fakeserver/routes/auth.json5",
		".fakeserver/routes/files.json5",
		".fakeserver/routes/proxy.json5",
		".fakeserver/fixtures/report.json",
		".fakeserver/fixtures/readme.txt",
	}
	for _, rel := range wantFiles {
		if _, err := os.Stat(filepath.Join(tmpDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected generated file %s: %v", rel, err)
		}
	}

	cfgPath := filepath.Join(tmpDir, "fakeserver.json5")
	cfg, err := config.Load([]string{cfgPath}, "dev", nil)
	if err != nil {
		t.Fatalf("full config should load: %v", err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		t.Fatalf("full config should validate, got %v", errs)
	}
	if len(cfg.Routes) < 10 {
		t.Fatalf("full config should include many demo routes, got %d", len(cfg.Routes))
	}
}

func TestRunInitFull_RefusesOverwriteByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "fakeserver.json5")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runInit(initOptions{cwd: tmpDir, full: true})
	if err == nil {
		t.Fatal("expected full init to refuse overwrite")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "existing" {
		t.Fatalf("existing config was overwritten without force")
	}
}

func TestRunInitFull_ForceOverwrites(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "fakeserver.json5")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runInit(initOptions{cwd: tmpDir, full: true, force: true}); err != nil {
		t.Fatalf("runInit full force: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "existing" {
		t.Fatal("force should overwrite existing config")
	}
	if !strings.Contains(string(got), "@.fakeserver/routes/health.json5") {
		t.Fatalf("force output does not look like full config: %s", got)
	}
}

func TestRunInit_MinimalForceOverwrites(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "fakeserver.json5")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runInit(initOptions{cwd: tmpDir, force: true}); err != nil {
		t.Fatalf("runInit force: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) == "existing" {
		t.Fatal("minimal force should overwrite existing config")
	}
	if !strings.Contains(string(got), "routes:") {
		t.Fatalf("minimal force output missing routes: %s", got)
	}
}
