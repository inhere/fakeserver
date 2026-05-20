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
	if IsAlive(9999999) {
		t.Error("IsAlive(9999999) should be false")
	}
}
