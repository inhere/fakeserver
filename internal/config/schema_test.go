package config

import "testing"

func TestApplyDefaults_ZeroValues(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 5090 {
		t.Errorf("expected default port 5090, got %d", cfg.Server.Port)
	}
	if cfg.Server.MaxBodySize != "1MiB" {
		t.Errorf("expected default maxBodySize 1MiB, got %q", cfg.Server.MaxBodySize)
	}
	if cfg.Server.AdminEnabled == nil || !*cfg.Server.AdminEnabled {
		t.Errorf("expected default adminEnabled=true (pointer); got %v", cfg.Server.AdminEnabled)
	}
	if cfg.Server.HistorySize != 200 {
		t.Errorf("expected default historySize 200, got %d", cfg.Server.HistorySize)
	}
	if cfg.Server.Capture.Enabled {
		t.Errorf("expected default capture.enabled=false")
	}
	if cfg.Server.Capture.MaxBodySize != "64KiB" {
		t.Errorf("expected default capture.maxBodySize 64KiB, got %q", cfg.Server.Capture.MaxBodySize)
	}
	for _, key := range []string{"authorization", "cookie", "password", "token", "secret"} {
		if !containsString(cfg.Server.Capture.RedactKeys, key) {
			t.Errorf("expected default capture.redactKeys to contain %q; got %#v", key, cfg.Server.Capture.RedactKeys)
		}
	}
	if cfg.Fallback != "echo" {
		t.Errorf("expected default fallback=echo, got %q", cfg.Fallback)
	}
}

func TestApplyDefaults_CaptureDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if cfg.Server.Capture.Enabled {
		t.Fatal("capture.enabled default should be false")
	}
	if cfg.Server.Capture.MaxBodySize != "64KiB" {
		t.Fatalf("capture.maxBodySize=%q, want 64KiB", cfg.Server.Capture.MaxBodySize)
	}
	wantKeys := []string{"authorization", "cookie", "password", "token", "secret"}
	for _, key := range wantKeys {
		if !containsString(cfg.Server.Capture.RedactKeys, key) {
			t.Fatalf("capture.redactKeys=%#v missing %q", cfg.Server.Capture.RedactKeys, key)
		}
	}
}

func TestApplyDefaults_ScenarioDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Server.Scenario != "" {
		t.Fatalf("server.scenario default = %q, want empty", cfg.Server.Scenario)
	}
	if cfg.Scenarios == nil {
		t.Fatalf("scenarios map should be initialized")
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

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
