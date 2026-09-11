package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcher_DebouncedSingleCallback(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls int32
	w, err := NewWatcher([]string{fp}, 150*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// 200ms 内连续 5 次改动 → 单次回调
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(fp, []byte(`{"a":`+string(rune('0'+i))+"}"), 0644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 等防抖窗口结束 + 余量
	time.Sleep(400 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got != 1 {
		t.Errorf("burst of writes should yield 1 callback; got %d", got)
	}
}

func TestWatcher_MultipleBurstsMultipleCallbacks(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	_ = os.WriteFile(fp, []byte("{}"), 0644)

	var calls int32
	w, err := NewWatcher([]string{fp}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Burst 1
	_ = os.WriteFile(fp, []byte(`{"a":1}`), 0644)
	time.Sleep(300 * time.Millisecond)
	// Burst 2 — 离前次足够远
	_ = os.WriteFile(fp, []byte(`{"a":2}`), 0644)
	time.Sleep(300 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got != 2 {
		t.Errorf("two separated bursts should yield 2 callbacks; got %d", got)
	}
}

func TestWatcher_StopPreventsCallback(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	_ = os.WriteFile(fp, []byte("{}"), 0644)

	var calls int32
	w, err := NewWatcher([]string{fp}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
	// 改动应被忽略
	_ = os.WriteFile(fp, []byte(`{"a":1}`), 0644)
	time.Sleep(300 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got != 0 {
		t.Errorf("after Stop, no callback; got %d", got)
	}
}

func TestWatcher_MultiplePathsSameCallback(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.json5")
	b := filepath.Join(tmp, "b.json5")
	_ = os.WriteFile(a, []byte("{}"), 0644)
	_ = os.WriteFile(b, []byte("{}"), 0644)

	var calls int32
	w, err := NewWatcher([]string{a, b}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	_ = os.WriteFile(a, []byte(`{"x":1}`), 0644)
	_ = os.WriteFile(b, []byte(`{"y":1}`), 0644)
	time.Sleep(300 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got != 1 {
		t.Errorf("two paths in one debounce window → 1 callback; got %d", got)
	}
}

func TestWatcher_AtomicRenameStillTriggers(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	_ = os.WriteFile(fp, []byte("{}"), 0644)

	var calls int32
	w, err := NewWatcher([]string{fp}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// 模拟编辑器原子保存
	tmpf := filepath.Join(tmp, "cfg.json5.tmp")
	if err := os.WriteFile(tmpf, []byte(`{"x":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpf, fp); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	got := atomic.LoadInt32(&calls)
	if got < 1 {
		t.Errorf("atomic rename should trigger at least once; got %d", got)
	}
}

// 以下用例覆盖轮询兜底：fsnotify 在 9p/drvfs/NFS/SMB 等挂载上不可靠，监听建立后可能一个事件都不来。
// WSL2 容器 /d 盘实测：上面这组用例在 /tmp 全过、放到 /d 上回调次数全是 0；长时间运行的实例改配置
// 不再重载，新起的实例却能收到——时好时坏，所以不能只靠事件。

func TestWatcher_PollingDetectsChangeWithoutNotify(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls int32
	w, err := NewWatcher([]string{fp}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	}, withoutNotify(), WithPollInterval(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// 换一个长度不同的内容：即使 mtime 精度很粗，size 也能区分出变化
	if err := os.WriteFile(fp, []byte(`{"changed":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("poll-only watcher should detect the write once; got %d", got)
	}
}

func TestWatcher_PollingNoSpuriousCallback(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls int32
	w, err := NewWatcher([]string{fp}, 50*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	}, withoutNotify(), WithPollInterval(30*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	time.Sleep(300 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("unchanged file must not trigger reload; got %d callbacks", got)
	}
}

// fsnotify 与轮询同时开着时，一次保存只能重载一次：事件路径会先刷新轮询基线。
func TestWatcher_NotifyAndPollSingleReload(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "cfg.json5")
	if err := os.WriteFile(fp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls int32
	w, err := NewWatcher([]string{fp}, 100*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	}, WithPollInterval(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	if err := os.WriteFile(fp, []byte(`{"once":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	// 覆盖若干个轮询周期 + 防抖窗口
	time.Sleep(600 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("one save should reload exactly once with notify+poll; got %d", got)
	}
}

// 一直存在的 stat 错误要报出来，而且只报一次，不能每个轮询周期刷屏。
func TestWatcher_ReportsStatErrorOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on ENOTDIR, which Windows reports as not-exist")
	}
	tmp := t.TempDir()
	notDir := filepath.Join(tmp, "plain-file")
	if err := os.WriteFile(notDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// 父路径是普通文件：stat 得到 ENOTDIR，而不是"文件不存在"
	fp := filepath.Join(notDir, "cfg.json5")

	var reports int32
	w, err := NewWatcher([]string{fp}, 50*time.Millisecond, func() {},
		withoutNotify(), WithPollInterval(30*time.Millisecond),
		WithErrorHandler(func(error) { atomic.AddInt32(&reports, 1) }))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	time.Sleep(250 * time.Millisecond)
	if got := atomic.LoadInt32(&reports); got != 1 {
		t.Errorf("persistent stat error should be reported exactly once; got %d", got)
	}
}
