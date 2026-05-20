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
