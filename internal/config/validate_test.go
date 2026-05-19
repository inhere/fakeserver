package config

import (
	"strings"
	"testing"
)

func loadOrFatal(t *testing.T, path string) *Config {
	t.Helper()
	cfg, err := Load([]string{path}, "", nil)
	if err != nil {
		t.Fatalf("Load %q: %v", path, err)
	}
	return cfg
}

func TestValidate_HappyPath(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/valid/single-full.json5")
	errs := Validate(cfg)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_BodyAndBodyFile(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/body-and-bodyfile.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "body", "bodyFile") {
		t.Errorf("expected error mentioning body/bodyFile mutex; got %v", errs)
	}
}

func TestValidate_DuplicateRoutes(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/duplicate-routes.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "duplicate", "/users") {
		t.Errorf("expected duplicate-route error; got %v", errs)
	}
}

func TestValidate_MissingMethod(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/missing-method.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "method") {
		t.Errorf("expected missing-method error; got %v", errs)
	}
}

func TestValidate_ReservedPrefix(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/reserved-prefix.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "__fakeserver") {
		t.Errorf("expected reserved-prefix error; got %v", errs)
	}
}

func TestValidate_ProxyAndBody(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/proxy-and-body.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "proxy", "body") {
		t.Errorf("expected proxy/body mutex error; got %v", errs)
	}
}

func TestValidate_ProxyBadScheme(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/proxy-bad-scheme.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "ftp", "scheme") {
		t.Errorf("expected proxy scheme error; got %v", errs)
	}
}

func TestValidate_BodyFileMissing(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bodyfile-missing.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "no-such-file") {
		t.Errorf("expected missing-file error; got %v", errs)
	}
}

func TestValidate_BadFallback(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bad-fallback.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "fallback") {
		t.Errorf("expected fallback enum error; got %v", errs)
	}
}

func TestValidate_BadStrategy(t *testing.T) {
	cfg := loadOrFatal(t, "testdata/invalid/bad-strategy.json5")
	errs := Validate(cfg)
	if !containsErrorWith(errs, "strategy") {
		t.Errorf("expected strategy enum error; got %v", errs)
	}
}

// containsErrorWith returns true if any error message contains every one
// of the given substrings (case-sensitive).
func containsErrorWith(errs []error, subs ...string) bool {
	for _, e := range errs {
		msg := e.Error()
		ok := true
		for _, s := range subs {
			if !strings.Contains(msg, s) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
