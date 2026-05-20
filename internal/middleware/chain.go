package middleware

import "net/http"

// Chain composes mws around handler, onion-style: the FIRST middleware in
// mws is the outermost layer.
//
//	Chain(h, A, B, C) ≡ A(B(C(h)))
//
// Empty mws returns handler unchanged.
func Chain(handler http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	// Iterate in reverse so the first middleware ends up outermost.
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}
