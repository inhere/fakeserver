package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/admin"
	"github.com/inhere/fakeserver/internal/buildinfo"
	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/echo"
	"github.com/inhere/fakeserver/internal/middleware"
	"github.com/inhere/fakeserver/internal/mock"
	"github.com/inhere/fakeserver/internal/proxy"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/registry"
	"github.com/inhere/fakeserver/internal/scenario"
	"github.com/inhere/fakeserver/internal/sizeparse"
	"github.com/inhere/fakeserver/internal/tpl"
	"github.com/inhere/fakeserver/internal/webui"
)

type serveOptions struct {
	Port         int
	Host         string
	ConfigFlag   string
	Quiet        bool
	NoCORS       bool
	NoWatch      bool
	Scenario     string
	EnvName      string       // --env / -e <name>; "" → fallback FAKESERVER_ENV → file $active → first segment
	VarOverrides gcli.Strings // --var key=val (multi-flag accumulating; CSV inside single flag allowed)
	HistoryFile  string       // --history-file <path>; overrides server.historyFile
	HistoryBody  bool         // --history-body; overrides server.historyBody

	// historyWriter is run-scoped plumbing opened once in runServe and shared by
	// every assembleHandler call (initial mount + hot reload). nil = disabled.
	historyWriter *recorder.HistoryWriter
}

const (
	defaultListenHost = "0.0.0.0"
	defaultListenPort = 5090
)

func resolveListenAddr(cfg *config.Config, opts serveOptions) (string, int) {
	host, port := defaultListenHost, defaultListenPort
	if cfg != nil {
		if cfg.Server.Host != "" {
			host = cfg.Server.Host
		}
		if cfg.Server.Port != 0 {
			port = cfg.Server.Port
		}
	}
	if opts.Host != "" {
		host = opts.Host
	}
	if opts.Port != 0 {
		port = opts.Port
	}
	return host, port
}

func isNonLoopbackListenHost(host string) bool {
	if host == "" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host != "localhost"
	}
	return !ip.IsLoopback()
}

func listenAddrReloadWarning(curHost string, curPort int, newCfg *config.Config, opts serveOptions) string {
	newHost, newPort := resolveListenAddr(newCfg, opts)
	if newHost == curHost && newPort == curPort {
		return ""
	}
	return fmt.Sprintf("warn: config now resolves listen address to %s:%d, still listening on %s:%d; restart to apply", newHost, newPort, curHost, curPort)
}

