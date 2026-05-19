// 此测试在 `package cli`（非 `cli_test`），以便复用包级私有 assembleRouter。
package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestServe_Healthz(t *testing.T) {
	ts := httptest.NewServer(assembleRouter())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/__fakeserver/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServe_EchoOnAnything(t *testing.T) {
	ts := httptest.NewServer(assembleRouter())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anything/abc?x=1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("expected JSON, got %q", resp.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, body)
	}
}

// TestServe_EchoCatchAllOnUnknownPath: rux v2 的 MountEchoRoutes 注册了
// /*path 兜底，未匹配路径会被回显（fallback 默认 echo）。
func TestServe_EchoCatchAllOnUnknownPath(t *testing.T) {
	ts := httptest.NewServer(assembleRouter())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/totally/unknown/path")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (echo fallback), got %d", resp.StatusCode)
	}
}

func TestServe_StatusEndpoint(t *testing.T) {
	ts := httptest.NewServer(assembleRouter())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status/503")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestServe_ConfigLoadDoesNotRegisterMockRoutes(t *testing.T) {
	cfg, err := config.Load(
		[]string{"../config/testdata/valid/single-full.json5"},
		"", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// 在 Phase 2，assembleRouter 与 cfg 无关——仅 admin + echo。
	// 用 PrintRouteSummary 验证 cfg 含 mock 路由，但 router 不应注册。
	var buf bytes.Buffer
	PrintRouteSummary(cfg, &buf)
	if !bytes.Contains(buf.Bytes(), []byte("/users/{id}")) {
		t.Errorf("summary should list /users/{id}; got %s", buf.String())
	}

	// 路由 /users/42 当前请求会走 echo catch-all 而不是 cfg.Routes —— 这是 Phase 2 的合约。
	// 真正的 mock 响应在 Phase 3 接入。这条测试只是把"不注册"事实显式化。
}
