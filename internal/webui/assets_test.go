package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/recorder"
)

func TestUIAssets_IndexServedAtRoot(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type=%q, want text/html", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<title>fakeserver</title>") {
		t.Fatalf("index body missing title: %s", body)
	}
	for _, want := range []string{"#projects", "#routes", "#history", "#config", "style.css", "main.js"} {
		if !strings.Contains(body, want) {
			t.Fatalf("index body missing %q: %s", want, body)
		}
	}
}

func TestUIAssets_IndexServedByFilename(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/index.html", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<title>fakeserver</title>") {
		t.Fatal("index.html response missing app title")
	}
}

func TestUIAssets_MissingFileReturns404(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/missing.css", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestUIAssets_CSSAndJSContracts(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10))

	css := httptest.NewRecorder()
	router.ServeHTTP(css, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/style.css", nil))
	if css.Code != http.StatusOK {
		t.Fatalf("style.css status=%d, want 200", css.Code)
	}
	if ct := css.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("style.css Content-Type=%q, want text/css", ct)
	}
	if !strings.Contains(css.Body.String(), ".sidebar") {
		t.Fatal("style.css missing .sidebar selector")
	}

	js := httptest.NewRecorder()
	router.ServeHTTP(js, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/main.js", nil))
	if js.Code != http.StatusOK {
		t.Fatalf("main.js status=%d, want 200", js.Code)
	}
	if ct := js.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("main.js Content-Type=%q, want javascript", ct)
	}
	if !strings.Contains(js.Body.String(), "EventSource") {
		t.Fatal("main.js missing EventSource wiring")
	}
}

func TestUIAssets_AdminDisabledNoEndpoints(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(false)}}
	Mount(router, cfg, "", recorder.New(10))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}
