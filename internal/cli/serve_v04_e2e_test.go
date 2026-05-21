package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestServe_v04_SSE_EventsEndpoint 验证 v0.4 Phase 2 集成：完整 assembleHandler
// 装配下，/__fakeserver/events 端点可达，Append 后 client 收到 request 事件。
func TestServe_v04_SSE_EventsEndpoint(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	_ = os.WriteFile(cfgPath, []byte(`{ server: { adminEnabled: true }, routes: [{ method: "GET", path: "/hi", body: "y" }] }`), 0644)

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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/__fakeserver/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Errorf("Content-Type=%q, want text/event-stream prefix", resp.Header.Get("Content-Type"))
	}

	// 触发一次请求 → logger 中间件 Append ring → SSE 应推 request 事件
	go func() {
		time.Sleep(80 * time.Millisecond)
		hresp, _ := http.Get(srv.URL + "/hi")
		hresp.Body.Close()
	}()

	scanner := bufio.NewScanner(resp.Body)
	var seenRequest bool
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) && scanner.Scan() {
		if scanner.Text() == "event: request" {
			seenRequest = true
			break
		}
	}
	if !seenRequest {
		t.Error("SSE /events should deliver request event after mock route is hit")
	}
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
