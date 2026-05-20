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
