package config

import "testing"

// Smoke: LoadEnvFile exists with the expected signature and returns
// (map[string]any, string, error). Tasks 4-5 add real behavior tests.
func TestLoadEnvFile_SignatureSmoke(t *testing.T) {
	env, active, err := LoadEnvFile("/non/existent/path.json5", "")
	if err != nil {
		t.Errorf("missing file should not error in stub; got %v", err)
	}
	if env == nil {
		t.Error("env should never be nil (use empty map instead)")
	}
	if active != "" {
		t.Errorf("missing file → active should be empty; got %q", active)
	}
}
