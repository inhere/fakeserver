package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverer_PanicProducesJSON500(t *testing.T) {
	var stderr bytes.Buffer
	log.SetOutput(&stderr)
	defer log.SetOutput(io.Discard)

	bad := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	Recoverer(bad).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status=%d want 500", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("CT=%q want application/json prefix", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v (raw=%s)", err, rec.Body.String())
	}
	if body["error"] != "internal error" {
		t.Errorf("body.error=%v want 'internal error'", body["error"])
	}
	if body["panic"] != "kaboom" {
		t.Errorf("body.panic=%v want 'kaboom'", body["panic"])
	}
	if !strings.Contains(stderr.String(), "kaboom") {
		t.Errorf("stderr should contain panic msg; got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "goroutine") {
		t.Errorf("stderr should contain stack trace; got %q", stderr.String())
	}
}

func TestRecoverer_NormalRequestPassesThrough(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		_, _ = w.Write([]byte("ok"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	Recoverer(ok).ServeHTTP(rec, req)

	if rec.Code != 202 || rec.Body.String() != "ok" {
		t.Errorf("normal request not passed through: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestRecoverer_SubsequentRequestsOK 锁定 "panic 不杀进程"：同一个 middleware
// 实例服务两次请求，第一次 panic，第二次必须 200。
func TestRecoverer_SubsequentRequestsOK(t *testing.T) {
	log.SetOutput(io.Discard)
	count := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			panic("first call panics")
		}
		w.WriteHeader(200)
	})
	wrap := Recoverer(h)

	// First: 500 from recover
	rec1 := httptest.NewRecorder()
	wrap.ServeHTTP(rec1, httptest.NewRequest("GET", "/", nil))
	if rec1.Code != 500 {
		t.Errorf("first call: code=%d want 500", rec1.Code)
	}

	// Second: 200
	rec2 := httptest.NewRecorder()
	wrap.ServeHTTP(rec2, httptest.NewRequest("GET", "/", nil))
	if rec2.Code != 200 {
		t.Errorf("second call: code=%d want 200", rec2.Code)
	}
}
