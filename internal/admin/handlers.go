// Package admin exposes the fakeserver-internal endpoints under
// /__fakeserver/*. Phase 1 only provides healthz; later phases will add
// /routes, /api/*, /ui/*, /events.
package admin

import (
	"net/http"

	"github.com/gookit/rux/v2"
)

// Mount 注册所有 admin 端点。调用方必须保证 path "/__fakeserver/*"
// 是保留前缀（design §4.6）。
func Mount(r *rux.Router) {
	r.GET("/__fakeserver/healthz", healthzHandler)
}

func healthzHandler(c *rux.Context) {
	c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