func newServeCmd() *gcli.Command {
	opts := serveOptions{}
	c := &gcli.Command{
		Name: "serve",
		Desc: "Start the fakeserver HTTP server",
		Config: func(cmd *gcli.Command) {
			cmd.IntOpt2(&opts.Port, "port,p", "Listening port (default: config server.port, else 5090)")
			cmd.StrOpt2(&opts.Host, "host", "Listening host (default: config server.host, else 0.0.0.0)")
			cmd.StrOpt2(&opts.ConfigFlag, "config,c", "Comma-separated config paths (default: search CWD)")
			cmd.BoolOpt2(&opts.Quiet, "quiet,q", "Suppress request access log")
			cmd.BoolOpt2(&opts.NoCORS, "no-cors", "Disable CORS middleware")
			cmd.BoolOpt2(&opts.NoWatch, "no-watch", "Disable hot-reload watcher")
			cmd.StrOpt2(&opts.Scenario, "scenario", "Default scenario name for this serve process")
			cmd.StrOpt2(&opts.EnvName, "env,e", "Environment segment name (override env file $active and FAKESERVER_ENV)")
			cmd.VarOpt2(&opts.VarOverrides, "var", "Variable override key=val (repeatable; comma-separated allowed)")
			cmd.StrOpt2(&opts.HistoryFile, "history-file", "Append request history as JSONL to this file (overrides server.historyFile)")
			cmd.BoolOpt2(&opts.HistoryBody, "history-body", "Also record request/response bodies in the history file (overrides server.historyBody)")
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
func assembleHandler(cfg *config.Config, renderer tpl.Renderer, opts serveOptions, ring *recorder.Ring, scenarioStore *scenario.Store) http.Handler {
	r := rux.New()
	_ = mock.MountWithRuntime(r, cfg, renderer, mock.RuntimeOptions{
		ScenarioStore: scenarioStore,
		CLIScenario:   opts.Scenario,
	})
	_ = proxy.Mount(r, cfg, renderer)
	if adminOn(cfg) {
		admin.Mount(r, cfg)
		webui.Mount(r, cfg, userRegistryPath(), ring, scenarioStore)
	} else {
		// design §11.6：adminEnabled=false → 所有 /__fakeserver/* 端点不挂载，
		// UI 也不可达。注册一个明确的 404 catch-all 阻断 echo 兜底。
		r.Any("/__fakeserver/*path", func(c *rux.Context) {
			c.Resp.WriteHeader(http.StatusNotFound)
		})
	}
	if cfg == nil || fallbackName(cfg) == "echo" {
		echo.Mount(r)
	} else {
		r.Any("/*path", fallbackHandler(cfg, renderer))
	}

	var mws []func(http.Handler) http.Handler
	mws = append(mws, middleware.Recoverer)
	mws = append(mws, middleware.Logger(os.Stderr, loggerOptionsFromConfig(cfg, opts), ring))

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

	h := middleware.Chain(r, mws...)
	if cfg == nil || fallbackName(cfg) == "echo" {
		next := h
		h = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Fakeserver-Fallback", "echo")
			next.ServeHTTP(w, req)
		})
	}
	if cfg != nil && adminOn(cfg) && !cfg.Server.AdminAllowRemote {
		h = adminLocalOnly(h)
	}
	return h
}

func adminLocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/__fakeserver/") && req.URL.Path != "/__fakeserver/healthz" {
			host := req.RemoteAddr
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || !ip.IsLoopback() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"admin endpoints are restricted to loopback clients; set server.adminAllowRemote: true to allow remote access"}`))
				return
			}
		}
		next.ServeHTTP(w, req)
	})
}

func fallbackName(cfg *config.Config) string {
	if cfg == nil || cfg.Fallback == nil {
		return "echo"
	}
	switch f := cfg.Fallback.(type) {
	case string:
		return f
	default:
		return "custom"
	}
}

func fallbackHandler(cfg *config.Config, renderer tpl.Renderer) rux.HandlerFunc {
	return func(c *rux.Context) {
		name := fallbackName(cfg)
		if name == "404" {
			c.Resp.Header().Set("X-Fakeserver-Fallback", "404")
			c.JSON(http.StatusNotFound, map[string]any{"error": "route not found", "method": c.Req.Method, "path": c.Req.URL.Path})
			return
		}
		f, _ := cfg.Fallback.(map[string]any)
		route := &config.Route{Status: 404, Headers: map[string]string{}, Body: map[string]any{"error": "route not found", "method": c.Req.Method, "path": c.Req.URL.Path}}
		if v, ok := f["status"].(float64); ok {
			route.Status = int(v)
		}
		if h, ok := f["headers"].(map[string]any); ok {
			for k, v := range h {
				if s, ok := v.(string); ok {
					route.Headers[k] = s
				}
			}
		}
		if v, ok := f["body"]; ok {
			route.Body = v
		}
		if p, ok := f["bodyFile"].(string); ok {
			route.BodyFile = p
			route.Body = nil
		}
		route.Headers["X-Fakeserver-Fallback"] = "custom"
		mock.Respond(c, route, -1, renderer, cfg.Env)
	}
}

func loggerOptionsFromConfig(cfg *config.Config, opts serveOptions) middleware.LoggerOptions {
	out := middleware.LoggerOptions{Quiet: opts.Quiet, HistoryFile: opts.historyWriter, HistoryBody: historyBodyEnabled(cfg, opts), HistoryMaxBytes: historyBodyMaxBytes(cfg)}
	if cfg == nil {
		return out
	}
	out.CaptureEnabled = cfg.Server.Capture.Enabled
	out.CaptureMaxBytes = 64 << 10
	if cfg.Server.Capture.MaxBodySize != "" {
		if n, err := sizeparse.ParseByteSize(cfg.Server.Capture.MaxBodySize); err == nil {
			out.CaptureMaxBytes = n
		} else {
			fmt.Fprintf(os.Stderr, "warn: capture.maxBodySize %q: %v; defaulting to 64KiB\n", cfg.Server.Capture.MaxBodySize, err)
		}
	}
	out.RedactKeys = cfg.Server.Capture.RedactKeys
	return out
}

// resolveHistoryPath returns the JSONL history file to append to: the
// --history-file flag wins over server.historyFile; "" disables persistence.
func resolveHistoryPath(cfg *config.Config, opts serveOptions) string {
	if opts.HistoryFile != "" {
		return opts.HistoryFile
	}
	if cfg == nil {
		return ""
	}
	return cfg.Server.HistoryFile
}

// historyBodyEnabled reports whether request/response bodies are recorded in
// the history file (--history-body wins over server.historyBody).
func historyBodyEnabled(cfg *config.Config, opts serveOptions) bool {
	if opts.HistoryBody {
		return true
	}
	if cfg == nil {
		return false
	}
	return cfg.Server.HistoryBody
}

// historyBodyMaxBytes returns the per-body cap for history recording
// (server.historyBodyMaxSize, default 64KiB). Invalid values fall back to the
// default instead of blocking startup.
func historyBodyMaxBytes(cfg *config.Config) int64 {
	const def = 64 << 10
	if cfg == nil || cfg.Server.HistoryBodyMaxSize == "" {
		return def
	}
	n, err := sizeparse.ParseByteSize(cfg.Server.HistoryBodyMaxSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: historyBodyMaxSize %q: %v; defaulting to 64KiB\n", cfg.Server.HistoryBodyMaxSize, err)
		return def
	}
	return n
}

// openHistoryWriter opens the history file for appending. Failures are
// advisory: the caller warns and serves without persistence.
func openHistoryWriter(path string) *recorder.HistoryWriter {
	if path == "" {
		return nil
	}
	hw, err := recorder.OpenHistoryFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: history file %s: %v (history persistence disabled)\n", path, err)
		return nil
	}
	fmt.Fprintf(os.Stderr, "info: request history appending to %s\n", path)
	return hw
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

	listenHost, listenPort := resolveListenAddr(cfg, opts)
	addr := fmt.Sprintf("%s:%d", listenHost, listenPort)

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

	// v0.4 Phase 1：请求历史环形缓冲。容量取 cfg.Server.HistorySize，
	// cfg=nil 时用默认 200（design §11.4）。
	historySize := 200
	if cfg != nil && cfg.Server.HistorySize > 0 {
		historySize = cfg.Server.HistorySize
	}
	ring := recorder.New(historySize)
	scenarioStore := scenario.NewStore()

	// 请求历史落盘（server.historyFile / --history-file）：启动即按 append 打开，
	// 不覆盖既有内容；打开失败只 warn，服务照常运行。
	opts.historyWriter = openHistoryWriter(resolveHistoryPath(cfg, opts))
	if opts.historyWriter != nil {
		defer opts.historyWriter.Close()
	}

	// v0.4 Phase 1：0.0.0.0 + adminEnabled 组合发 WARNING（design §11.6）。
	if listenHost == "0.0.0.0" && cfg != nil && cfg.Server.AdminEnabled != nil && *cfg.Server.AdminEnabled {
		if isNonLoopbackListenHost(listenHost) && cfg != nil && adminOn(cfg) && cfg.Server.AdminAllowRemote {
			fmt.Fprintln(os.Stderr, "WARNING: listening on a non-loopback address with adminAllowRemote=true exposes admin endpoints to the network")
		}
	}

	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, renderer, opts, ring, scenarioStore))
	currentCfg := cfg

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
			if warning := listenAddrReloadWarning(listenHost, listenPort, newCfg, opts); warning != "" {
				fmt.Fprintln(os.Stderr, warning)
			}
			holder.Swap(assembleHandler(newCfg, newRenderer, opts, ring, scenarioStore))
			emitRouteReload(ring, currentCfg, newCfg)
			currentCfg = newCfg
			fmt.Fprintln(os.Stderr, "info: config reloaded; router swapped")
		}, config.WithErrorHandler(func(werr error) {
			fmt.Fprintln(os.Stderr, "warn: config watcher:", werr)
		}))
		if err != nil {
			return fmt.Errorf("watcher init: %w", err)
		}
		defer watcher.Stop()
	}

	srv := &http.Server{Addr: addr, Handler: holder}

	// 先把端口占住，再做任何有副作用的事：端口被占时直接返回，
	// 不打印 banner、不写注册表、不碰 pid 文件。
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	// v0.3：注册当前项目 + 写 PID 文件（失败不阻塞 serve）。
	// 必须放在端口监听成功之后：先写再监听的话，端口被占的第二个实例会先覆盖
	// 正在运行那个实例的 run.pid，退出时再把它删掉。
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
			LastPort:   listenPort,
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
		if werr := registry.WritePIDFile(pidPath, os.Getpid(), listenPort, proj.LastRunAt); werr != nil {
			fmt.Fprintf(os.Stderr, "warn: write pid file %s: %v\n", pidPath, werr)
		}
		defer func() {
			if err := registry.RemovePIDFileIfOwned(pidPath, os.Getpid()); err != nil {
				fmt.Fprintf(os.Stderr, "warn: remove pid file %s: %v\n", pidPath, err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	serverErr := make(chan error, 1)
	go func() {
		printBanner(os.Stdout, cfg, version(), addr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case sig := <-stop:
		fmt.Printf("\nreceived %s, shutting down...\n", sig)
	}

	ring.CloseSubscribers()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	fmt.Println("bye.")
	return nil
}

func emitRouteReload(ring *recorder.Ring, oldCfg, newCfg *config.Config) {
	if ring == nil {
		return
	}
	ring.EmitReload(diffRoutes(oldCfg, newCfg))
}

func diffRoutes(oldCfg, newCfg *config.Config) recorder.ReloadDiff {
	oldRoutes := routeDetails(oldCfg)
	newRoutes := routeDetails(newCfg)
	diff := recorder.ReloadDiff{}
	for sig, detail := range newRoutes {
		oldDetail, ok := oldRoutes[sig]
		if !ok {
			diff.Added = append(diff.Added, sig)
			continue
		}
		if oldDetail != detail {
			diff.Changed = append(diff.Changed, sig)
		}
	}
	for sig := range oldRoutes {
		if _, ok := newRoutes[sig]; !ok {
			diff.Removed = append(diff.Removed, sig)
		}
	}
	return diff
}

func routeDetails(cfg *config.Config) map[string]string {
	out := map[string]string{}
	if cfg == nil {
		return out
	}
	for _, route := range cfg.Routes {
		for _, method := range route.Method {
			sig := strings.ToUpper(method) + " " + route.Path
			out[sig] = routeDetail(route)
		}
	}
	return out
}

func routeDetail(route config.Route) string {
	mode := "mock"
	proxyTarget := ""
	if route.Proxy != nil {
		mode = "proxy"
		proxyTarget = route.Proxy.Target
	} else if len(route.Cases) > 0 {
		mode = "cases"
	}
	return fmt.Sprintf("%s|status=%d|cases=%d|strategy=%s|proxy=%s|body=%t|bodyFile=%s",
		mode, route.Status, len(route.Cases), route.Strategy, proxyTarget, route.Body != nil, route.BodyFile)
}

func version() string {
	v := buildinfo.Version
	if v == "" {
		v = "dev"
	}
	if buildinfo.GitHash != "" && buildinfo.GitHash != "unknown" {
		v += " (" + buildinfo.GitHash + ")"
	}
	return v
}

// adminOn 返回 cfg 是否启用 admin/webui 端点（design §11.6）。
// cfg=nil（echo-only 模式）→ true，保留 v0.1 起 /__fakeserver/healthz 可达的契约。
// cfg.AdminEnabled=nil 不会在正常路径出现（applyDefaults 已 set）；保险返回 true。
// 仅当 cfg.Server.AdminEnabled 显式为 *false 时整体禁用 admin/webui。
func adminOn(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.Server.AdminEnabled == nil {
		return true
	}
	return *cfg.Server.AdminEnabled
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
