package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestChain_OnionOrder 锁定洋葱模型：第一个 middleware 是最外层，
// 请求顺序进入，响应反向退出。
func TestChain_OnionOrder(t *testing.T) {
	var trace []string
	mwName := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				trace = append(trace, "→"+name)
				next.ServeHTTP(w, r)
				trace = append(trace, name+"←")
			})
		}
	}
	core := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace = append(trace, "*core*")
		w.WriteHeader(200)
	})

	h := Chain(core, mwName("A"), mwName("B"), mwName("C"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	got := strings.Join(trace, " ")
	want := "→A →B →C *core* C← B← A←"
	if got != want {
		t.Errorf("order:\n  got  %q\n  want %q", got, want)
	}
}

func TestChain_NoMiddlewareReturnsCore(t *testing.T) {
	core := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	h := Chain(core)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 204 {
		t.Errorf("status=%d want 204", rec.Code)
	}
}
