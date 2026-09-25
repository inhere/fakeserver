package cli

import (
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestResolveHistoryPath_CLIOverridesConfig(t *testing.T) {
	cfg := &config.Config{Server: config.ServerOpts{HistoryFile: "from-config.jsonl"}}
	got := resolveHistoryPath(cfg, serveOptions{HistoryFile: "from-flag.jsonl"})
	if got != "from-flag.jsonl" {
		t.Fatalf("history path=%q, want the CLI value", got)
	}
}

func TestResolveHistoryPath_FallsBackToConfigThenEmpty(t *testing.T) {
	cfg := &config.Config{Server: config.ServerOpts{HistoryFile: "from-config.jsonl"}}
	if got := resolveHistoryPath(cfg, serveOptions{}); got != "from-config.jsonl" {
		t.Fatalf("history path=%q, want the config value", got)
	}
	if got := resolveHistoryPath(nil, serveOptions{}); got != "" {
		t.Fatalf("history path=%q, want empty (disabled)", got)
	}
}

func TestHistoryBodyEnabled_FlagOverridesConfig(t *testing.T) {
	cfg := &config.Config{Server: config.ServerOpts{HistoryFile: "h.jsonl"}}
	if historyBodyEnabled(cfg, serveOptions{}) {
		t.Fatal("history body should default to off")
	}
	cfg.Server.HistoryBody = true
	if !historyBodyEnabled(cfg, serveOptions{}) {
		t.Fatal("config historyBody=true should enable bodies")
	}
	if !historyBodyEnabled(nil, serveOptions{HistoryBody: true}) {
		t.Fatal("--history-body should enable bodies without a config")
	}
}

func TestHistoryBodyMaxBytes_DefaultsTo64KiB(t *testing.T) {
	if got := historyBodyMaxBytes(nil); got != 64<<10 {
		t.Fatalf("default history body max=%d, want 64KiB", got)
	}
	cfg := &config.Config{Server: config.ServerOpts{HistoryBodyMaxSize: "1KiB"}}
	if got := historyBodyMaxBytes(cfg); got != 1<<10 {
		t.Fatalf("configured history body max=%d, want 1KiB", got)
	}
	// 非法值回退默认，不阻塞启动
	cfg.Server.HistoryBodyMaxSize = "not-a-size"
	if got := historyBodyMaxBytes(cfg); got != 64<<10 {
		t.Fatalf("invalid history body max=%d, want 64KiB fallback", got)
	}
}
