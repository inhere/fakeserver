package webui

import (
	"net/http"
	"strings"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/registry"
)

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
