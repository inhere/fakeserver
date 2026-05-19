package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestPrintRouteSummary_FormatsMockAndProxyRows(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/ping"},
			{Method: []string{"GET", "HEAD"}, Path: "/users/{id}", Cases: []config.RouteCase{{}, {}}, Strategy: "random"},
			{Method: []string{"*"}, Path: "/api/*rest", Proxy: &config.ProxyConfig{Target: "http://upstream:8080"}},
		},
	}
	var buf bytes.Buffer
	PrintRouteSummary(cfg, &buf)
	out := buf.String()

	for _, want := range []string{"/ping", "/users/{id}", "/api/*rest", "mock", "proxy", "http://upstream:8080"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary should contain %q; got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "2 cases") && !strings.Contains(out, "cases: 2") {
		t.Errorf("summary should mention case count; got:\n%s", out)
	}
}

func TestPrintRouteSummary_EmptyRoutesPrintsHeaderOnly(t *testing.T) {
	cfg := &config.Config{Routes: nil}
	var buf bytes.Buffer
	PrintRouteSummary(cfg, &buf)
	if strings.TrimSpace(buf.String()) == "" {
		t.Errorf("expected at least a header line; got empty")
	}
}
