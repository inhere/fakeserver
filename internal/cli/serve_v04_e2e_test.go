package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/middleware"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/tpl"
)

// TestServe_v04_WebUIAPIs 验证 v0.4 Phase 1 DoD #4/#5：
// webui.Mount 注册的 3 个 JSON 端点可达，且 /api/history 包含刚刚打过的请求。
func TestServe_v04_WebUIAPIs(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ server: { adminEnabled: true }, routes: [{ method: "GET", path: "/hello", body: "world" }] }`), 0644)

	cfg, err := config.Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	ring := recorder.New(50)

	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true}, ring))
	srv := httptest.NewServer(holder)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/hello")
	resp.Body.Close()

	resp, _ = http.Get(srv.URL + "/__fakeserver/api/history")
	var entries []recorder.Entry
	_ = json.NewDecoder(resp.Body).Decode(&entries)
	resp.Body.Close()
	if len(entries) < 1 {
		t.Errorf("api/history should contain at least 1 entry; got %d", len(entries))
	}
	found := false
	for _, e := range entries {
		if e.Path == "/hello" && e.Status == 200 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("/hello request not in history: %+v", entries)
	}

	resp, _ = http.Get(srv.URL + "/__fakeserver/api/config")
	if resp.StatusCode != 200 {
		t.Errorf("api/config status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = http.Get(srv.URL + "/__fakeserver/api/projects")
	if resp.StatusCode != 200 {
		t.Errorf("api/projects status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestServe_v04_AdminDisabled_NoUIEndpoints 验证 adminEnabled=false 全部 404。
func TestServe_v04_AdminDisabled_NoUIEndpoints(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ server: { adminEnabled: false }, routes: [{ method: "GET", path: "/p", body: "x" }] }`), 0644)

	cfg, err := config.Load([]string{cfgPath}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	ring := recorder.New(50)
	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true}, ring))
	srv := httptest.NewServer(holder)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/__fakeserver/api/history")
	if resp.StatusCode != 404 {
		t.Errorf("api/history with adminEnabled=false should be 404; got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
