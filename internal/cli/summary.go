package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/inhere/fakeserver/internal/config"
)

// PrintRouteSummary writes a human-readable, one-line-per-route summary of
// the loaded config. Used by both the `routes` subcommand and the serve
// startup banner. Format intentionally mirrors design §5.1's example:
//
//	GET    /ping                            → mock
//	GET    /users/{id}                      → mock (2 cases, random)
//	*      /api/*rest                       → proxy http://upstream:8080
func PrintRouteSummary(cfg *config.Config, w io.Writer) {
	fmt.Fprintf(w, "Routes (%d):\n", len(cfg.Routes))
	for _, r := range cfg.Routes {
		method := strings.Join(r.Method, ",")
		if method == "" {
			method = "?"
		}
		var note string
		if r.Proxy != nil {
			note = "→ proxy " + r.Proxy.Target
		} else if len(r.Cases) > 0 {
			strat := r.Strategy
			if strat == "" {
				strat = "random"
			}
			note = fmt.Sprintf("→ mock (%d cases, %s)", len(r.Cases), strat)
		} else {
			note = "→ mock"
		}
		fmt.Fprintf(w, "  %-6s %-32s %s\n", method, r.Path, note)
	}
}
