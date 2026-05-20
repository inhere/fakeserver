package middleware

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/inhere/fakeserver/internal/recorder"
)

// Logger returns a middleware that writes one access-log line per request
// to out. Format:
//
//	2026-05-20T15:23:45.123Z GET /users 201 3.2ms
//
// When quiet is true and ring is nil, the middleware degrades to a
// transparent pass-through. When ring is non-nil, every request also
// produces a recorder.Entry appended to ring (v0.4 Phase 1, design §11.4).
// The middleware never logs to anything other than out, so callers can
// route logs to a file, stderr, or any io.Writer without global state.
func Logger(out io.Writer, quiet bool, ring *recorder.Ring) func(http.Handler) http.Handler {
	if (quiet || out == nil) && ring == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			lr := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(lr, r)
			dur := time.Since(start)
			if !quiet && out != nil {
				fmt.Fprintf(out, "%s %s %s %d %s\n",
					start.UTC().Format("2006-01-02T15:04:05.000Z"),
					r.Method, r.URL.Path, lr.status, dur,
				)
			}
			if ring != nil {
				ring.Append(recorder.Entry{
					TS:         start.UTC(),
					Method:     r.Method,
					Path:       r.URL.Path,
					Status:     lr.status,
					DurationMs: float64(dur.Microseconds()) / 1000.0,
					ClientIP:   clientIP(r),
				})
			}
		})
	}
}

// clientIP 从 r.RemoteAddr 提取 host 部分（去掉端口）。失败时返回原值。
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// loggingResponseWriter captures the response status code for the access
// log line. Status defaults to 200 — matching net/http's implicit behavior
// when handlers write a body without calling WriteHeader first.
type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	if !lw.wroteHeader {
		lw.status = code
		lw.wroteHeader = true
	}
	lw.ResponseWriter.WriteHeader(code)
}

func (lw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !lw.wroteHeader {
		lw.wroteHeader = true // implicit 200
	}
	return lw.ResponseWriter.Write(b)
}

// Flush passes through to the underlying ResponseWriter when it implements
// http.Flusher. This matters for streaming/chunked responses (proxy upstream
// responses, SSE handlers): without this method, the embedded
// ResponseWriter's Flusher would be shadowed.
func (lw *loggingResponseWriter) Flush() {
	if f, ok := lw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack passes through to the underlying ResponseWriter when it implements
// http.Hijacker. Reserved for WebSocket upgrade handlers. Returns an
// error if the underlying writer doesn't support hijacking.
func (lw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := lw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}
