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
	"github.com/inhere/fakeserver/internal/mock"
	"github.com/inhere/fakeserver/internal/proxy"
	"github.com/inhere/fakeserver/internal/tpl"
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

// assembleRouter mounts mock routes first, then proxy routes, then admin
// endpoints, then echo as fallback.
//
// Order matters: rux resolves equal-path conflicts by registration order
// in the radix tree. By mounting mock before proxy, an exact mock route
// like /api/users wins over a wildcard proxy route like /api/*rest — which
// is the typical "intercept one path, forward the rest" pattern. Reverse
// the order and the wildcard would swallow the exact route.
//
// rux's tree priority (static > param > wildcard) still applies inside
// each mount, so /api/users (static) beats /api/{id} (param) regardless
// of registration order.
//
// cfg may be nil — in that case both Mount calls are no-ops and the server
// behaves identically to Phase 1's zero-config mode.
func assembleRouter(cfg *config.Config, renderer tpl.Renderer) *rux.Router {
	r := rux.New()
	_ = mock.Mount(r, cfg, renderer)  // single-response + cases
	_ = proxy.Mount(r, cfg, renderer) // proxy routes
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
// Phase 4: cfg.Routes include single-response mocks (Respond), multi-
// response cases routes (RespondCases via mock.Mount), and proxy routes
// (proxy.Mount). Renderer is constructed from cfg.Globals/OSEnvWhitelist/
// FakerSeed (if present) and shared across all three handlers. config.Warn
// advisories print to stderr before listen.
func runServe(opts serveOptions) error {
	cfg, err := loadServeConfig(opts)
	if err != nil {
		return err
	}

	// Phase 4: emit non-fatal advisories from config.Warn to stderr.
	if cfg != nil {
		for _, w := range config.Warn(cfg) {
			fmt.Fprintln(os.Stderr, "warn:", w)
		}
	}

	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	var (
		globals map[string]any
		osenvWl []string
		seed    int64
	)
	if cfg != nil {
		globals = cfg.Globals
		osenvWl = cfg.Server.OSEnvWhitelist
		seed = cfg.Server.FakerSeed
	}
	renderer := tpl.NewRenderer(globals, osenvWl, seed)

	srv := &http.Server{
		Addr:    addr,
		Handler: assembleRouter(cfg, renderer),
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		fmt.Printf("fakeserver listening on http://%s\n", addr)
		if cfg != nil {
			PrintRouteSummary(cfg, os.Stdout)
			fmt.Println("(Phase 4: mock + cases + proxy routes are registered and served)")
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
