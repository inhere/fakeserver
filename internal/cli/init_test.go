package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
