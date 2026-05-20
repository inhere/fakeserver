package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func handler200() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
}

func handler404() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
}

func TestCORS_ReflectOriginByDefault(t *testing.T) {
	mw := CORS(CORSOpts{})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("reflect origin: got %q want http://localhost:5173", got)
	}
}

func TestCORS_AllowedOriginExplicit(t *testing.T) {
	mw := CORS(CORSOpts{Origins: []string{"http://allowed.example"}})(handler200())
	// allowed
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://allowed.example")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://allowed.example" {
		t.Errorf("allowed: got %q", got)
	}
	// disallowed
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://evil.example")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin should not be reflected; got %q", got)
	}
}

func TestCORS_AdminPathExempt(t *testing.T) {
	mw := CORS(CORSOpts{})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/__fakeserver/healthz", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("admin path should be CORS-exempt; got Origin header %q", got)
	}
}

// TestCORS_OptionsHandledByRoute 用户路由自己处理了 OPTIONS（返回 200）→
// CORS 不改写响应，只追加 header。
func TestCORS_OptionsHandledByRoute(t *testing.T) {
	mw := CORS(CORSOpts{})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("status=%d want 200 (route handled OPTIONS)", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("CORS header missing on route-handled OPTIONS: %q", got)
	}
}

// TestCORS_OptionsNotMatched 用户路由没处理 OPTIONS（返回 404）→ CORS
// 改写为 204 + preflight headers（Access-Control-Allow-Methods/Headers）。
func TestCORS_OptionsNotMatched(t *testing.T) {
	mw := CORS(CORSOpts{
		Methods: []string{"GET", "POST"},
		Headers: []string{"Content-Type", "X-Custom"},
	})(handler404())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Errorf("status=%d want 204 (preflight short-circuit)", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Errorf("Allow-Methods missing on preflight: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Errorf("Allow-Headers missing on preflight: %q", got)
	}
}

// TestCORS_AllowCredentials 当配置允许凭据时，输出 ACAC=true。
func TestCORS_AllowCredentials(t *testing.T) {
	mw := CORS(CORSOpts{AllowCredentials: true})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("ACAC=%q want true", got)
	}
}

// TestCORS_OptionsNoOrigin — OPTIONS request without Origin header (e.g.
// curl without -H "Origin:"). Should NOT 204-rewrite to preflight: there's
// no cross-origin context; route's response should pass through. If the
// route returns 404, we still get 404 (no CORS rewrite because there's no
// preflight semantic).
//
// Note: this captures the current implementation's behavior — no Origin
// means no CORS injection. The 404→204 rewrite logic stays the same; the
// preflight headers go on regardless of Origin presence since they apply
// to the preflight protocol itself.
func TestCORS_OptionsNoOrigin(t *testing.T) {
	mw := CORS(CORSOpts{})(handler200())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/x", nil) // no Origin
	mw.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("status=%d want 200 (route handled OPTIONS, no Origin → no CORS injection but route still wins)", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("no Origin → no ACAO; got %q", got)
	}
}

// TestCORS_PreflightHasMaxAge confirms the Max-Age header default.
func TestCORS_PreflightHasMaxAge(t *testing.T) {
	mw := CORS(CORSOpts{})(handler404())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	mw.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Errorf("status=%d want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got == "" {
		t.Errorf("Access-Control-Max-Age missing on preflight")
	}
}
