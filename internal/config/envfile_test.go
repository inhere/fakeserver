package config

import (
	"testing"
)

func TestLoadEnvFile_SignatureSmoke(t *testing.T) {
	env, active, err := LoadEnvFile("/non/existent/path.json5", "")
	if err != nil {
		t.Errorf("missing file should not error; got %v", err)
	}
	if env == nil {
		t.Error("env should never be nil")
	}
	if active != "" {
		t.Errorf("active=%q, want empty", active)
	}
}

func TestLoadEnvFile_DefaultOnly(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/default-only.json5", "")
	if err != nil {
		t.Fatal(err)
	}
	// No non-$default segments → no segment chosen → empty result
	if len(env) != 0 {
		t.Errorf("default-only with no envName → empty map; got %v", env)
	}
	if active != "" {
		t.Errorf("active=%q want empty", active)
	}
}

func TestLoadEnvFile_MultiEnv_ChooseDevByName(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/multi-env.json5", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if active != "dev" {
		t.Errorf("active=%q want 'dev'", active)
	}
	if env["apiHost"] != "localhost:5090" {
		t.Errorf("apiHost=%v want 'localhost:5090' (dev overrides $default)", env["apiHost"])
	}
	if env["timeout"] != "5s" {
		t.Errorf("timeout=%v want '5s' (inherited from $default)", env["timeout"])
	}
	if env["token"] != "dev-xxx" {
		t.Errorf("token=%v want 'dev-xxx'", env["token"])
	}
}

func TestLoadEnvFile_MultiEnv_ChooseStagingByName(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/multi-env.json5", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if active != "staging" {
		t.Errorf("active=%q want 'staging'", active)
	}
	if env["token"] != "stg-yyy" {
		t.Errorf("token=%v want 'stg-yyy'", env["token"])
	}
}

func TestLoadEnvFile_MultiEnv_FirstSegmentByDefault(t *testing.T) {
	// envName="" + no $active → iteration order picks first non-$default.
	// Go map iteration order is randomized, so we tolerate either dev or staging.
	env, active, err := LoadEnvFile("testdata/env/multi-env.json5", "")
	if err != nil {
		t.Fatal(err)
	}
	if active != "dev" && active != "staging" {
		t.Errorf("active=%q want 'dev' or 'staging' (first non-$default segment)", active)
	}
	if _, has := env["apiHost"]; !has {
		t.Errorf("no apiHost in env: %v", env)
	}
}

func TestLoadEnvFile_WithActiveField(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/with-active.json5", "")
	if err != nil {
		t.Fatal(err)
	}
	if active != "staging" {
		t.Errorf("active=%q want 'staging' ($active field should win)", active)
	}
	if env["apiHost"] != "stage.api.com" {
		t.Errorf("apiHost=%v want 'stage.api.com'", env["apiHost"])
	}
}

func TestLoadEnvFile_WithActive_EnvNameOverrides(t *testing.T) {
	// Explicit envName takes precedence over $active
	env, active, err := LoadEnvFile("testdata/env/with-active.json5", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if active != "dev" {
		t.Errorf("envName=dev should win over $active=staging; active=%q", active)
	}
	if env["token"] != "dev-xxx" {
		t.Errorf("token=%v want 'dev-xxx'", env["token"])
	}
}

func TestLoadEnvFile_MissingDefault(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/missing-default.json5", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if active != "dev" {
		t.Errorf("active=%q want 'dev'", active)
	}
	if env["apiHost"] != "dev.local" {
		t.Errorf("apiHost=%v want 'dev.local'", env["apiHost"])
	}
	if _, has := env["timeout"]; has {
		t.Errorf("no $default → no timeout key; got %v", env)
	}
}

func TestLoadEnvFile_EnvNameNotFound(t *testing.T) {
	_, _, err := LoadEnvFile("testdata/env/multi-env.json5", "production")
	if err == nil {
		t.Fatal("envName='production' not in file → expected error")
	}
}
