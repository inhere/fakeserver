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
