package scenario

import (
	"net/http"
	"strings"

	"github.com/inhere/fakeserver/internal/config"
)

const HeaderName = "X-Fakeserver-Scenario"

func Resolve(req *http.Request, store *Store, cfg *config.Config, cliScenario string) (name string, source string) {
	if req != nil {
		if v := strings.TrimSpace(req.Header.Get(HeaderName)); v != "" {
			return v, "header"
		}
	}
	if store != nil {
		if v := store.Selected(); v != "" {
			return v, "ui"
		}
	}
	if v := strings.TrimSpace(cliScenario); v != "" {
		return v, "cli"
	}
	if cfg != nil {
		if v := strings.TrimSpace(cfg.Server.Scenario); v != "" {
			return v, "config"
		}
	}
	return "", ""
}
