package echo_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/echo"
)

// 启动一个仅挂了 echo 的 router 实例供下面用例共享。
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	r := rux.New()
	echo.Mount(r)
	return httptest.NewServer(r)
}

func TestEcho_AnythingReturnsJSON(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/anything/foo", strings.NewReader(`{"hello":"world"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test", "yes")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("expected JSON Content-Type, got %q", resp.Header.Get("Content-Type"))
	}

	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %v\nbody: %s", err, body)
	}
	if len(parsed) == 0 {
		t.Errorf("expected non-empty JSON object, body=%s", body)
	}
}

func TestEcho_StatusCodeRoute(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status/418")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 418 {
		t.Errorf("expected status 418, got %d", resp.StatusCode)
	}
}

// TestEcho_StatusCodeInvalidFallsBackTo200 documents rux v2 behavior:
// out-of-range codes silently fall back to 200 rather than 400. We test
// the documented behavior (not a hypothetical "should be 400") because
// echo is fakeserver's fallback diagnostic surface — being lenient on
// inputs is the correct trade-off.
func TestEcho_StatusCodeInvalidFallsBackTo200(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status/999999")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (rux v2 fallback for invalid codes), got %d", resp.StatusCode)
	}
}

func TestEcho_HeadersEndpoint(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/headers", nil)
	req.Header.Set("X-Custom-Header", "abc")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "X-Custom-Header") {
		t.Errorf("expected response to mention X-Custom-Header, body=%s", body)
	}
}

// TestEcho_IPEndpointReturnsOrigin: rux v2 returns the client IP under
// the key "origin" (httpbin-compatible) — not "ip" as our v1 plan
// assumed. We test the actual key.
func TestEcho_IPEndpointReturnsOrigin(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ip")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
	if _, ok := parsed["origin"]; !ok {
		t.Errorf("expected 'origin' field in response, body=%s", body)
	}
}

// TestEcho_CatchAllOnUnknownPath: rux v2's MountEchoRoutes registers a
// final /*path catch-all that echoes any otherwise-unhandled request.
// Confirms our zero-config fallback works for arbitrary paths.
func TestEcho_CatchAllOnUnknownPath(t *testing.T) {
	ts := newServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/totally/unmapped/path/xyz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (echo catch-all), got %d", resp.StatusCode)
	}
}
