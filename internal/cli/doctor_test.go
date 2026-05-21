package cli

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctor_ValidProject(t *testing.T) {
	tmp := t.TempDir()
	if err := runInit(initOptions{cwd: tmp, full: true}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runDoctor(doctorOptions{cwd: tmp, configFlag: "fakeserver.json5", envName: "dev", out: &buf})
	if err != nil {
		t.Fatalf("doctor should pass: %v\n%s", err, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"OK   config", "OK   env", "OK   includes", "OK   bodyFile", "OK   webui"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDoctor_MissingConfigFails(t *testing.T) {
	var buf bytes.Buffer
	err := runDoctor(doctorOptions{cwd: t.TempDir(), out: &buf})
	if err == nil {
		t.Fatal("expected missing config to fail")
	}
	if out := buf.String(); !strings.Contains(out, "FAIL config") {
		t.Fatalf("missing config output should contain FAIL config, got:\n%s", out)
	}
}

func TestRunDoctor_MissingBodyFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	body := `{
		routes: [{ method: "GET", path: "/report", bodyFile: "missing.json" }],
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runDoctor(doctorOptions{cwd: tmp, configFlag: "fakeserver.json5", out: &buf})
	if err == nil {
		t.Fatal("expected missing bodyFile to fail")
	}
	out := buf.String()
	for _, want := range []string{"FAIL bodyFile", "fix:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing bodyFile output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDoctor_AdminExposureWarn(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	body := `{
		server: { host: "0.0.0.0", adminEnabled: true },
		routes: [{ method: "GET", path: "/ping", body: "pong" }],
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runDoctor(doctorOptions{cwd: tmp, configFlag: "fakeserver.json5", out: &buf})
	if err != nil {
		t.Fatalf("admin exposure should warn, not fail: %v\n%s", err, buf.String())
	}
	if out := buf.String(); !strings.Contains(out, "WARN admin") {
		t.Fatalf("admin warning missing:\n%s", out)
	}
}

func TestRunDoctor_PortOccupiedFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	body := fmt.Sprintf(`{
		server: { host: "127.0.0.1", port: %d },
		routes: [{ method: "GET", path: "/ping", body: "pong" }],
	}`, port)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err = runDoctor(doctorOptions{cwd: tmp, configFlag: "fakeserver.json5", out: &buf})
	if err == nil {
		t.Fatal("expected occupied port to fail")
	}
	if out := buf.String(); !strings.Contains(out, "FAIL port") {
		t.Fatalf("port failure missing:\n%s", out)
	}
}
