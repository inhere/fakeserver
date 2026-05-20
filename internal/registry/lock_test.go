package registry

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithLock_SerializesGoroutines_LowContention 在低竞争场景验证 WithLock
// 真正串行化（goroutine 数小、临界区短，5 次重试退避总预算 310ms 足以覆盖）。
//
// design §10.3 明确"上锁失败重试 5 次（10ms~160ms 退避），仍失败仅 warn 不阻塞"
// ——所以高竞争场景的串行性是放弃的契约（warn 路径会同时放行多个 caller）。
// 高竞争的"不阻塞"契约由 TestWithLock_HighContention_DoesNotBlock 覆盖。
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

// TestWithLock_HighContention_DoesNotBlock 验证 design §10.3 "仍失败仅 warn 不阻塞"
// ——即使高竞争超出 5 次重试预算，所有 caller 仍能完成（warn 路径放行）。
func TestWithLock_HighContention_DoesNotBlock(t *testing.T) {
	d := t.TempDir()
	lockPath := filepath.Join(d, "lock")

	const N = 30
	var wg sync.WaitGroup
	wg.Add(N)
	start := time.Now()
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			err := WithLock(lockPath, func() error {
				time.Sleep(50 * time.Millisecond) // 故意超出 5 次退避总预算
				return nil
			})
			if err != nil {
				t.Errorf("WithLock should never error in fallback path: %v", err)
			}
		}()
	}
	wg.Wait()
	// 30 个串行执行 50ms 临界区 = 1500ms；fallback 放行下应远短于此
	if time.Since(start) > 1200*time.Millisecond {
		t.Errorf("high-contention WithLock took %v, fallback should not serialize", time.Since(start))
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
