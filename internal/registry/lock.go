package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// WithLock 拿到 path 指向的文件锁后调用 fn，最后释放锁。
// 该锁保护 registry 的 deserialize → modify → serialize 临界区，不能在竞争时绕过。
func WithLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir lock dir: %w", err)
	}
	fl := flock.New(path)
	if err := fl.Lock(); err != nil {
		return fmt.Errorf("flock Lock: %w", err)
	}
	defer func() { _ = fl.Unlock() }()
	return fn()
}
