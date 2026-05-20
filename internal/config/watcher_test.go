package config

import (
	"os"
	"path/filepath"
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
