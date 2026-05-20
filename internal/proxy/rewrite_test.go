package proxy

import (
	"testing"
)

func TestCompileRewrites_Nil(t *testing.T) {
	rules, err := compileRewrites(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("nil input → 0 rules; got %d", len(rules))
	}
}

func TestCompileRewrites_SingleString(t *testing.T) {
	rules, err := compileRewrites(`^/api/users => /v2/users`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules", len(rules))
	}
	out, ok := applyRewrites(rules, "/api/users/42")
	if !ok || out != "/v2/users/42" {
		t.Errorf("apply: out=%q ok=%v", out, ok)
	}
}

func TestCompileRewrites_Array(t *testing.T) {
	rules, err := compileRewrites([]any{
		`^/api/v1/(.+) => /legacy/$1`,
		`^/api/v2/(.+) => /modern/$1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules", len(rules))
	}
	out, _ := applyRewrites(rules, "/api/v2/users")
	if out != "/modern/users" {
		t.Errorf("apply v2: %q", out)
	}
	out, _ = applyRewrites(rules, "/api/v1/orders/9")
	if out != "/legacy/orders/9" {
		t.Errorf("apply v1: %q", out)
	}
}

func TestCompileRewrites_StringSlice(t *testing.T) {
	// JSON5 也可能把 array of string decode 成 []string 而非 []any
	rules, err := compileRewrites([]string{
		`^/a => /A`,
		`^/b => /B`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules", len(rules))
	}
}

func TestCompileRewrites_NoMatchPreservesPath(t *testing.T) {
	rules, _ := compileRewrites(`^/api/v1/(.+) => /legacy/$1`)
	out, ok := applyRewrites(rules, "/health")
	if ok {
		t.Error("no-match should signal ok=false")
	}
	if out != "/health" {
		t.Errorf("no-match should preserve path; got %q", out)
	}
}

func TestCompileRewrites_FirstMatchWins(t *testing.T) {
	rules, _ := compileRewrites([]any{
		`^/api/users => /v2/users`,
		`^/api => /catchall`, // 后规则应被首条 swallow
	})
	out, _ := applyRewrites(rules, "/api/users/1")
	if out != "/v2/users/1" {
		t.Errorf("first match should win; got %q", out)
	}
}

func TestCompileRewrites_BadSyntax(t *testing.T) {
	_, err := compileRewrites(`/api/users /v2/users`) // missing =>
	if err == nil {
		t.Error("expected error on missing arrow")
	}
}

func TestCompileRewrites_BadRegex(t *testing.T) {
	_, err := compileRewrites(`[invalid => /x`)
	if err == nil {
		t.Error("expected regex compile error")
	}
}
