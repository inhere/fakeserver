package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
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

func TestCheck_FirstMatchNoFallback_PrintsWarnButExitOk(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "cfg.json5")
	body := `{
		routes: [{
			method: "GET", path: "/x", strategy: "first-match",
			cases: [
				{ when: "request.query.a == \"1\"", status: 200, body: "a" },
				{ when: "request.query.b == \"1\"", status: 200, body: "b" },
			],
		}],
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	// Capture stderr
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	// checkOptions 使用 paths + out（实际字段名）
	var buf bytes.Buffer
	err := runCheck(checkOptions{paths: []string{cfgPath}, out: &buf})
	w.Close()
	stderrOut, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("check should pass (warn-only), got err=%v", err)
	}
	if !strings.Contains(string(stderrOut), "first-match") {
		t.Errorf("stderr missing warn: %q", string(stderrOut))
	}
}
