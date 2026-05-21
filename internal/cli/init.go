package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/goutil/errorx"

	"github.com/inhere/fakeserver/internal/config"
)

type initOptions struct {
	cwd     string
	withEnv bool
	full    bool
	force   bool
}

func newInitCmd() *gcli.Command {
	opts := initOptions{}
	return &gcli.Command{
		Name: "init",
		Desc: "Generate a starter fakeserver.json5 in the current directory",
		Config: func(cmd *gcli.Command) {
			cmd.BoolOpt2(&opts.withEnv, "with-env", "Also generate fakeserver.env.json5")
			cmd.BoolOpt2(&opts.full, "full", "Generate a complete frontend-friendly example project")
			cmd.BoolOpt2(&opts.force, "force", "Overwrite existing generated config files")
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
	if opts.full {
		for _, file := range fullInitFiles() {
			if err := writeGeneratedFile(opts.cwd, file, opts.force); err != nil {
				return err
			}
			fmt.Printf("created %s\n", filepath.Join(opts.cwd, filepath.FromSlash(file.Path)))
		}
		return nil
	}

	target := filepath.Join(opts.cwd, "fakeserver.json5")
	if _, err := os.Stat(target); err == nil {
		if !opts.force {
			return errorx.Failf(1, "init: %q already exists (refusing to overwrite; delete it first if intentional)", target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errorx.Failf(1, "init: stat %q: %s", target, err.Error())
	}
	if err := os.WriteFile(target, []byte(initTemplate), 0o644); err != nil {
		return errorx.Failf(1, "init: write %q: %s", target, err.Error())
	}
	fmt.Printf("created %s\n", target)

	if opts.withEnv {
		envTarget := filepath.Join(opts.cwd, "fakeserver.env.json5")
		if _, err := os.Stat(envTarget); err == nil {
			if !opts.force {
				return errorx.Failf(1, "init --with-env: %q already exists (refusing to overwrite)", envTarget)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return errorx.Failf(1, "init: stat %q: %s", envTarget, err.Error())
		}
		if err := os.WriteFile(envTarget, []byte(initEnvTemplate), 0o644); err != nil {
			return errorx.Failf(1, "init: write %q: %s", envTarget, err.Error())
		}
		fmt.Printf("created %s\n", envTarget)
	}
	return nil
}

func writeGeneratedFile(root string, file generatedFile, force bool) error {
	target := filepath.Join(root, filepath.FromSlash(file.Path))
	if _, err := os.Stat(target); err == nil && !force {
		return errorx.Failf(1, "init --full: %q already exists (refusing to overwrite; pass --force if intentional)", target)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorx.Failf(1, "init --full: stat %q: %s", target, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return errorx.Failf(1, "init --full: mkdir %q: %s", filepath.Dir(target), err.Error())
	}
	if err := os.WriteFile(target, []byte(file.Body), 0o644); err != nil {
		return errorx.Failf(1, "init --full: write %q: %s", target, err.Error())
	}
	return nil
}

// loadConfig is a small testing/CLI helper that loads + applies defaults.
// Validation is the caller's job. Used by init_test.go and the routes/check
// commands.
func loadConfig(path string) (*config.Config, error) {
	return config.Load([]string{path}, "", nil)
}
