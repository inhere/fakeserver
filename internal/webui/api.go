package webui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/registry"
	"github.com/inhere/fakeserver/internal/scenario"
)

type scenarioSetRequest struct {
	Selected string `json:"selected"`
}

type overrideRequest struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	CaseName  string `json:"caseName"`
	Mode      string `json:"mode"`
	Remaining int    `json:"remaining"`
}

func apiProjectsHandler(regPath string) rux.HandlerFunc {
	return func(c *rux.Context) {
		if regPath == "" {
			c.JSON(http.StatusOK, []registry.Project{})
			return
		}
		reg, err := registry.Load(regPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, reg.Projects)
	}
}

func apiConfigHandler(cfg *config.Config) rux.HandlerFunc {
	return func(c *rux.Context) {
		if cfg == nil {
			c.JSON(http.StatusOK, map[string]any{})
			return
		}
		c.JSON(http.StatusOK, redactConfig(cfg))
	}
}

func apiHistoryHandler(ring *recorder.Ring) rux.HandlerFunc {
	return func(c *rux.Context) {
		if ring == nil {
			c.JSON(http.StatusOK, []recorder.Entry{})
			return
		}
		c.JSON(http.StatusOK, ring.Snapshot())
	}
}

func apiHistoryDetailHandler(ring *recorder.Ring) rux.HandlerFunc {
	return func(c *rux.Context) {
		rawID := c.Param("id")
		id, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid history id"})
			return
		}
		if ring == nil {
			c.JSON(http.StatusNotFound, map[string]string{"error": "history entry not found"})
			return
		}
		entry, ok := ring.Get(id)
		if !ok {
			c.JSON(http.StatusNotFound, map[string]string{"error": "history entry not found"})
			return
		}
		c.JSON(http.StatusOK, entry)
	}
}

func apiScenarioStateHandler(store *scenario.Store) rux.HandlerFunc {
	return func(c *rux.Context) {
		if store == nil {
			c.JSON(http.StatusOK, scenario.State{Overrides: map[string]scenario.Override{}})
			return
		}
		c.JSON(http.StatusOK, store.Snapshot())
	}
}

func apiScenarioSetSelectedHandler(store *scenario.Store) rux.HandlerFunc {
	return func(c *rux.Context) {
		if store == nil {
			c.JSON(http.StatusNotFound, map[string]string{"error": "scenario store disabled"})
			return
		}
		var req scenarioSetRequest
		if err := json.NewDecoder(c.Req.Body).Decode(&req); err != nil {
			c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		store.SetSelected(req.Selected)
		c.JSON(http.StatusOK, store.Snapshot())
	}
}

func apiScenarioSetOverrideHandler(store *scenario.Store) rux.HandlerFunc {
	return func(c *rux.Context) {
		if store == nil {
			c.JSON(http.StatusNotFound, map[string]string{"error": "scenario store disabled"})
			return
		}
		var req overrideRequest
		if err := json.NewDecoder(c.Req.Body).Decode(&req); err != nil {
			c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		mode := strings.TrimSpace(req.Mode)
		if mode == "" {
			mode = "always"
		}
		if !validOverrideMode(mode) {
			c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid override mode"})
			return
		}
		store.SetOverride(scenario.NewRouteKey(req.Method, req.Path), scenario.Override{
			CaseName:  strings.TrimSpace(req.CaseName),
			Mode:      mode,
			Remaining: req.Remaining,
		})
		c.JSON(http.StatusOK, store.Snapshot())
	}
}

func apiScenarioClearOverrideHandler(store *scenario.Store) rux.HandlerFunc {
	return func(c *rux.Context) {
		if store == nil {
			c.JSON(http.StatusNotFound, map[string]string{"error": "scenario store disabled"})
			return
		}
		req := overrideRequest{
			Method: c.Req.URL.Query().Get("method"),
			Path:   c.Req.URL.Query().Get("path"),
		}
		if req.Method == "" && req.Path == "" && c.Req.Body != nil {
			if err := json.NewDecoder(c.Req.Body).Decode(&req); err != nil {
				c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
				return
			}
		}
		store.ClearOverride(scenario.NewRouteKey(req.Method, req.Path))
		c.JSON(http.StatusOK, store.Snapshot())
	}
}

func validOverrideMode(mode string) bool {
	switch mode {
	case "always", "next", "count":
		return true
	default:
		return false
	}
}

// redactConfig 返回 cfg 的可序列化拷贝，env 段中 key 含 token/secret/password
// 子串（不区分大小写）的值被替换为 "***"。返回 map 而非 *config.Config，
// 避免修改原对象 + 避免 json:"-" 字段污染输出。
func redactConfig(cfg *config.Config) map[string]any {
	out := map[string]any{
		"server":  cfg.Server,
		"globals": cfg.Globals,
		"routes":  cfg.Routes,
	}
	if cfg.Env != nil {
		out["env"] = redactMap(cfg.Env)
	}
	return out
}

func redactMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if isSensitiveKey(k) {
			out[k] = "***"
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			out[k] = redactMap(nested)
			continue
		}
		out[k] = v
	}
	return out
}

func isSensitiveKey(k string) bool {
	lk := strings.ToLower(k)
	for _, needle := range []string{"token", "secret", "password"} {
		if strings.Contains(lk, needle) {
			return true
		}
	}
	return false
}
