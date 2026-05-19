package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gookit/rux"

	"github.com/inhere/fakeserver/internal/admin"
)

func TestHealthz_Returns200OK(t *testing.T) {
	r := rux.New()
	admin.Mount(r)
	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/__fakeserver/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if parsed["status"] != "ok" {
		t.Errorf(`expected status=="ok", got %q`, parsed["status"])
	}
}
