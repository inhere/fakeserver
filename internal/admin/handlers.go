// Package admin exposes the fakeserver-internal endpoints under
// /__fakeserver/*. Phase 1 only provides healthz; Phase 5 adds /routes.
package admin

import (
	"net/http"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
)

// Mount registers all admin endpoints. design §5.6 lists the surface:
//
//	GET /__fakeserver/healthz — liveness probe
//	GET /__fakeserver/routes  — JSON list of effective routes
//
// cfg is captured by the /routes handler closure so each (re-)Mount sees
// the cfg active at assembly time. With the hot-reload Holder pattern,
// the watcher constructs a fresh router (and admin.Mount call) on every
// successful reload, so /routes always reflects the current cfg.
func Mount(r *rux.Router, cfg *config.Config) {
	r.GET("/__fakeserver/healthz", healthzHandler)
	r.GET("/__fakeserver/routes", routesHandler(cfg))
}

func healthzHandler(c *rux.Context) {
	c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// routesHandler returns a handler that emits the route table as JSON.
// Each entry: {method, path, mode} where mode is one of "mock", "cases",
// "proxy". For routes with multiple methods, one entry per (method, path).
func routesHandler(cfg *config.Config) rux.HandlerFunc {
	return func(c *rux.Context) {
		out := []map[string]any{}
		if cfg != nil {
			for i, route := range cfg.Routes {
				mode := "mock"
				if route.Proxy != nil {
					mode = "proxy"
				} else if len(route.Cases) > 0 {
					mode = "cases"
				}
				for _, m := range route.Method {
					item := map[string]any{
						"index":  i,
						"method": m,
						"path":   route.Path,
						"mode":   mode,
						"source": route.SourceFile,
						"params": routeParams(route.Path),
					}
					if len(route.Cases) > 0 {
						cases := make([]map[string]any, len(route.Cases))
						for ci, cs := range route.Cases {
							cases[ci] = map[string]any{
								"index":  ci,
								"when":   cs.When,
								"status": cs.Status,
							}
						}
						item["cases"] = cases
					}
					if route.Proxy != nil {
						item["proxyTarget"] = route.Proxy.Target
					}
					out = append(out, item)
				}
			}
		}
		c.JSON(http.StatusOK, out)
	}
}

func routeParams(path string) []string {
	var out []string
	for _, part := range strings.Split(path, "/") {
		if len(part) >= 3 && strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			out = append(out, strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}"))
			continue
		}
		if strings.HasPrefix(part, "*") && len(part) > 1 {
			out = append(out, strings.TrimPrefix(part, "*"))
		}
	}
	return out
}
