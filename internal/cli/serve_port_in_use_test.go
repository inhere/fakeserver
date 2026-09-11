package cli

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inhere/fakeserver/internal/registry"
)

// 端口被占时 serve 必须在做任何有副作用的事之前退出：不能覆盖、再在退出时删掉
// 正在运行那个实例的 run.pid，也不能往注册表里记一次并没有发生的运行。
// 旧实现先写 pid 再 ListenAndServe，第二个实例 bind 失败后正是这样把记录弄丢的。
func TestRunServe_PortInUse_LeavesPIDFileAndRegistryUntouched(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp) // pid 文件路径取自 cwd
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // Windows 上 os.UserHomeDir 读这个

	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	if err := os.WriteFile(cfgPath, []byte(`{ routes: [ { method: "GET", path: "/a", body: "A" } ] }`), 0644); err != nil {
		t.Fatal(err)
	}

	// 占住一个端口，模拟"已有实例在跑"
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	// 那个已有实例写下的 pid 文件
	pidPath := filepath.Join(tmp, ".fakeserver", "run.pid")
	const runningPID = 424242
	started := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	if err := registry.WritePIDFile(pidPath, runningPID, port, started); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- runServe(serveOptions{Host: "127.0.0.1", Port: port, ConfigFlag: cfgPath, Quiet: true, NoWatch: true})
	}()
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runServe should fail fast when the port is already taken")
	}
	if err == nil || !strings.Contains(err.Error(), "server failed") {
		t.Fatalf("want bind failure, got %v", err)
	}

	pid, gotPort, gotStarted, rerr := registry.ReadPIDFile(pidPath)
	if rerr != nil {
		t.Fatalf("pid file of the running instance must survive: %v", rerr)
	}
	if pid != runningPID || gotPort != port || !gotStarted.Equal(started) {
		t.Errorf("pid file was modified: pid=%d port=%d started=%v", pid, gotPort, gotStarted)
	}
	if _, serr := os.Stat(filepath.Join(tmp, ".config", "fakeserver", "projects.json")); !os.IsNotExist(serr) {
		t.Errorf("registry must not be written when bind fails; stat err=%v", serr)
	}
}
