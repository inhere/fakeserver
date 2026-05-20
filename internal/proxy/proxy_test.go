package proxy

import (
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

func TestProxy_StripPathPrefix(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy:  &config.ProxyConfig{Target: upstream.URL, StripPathPrefix: "/api"},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/api/users/1")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(b), "UP:GET:/users/1") {
		t.Errorf("expected /users/1 after strip; got %q", string(b))
	}
}

func TestProxy_Rewrite(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy: &config.ProxyConfig{
				Target:  upstream.URL,
				Rewrite: `^/api/v1/(.+) => /legacy/$1`,
			},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/api/v1/orders")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(b), "UP:GET:/legacy/orders") {
		t.Errorf("expected /legacy/orders; got %q", string(b))
	}
}

func TestProxy_StripThenRewrite(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"*"}, Path: "/api/*rest",
			Proxy: &config.ProxyConfig{
				Target:          upstream.URL,
				StripPathPrefix: "/api",
				Rewrite:         `^/v1/(.+) => /legacy/$1`,
			},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	// /api/v1/orders → strip → /v1/orders → rewrite → /legacy/orders
	resp, _ := http.Get(srv.URL + "/api/v1/orders")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(b), "UP:GET:/legacy/orders") {
		t.Errorf("strip+rewrite chain: got %q", string(b))
	}
}

func TestProxy_RequestHeaderInjection(t *testing.T) {
	var upstreamSawHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamSawHeader = r.Header.Get("X-Forwarded-By")
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy: &config.ProxyConfig{
				Target: upstream.URL,
				Headers: map[string]string{
					"X-Forwarded-By": "fakeserver-{{ .request.method }}",
				},
			},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/x")
	resp.Body.Close()
	if upstreamSawHeader != "fakeserver-GET" {
		t.Errorf("upstream X-Forwarded-By=%q (template not rendered?)", upstreamSawHeader)
	}
}

func TestProxy_ResponseHeaderInjection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy: &config.ProxyConfig{
				Target: upstream.URL,
				ResponseHeaders: map[string]string{
					"X-Mocked-By": "fakeserver-proxy",
				},
			},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/x")
	resp.Body.Close()
	if got := resp.Header.Get("X-Mocked-By"); got != "fakeserver-proxy" {
		t.Errorf("response X-Mocked-By=%q", got)
	}
}

func TestProxy_BodyLimit_413(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"POST"}, Path: "/x",
			Proxy:  &config.ProxyConfig{Target: upstream.URL, BodyLimit: "16B"},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/x", "text/plain", strings.NewReader("this body is more than sixteen bytes long"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Errorf("status=%d want 413", resp.StatusCode)
	}
}

func TestProxy_Timeout_504(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
			w.WriteHeader(200)
		}
	}))
	defer slow.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy:  &config.ProxyConfig{Target: slow.URL, Timeout: "50ms"},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 504 {
		t.Errorf("timeout status=%d want 504", resp.StatusCode)
	}
}

func TestProxy_PreserveHost(t *testing.T) {
	var sawHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHost = r.Host
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"GET"}, Path: "/x",
			Proxy:  &config.ProxyConfig{Target: upstream.URL, PreserveHost: true},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/x", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	// PreserveHost: upstream should see the client's Host (the fakeserver listener address)
	want := strings.TrimPrefix(srv.URL, "http://")
	if sawHost != want {
		t.Errorf("preserveHost: upstream saw Host=%q want %q", sawHost, want)
	}
}

func TestProxy_BodyLimit_ExactlyAtLimit(t *testing.T) {
	upstream := echoUpstream(t)
	defer upstream.Close()

	cfg := &config.Config{
		Routes: []config.Route{{
			Method: []string{"POST"}, Path: "/x",
			Proxy:  &config.ProxyConfig{Target: upstream.URL, BodyLimit: "16B"},
		}},
	}
	srv := startProxyServer(t, cfg)
	defer srv.Close()

	// exactly 16 bytes — must pass through (not 413)
	body := strings.Repeat("a", 16)
	resp, err := http.Post(srv.URL+"/x", "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("exact-limit body should be forwarded; got status %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.HasSuffix(string(b), ":"+body) {
		t.Errorf("upstream received body=%q (truncated?)", string(b))
	}
}

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		err  bool
	}{
		{"16", 16, false},
		{"16B", 16, false},
		{"1KB", 1000, false},
		{"1KiB", 1024, false},
		{"10MiB", 10 * 1024 * 1024, false},
		{"16b", 0, true},  // lowercase rejected
		{"16XB", 0, true}, // unknown suffix
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseByteSize(tt.in)
			if (err != nil) != tt.err {
				t.Errorf("err=%v want err=%v", err, tt.err)
			}
			if !tt.err && got != tt.want {
				t.Errorf("got %d want %d", got, tt.want)
			}
		})
	}
}
