package registry

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithLock_SerializesGoroutines_LowContention 在低竞争场景验证 WithLock
// 真正串行化。
func TestWithLock_SerializesGoroutines_LowContention(t *testing.T) {
	d := t.TempDir()
	lockPath := filepath.Join(d, "lock")

	var (
		inside int32
		maxIn  int32
	)
	const N = 5
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			err := WithLock(lockPath, func() error {
				cur := atomic.AddInt32(&inside, 1)
				defer atomic.AddInt32(&inside, -1)
				for {
					m := atomic.LoadInt32(&maxIn)
					if cur <= m || atomic.CompareAndSwapInt32(&maxIn, m, cur) {
						break
					}
				}
				time.Sleep(1 * time.Millisecond)
				return nil
			})
			if err != nil {
				t.Errorf("WithLock: %v", err)
			}
		}()
	}
	wg.Wait()
	if maxIn > 1 {
		t.Errorf("max concurrent inside lock = %d, want 1 (low contention should serialize)", maxIn)
	}
}

// TestWithLock_SerializesGoroutines_HighContention 验证锁竞争不能绕过锁进入
// deserialize → modify → serialize 临界区，否则跨进程 registry 写入会丢更新。
func TestWithLock_SerializesGoroutines_HighContention(t *testing.T) {
	d := t.TempDir()
	lockPath := filepath.Join(d, "lock")

	const N = 8
	var (
		inside int32
		maxIn  int32
	)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			err := WithLock(lockPath, func() error {
				cur := atomic.AddInt32(&inside, 1)
				defer atomic.AddInt32(&inside, -1)
				for {
					m := atomic.LoadInt32(&maxIn)
					if cur <= m || atomic.CompareAndSwapInt32(&maxIn, m, cur) {
						break
					}
				}
				time.Sleep(80 * time.Millisecond)
				return nil
			})
			if err != nil {
				t.Errorf("WithLock: %v", err)
			}
		}()
	}
	wg.Wait()
	if maxIn > 1 {
		t.Errorf("max concurrent inside high-contention lock = %d, want 1", maxIn)
	}
}

// TestWithLock_PropagatesFnError 验证 fn 返回的错误透传，且锁仍被正确释放。
func TestWithLock_PropagatesFnError(t *testing.T) {
	d := t.TempDir()
	lockPath := filepath.Join(d, "lock")

	want := errSentinel{}
	got := WithLock(lockPath, func() error { return want })
	if got != want {
		t.Errorf("WithLock err=%v, want %v", got, want)
	}
	if err := WithLock(lockPath, func() error { return nil }); err != nil {
		t.Errorf("second WithLock should succeed: %v", err)
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "sentinel" }
