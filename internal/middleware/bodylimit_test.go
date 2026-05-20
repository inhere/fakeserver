package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimit_AllowsUnderLimit(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	mw := BodyLimit(100)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader("short"))
	mw.ServeHTTP(rec, req)
	if !called {
		t.Error("handler should have been called")
	}
	if rec.Code != 200 {
		t.Errorf("status=%d want 200", rec.Code)
	}
}

func TestBodyLimit_RejectsOverLimit(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := BodyLimit(10)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 100)))
	mw.ServeHTTP(rec, req)
	if called {
		t.Error("handler should NOT have been called on oversized body")
	}
	if rec.Code != 413 {
		t.Errorf("status=%d want 413", rec.Code)
	}
}

func TestBodyLimit_ExactlyAtLimit(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	mw := BodyLimit(16)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 16)))
	mw.ServeHTTP(rec, req)
	if !called {
		t.Error("exact-limit body should pass through")
	}
	if rec.Code != 200 {
		t.Errorf("status=%d want 200", rec.Code)
	}
}

func TestBodyLimit_ZeroLimitMeansNoLimit(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	mw := BodyLimit(0)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 10000)))
	mw.ServeHTTP(rec, req)
	if !called {
		t.Error("0 limit should disable check")
	}
}

func TestBodyLimit_NoBodyOk(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	mw := BodyLimit(10)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	mw.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("status=%d want 200 (no body, no rejection)", rec.Code)
	}
}

// TestBodyLimit_OneOverLimit 锁定 max+1 字节边界 bug：body 恰好为 limit+1
// 字节时必须返回 413，否则限制可被 off-by-one 绕过。
func TestBodyLimit_OneOverLimit(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := BodyLimit(16)(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(strings.Repeat("a", 17)))
	mw.ServeHTTP(rec, req)
	if called {
		t.Error("handler should NOT have been called on body=limit+1")
	}
	if rec.Code != 413 {
		t.Errorf("status=%d want 413 (body=17 must exceed limit=16)", rec.Code)
	}
}
