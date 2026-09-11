package cli

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestResolveListenAddr(t *testing.T) {
	port := 6001
	cfg := &config.Config{Server: config.ServerOpts{Host: "127.0.0.1", Port: 6000}}
	tests := []struct {
		name, wantHost string
		wantPort       int
		cfg            *config.Config
		opts           serveOptions
	}{
		{"none", "0.0.0.0", 5090, nil, serveOptions{}},
		{"config", "127.0.0.1", 6000, cfg, serveOptions{}},
		{"host", "localhost", 6000, cfg, serveOptions{Host: "localhost"}},
		{"port", "127.0.0.1", port, cfg, serveOptions{Port: port}},
		{"both", "localhost", port, cfg, serveOptions{Host: "localhost", Port: port}},
		{"explicit defaults", "0.0.0.0", 5090, cfg, serveOptions{Host: "0.0.0.0", Port: 5090}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, p := resolveListenAddr(tt.cfg, tt.opts)
			if h != tt.wantHost || p != tt.wantPort {
				t.Fatalf("got %s:%d", h, p)
			}
		})
	}
}

func TestListenAddrReloadWarning(t *testing.T) {
	base := &config.Config{Server: config.ServerOpts{Host: "127.0.0.1", Port: 6000}}
	tests := []struct {
		name, want string
		cfg        *config.Config
		opts       serveOptions
	}{
		{"unchanged", "", base, serveOptions{}},
		{"port changed", "127.0.0.1:6001", &config.Config{Server: config.ServerOpts{Host: "127.0.0.1", Port: 6001}}, serveOptions{}},
		{"port overridden", "", &config.Config{Server: config.ServerOpts{Host: "127.0.0.1", Port: 6001}}, serveOptions{Port: 6000}},
		{"host changed", "127.0.0.2:6000", &config.Config{Server: config.ServerOpts{Host: "127.0.0.2", Port: 6000}}, serveOptions{}},
		{"host overridden", "", &config.Config{Server: config.ServerOpts{Host: "127.0.0.2", Port: 6000}}, serveOptions{Host: "127.0.0.1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := listenAddrReloadWarning("127.0.0.1", 6000, tt.cfg, tt.opts)
			if tt.want == "" {
				if got != "" {
					t.Fatalf("got warning %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) || !strings.Contains(got, "127.0.0.1:6000") {
				t.Fatalf("warning %q does not contain new %q and old address", got, tt.want)
			}
		})
	}
}

func TestRunServe_UsesConfigPort(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	p := ln.Addr().(*net.TCPAddr).Port
	path := filepath.Join(tmp, "fakeserver.json5")
	if err := os.WriteFile(path, []byte("{ server: { host: '127.0.0.1', port: "+strconv.Itoa(p)+" }, routes: [] }"), 0644); err != nil {
		t.Fatal(err)
	}
	err = runServe(serveOptions{ConfigFlag: path, Quiet: true, NoWatch: true})
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(p)) {
		t.Fatalf("got %v", err)
	}
}

func TestRunServe_UsesConfigHost(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	path := filepath.Join(tmp, "fakeserver.json5")
	if err := os.WriteFile(path, []byte("{ server: { host: '192.0.2.1', port: 5090 }, routes: [] }"), 0644); err != nil {
		t.Fatal(err)
	}
	err := runServe(serveOptions{ConfigFlag: path, Quiet: true, NoWatch: true})
	if err == nil || !strings.Contains(err.Error(), "192.0.2.1") {
		t.Fatalf("got %v", err)
	}
}
