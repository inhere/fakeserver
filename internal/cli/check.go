package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gookit/gcli/v3"

	"github.com/inhere/fakeserver/internal/config"
)

type checkOptions struct {
	paths []string
	out   io.Writer
}

func newCheckCmd() *gcli.Command {
	var configFlag string
	return &gcli.Command{
		Name: "check",
		Desc: "Load and validate a fakeserver config (does not start the server)",
		Config: func(cmd *gcli.Command) {
			cmd.StrOpt2(&configFlag, "config,c", "Comma-separated config paths")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			paths := splitConfigPaths(configFlag)
			return runCheck(checkOptions{paths: paths, out: os.Stdout})
		},
	}
}

func runCheck(opts checkOptions) error {
	if len(opts.paths) == 0 {
		return fmt.Errorf("check: at least one --config path is required")
	}
	cfg, err := config.Load(opts.paths, "", nil)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}
	errs := config.Validate(cfg)
	if len(errs) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("check: %d problem(s):\n", len(errs)))
		for i, e := range errs {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, e.Error()))
		}
		return fmt.Errorf("%s", sb.String())
	}
	fmt.Fprintf(opts.out, "OK: %d routes loaded\n", len(cfg.Routes))
	return nil
}

// splitConfigPaths parses the comma-separated -c value into a path slice.
// Empty input returns nil so the caller can fall back to defaults.
func splitConfigPaths(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
