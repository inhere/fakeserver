package middleware

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Holder is a swap-able http.Handler container backed by atomic.Pointer.
// Used by the hot-reload watcher: when config changes, the watcher
// constructs a brand-new (middleware-chain + router) http.Handler and
// hands it to Swap. In-flight requests finish on the previous handler;
// new requests get the new one.
//
// A zero-value Holder (NewHolder return value, before any Swap) responds
// with 503 Service Unavailable. This is a defensive default — the server
// should always Swap an initial handler before starting to listen.
type Holder struct {
	inner atomic.Pointer[http.Handler]
}

// NewHolder returns an empty Holder. Call Swap before listening.
func NewHolder() *Holder {
	return &Holder{}
}

// Swap atomically replaces the current handler. Safe for concurrent calls.
func (h *Holder) Swap(handler http.Handler) {
	h.inner.Store(&handler)
}

// ServeHTTP dispatches to the current handler, or returns 503 if Swap has
// not yet been called.
func (h *Holder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ptr := h.inner.Load()
	if ptr == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   "server not ready",
			"message": "handler not yet initialized",
		})
		return
	}
	(*ptr).ServeHTTP(w, r)
}
