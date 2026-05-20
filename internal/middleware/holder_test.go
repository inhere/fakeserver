package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestHolder_InitialNilReturns503(t *testing.T) {
	h := NewHolder()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 503 {
		t.Errorf("uninitialized holder should return 503; got %d", rec.Code)
	}
}

func TestHolder_SwapServesNewHandler(t *testing.T) {
	h := NewHolder()
	h.Swap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("v1"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "v1" {
		t.Errorf("first: body=%q", rec.Body.String())
	}

	h.Swap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("v2"))
	}))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "v2" {
		t.Errorf("after swap: body=%q", rec.Body.String())
	}
}

// TestHolder_ConcurrentSwapAndServe 锁定并发 Swap 与 ServeHTTP 不 race。
// 必须用 `go test -race` 验证（CI 应启用）。
func TestHolder_ConcurrentSwapAndServe(t *testing.T) {
	h := NewHolder()
	h.Swap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			}
		}()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			h.Swap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			}))
		}(i)
	}
	wg.Wait()
}
