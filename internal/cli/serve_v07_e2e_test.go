package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/scenario"
	"github.com/inhere/fakeserver/internal/tpl"
)

func TestServeV07_ScenarioControlE2E(t *testing.T) {
	enabled := true
	cfg := &config.Config{
		Server: config.ServerOpts{AdminEnabled: &enabled, Capture: config.CaptureConfig{Enabled: true, MaxBodySize: "64KiB"}},
		Routes: []config.Route{{
			Method:   []string{"GET"},
			Path:     "/api/users",
			Strategy: "first-match",
			Cases: []config.RouteCase{
				{Name: "success", Status: 200, Body: map[string]any{"state": "success"}},
				{Name: "empty", Status: 200, Body: map[string]any{"state": "empty"}},
				{Name: "server-error", Status: 500, Body: map[string]any{"state": "error"}},
			},
		}},
		Scenarios: map[string]config.ScenarioConfig{
			"emptyUsers": {Routes: map[string]string{"GET /api/users": "empty"}},
		},
	}
	store := scenario.NewStore()
	ring := recorder.New(20)
	handler := assembleHandler(cfg, tpl.NewRenderer(nil, nil, 0), serveOptions{Quiet: true, NoCORS: true}, ring, store)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/users", nil)
	req.Header.Set(scenario.HeaderName, "emptyUsers")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	body := readBodyString(t, resp)
	if !strings.Contains(body, `"state":"empty"`) {
		t.Fatalf("header scenario response = %s", body)
	}

	rr := httptest.NewRecorder()
	apiReq := httptest.NewRequest("PUT", "/__fakeserver/api/scenario/overrides", strings.NewReader(`{"method":"GET","path":"/api/users","caseName":"server-error","mode":"next"}`))
	apiReq.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rr, apiReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("override api code=%d body=%s", rr.Code, rr.Body.String())
	}

	resp, err = http.Get(srv.URL + "/api/users")
	if err != nil {
		t.Fatalf("request override: %v", err)
	}
	body = readBodyString(t, resp)
	if !strings.Contains(body, `"state":"error"`) {
		t.Fatalf("next override response = %s", body)
	}

	entries := ring.Snapshot()
	if len(entries) == 0 {
		t.Fatal("expected history entries")
	}
	last := entries[len(entries)-1]
	if last.CaseName != "server-error" || last.OverrideSource != "override:next" {
		t.Fatalf("last history = %#v", last)
	}
}
