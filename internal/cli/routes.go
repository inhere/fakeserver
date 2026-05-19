package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/gookit/gcli/v3"

	"github.com/inhere/fakeserver/internal/config"
)

type routesOptions struct {
	paths []string
	out   io.Writer
}

func newRoutesCmd() *gcli.Command {
	var configFlag string
	return &gcli.Command{
		Name: "routes",
		Desc: "Print the route summary for a config (does not start the server)",
		Config: func(cmd *gcli.Command) {
			cmd.StrOpt2(&configFlag, "config,c", "Comma-separated config paths")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			return runRoutes(routesOptions{
				paths: splitConfigPaths(configFlag),
				out:   os.Stdout,
			})
		},
	}
}

func runRoutes(opts routesOptions) error {
	if len(opts.paths) == 0 {
		return fmt.Errorf("routes: at least one --config path is required")
	}
	cfg, err := config.Load(opts.paths, "", nil)
	if err != nil {
		return fmt.Errorf("routes: %w", err)
	}
	if errs := config.Validate(cfg); len(errs) > 0 {
		return fmt.Errorf("routes: config has %d validation error(s); run `fakeserver check` for details", len(errs))
	}
	PrintRouteSummary(cfg, opts.out)
	return nil
}
