// Package middleware implements fakeserver's cross-cutting HTTP middleware.
// design §5 lists the full middleware roster — Phase 5 introduces all four
// of them (recoverer / logger / cors / bodylimit) plus Chain composition
// helper and an atomic-swap Holder for hot reload.
package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

// Recoverer recovers from any downstream panic, writes a 500 + JSON error
// body to the client, and logs the panic + stack to stderr. The wrapped
// handler is otherwise unmodified.
//
// design §6: a panic must NOT kill the server — fakeserver is interactive
// developer tooling, not a hardened production service. Subsequent requests
// continue to be served from the same process.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[fakeserver] panic in %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				// Best-effort: if downstream already wrote headers, this is
				// a no-op on the wire (ResponseWriter rejects double WriteHeader).
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": "internal error",
					"route": r.Method + " " + r.URL.Path,
					"panic": panicMsg(rec),
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// panicMsg coerces recover()'s any-typed result into a string for the
// response body. Most code panics with a string or an error; everything
// else falls back to a %v dump.
func panicMsg(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return fmt.Sprintf("%v", v)
	}
}
