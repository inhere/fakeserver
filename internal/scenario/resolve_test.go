package scenario

import (
	"net/http/httptest"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestResolveScenarioPriority(t *testing.T) {
	store := NewStore()
	store.SetSelected("ui")
	cfg := &config.Config{Server: config.ServerOpts{Scenario: "config"}}

	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set(HeaderName, "header")
	got, source := Resolve(req, store, cfg, "cli")
	if got != "header" || source != "header" {
		t.Fatalf("header priority got scenario=%q source=%q", got, source)
	}

	req = httptest.NewRequest("GET", "/api/users", nil)
	got, source = Resolve(req, store, cfg, "cli")
	if got != "ui" || source != "ui" {
		t.Fatalf("ui priority got scenario=%q source=%q", got, source)
	}

	store.SetSelected("")
	got, source = Resolve(req, store, cfg, "cli")
	if got != "cli" || source != "cli" {
		t.Fatalf("cli priority got scenario=%q source=%q", got, source)
	}

	got, source = Resolve(req, store, cfg, "")
	if got != "config" || source != "config" {
		t.Fatalf("config priority got scenario=%q source=%q", got, source)
	}
}
