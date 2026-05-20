package cli

import (
	"fmt"
	"io"

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
	}
	fmt.Fprintf(w, "│  config:      %s (+%d includes)\n", primary, extra)
	fmt.Fprintf(w, "│  routes:      %d mock, %d cases, %d proxy, fallback=%s\n", mockN, casesN, proxyN, cfg.Fallback)
	fmt.Fprintln(w, "│  env:         (none)")
	fmt.Fprintln(w, "╰─")
}
