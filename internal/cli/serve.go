package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/admin"
	"github.com/inhere/fakeserver/internal/echo"
)

type serveOptions struct {
	Port int
	Host string
}

func newServeCmd() *gcli.Command {
	opts := serveOptions{
		Port: 3000,
		Host: "0.0.0.0",
	}
	c := &gcli.Command{
		Name: "serve",
		Desc: "Start the fakeserver HTTP server",
		Config: func(cmd *gcli.Command) {
			cmd.IntOpt2(&opts.Port, "port,p", "Listening port")
			cmd.StrOpt2(&opts.Host, "host", "Listening host")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			return runServe(opts)
		},
	}
	return c
}

// assembleRouter 集中装配 router 的所有 mount 操作。
// 抽成独立函数是为了让 E2E 测试（serve_test.go）复用，
// 而不必启动真实 listener。
func assembleRouter() *rux.Router {
	r := rux.New()
	admin.Mount(r)
	echo.Mount(r)
	return r
}

// runServe 装配 router 并启动 HTTP server。Phase 1 阶段：
//   - 注册 admin 端点 (healthz)
//   - 注册 echo handler (兜底 + 固定路径)
//   - 监听 SIGINT/SIGTERM，触发后等待最多 5s 优雅退出
func runServe(opts serveOptions) error {
	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: assembleRouter(),
	}

	// 监听退出信号
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		fmt.Printf("fakeserver listening on http://%s\n", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// 等待退出信号或致命错误
	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case sig := <-stop:
		fmt.Printf("\nreceived %s, shutting down...\n", sig)
	}

	// 5s 内尝试优雅退出
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	fmt.Println("bye.")
	return nil
}
