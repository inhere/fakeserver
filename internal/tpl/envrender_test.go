package tpl

import (
	"os"
	"testing"
)

func TestRenderEnvValues_OsenvAllowed(t *testing.T) {
	os.Setenv("FAKESERVER_T_KEY", "secret")
	defer os.Unsetenv("FAKESERVER_T_KEY")
	env := map[string]any{
		"token": `{{ osenv "FAKESERVER_T_KEY" }}`,
	}
	if err := RenderEnvValues(env, []string{"FAKESERVER_T_KEY"}, nil); err != nil {
		t.Fatal(err)
	}
	if env["token"] != "secret" {
		t.Errorf("token=%v want 'secret'", env["token"])
	}
}

func TestRenderEnvValues_OsenvBlockedByWhitelist(t *testing.T) {
	os.Setenv("FAKESERVER_T_BLOCKED", "topsecret")
	defer os.Unsetenv("FAKESERVER_T_BLOCKED")
	env := map[string]any{
		"token": `{{ osenv "FAKESERVER_T_BLOCKED" }}`,
	}
	if err := RenderEnvValues(env, []string{"OTHER_KEY"}, nil); err != nil {
		t.Fatal(err)
	}
	if env["token"] != "" {
		t.Errorf("token=%q want empty (blocked)", env["token"])
	}
}

func TestRenderEnvValues_NestedMapAndSlice(t *testing.T) {
	env := map[string]any{
		"plain":  "no-template",
		"templ":  `{{ "rendered" }}`,
		"nested": map[string]any{"k": `{{ "deep" }}`},
		"list":   []any{`{{ "first" }}`, "literal"},
	}
	if err := RenderEnvValues(env, nil, nil); err != nil {
		t.Fatal(err)
	}
	if env["templ"] != "rendered" {
		t.Errorf("templ=%v", env["templ"])
	}
	if env["nested"].(map[string]any)["k"] != "deep" {
		t.Errorf("nested.k=%v", env["nested"].(map[string]any)["k"])
	}
	list := env["list"].([]any)
	if list[0] != "first" || list[1] != "literal" {
		t.Errorf("list=%v", list)
	}
}

func TestRenderEnvValues_NoTemplateInNonString(t *testing.T) {
	env := map[string]any{"num": 42, "bool": true}
	if err := RenderEnvValues(env, nil, nil); err != nil {
		t.Fatal(err)
	}
	if env["num"] != 42 || env["bool"] != true {
		t.Errorf("non-string values must pass through")
	}
}
