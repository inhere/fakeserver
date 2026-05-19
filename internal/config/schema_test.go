package config

import "testing"

func TestApplyDefaults_ZeroValues(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("expected default port 3000, got %d", cfg.Server.Port)
	}
	if cfg.Server.MaxBodySize != "1MiB" {
		t.Errorf("expected default maxBodySize 1MiB, got %q", cfg.Server.MaxBodySize)
	}
	if !cfg.Server.AdminEnabled {
		t.Errorf("expected default adminEnabled=true")
	}
	if cfg.Server.HistorySize != 200 {
		t.Errorf("expected default historySize 200, got %d", cfg.Server.HistorySize)
	}
	if cfg.Fallback != "echo" {
		t.Errorf("expected default fallback=echo, got %q", cfg.Fallback)
	}
}

func TestApplyDefaults_PreservesNonZero(t *testing.T) {
	cfg := &Config{
		Server:   ServerOpts{Port: 9000, MaxBodySize: "5MiB"},
		Fallback: "404",
	}
	applyDefaults(cfg)

	if cfg.Server.Port != 9000 {
		t.Errorf("non-zero port should be preserved")
	}
	if cfg.Server.MaxBodySize != "5MiB" {
		t.Errorf("non-zero maxBodySize should be preserved")
	}
	if cfg.Fallback != "404" {
		t.Errorf("non-zero fallback should be preserved")
	}
	// 但默认 host 仍应被填上
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("zero host should still get default")
	}
}

func TestDefaultPaths_ReturnsThreeCandidates(t *testing.T) {
	paths := DefaultPaths()
	if len(paths) != 3 {
		t.Fatalf("expected 3 default candidates, got %d", len(paths))
	}
	expected := []string{
		"fakeserver.json5",
		"fakeserver.json",
		".fakeserver/config.json5",
	}
	for i, want := range expected {
		if paths[i] != want {
			t.Errorf("paths[%d]: want %q, got %q", i, want, paths[i])
		}
	}
}
