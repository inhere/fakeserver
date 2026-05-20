package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
	"github.com/inhere/fakeserver/internal/registry"
)

func TestAPIProjects_ReturnsRegistryProjects(t *testing.T) {
	d := t.TempDir()
	regPath := filepath.Join(d, "projects.json")
	reg := &registry.Registry{Version: 1, Projects: []registry.Project{
		{ID: "abc", Name: "alpha"}, {ID: "def", Name: "beta"},
	}}
	if err := registry.Save(regPath, reg); err != nil {
		t.Fatal(err)
	}

	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: true}}
	Mount(router, cfg, regPath, recorder.New(10))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/__fakeserver/api/projects", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	var got []registry.Project
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d projects, want 2", len(got))
	}
}

func TestAPIConfig_RedactsSensitiveKeys(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerOpts{AdminEnabled: true},
		Env: map[string]any{
			"apiHost":   "dev.local",
			"token":     "should-be-redacted",
			"apiSecret": "also-redacted",
			"password":  "ditto",
			"someToken": "ditto",
		},
	}
	router := rux.New()
	Mount(router, cfg, "", recorder.New(10))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/__fakeserver/api/config", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"apiHost"`) || !strings.Contains(body, "dev.local") {
		t.Errorf("apiHost should be visible; got: %s", body)
	}
	if strings.Contains(body, "should-be-redacted") || strings.Contains(body, "also-redacted") {
		t.Errorf("sensitive value leaked: %s", body)
	}
	if !strings.Contains(body, `"***"`) {
		t.Errorf("redacted placeholder *** missing: %s", body)
	}
}

func TestAPIHistory_ReturnsRingSnapshot(t *testing.T) {
	ring := recorder.New(10)
	ring.Append(recorder.Entry{Method: "GET", Path: "/p", Status: 200})
	ring.Append(recorder.Entry{Method: "POST", Path: "/x", Status: 201})

	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: true}}
	Mount(router, cfg, "", ring)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/__fakeserver/api/history", nil))
	var got []recorder.Entry
	_ = json.NewDecoder(w.Body).Decode(&got)
	if len(got) != 2 || got[0].Path != "/p" || got[1].Path != "/x" {
		t.Errorf("history mismatch: %+v", got)
	}
}

func TestMount_AdminDisabled_NoEndpoints(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: false}}
	Mount(router, cfg, "", recorder.New(10))

	endpoints := []string{
		"/__fakeserver/api/projects",
		"/__fakeserver/api/config",
		"/__fakeserver/api/history",
	}
	for _, ep := range endpoints {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", ep, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("endpoint %s with adminEnabled=false should be 404, got %d", ep, w.Code)
		}
	}
}

func TestRedactMap_NestedAndCaseInsensitive(t *testing.T) {
	in := map[string]any{
		"API_TOKEN":  "secret-1",
		"nested":     map[string]any{"DB_PASSWORD": "secret-2", "user": "alice"},
		"plain":      "visible",
		"clientSecret": "secret-3",
	}
	out := redactMap(in)
	if out["API_TOKEN"] != "***" {
		t.Errorf("API_TOKEN should be redacted (case-insensitive); got %v", out["API_TOKEN"])
	}
	if out["clientSecret"] != "***" {
		t.Errorf("clientSecret should be redacted; got %v", out["clientSecret"])
	}
	if nested, ok := out["nested"].(map[string]any); !ok {
		t.Fatal("nested map missing")
	} else {
		if nested["DB_PASSWORD"] != "***" {
			t.Errorf("nested password should be redacted; got %v", nested["DB_PASSWORD"])
		}
		if nested["user"] != "alice" {
			t.Errorf("nested user should be visible; got %v", nested["user"])
		}
	}
	if out["plain"] != "visible" {
		t.Errorf("plain should be visible; got %v", out["plain"])
	}
}
