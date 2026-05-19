package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gookit/gcli/v3"

	"github.com/inhere/fakeserver/internal/config"
)

type initOptions struct {
	cwd     string
	withEnv bool
}

func newInitCmd() *gcli.Command {
	opts := initOptions{}
	return &gcli.Command{
		Name: "init",
		Desc: "Generate a starter fakeserver.json5 in the current directory",
		Config: func(cmd *gcli.Command) {
			cmd.BoolOpt2(&opts.withEnv, "with-env", "Also generate fakeserver.env.json5")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getwd: %w", err)
			}
			opts.cwd = wd
			return runInit(opts)
		},
	}
}

func runInit(opts initOptions) error {
	target := filepath.Join(opts.cwd, "fakeserver.json5")
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("init: %q already exists (refusing to overwrite; delete it first if intentional)", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("init: stat %q: %w", target, err)
	}
	if err := os.WriteFile(target, []byte(initTemplate), 0o644); err != nil {
		return fmt.Errorf("init: write %q: %w", target, err)
	}
	fmt.Printf("created %s\n", target)

	if opts.withEnv {
		envTarget := filepath.Join(opts.cwd, "fakeserver.env.json5")
		if _, err := os.Stat(envTarget); err == nil {
			return fmt.Errorf("init --with-env: %q already exists (refusing to overwrite)", envTarget)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("init: stat %q: %w", envTarget, err)
		}
		if err := os.WriteFile(envTarget, []byte(initEnvTemplate), 0o644); err != nil {
			return fmt.Errorf("init: write %q: %w", envTarget, err)
		}
		fmt.Printf("created %s\n", envTarget)
	}
	return nil
}

// loadConfig is a small testing/CLI helper that loads + applies defaults.
// Validation is the caller's job. Used by init_test.go and the routes/check
// commands.
func loadConfig(path string) (*config.Config, error) {
	return config.Load([]string{path}, "", nil)
}
