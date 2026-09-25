package cli

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/buildinfo"
)

func withBuildInfo(t *testing.T, version, commit, buildTime string) {
	t.Helper()
	prevV, prevC, prevB := buildinfo.Version, buildinfo.GitHash, buildinfo.BuildTime
	buildinfo.Set(version, buildTime, commit)
	t.Cleanup(func() { buildinfo.Set(prevV, prevB, prevC) })
}

// TestRunVersion_PlainTextMatchesVersionFlag 锁定 `fakeserver version` 与
// `fakeserver --version` 输出同一文本（app.Version 就是 versionLine()）。
func TestRunVersion_PlainTextMatchesVersionFlag(t *testing.T) {
	withBuildInfo(t, "v9.9.9", "abcdef1", "2026-01-02T03:04:05Z")

	var buf bytes.Buffer
	if err := runVersion(versionOptions{out: &buf}); err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	want := "Version: v9.9.9, abcdef1, 2026-01-02T03:04:05Z\n"
	if got := buf.String(); got != want {
		t.Fatalf("version output=%q, want %q", got, want)
	}
	if !strings.HasPrefix(want, "Version: "+versionLine()) {
		t.Fatalf("version output drifted from the --version line %q", versionLine())
	}
}

func TestRunVersion_JSON(t *testing.T) {
	withBuildInfo(t, "v9.9.9", "abcdef1", "2026-01-02T03:04:05Z")

	var buf bytes.Buffer
	if err := runVersion(versionOptions{JSON: true, out: &buf}); err != nil {
		t.Fatalf("runVersion --json: %v", err)
	}
	if lines := strings.Count(strings.TrimRight(buf.String(), "\n"), "\n"); lines != 0 {
		t.Fatalf("version --json must print a single line, got %q", buf.String())
	}
	var got struct {
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		BuildTime string `json:"buildTime"`
		GoVersion string `json:"goVersion"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("version --json is not JSON (%q): %v", buf.String(), err)
	}
	if got.Version != "v9.9.9" || got.Commit != "abcdef1" || got.BuildTime != "2026-01-02T03:04:05Z" {
		t.Fatalf("version --json mismatch: %+v", got)
	}
	if got.GoVersion != runtime.Version() || !strings.HasPrefix(got.GoVersion, "go") {
		t.Fatalf("goVersion=%q, want %q", got.GoVersion, runtime.Version())
	}
}
