package webui

import (
	"io/fs"
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
	Mount(router, cfg, "", recorder.New(10), nil)

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

func TestUIAssets_IndexServedWithoutTrailingSlash(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10), nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`href="/__fakeserver/ui/style.css"`, `src="/__fakeserver/ui/main.js"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("index body missing absolute asset reference %q: %s", want, body)
		}
	}
}

func TestUIAssets_IndexServedByFilename(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10), nil)

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
	Mount(router, cfg, "", recorder.New(10), nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/missing.css", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestUIAssets_CSSAndJSContracts(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10), nil)

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

func TestUIAssets_DebugConsoleContracts(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10), nil)

	index := httptest.NewRecorder()
	router.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/", nil))
	for _, want := range []string{
		"history-detail-drawer",
		"history-detail-body",
		"copy-curl-button",
		"replay-button",
		"route-tester",
	} {
		if !strings.Contains(index.Body.String(), want) {
			t.Fatalf("index missing %q", want)
		}
	}

	js := httptest.NewRecorder()
	router.ServeHTTP(js, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/main.js", nil))
	for _, want := range []string{"function buildCurl", "openHistoryDetail", "copy-curl-button"} {
		if !strings.Contains(js.Body.String(), want) {
			t.Fatalf("main.js missing %q", want)
		}
	}
}

func TestUIAssets_ReplayContracts(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10), nil)

	js := httptest.NewRecorder()
	router.ServeHTTP(js, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/main.js", nil))
	body := js.Body.String()
	for _, want := range []string{"function replayEntry", "replay-result", "forbiddenHeaders"} {
		if !strings.Contains(body, want) {
			t.Fatalf("main.js missing %q", want)
		}
	}
}

func TestUIAssets_RouteTesterContracts(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(true)}}
	Mount(router, cfg, "", recorder.New(10), nil)

	index := httptest.NewRecorder()
	router.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/", nil))
	for _, want := range []string{
		"route-tester",
		"tester-method",
		"tester-path",
		"tester-query",
		"tester-headers",
		"tester-body",
		"tester-send",
		"tester-response",
	} {
		if !strings.Contains(index.Body.String(), want) {
			t.Fatalf("index missing %q", want)
		}
	}
	body := index.Body.String()
	routesIdx := strings.Index(body, `id="view-routes"`)
	testerIdx := strings.Index(body, `id="route-tester"`)
	historyIdx := strings.Index(body, `id="view-history"`)
	if routesIdx < 0 || testerIdx < 0 || historyIdx < 0 {
		t.Fatalf("index missing route tester landmarks")
	}
	if !(routesIdx < testerIdx && testerIdx < historyIdx) {
		t.Fatalf("route tester must be inside Routes view before History view")
	}
}

func readAsset(t *testing.T, name string) string {
	t.Helper()
	data, err := fs.ReadFile(embeddedAssets, name)
	if err != nil {
		t.Fatalf("read asset %s: %v", name, err)
	}
	return string(data)
}

func TestUIAssets_ScenarioControlContracts(t *testing.T) {
	data := readAsset(t, "assets/index.html")
	for _, want := range []string{
		`scenario-select`,
		`scenario-clear`,
		`route-override-panel`,
		`override-case`,
		`override-mode`,
		`override-remaining`,
		`override-apply`,
		`override-clear`,
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("index missing %q", want)
		}
	}

	js := readAsset(t, "assets/main.js")
	for _, want := range []string{
		`loadScenarioState`,
		`setSelectedScenario`,
		`applyRouteOverride`,
		`clearRouteOverride`,
		`/__fakeserver/api/scenario`,
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("main.js missing %q", want)
		}
	}
}

func TestUIAssets_AdminDisabledNoEndpoints(t *testing.T) {
	router := rux.New()
	cfg := &config.Config{Server: config.ServerOpts{AdminEnabled: boolPtr(false)}}
	Mount(router, cfg, "", recorder.New(10), nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/__fakeserver/ui/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}
