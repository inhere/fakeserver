package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/middleware"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/tpl"
)

func TestServeV06_WebUIDebugConsoleE2E(t *testing.T) {
	tmp := t.TempDir()
	if err := runInit(initOptions{cwd: tmp, full: true}); err != nil {
		t.Fatalf("runInit full: %v", err)
	}
	cfgPath := filepath.Join(tmp, "fakeserver.json5")
	cfg, err := config.Load([]string{cfgPath}, "dev", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rdr := tpl.NewRenderer(cfg.Globals, cfg.Server.OSEnvWhitelist, cfg.Server.FakerSeed)
	ring := recorder.New(50)
	holder := middleware.NewHolder()
	holder.Swap(assembleHandler(cfg, rdr, serveOptions{Quiet: true, NoCORS: true}, ring))
	srv := httptest.NewServer(holder)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/users")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/__fakeserver/api/history")
	if err != nil {
		t.Fatal(err)
	}
	var entries []recorder.Entry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(entries) == 0 {
		t.Fatal("history should contain /api/users request")
	}
	entry := entries[len(entries)-1]
	if entry.ID == 0 {
		t.Fatalf("entry ID not assigned: %+v", entry)
	}

	resp, err = http.Get(srv.URL + "/__fakeserver/api/history/" + jsonNumber(entry.ID))
	if err != nil {
		t.Fatal(err)
	}
	var detail recorder.Entry
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if detail.Method != "GET" || detail.Path != "/api/users" || detail.Status != 200 {
		t.Fatalf("detail mismatch: %+v", detail)
	}
	if detail.RouteMode == "" || detail.RouteIndex == nil || detail.RouteSource == "" {
		t.Fatalf("detail missing route metadata: %+v", detail)
	}
	if detail.Request.Headers == nil || detail.Response.Body == "" {
		t.Fatalf("detail missing capture data: %+v", detail)
	}

	resp, err = http.Get(srv.URL + "/__fakeserver/routes")
	if err != nil {
		t.Fatal(err)
	}
	var routes []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var found bool
	for _, route := range routes {
		if route["path"] == "/api/users" && route["method"] == "GET" {
			found = true
			if route["mode"] == "" || route["source"] == "" || route["index"] == nil {
				t.Fatalf("route metadata incomplete: %+v", route)
			}
		}
	}
	if !found {
		t.Fatalf("/api/users route not found in metadata: %+v", routes)
	}

	resp, err = http.Get(srv.URL + "/__fakeserver/ui/")
	if err != nil {
		t.Fatal(err)
	}
	ui := readBodyString(t, resp)
	for _, want := range []string{"history-detail-drawer", "route-tester", "copy-curl-button"} {
		if !strings.Contains(ui, want) {
			t.Fatalf("ui missing %q", want)
		}
	}
}

func jsonNumber(v uint64) string {
	data, _ := json.Marshal(v)
	return string(data)
}
