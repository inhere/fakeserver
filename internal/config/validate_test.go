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

func TestValidate_WhenSyntaxError(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"}, Path: "/x",
			Cases: []RouteCase{
				{When: `request.query.fail ==`, Status: 200, Body: "a"},
			},
		}},
	}
	errs := Validate(cfg)
	if !anyErrContains(errs, "when") {
		t.Errorf("expected when-syntax error, got %v", errs)
	}
}

func TestValidate_WhenEmpty_NoError(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"}, Path: "/x",
			Cases: []RouteCase{
				{Status: 200, Body: "a"}, // no when — 永远匹配
			},
		}},
	}
	errs := Validate(cfg)
	if len(errs) > 0 {
		t.Errorf("empty when should not error, got %v", errs)
	}
}

func TestWarn_FirstMatchNoFallback(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"}, Path: "/x", Strategy: "first-match",
			Cases: []RouteCase{
				{When: `request.query.a == "1"`, Status: 200, Body: "a"},
				{When: `request.query.b == "1"`, Status: 200, Body: "b"},
			},
		}},
	}
	warns := Warn(cfg)
	if len(warns) == 0 {
		t.Fatal("expected warn for first-match with no fallback")
	}
	if !strings.Contains(warns[0], "first-match") || !strings.Contains(warns[0], "no fallback") {
		t.Errorf("warn msg=%q", warns[0])
	}
}

func TestWarn_FirstMatchWithFallback_Silent(t *testing.T) {
	cfg := &Config{
		Fallback: "echo",
		Routes: []Route{{
			Method: []string{"GET"}, Path: "/x", Strategy: "first-match",
			Cases: []RouteCase{
				{When: `request.query.a == "1"`, Status: 200, Body: "a"},
				{Status: 200, Body: "fallback"}, // 兜底 case
			},
		}},
	}
	warns := Warn(cfg)
	for _, w := range warns {
		if strings.Contains(w, "first-match") {
			t.Errorf("should not warn when fallback case exists; got %q", w)
		}
	}
}

// anyErrContains returns true if any error message contains sub (case-sensitive).
func anyErrContains(errs []error, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), sub) {
			return true
		}
	}
	return false
}

func TestWarn_ProxyTargetPrivateHost(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		expectWarn bool
	}{
		{"localhost", "http://localhost:8080", true},
		{"127.0.0.1", "http://127.0.0.1:8080", true},
		{"192.168.x", "http://192.168.1.10:8080", true},
		{"10.x", "http://10.0.0.1:8080", true},
		{"172.16-31 lower", "http://172.16.0.1:8080", true},
		{"172.16-31 upper", "http://172.31.255.254:8080", true},
		{"172 outside range", "http://172.32.0.1:8080", false},
		{"public", "http://api.example.com:8080", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Fallback: "echo",
				Routes: []Route{{
					Method: []string{"*"}, Path: "/api/*rest",
					Proxy:  &ProxyConfig{Target: tt.target},
				}},
			}
			warns := Warn(cfg)
			hasPrivateWarn := false
			for _, w := range warns {
				if strings.Contains(w, "private") || strings.Contains(w, "localhost") {
					hasPrivateWarn = true
				}
			}
			if hasPrivateWarn != tt.expectWarn {
				t.Errorf("target=%q expected warn=%v, got warns=%v", tt.target, tt.expectWarn, warns)
			}
		})
	}
}
