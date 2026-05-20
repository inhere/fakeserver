package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TestFsnotifySmokeSingleWrite 锁定 fsnotify v1.x 最小可用 API：
//   - NewWatcher() (*Watcher, error)
//   - (*Watcher).Add(path) error 可对单文件订阅
//   - Events chan 在 os.WriteFile 后产生至少一次 Write 事件
//
// 该 smoke 不依赖我们自己的 Watcher 类型，只确认 fsnotify 库本身的契约。
func TestFsnotifySmokeSingleWrite(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(fp); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// 给 watcher 一点时间初始化（Windows 上 race 概率较高）
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(fp, []byte(`{"a":1}`), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events:
		t.Logf("got event Name=%s Op=%s (platform=%s)", ev.Name, ev.Op, runtime.GOOS)
	case <-time.After(2 * time.Second):
		t.Fatal("no fsnotify event after WriteFile")
	}
}

// TestFsnotifySmokeRenameInPlace 锁定编辑器"写 tmp → rename"原子保存的
// 事件序列。Windows 上 VS Code 默认行为，Linux 上 vim 也常见。
func TestFsnotifySmokeRenameInPlace(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// 注意：监听**目录**而非单文件——单文件订阅在 rename 后失效
	if err := w.Add(tmp); err != nil {
		t.Fatalf("Add: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// 模拟编辑器原子保存：写 tmp 文件 → rename 覆盖目标
	tmpf := filepath.Join(tmp, "cfg.json5.tmp")
	if err := os.WriteFile(tmpf, []byte(`{"b":2}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpf, fp); err != nil {
		t.Fatal(err)
	}

	// 收集 500ms 内的所有事件
	var events []fsnotify.Event
	deadline := time.After(500 * time.Millisecond)
collect:
	for {
		select {
		case ev := <-w.Events:
			events = append(events, ev)
		case <-deadline:
			break collect
		}
	}
	if len(events) == 0 {
		t.Fatal("rename-style atomic save produced 0 events")
	}
	t.Logf("rename atomic save => %d events:", len(events))
	for _, ev := range events {
		t.Logf("  %s %s", ev.Op, ev.Name)
	}
}
