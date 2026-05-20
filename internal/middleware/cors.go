package middleware

import (
	"net/http"
	"strings"
)

// CORSOpts is the configurable surface of the CORS middleware.
//
// design §5.4 conventions:
//   - Origins empty → reflect the request's Origin header (developer-friendly
//     default; matches "CORS: true" in JSON5 config)
//   - Origins non-empty → strict allowlist
//   - Methods/Headers populate the preflight Access-Control-Allow-* responses
//   - AllowCredentials controls the ACAC header
//
// SECURITY WARNING: combining AllowCredentials=true with an empty Origins
// list (reflect-mode) is INSECURE on any network-accessible deployment — any
// origin can issue credentialed requests. fakeserver is local-only dev
// tooling so this is acceptable by default, but document/audit if exposing
// the listener beyond loopback.
//
// Path-level behavior: requests to /__fakeserver/* are exempted entirely
// (no headers added, no preflight rewrite). This protects the internal
// endpoints from being usable as a CORS bypass surface from arbitrary
// origins.
type CORSOpts struct {
	Origins          []string
	Methods          []string
	Headers          []string
	AllowCredentials bool
}

const adminPrefix = "/__fakeserver/"

// CORS returns the middleware. design §5.4:
//   - non-OPTIONS: pass through, append CORS response headers
//   - OPTIONS:
//     * route handled (status != 404) → keep route response, append headers
//     * route returned 404            → rewrite to 204 + preflight headers
//   - any /__fakeserver/* path        → pass through unchanged
func CORS(opts CORSOpts) func(http.Handler) http.Handler {
	allowMethods := strings.Join(defaultIfEmpty(opts.Methods, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}), ", ")
	allowHeaders := strings.Join(defaultIfEmpty(opts.Headers, []string{"Content-Type", "Authorization"}), ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Admin endpoints get no CORS, by design
			if strings.HasPrefix(r.URL.Path, adminPrefix) {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			allowOrigin := resolveOrigin(opts.Origins, origin)

			// For non-OPTIONS: write CORS headers, then delegate.
			if r.Method != http.MethodOptions {
				if allowOrigin != "" {
					setCORSHeaders(w, allowOrigin, opts.AllowCredentials)
				}
				next.ServeHTTP(w, r)
				return
			}

			// OPTIONS path: buffer the downstream response.
			buf := &bufferedWriter{header: http.Header{}}
			next.ServeHTTP(buf, r)
			if buf.status == 404 || buf.status == 0 {
				// Route didn't handle preflight — short-circuit with 204.
				if allowOrigin != "" {
					setCORSHeaders(w, allowOrigin, opts.AllowCredentials)
				}
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", "600") // browser caches preflight for 10 min
				w.WriteHeader(http.StatusNoContent)
				return
			}
			// Route handled OPTIONS — flush its response, then add CORS headers
			// Use Del+Add to avoid duplicating single-value headers (e.g. Content-Type)
			// when the downstream handler set them explicitly.
			for k, v := range buf.header {
				w.Header().Del(k)
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
			if allowOrigin != "" {
				setCORSHeaders(w, allowOrigin, opts.AllowCredentials)
			}
			w.WriteHeader(buf.status)
			_, _ = w.Write(buf.body)
		})
	}
}

func resolveOrigin(allow []string, requested string) string {
	if requested == "" {
		return ""
	}
	if len(allow) == 0 {
		return requested // reflect-mode default
	}
	for _, a := range allow {
		if a == requested {
			return requested
		}
	}
	return ""
}

func setCORSHeaders(w http.ResponseWriter, origin string, allowCred bool) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	if allowCred {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

func defaultIfEmpty(v, def []string) []string {
	if len(v) == 0 {
		return def
	}
	return v
}

// bufferedWriter captures status/headers/body from downstream so the CORS
// middleware can decide between "let route handle OPTIONS" and "rewrite to
// 204 preflight short-circuit".
type bufferedWriter struct {
	header http.Header
	status int
	body   []byte
}

func (bw *bufferedWriter) Header() http.Header { return bw.header }
func (bw *bufferedWriter) WriteHeader(code int) { bw.status = code }
func (bw *bufferedWriter) Write(b []byte) (int, error) {
	if bw.status == 0 {
		bw.status = 200
	}
	bw.body = append(bw.body, b...)
	return len(b), nil
}
