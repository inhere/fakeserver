// Package webui 提供 fakeserver 的轻量 Web UI 与配套 JSON / SSE 端点。
// design §11。
//
// Phase 1：仅 3 个 JSON 端点（projects/config/history）+ adminEnabled 护栏。
// SSE 留 Phase 2；静态资源留 Phase 3。
package webui

import (
	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
)

// Mount 注册 webui 端点到 router。
// 当 cfg.Server.AdminEnabled == false 时整体跳过（design §11.6）。
// regPath：~/.config/fakeserver/projects.json 的绝对路径，用于 /api/projects。
func Mount(r *rux.Router, cfg *config.Config, regPath string, ring *recorder.Ring) {
	if cfg == nil || cfg.Server.AdminEnabled == nil || !*cfg.Server.AdminEnabled {
		return
	}
	r.GET("/__fakeserver/api/projects", apiProjectsHandler(regPath))
	r.GET("/__fakeserver/api/config", apiConfigHandler(cfg))
	r.GET("/__fakeserver/api/history", apiHistoryHandler(ring))
	r.GET("/__fakeserver/events", sseEventsHandler(ring, defaultHeartbeat))
}
