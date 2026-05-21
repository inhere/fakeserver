package cli

import (
	"bytes"
	"path/filepath"
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
	if !strings.Contains(s, "ui:") || !strings.Contains(s, "http://127.0.0.1:5090/__fakeserver/ui/") {
		t.Errorf("missing ui url: %q", s)
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

func TestBanner_AdminDisabledShowsUIDisabled(t *testing.T) {
	off := false
	cfg := &config.Config{
		Fallback: "echo",
		Server:   config.ServerOpts{AdminEnabled: &off},
	}
	var buf bytes.Buffer
	printBanner(&buf, cfg, "v0.1.0", "127.0.0.1:5090")
	if s := buf.String(); !strings.Contains(s, "ui:          (disabled)") {
		t.Fatalf("admin disabled banner should hide ui url: %q", s)
	}
}

func TestBanner_ShowsEnvSourceAndActive(t *testing.T) {
	on := true
	cfg := &config.Config{
		Fallback:    "echo",
		Server:      config.ServerOpts{AdminEnabled: &on},
		EnvSource:   filepath.Join("demo", "fakeserver.env.json5"),
		EnvName:     "dev",
		SourcePaths: []string{"fakeserver.json5", filepath.Join("demo", "fakeserver.env.json5")},
	}
	var buf bytes.Buffer
	printBanner(&buf, cfg, "v0.1.0", "127.0.0.1:5090")
	if s := buf.String(); !strings.Contains(s, "env:         dev (fakeserver.env.json5)") {
		t.Fatalf("banner should include active env and env file: %q", s)
	}
}
