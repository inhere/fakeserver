package mock

import (
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Mount registers all single-response mock routes from cfg onto r. Routes
// with cases[] or proxy{} are skipped in Phase 3 — Phase 4 handles them.
//
// Returns an error only for misconfigured method names; routes with valid
// method sets are guaranteed to register.
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		route := &cfg.Routes[i] // pointer so handler closure sees the same instance
		if route.Proxy != nil || len(route.Cases) > 0 {
			continue // Phase 4
		}
		handler := makeHandler(route, renderer)
		for _, m := range route.Method {
			method := strings.ToUpper(m)
			if method == "*" {
				r.Any(route.Path, handler)
			} else {
				// rux v2: Add(path, handler, methods ...string)
				r.Add(route.Path, handler, method)
			}
		}
	}
	return nil
}

func makeHandler(route *config.Route, renderer tpl.Renderer) rux.HandlerFunc {
	return func(c *rux.Context) {
		Respond(c, route, renderer)
	}
}
