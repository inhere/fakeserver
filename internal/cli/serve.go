package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/admin"
	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/echo"
	"github.com/inhere/fakeserver/internal/middleware"
	"github.com/inhere/fakeserver/internal/mock"
	"github.com/inhere/fakeserver/internal/proxy"
	"github.com/inhere/fakeserver/internal/registry"
	"github.com/inhere/fakeserver/internal/tpl"
)

type serveOptions struct {
	Port       int
	Host       string
	ConfigFlag string
	Quiet      bool
	NoCORS     bool
	NoWatch    bool
	EnvName     string       // --env / -e <name>; "" → fallback FAKESERVER_ENV → file $active → first segment
	VarOverrides gcli.Strings // --var key=val (multi-flag accumulating; CSV inside single flag allowed)
}

func newServeCmd() *gcli.Command {
	opts := serveOptions{
		Port: 5090,
		Host: "0.0.0.0",
	}
	c := &gcli.Command{
		Name: "serve",
		Desc: "Start the fakeserver HTTP server",
		Config: func(cmd *gcli.Command) {
			cmd.IntOpt2(&opts.Port, "port,p", "Listening port")
			cmd.StrOpt2(&opts.Host, "host", "Listening host")
			cmd.StrOpt2(&opts.ConfigFlag, "config,c", "Comma-separated config paths (default: search CWD)")
			cmd.BoolOpt2(&opts.Quiet, "quiet,q", "Suppress request access log")
			cmd.BoolOpt2(&opts.NoCORS, "no-cors", "Disable CORS middleware")
			cmd.BoolOpt2(&opts.NoWatch, "no-watch", "Disable hot-reload watcher")
			cmd.StrOpt2(&opts.EnvName, "env,e", "Environment segment name (override env file $active and FAKESERVER_ENV)")
			cmd.VarOpt2(&opts.VarOverrides, "var", "Variable override key=val (repeatable; comma-separated allowed)")
		},
		Func: func(cmd *gcli.Command, _ []string) error {
			return runServe(opts)
		},
	}
	return c
}

// assembleHandler builds the full request-handling stack:
//
//	middleware chain → rux router → mock+proxy+admin+echo
//
// The order of middlewares (outermost first) is:
//
//  1. Recoverer  — catches downstream panics, always 500 JSON
//  2. Logger     — access log (no-op when opts.Quiet)
//  3. BodyLimit  — 413 on requests exceeding cfg.Server.MaxBodySize
//  4. CORS       — header injection + OPTIONS post-route 204 (no-op when opts.NoCORS)
//
// cfg may be nil — in that case CORS / BodyLimit degrade to no-ops and
// the chain reduces to recoverer → logger → router.
func assembleHandler(cfg *config.Config, renderer tpl.Renderer, opts serveOptions) http.Handler {
	r := rux.New()
	_ = mock.Mount(r, cfg, renderer)
	_ = proxy.Mount(r, cfg, renderer)
	admin.Mount(r, cfg)
	echo.Mount(r)

	var mws []func(http.Handler) http.Handler
	mws = append(mws, middleware.Recoverer)
	mws = append(mws, middleware.Logger(os.Stderr, opts.Quiet, nil)) // ring=nil; v0.4 Phase 1 Task 4 接入

	var maxBody int64
	if cfg != nil {
		maxBody = parseMaxBodySize(cfg.Server.MaxBodySize)
	}
	if maxBody > 0 {
		mws = append(mws, middleware.BodyLimit(maxBody))
	}

	if !opts.NoCORS {
		corsOpts, enabled := corsOptsFromCfg(cfg)
		if enabled {
			mws = append(mws, middleware.CORS(corsOpts))
		}
	}

	return middleware.Chain(r, mws...)
}

// parseMaxBodySize tolerates empty/invalid values by returning 1MiB default.
// design §5 says "1MiB" default — but we keep parsing forgiving so a
// missing field doesn't kill startup.
func parseMaxBodySize(s string) int64 {
	if s == "" {
		return 1 << 20 // 1MiB default
	}
	n, err := proxy.ParseByteSize(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: maxBodySize %q: %v; defaulting to 1MiB\n", s, err)
		return 1 << 20
	}
	return n
}

