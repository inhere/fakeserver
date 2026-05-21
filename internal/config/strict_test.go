package config

import (
	"strings"
	"testing"
)

func TestStrictValidate_CatchesBodyTemplateSyntax(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method: []string{"GET"},
		Path:   "/bad",
		Body:   "{{ .unclosed",
	}}}

	errs := StrictValidate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected strict template problem")
	}
	got := errs[0].Error()
	for _, want := range []string{"GET /bad", "body", "unclosed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("strict error missing %q: %s", want, got)
		}
	}
}

func TestStrictValidate_CatchesHeaderTemplateSyntax(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method:  []string{"GET"},
		Path:    "/bad-header",
		Headers: map[string]string{"X-Trace-Id": "{{ .unclosed"},
		Body:    "ok",
	}}}

	errs := StrictValidate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected strict header problem")
	}
	if got := errs[0].Error(); !strings.Contains(got, "headers.X-Trace-Id") {
		t.Fatalf("strict error should mention header field, got %s", got)
	}
}

func TestStrictValidate_HintsHeaderKeyWithDash(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method: []string{"GET"},
		Path:   "/ua",
		Body:   `{{ .request.headers.User-Agent }}`,
	}}}

	errs := StrictValidate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected strict problem")
	}
	got := errs[0].Error()
	if !strings.Contains(got, `index .request.headers "User-Agent"`) {
		t.Fatalf("strict error should include header index hint, got %s", got)
	}
}

func TestStrictValidate_CaseBodyTemplateSyntax(t *testing.T) {
	cfg := &Config{Routes: []Route{{
		Method: []string{"POST"},
		Path:   "/cases",
		Cases: []RouteCase{{
			Status: 200,
			Body:   map[string]any{"message": "{{ .unclosed"},
		}},
	}}}

	errs := StrictValidate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected strict case problem")
	}
	got := errs[0].Error()
	for _, want := range []string{"POST /cases", "cases[0].body.message"} {
		if !strings.Contains(got, want) {
			t.Fatalf("strict error missing %q: %s", want, got)
		}
	}
}

func TestStrictValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerOpts{OSEnvWhitelist: []string{"USER"}},
		Routes: []Route{{
			Method:  []string{"GET"},
			Path:    "/ok",
			Headers: map[string]string{"X-Trace-Id": "{{ shortid }}"},
			Body: map[string]any{
				"id": "{{ uuid }}",
				"ua": `{{ index .request.headers "User-Agent" }}`,
			},
		}},
	}

	if errs := StrictValidate(cfg); len(errs) > 0 {
		t.Fatalf("expected no strict errors, got %v", errs)
	}
}
