package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRoutes_ValidConfigPrintsSummary(t *testing.T) {
	var buf bytes.Buffer
	err := runRoutes(routesOptions{
		paths: []string{"../config/testdata/valid/single-full.json5"},
		out:   &buf,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "/ping") || !strings.Contains(out, "/users/{id}") {
		t.Errorf("expected routes in output, got:\n%s", out)
	}
}

func TestRunRoutes_InvalidConfigReturnsError(t *testing.T) {
	err := runRoutes(routesOptions{
		paths: []string{"../config/testdata/invalid/body-and-bodyfile.json5"},
		out:   &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
