package registry

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestMain 把测试进程双用为 helper 子进程。
// 当环境变量 FAKESERVER_REG_HELPER=1 时，进程直接执行单条 Upsert（参数从其他
// FAKESERVER_REG_HELPER_* 环境变量取），然后退出；否则按正常测试入口走。
func TestMain(m *testing.M) {
	if os.Getenv("FAKESERVER_REG_HELPER") == "1" {
		regPath := os.Getenv("FAKESERVER_REG_HELPER_REG")
		id := os.Getenv("FAKESERVER_REG_HELPER_ID")
		if regPath == "" || id == "" {
			fmt.Fprintln(os.Stderr, "helper: missing REG/ID env")
			os.Exit(2)
		}
		err := WithLock(regPath+".lock", func() error {
			reg, lerr := Load(regPath)
			if lerr != nil {
				return lerr
			}
			Upsert(reg, Project{
				ID:        id,
				Name:      "helper-" + id,
				LastRunAt: time.Now().UTC(),
			})
			return Save(regPath, reg)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "helper:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestWithLock_CrossProcess_NoLostUpdates 起 N 个子进程同时 Upsert 不同 id，
// 验证 projects.json 最终含 N 条记录（无丢失）——跨进程文件锁的核心契约。
// design §10.3：跨进程串行化保证 deserialize → modify → serialize 不丢字段。
func TestWithLock_CrossProcess_NoLostUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cross-process test in -short mode")
	}
	// 找当前测试二进制路径（go test 编译出来的可执行）
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")

	const N = 4
	var wg sync.WaitGroup
	wg.Add(N)
	errCh := make(chan error, N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			// 用唯一 test 名让子进程不真跑任何测试，只走 TestMain helper 分支
			cmd := exec.Command(exe, "-test.run", "TestMain__helper__no_match__"+strconv.Itoa(i))
			cmd.Env = append(os.Environ(),
				"FAKESERVER_REG_HELPER=1",
				"FAKESERVER_REG_HELPER_REG="+regPath,
				"FAKESERVER_REG_HELPER_ID=proj-"+strconv.Itoa(i),
			)
			out, runErr := cmd.CombinedOutput()
			if runErr != nil {
				errCh <- fmt.Errorf("worker %d (%s): %w\noutput:\n%s", i, runtime.GOOS, runErr, string(out))
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Error(e)
	}

	reg, err := Load(regPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reg.Projects) != N {
		t.Errorf("expected %d projects after cross-process upserts; got %d (%+v)",
			N, len(reg.Projects), reg.Projects)
	}
}
