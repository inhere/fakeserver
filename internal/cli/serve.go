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
	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/echo"
)

type serveOptions struct {
	Port       int
	Host       string
	ConfigFlag string
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
			cmd.StrOpt2(&opts.ConfigFlag, "config,c", "Comma-separated config paths (default: search CWD)")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			return runServe(opts)
		},
	}
	return c
}

// assembleRouter is unchanged from Phase 1: admin + echo only.
// Phase 3 will add mock-route registration here; Phase 4 adds proxy;
// Phase 5 adds middleware.
func assembleRouter() *rux.Router {
	r := rux.New()
	admin.Mount(r)
	echo.Mount(r)
	return r
}

// loadServeConfig resolves the -c flag (or default CWD search) into an
// optional *Config. Returns (nil, nil) when no -c was given and no
// default-search candidate exists — caller treats this as "echo-only".
func loadServeConfig(opts serveOptions) (*config.Config, error) {
	paths := splitConfigPaths(opts.ConfigFlag)
	if len(paths) > 0 {
		cfg, err := config.Load(paths, "", nil)
		if err != nil {
			return nil, err
		}
		if errs := config.Validate(cfg); len(errs) > 0 {
			return nil, fmt.Errorf("config invalid:\n%s", joinErrs(errs))
		}
		return cfg, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	cfg, err := config.LoadDefault(wd)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		if errs := config.Validate(cfg); len(errs) > 0 {
			return nil, fmt.Errorf("config invalid:\n%s", joinErrs(errs))
		}
	}
	return cfg, nil
}

func joinErrs(errs []error) string {
	var sb []byte
	for i, e := range errs {
		sb = append(sb, fmt.Sprintf("  %d. %s\n", i+1, e.Error())...)
	}
	return string(sb)
}

// runServe assembles the router, optionally loads and prints config, then
// runs the HTTP server with signal-driven graceful shutdown.
//
// Phase 2 caveat: cfg.Routes are PRINTED to stdout but NOT registered to
// the router — actual mock response handling lands in Phase 3.
func runServe(opts serveOptions) error {
	cfg, err := loadServeConfig(opts)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: assembleRouter(),
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		fmt.Printf("fakeserver listening on http://%s\n", addr)
		if cfg != nil {
			PrintRouteSummary(cfg, os.Stdout)
			fmt.Println("(Phase 2: mock routes are listed but not yet served; requests still echo)")
		} else {
			fmt.Println("no config; running in echo-only mode")
		}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case sig := <-stop:
		fmt.Printf("\nreceived %s, shutting down...\n", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	fmt.Println("bye.")
	return nil
}
