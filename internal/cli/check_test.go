package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCheck_ValidConfigReturnsNilAndCountsRoutes(t *testing.T) {
	var buf bytes.Buffer
	err := runCheck(checkOptions{
		paths: []string{"../config/testdata/valid/single-full.json5"},
		out:   &buf,
	})
	if err != nil {
		t.Fatalf("expected nil err for valid config, got %v", err)
	}
	if !strings.Contains(buf.String(), "OK") {
		t.Errorf("expected OK in output, got %q", buf.String())
	}
}

func TestRunCheck_InvalidConfigReturnsErrorWithAllProblems(t *testing.T) {
	var buf bytes.Buffer
	err := runCheck(checkOptions{
		paths: []string{"../config/testdata/invalid/body-and-bodyfile.json5"},
		out:   &buf,
	})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "body") {
		t.Errorf("error should mention body/bodyFile mutex, got %v", err)
	}
}

func TestRunCheck_MissingPathReturnsError(t *testing.T) {
	var buf bytes.Buffer
	err := runCheck(checkOptions{
		paths: []string{"does-not-exist.json5"},
		out:   &buf,
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
