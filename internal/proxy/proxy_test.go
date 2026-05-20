package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// startProxyServer mounts cfg's proxy routes onto a fresh rux router and
// wraps it in an httptest.Server. Returns the server.
func startProxyServer(t *testing.T, cfg *config.Config) *httptest.Server {
	t.Helper()
	r := rux.New()
	rdr := tpl.NewRenderer(nil, nil, 1)
	if err := Mount(r, cfg, rdr); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	return httptest.NewServer(r)
}

// echoUpstream answers any request with method+path+body in a stable format.
func echoUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("UP:" + r.Method + ":" + r.URL.Path + ":" + string(b)))
	}))
}

func TestProxy_BasicForward(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy: &config.ProxyConfig{Target: upstream.URL},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/users/1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d want 200", resp.StatusCode)
	}
	if resp.Header.Get("X-Upstream") != "yes" {
		t.Error("missing X-Upstream → response not coming from upstream")
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(b), "UP:GET:/api/users/1") {
		t.Errorf("body=%q", string(b))
	}
}

func TestProxy_DialFailure_502(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy: &config.ProxyConfig{Target: "http://127.0.0.1:1"}, // 无端口监听
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Errorf("dial-fail status=%d want 502", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("error body should be JSON; CT=%q", ct)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `"target"`) {
		t.Errorf("error body should mention target; got %s", string(b))
	}
}

func TestProxy_PostBodyForwarded(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"POST"}, Path: "/api/*rest",
			Proxy: &config.ProxyConfig{Target: upstream.URL},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/echo", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.HasSuffix(string(b), ":hello") {
		t.Errorf("body=%q upstream did not see request body", string(b))
	}
}
