package mock

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// newRespondServer wires a rux router that calls mock.Respond for the
// configured route and returns a running httptest.Server. Using a real
// dispatch path is the cleanest way to exercise rux v2's inline
// [16]Param container — c.Params() is populated by the router from the
// matched URL, which is the same code path production traffic uses.
//
// rux v2 path syntax: "{name}" for path params (verified against
// internal/echo/probe.md and server.MountEchoRoutes' "/status/{code}"
// route). The handler receives a *rux.Context with c.Req == real
// *http.Request and c.Resp wrapping the underlying httptest writer.
func newRespondServer(t *testing.T, method, registerPath string, route *config.Route, renderer tpl.Renderer) *httptest.Server {
	t.Helper()
	r := rux.New()
	r.Add(registerPath, func(c *rux.Context) {
		Respond(c, route, renderer)
	}, method)
	return httptest.NewServer(r)
}

func TestRespond_PlainStringBodyInfersTextPlain(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/ping", Body: "pong"}
	ts := newRespondServer(t, "GET", "/ping", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("body: got %q", string(body))
	}
}

func TestRespond_MapBodyInfersJSON(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/x", Body: map[string]any{"k": "v"}}
	ts := newRespondServer(t, "GET", "/x", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
	if got["k"] != "v" {
		t.Errorf("body: got %v", got)
	}
}

func TestRespond_TemplateRendersParams(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method: []string{"GET"},
		Path:   "/u/{id}",
		Body: map[string]any{
			"id":   "{{ .Request.Params.id }}",
			"echo": "{{ .Request.Method }}",
		},
	}
	ts := newRespondServer(t, "GET", "/u/{id}", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/u/42")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
	if got["id"] != "42" {
		t.Errorf("id: got %v", got["id"])
	}
	if got["echo"] != "GET" {
		t.Errorf("echo: got %v", got["echo"])
	}
}

func TestRespond_BodyFileServesBytesAndInfersType(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	// BodyFile path is relative; SourceFile empty means it's resolved
	// against the test's working directory (the package dir), which is
	// exactly where testdata/fixtures/avatar.png lives.
	route := &config.Route{Method: []string{"GET"}, Path: "/avatar", BodyFile: "testdata/fixtures/avatar.png"}
	ts := newRespondServer(t, "GET", "/avatar", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/avatar")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/png") {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("expected non-empty body for bodyFile")
	}
}

func TestRespond_ExplicitContentTypePreserved(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{
		Method:  []string{"GET"},
		Path:    "/x",
		Headers: map[string]string{"Content-Type": "application/xml"},
		Body:    "<x/>",
	}
	ts := newRespondServer(t, "GET", "/x", route, r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "application/xml" {
		t.Errorf("Content-Type: got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<x/>" {
		t.Errorf("body: got %q", string(body))
	}
}

func TestRespond_DelayApplied(t *testing.T) {
	r := tpl.NewRenderer(nil, nil, 0)
	route := &config.Route{Method: []string{"GET"}, Path: "/x", Body: "x", Delay: "50ms"}
	ts := newRespondServer(t, "GET", "/x", route, r)
	defer ts.Close()

	start := time.Now()
	resp, err := http.Get(ts.URL + "/x")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Errorf("delay: %v (want ~50ms)", elapsed)
	}
}
