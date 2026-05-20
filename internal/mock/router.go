package mock

import (
	"fmt"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Mount registers mock routes (single-response and cases) onto r. Routes
// with proxy{} are skipped — proxy.Mount handles them.
//
// Phase 4 dispatch:
//
//	route.Proxy != nil       → skip (proxy.Mount registers separately)
//	len(route.Cases) > 0     → precompile matchers + selector, register cases handler
//	otherwise                → register the Phase 3 single-response handler
//
// Compile errors in any when-expression are returned as the first error
// (Validate normally catches these — this is defense-in-depth so the
// router never silently registers a half-broken route).
func Mount(r *rux.Router, cfg *config.Config, renderer tpl.Renderer) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		route := &cfg.Routes[i] // closure pointer
		if route.Proxy != nil {
			continue
		}

		envMap := cfg.Env // capture per-route so the closure is allocation-light
		var handler rux.HandlerFunc
		if len(route.Cases) > 0 {
			matchers := make([]*Matcher, len(route.Cases))
			for ci, c := range route.Cases {
				m, err := CompileMatcher(c.When)
				if err != nil {
					return fmt.Errorf("routes[%d] (%s %s): %w", i, strings.Join(route.Method, ","), route.Path, err)
				}
				matchers[ci] = m
			}
			selector := NewSelector(route.Strategy)
			handler = func(c *rux.Context) {
				RespondCases(c, route, matchers, selector, renderer, envMap)
			}
		} else {
			handler = func(c *rux.Context) {
				Respond(c, route, renderer, envMap)
			}
		}

		for _, m := range route.Method {
			method := strings.ToUpper(m)
			if method == "*" {
				r.Any(route.Path, handler)
			} else {
				r.Add(route.Path, handler, method)
			}
		}
	}
	return nil
}
