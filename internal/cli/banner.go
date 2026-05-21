package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/inhere/fakeserver/internal/config"
)

// printBanner writes the startup banner to w. design §5.1 末尾 sample:
//
//	╭─ fakeserver v0.1.0
//	│  listening on http://0.0.0.0:5090
//	│  config:      /abs/fakeserver.json5 (+0 includes)
//	│  routes:      1 mock, 1 cases, 1 proxy, fallback=echo
//	│  env:         (none)
//	╰─
//
// version is the build-injected version string. addr is the listener
// address. cfg may be nil — in which case the banner mentions echo-only.
func printBanner(w io.Writer, cfg *config.Config, version, addr string) {
	fmt.Fprintln(w, "╭─ fakeserver "+version)
	fmt.Fprintf(w, "│  listening on http://%s\n", addr)
	if cfg == nil {
		fmt.Fprintln(w, "│  config:      (none — echo-only mode)")
		fmt.Fprintln(w, "│  ui:          (echo-only mode)")
		fmt.Fprintln(w, "│  routes:      0")
		fmt.Fprintln(w, "│  env:         (none)")
		fmt.Fprintln(w, "╰─")
		return
	}
	var mockN, casesN, proxyN int
	for _, r := range cfg.Routes {
		switch {
		case r.Proxy != nil:
			proxyN++
		case len(r.Cases) > 0:
			casesN++
		default:
			mockN++
		}
	}
	primary := "(stdin)"
	extra := 0
	if len(cfg.SourcePaths) > 0 {
		primary = cfg.SourcePaths[0]
		extra = len(cfg.SourcePaths) - 1
		if cfg.EnvSource != "" {
			extra--
		}
		if extra < 0 {
			extra = 0
		}
	}
	fmt.Fprintf(w, "│  ui:          %s\n", bannerUIURL(cfg, addr))
	fmt.Fprintf(w, "│  config:      %s (+%d includes)\n", primary, extra)
	fmt.Fprintf(w, "│  routes:      %d mock, %d cases, %d proxy, fallback=%s\n", mockN, casesN, proxyN, cfg.Fallback)
	fmt.Fprintf(w, "│  env:         %s\n", bannerEnv(cfg))
	fmt.Fprintln(w, "╰─")
}

func bannerUIURL(cfg *config.Config, addr string) string {
	if cfg == nil {
		return "(disabled)"
	}
	if cfg.Server.AdminEnabled != nil && !*cfg.Server.AdminEnabled {
		return "(disabled)"
	}
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		return "http://" + addr + "/__fakeserver/ui/"
	}
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%s/__fakeserver/ui/", host, port)
}

func bannerEnv(cfg *config.Config) string {
	if cfg == nil || cfg.EnvSource == "" {
		return "(none)"
	}
	name := cfg.EnvName
	if name == "" {
		name = "(active unknown)"
	}
	return fmt.Sprintf("%s (%s)", name, filepath.Base(cfg.EnvSource))
}
