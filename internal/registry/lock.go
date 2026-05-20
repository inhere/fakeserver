package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// WithLock 拿到 path 指向的文件锁后调用 fn，最后释放锁。
// 上锁重试 5 次，指数退避 10ms~160ms（design §10.3）。
// 即便最终拿不到锁，仍然调用 fn（warn 日志），保证 registry 写入失败不阻塞 serve。
func WithLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir lock dir: %w", err)
	}
	fl := flock.New(path)
	backoff := 10 * time.Millisecond
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		ok, err := fl.TryLock()
		if err != nil {
			return fmt.Errorf("flock TryLock attempt %d: %w", attempt+1, err)
		}
		if ok {
			defer func() { _ = fl.Unlock() }()
			return fn()
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	fmt.Fprintf(os.Stderr, "warn: registry lock contention at %s; proceeding without lock\n", path)
	return fn()
}
