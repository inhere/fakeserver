package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestBanner_FullCfg(t *testing.T) {
	cfg := &config.Config{
		Fallback: "echo",
		Routes: []config.Route{
			{Method: []string{"GET"}, Path: "/ping", Body: "pong"},
			{Method: []string{"GET"}, Path: "/u/{id}", Strategy: "first-match",
				Cases: []config.RouteCase{{Status: 200}}},
			{Method: []string{"*"}, Path: "/api/*rest",
				Proxy: &config.ProxyConfig{Target: "http://up:80"}},
		},
		SourcePaths: []string{"/abs/fakeserver.json5"},
	}
	var buf bytes.Buffer
	printBanner(&buf, cfg, "v0.1.0", "0.0.0.0:5090")
	s := buf.String()
	if !strings.Contains(s, "fakeserver") || !strings.Contains(s, "v0.1.0") {
		t.Errorf("missing name/version: %q", s)
	}
	if !strings.Contains(s, "0.0.0.0:5090") {
		t.Errorf("missing addr: %q", s)
	}
	if !strings.Contains(s, "1 mock") || !strings.Contains(s, "1 cases") || !strings.Contains(s, "1 proxy") {
		t.Errorf("missing route counts: %q", s)
	}
	if !strings.Contains(s, "fallback=echo") {
		t.Errorf("missing fallback: %q", s)
	}
}

func TestBanner_NoCfg(t *testing.T) {
	var buf bytes.Buffer
	printBanner(&buf, nil, "v0.1.0", "0.0.0.0:5090")
	s := buf.String()
	if !strings.Contains(s, "echo-only") {
		t.Errorf("nil cfg should mention echo-only: %q", s)
	}
}
