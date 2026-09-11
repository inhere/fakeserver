package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WritePIDFile 写 pid + port + startedAt 到 path。
// 文件格式：每行一字段：
//
//	1234
//	5090
//	2026-05-20T10:00:00Z
//
// 父目录不存在自动创建（0700）。
func WritePIDFile(path string, pid int, port int, startedAt time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir pid dir: %w", err)
	}
	content := fmt.Sprintf("%d\n%d\n%s\n", pid, port, startedAt.UTC().Format(time.RFC3339))
	return os.WriteFile(path, []byte(content), 0600)
}

// ReadPIDFile 解析 path 指向的 PID 文件。空行容忍；多余行忽略。
func ReadPIDFile(path string) (pid int, port int, startedAt time.Time, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("read %s: %w", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 3 {
		return 0, 0, time.Time{}, fmt.Errorf("pid file %s: expected ≥3 lines, got %d", path, len(lines))
	}
	pid, err = strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("pid line: %w", err)
	}
	port, err = strconv.Atoi(strings.TrimSpace(lines[1]))
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("port line: %w", err)
	}
	startedAt, err = time.Parse(time.RFC3339, strings.TrimSpace(lines[2]))
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("startedAt line: %w", err)
	}
	return pid, port, startedAt, nil
}

// RemovePIDFile 删除 path 指向的 PID 文件；不存在不报错。
func RemovePIDFile(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove pid: %w", err)
	}
	return nil
}

// RemovePIDFileIfOwned 仅当 path 里记录的 pid 等于 owner 时才删除。
//
// 同一 cwd 下的多个实例共用一个 run.pid：后启动的实例会覆盖它。
// 退出时若不看归属直接删，先退出的实例会把仍在运行的那个实例的记录一起删掉，
// 之后 `kill $(cat .fakeserver/run.pid)` 就找不到真正在跑的进程了。
//
// 文件不存在、内容无法解析、或记录的是别的 pid 时都保持原样并返回 nil——
// 那不是本进程写的，没有资格删；过期记录由 list 按 IsAlive 识别。
// 读取与删除之间仍有极小的竞态窗口，对本地开发工具可以接受。
func RemovePIDFileIfOwned(path string, owner int) error {
	pid, _, _, err := ReadPIDFile(path)
	if err != nil || pid != owner {
		return nil
	}
	return RemovePIDFile(path)
}