// corsOptsFromCfg converts cfg.Server.CORS into middleware.CORSOpts plus
// an "enabled" boolean. Phase 5 forms accepted:
//
//	cors: true   → enabled=true, opts=reflect-mode
//	cors: false  → enabled=false, caller skips CORS middleware
//	cors: { ... }→ enabled=true, opts populated from map
//	cors: nil    → enabled=true, opts=reflect-mode (default for JSON5 "no cors field")
//
// If cfg is nil → enabled=true, opts=reflect-mode (echo-only fallback path).
func corsOptsFromCfg(cfg *config.Config) (middleware.CORSOpts, bool) {
	if cfg == nil {
		return middleware.CORSOpts{}, true
	}
	switch v := cfg.Server.CORS.(type) {
	case bool:
		return middleware.CORSOpts{}, v
	case map[string]any:
		o := middleware.CORSOpts{}
		if origins, ok := v["origins"].([]any); ok {
			for _, x := range origins {
				if s, ok := x.(string); ok {
					o.Origins = append(o.Origins, s)
				}
			}
		}
		if methods, ok := v["methods"].([]any); ok {
			for _, x := range methods {
				if s, ok := x.(string); ok {
					o.Methods = append(o.Methods, s)
				}
			}
		}
		if headers, ok := v["headers"].([]any); ok {
			for _, x := range headers {
				if s, ok := x.(string); ok {
					o.Headers = append(o.Headers, s)
				}
			}
		}
		if ac, ok := v["allowCredentials"].(bool); ok {
			o.AllowCredentials = ac
		}
		return o, true
	default:
		return middleware.CORSOpts{}, true
	}
}

