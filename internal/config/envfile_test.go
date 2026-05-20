package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadEnvFile_BadDefaultType(t *testing.T) {
	_, _, err := LoadEnvFile("testdata/env/bad-default-type.json5", "dev")
	if err == nil {
		t.Fatal("$default as string → expected error")
	}
	if !strings.Contains(err.Error(), "$default") {
		t.Errorf("error should mention $default; got %v", err)
	}
}

func TestLoadEnvFile_BadActiveType(t *testing.T) {
	_, _, err := LoadEnvFile("testdata/env/bad-active-type.json5", "")
	if err == nil {
		t.Fatal("$active as number → expected error")
	}
	if !strings.Contains(err.Error(), "$active") {
		t.Errorf("error should mention $active; got %v", err)
	}
}

func TestLoadEnvFile_RootArray_Rejected(t *testing.T) {
	_, _, err := LoadEnvFile("testdata/env/root-array.json5", "")
	if err == nil {
		t.Fatal("root array → expected error")
	}
	if !strings.Contains(err.Error(), "root must be an object") {
		t.Errorf("error should mention 'root must be an object'; got %v", err)
	}
}

func TestLoadEnvFile_HasInclude_Rejected(t *testing.T) {
	_, _, err := LoadEnvFile("testdata/env/has-include-error.json5", "dev")
	if err == nil {
		t.Fatal("env file with @include → expected error")
	}
	if !strings.Contains(err.Error(), "@include") {
		t.Errorf("error should mention '@include'; got %v", err)
	}
}

func TestLoadEnvFile_FileNotExist(t *testing.T) {
	env, active, err := LoadEnvFile("/totally/does/not/exist.json5", "")
	if err != nil {
		t.Errorf("missing file should NOT error; got %v", err)
	}
	if len(env) != 0 {
		t.Errorf("missing file → empty map; got %v", env)
	}
	if active != "" {
		t.Errorf("missing file → empty active; got %q", active)
	}
}

// TestLoadEnvFile_EscapedAtAllowed 锁定 `\@xxx` 转义语义：以反斜杠 + @
// 开头的字符串是字面值（loader 同 convention），不应被 @include 检测拦截。
func TestLoadEnvFile_EscapedAtAllowed(t *testing.T) {
	env, active, err := LoadEnvFile("testdata/env/with-escaped-at.json5", "dev")
	if err != nil {
		t.Fatalf("\\@-escaped literal should load successfully; got %v", err)
	}
	if active != "dev" {
		t.Errorf("active=%q want 'dev'", active)
	}
	// $default.literal carried into env (since chosen=dev wins on conflict,
	// but dev doesn't have "literal" key — so it inherits from $default).
	// Note: the loaded value is the literal string "\@not-an-include"
	// (loader.go's isIncludeString convention). Phase 2 will decide whether
	// to strip the leading backslash; for now we just verify it loaded.
	if _, has := env["literal"]; !has {
		t.Errorf("env should have 'literal' key from $default; got %v", env)
	}
}

// TestLoadEnvFile_ActiveFieldPointsToMissingSegment 锁定 $active 指向不存在
// segment 时的错误消息含 $active 上下文。
func TestLoadEnvFile_ActiveFieldPointsToMissingSegment(t *testing.T) {
	tmp := t.TempDir()
	fp := filepath.Join(tmp, "bad-active.env.json5")
	body := `{
		$active: "nonexistent",
		dev: { x: 1 },
	}`
	if err := os.WriteFile(fp, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadEnvFile(fp, "")
	if err == nil {
		t.Fatal("$active=nonexistent → expected error")
	}
	if !strings.Contains(err.Error(), "$active") {
		t.Errorf("error should reference $active; got %v", err)
	}
}
