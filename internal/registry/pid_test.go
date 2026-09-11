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

// TestWritePIDFile_ParentIsFile_ReturnsMkdirErr 验证父路径已为文件时 mkdir 失败。
func TestWritePIDFile_ParentIsFile_ReturnsMkdirErr(t *testing.T) {
	d := t.TempDir()
	blocker := filepath.Join(d, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(blocker, "run.pid")
	err := WritePIDFile(target, 1, 1, time.Now())
	if err == nil {
		t.Error("expected error when parent is regular file")
	}
}

func TestIsAlive_CurrentProcess_True(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Error("IsAlive(os.Getpid()) should be true")
	}
}

func TestIsAlive_NonExistentPID_False(t *testing.T) {
	if IsAlive(9999999) {
		t.Error("IsAlive(9999999) should be false")
	}
}

func TestReadPIDFile_NotExist_ReturnsError(t *testing.T) {
	d := t.TempDir()
	_, _, _, err := ReadPIDFile(filepath.Join(d, "no-such.pid"))
	if err == nil {
		t.Error("expected error for missing pid file")
	}
}

func TestReadPIDFile_MalformedContent_ReturnsError(t *testing.T) {
	cases := map[string]string{
		"too-few-lines":    "1234\n",
		"pid-not-numeric":  "abc\n5090\n2026-05-20T10:00:00Z\n",
		"port-not-numeric": "1234\nzz\n2026-05-20T10:00:00Z\n",
		"started-bad-fmt":  "1234\n5090\nnot-a-date\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			pidPath := filepath.Join(d, "run.pid")
			_ = os.WriteFile(pidPath, []byte(content), 0600)
			if _, _, _, err := ReadPIDFile(pidPath); err == nil {
				t.Errorf("[%s] expected error", name)
			}
		})
	}
}

// 同一 cwd 的多个实例共用 run.pid：先退出的实例不能把仍在运行那个实例的记录删掉。

func TestRemovePIDFileIfOwned_Owner_Removed(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "run.pid")
	if err := WritePIDFile(pidPath, 1234, 5090, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := RemovePIDFileIfOwned(pidPath, 1234); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("own pid file should be removed; stat err=%v", err)
	}
}

func TestRemovePIDFileIfOwned_OtherOwner_Kept(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "run.pid")
	if err := WritePIDFile(pidPath, 5678, 5090, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := RemovePIDFileIfOwned(pidPath, 1234); err != nil {
		t.Fatalf("remove: %v", err)
	}
	pid, _, _, err := ReadPIDFile(pidPath)
	if err != nil {
		t.Fatalf("pid file of another instance must be kept: %v", err)
	}
	if pid != 5678 {
		t.Errorf("pid=%d, want 5678 left untouched", pid)
	}
}

func TestRemovePIDFileIfOwned_NotExist_NoError(t *testing.T) {
	if err := RemovePIDFileIfOwned(filepath.Join(t.TempDir(), "absent.pid"), 1234); err != nil {
		t.Errorf("missing file should not be an error: %v", err)
	}
}

func TestRemovePIDFileIfOwned_Malformed_Kept(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "run.pid")
	if err := os.WriteFile(pidPath, []byte("not a pid file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RemovePIDFileIfOwned(pidPath, 1234); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(pidPath); err != nil {
		t.Errorf("file we cannot attribute must be kept: %v", err)
	}
}