// loadServeConfig resolves the -c flag (or default CWD search) into an
// optional *Config. Returns (nil, nil) when no -c was given and no
// default-search candidate exists — caller treats this as "echo-only".
func loadServeConfig(opts serveOptions) (*config.Config, error) {
	overrides := parseVarOverrides(opts.VarOverrides)
	paths := splitConfigPaths(opts.ConfigFlag)
	if len(paths) > 0 {
		cfg, err := config.Load(paths, opts.EnvName, overrides)
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
	cfg, err := config.LoadDefault(wd, opts.EnvName, overrides)
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

// runServe assembles the middleware chain + holder + watcher, then runs the
// HTTP server with signal-driven graceful shutdown.
//
// Phase 5: cfg.Server.MaxBodySize / CORS opts are consumed here via
// assembleHandler. Hot-reload watcher (unless --no-watch) re-assembles and
// Swaps the handler on every successful config reload.
func runServe(opts serveOptions) error {
	if opts.EnvName == "" {
		opts.EnvName = os.Getenv("FAKESERVER_ENV")
	}
	cfg, err := loadServeConfig(opts)
	if err != nil {
		return err
	}

	if cfg != nil {
		for _, w := range config.Warn(cfg) {
			fmt.Fprintln(os.Stderr, "warn:", w)
		}
	}

	addr := fmt.Sprintf("%s:%d", opts.Host, opts.Port)

	// v0.3：注册当前项目 + 写 PID 文件（失败不阻塞 serve）。
	// 仅当有主配置文件时注册（echo-only 模式跳过）；end-to-end 路径见 design §10。
	var pidPath string
	if cfg != nil && len(cfg.SourcePaths) > 0 {
		mainCfg := cfg.SourcePaths[0]
		projID := registry.ProjectID(mainCfg)
		cwd, _ := os.Getwd()
		pidPath = filepath.Join(cwd, ".fakeserver", "run.pid")
		envFilePath := filepath.Join(filepath.Dir(mainCfg), config.DefaultEnvFileName)
		proj := registry.Project{
			ID:         projID,
			Name:       filepath.Base(filepath.Dir(mainCfg)),
			ConfigPath: mainCfg,
			CWD:        cwd,
			Envs:       config.ExtractEnvNames(envFilePath),
			LastEnv:    opts.EnvName,
			LastPort:   opts.Port,
			LastRunAt:  time.Now().UTC(),
			PIDFile:    pidPath,
		}
		regPath := userRegistryPath()
		if rerr := registry.WithLock(regPath+".lock", func() error {
			reg, lerr := registry.Load(regPath)
			if lerr != nil {
				return lerr
			}
			registry.Upsert(reg, proj)
			return registry.Save(regPath, reg)
		}); rerr != nil {
			fmt.Fprintf(os.Stderr, "warn: registry write failed: %v\n", rerr)
		}
		if werr := registry.WritePIDFile(pidPath, os.Getpid(), opts.Port, proj.LastRunAt); werr != nil {
			fmt.Fprintf(os.Stderr, "warn: write pid file %s: %v\n", pidPath, werr)
		}
		defer func() {
			if err := registry.RemovePIDFile(pidPath); err != nil {
				fmt.Fprintf(os.Stderr, "warn: remove pid file %s: %v\n", pidPath, err)
			}
		}()
	}

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

	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, renderer, opts))

	// Start hot-reload watcher if config came from a file and --no-watch isn't set
	var watcher *config.Watcher
	if !opts.NoWatch && cfg != nil && len(cfg.SourcePaths) > 0 {
		paths := cfg.SourcePaths
		watcher, err = config.NewWatcher(paths, 300*time.Millisecond, func() {
			newCfg, lerr := config.Load(paths, opts.EnvName, parseVarOverrides(opts.VarOverrides))
			if lerr != nil {
				fmt.Fprintln(os.Stderr, "warn: reload load err:", lerr)
				return
			}
			if errs := config.Validate(newCfg); len(errs) > 0 {
				fmt.Fprintln(os.Stderr, "warn: reload validate failed; keeping previous router")
				for _, e := range errs {
					fmt.Fprintln(os.Stderr, "  -", e)
				}
				return
			}
			for _, w := range config.Warn(newCfg) {
				fmt.Fprintln(os.Stderr, "warn (reload):", w)
			}
			newRenderer := tpl.NewRenderer(newCfg.Globals, newCfg.Server.OSEnvWhitelist, newCfg.Server.FakerSeed)
			holder.Swap(assembleHandler(newCfg, newRenderer, opts))
			fmt.Fprintln(os.Stderr, "info: config reloaded; router swapped")
		})
		if err != nil {
			return fmt.Errorf("watcher init: %w", err)
		}
		defer watcher.Stop()
	}

	srv := &http.Server{Addr: addr, Handler: holder}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		printBanner(os.Stdout, cfg, version(), addr)
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

// version returns the build-injected version string. Currently a stub
// returning "v0.1.0"; cmd/fakeserver/main.go can override via ldflags.
func version() string {
	return "v0.1.0"
}

// userRegistryPath 返回 ~/.config/fakeserver/projects.json 的绝对路径。
// design §10.1：跨平台统一用 ~/.config（Windows 不走 %APPDATA%）。
// HomeDir 解析失败时回退到当前目录下的 ".fakeserver/projects.json"，仅 warn。
func userRegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: UserHomeDir: %v; using cwd-local registry\n", err)
		return filepath.Join(".fakeserver", "projects.json")
	}
	return filepath.Join(home, ".config", "fakeserver", "projects.json")
}

// parseVarOverrides converts ["a=1,b=2", "c=3"] → map[string]string.
// Supports repeated flag + CSV inside single flag.
// Empty entries and malformed (no "=") are skipped with stderr warn.
func parseVarOverrides(raw []string) map[string]string {
	out := map[string]string{}
	for _, entry := range raw {
		for _, pair := range strings.Split(entry, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			idx := strings.Index(pair, "=")
			if idx < 0 {
				fmt.Fprintf(os.Stderr, "warn: --var %q: missing '=' separator; skipped\n", pair)
				continue
			}
			out[strings.TrimSpace(pair[:idx])] = strings.TrimSpace(pair[idx+1:])
		}
	}
	return out
}
