package cli

import (
	"fmt"

	"github.com/gookit/gcli/v3"
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

// runServe 由 Task 7 填充。Phase 1 阶段先打印日志后退出，避免空命令报错。
func runServe(opts serveOptions) error {
	fmt.Printf("(TODO Task 7) serve on %s:%d\n", opts.Host, opts.Port)
	return nil
}
