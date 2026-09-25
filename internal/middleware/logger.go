package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/inhere/fakeserver/internal/recorder"
)

// LoggerOptions controls access logging and recorder capture.
type LoggerOptions struct {
	Quiet           bool
	CaptureEnabled  bool
	CaptureMaxBytes int64
	RedactKeys      []string
}

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
func Logger(out io.Writer, opts LoggerOptions, ring *recorder.Ring) func(http.Handler) http.Handler {
	if (opts.Quiet || out == nil) && ring == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx, trace := recorder.WithRequestTrace(r.Context())
			r = r.WithContext(ctx)
			reqCap, body := captureRequest(r, opts)
			if body != nil {
				r.Body = body
			}
			lr := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK, captureMaxBytes: opts.CaptureMaxBytes}
			next.ServeHTTP(lr, r)
			dur := time.Since(start)
			if !opts.Quiet && out != nil {
				line := fmt.Sprintf("%s %s %s %d %s",
					start.UTC().Format("2006-01-02T15:04:05.000Z"),
					r.Method, r.URL.Path, lr.status, dur,
				)
				if trace.WhenError != "" {
					line += " when_error=" + firstLine(trace.WhenError)
				}
				fmt.Fprintln(out, line)
			}
			if ring != nil {
				ring.Append(recorder.Entry{
					TS:             start.UTC(),
					Method:         r.Method,
					Path:           r.URL.Path,
					Status:         lr.status,
					DurationMs:     float64(dur.Microseconds()) / 1000.0,
					ClientIP:       clientIP(r),
					RouteIndex:     trace.RouteIndex,
					CaseIndex:      trace.CaseIndex,
					RouteMode:      trace.RouteMode,
					RouteSource:    trace.RouteSource,
					ProxyTarget:    trace.ProxyTarget,
					Scenario:       trace.Scenario,
					CaseName:       trace.CaseName,
					OverrideSource: trace.OverrideSource,
					WhenError:      trace.WhenError,
					Request:        finalizeCapture(*reqCap, r.Header.Get("Content-Type"), opts),
					Response:       finalizeCapture(lr.capture(), lr.Header().Get("Content-Type"), opts),
				})
			}
		})
	}
}

// firstLine collapses a possibly multi-line diagnostic (expr errors embed a
// source excerpt) into its first line so the access log keeps one line per
// request. The full text stays in the history entry and error bodies.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
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
	status          int
	wroteHeader     bool
	captureMaxBytes int64
	buf             bytes.Buffer
	bodySize        int64
	truncated       bool
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
	lw.captureBytes(b)
	return lw.ResponseWriter.Write(b)
}

func (lw *loggingResponseWriter) captureBytes(b []byte) {
	lw.bodySize += int64(len(b))
	if lw.captureMaxBytes <= 0 {
		return
	}
	remain := int(lw.captureMaxBytes) - lw.buf.Len()
	if remain <= 0 {
		if len(b) > 0 {
			lw.truncated = true
		}
		return
	}
	if len(b) > remain {
		_, _ = lw.buf.Write(b[:remain])
		lw.truncated = true
		return
	}
	_, _ = lw.buf.Write(b)
}

func (lw *loggingResponseWriter) capture() rawCapture {
	return rawCapture{
		Headers:   lw.Header(),
		Body:      lw.buf.Bytes(),
		BodySize:  lw.bodySize,
		Truncated: lw.truncated,
	}
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

type rawCapture struct {
	Headers   http.Header
	Body      []byte
	BodySize  int64
	Truncated bool
	Omitted   []string
}

type captureReadCloser struct {
	io.ReadCloser
	raw *rawCapture
	max int64
}

func (rc *captureReadCloser) Read(p []byte) (int, error) {
	n, err := rc.ReadCloser.Read(p)
	if n > 0 {
		rc.raw.BodySize += int64(n)
		if rc.max > 0 {
			remain := int(rc.max) - len(rc.raw.Body)
			if remain <= 0 {
				rc.raw.Truncated = true
			} else if n > remain {
				rc.raw.Body = append(rc.raw.Body, p[:remain]...)
				rc.raw.Truncated = true
			} else {
				rc.raw.Body = append(rc.raw.Body, p[:n]...)
			}
		}
	}
	return n, err
}

func captureRequest(r *http.Request, opts LoggerOptions) (*rawCapture, io.ReadCloser) {
	raw := &rawCapture{Headers: r.Header}
	if !opts.CaptureEnabled {
		raw.Omitted = append(raw.Omitted, "capture disabled")
		return raw, nil
	}
	if r.Body == nil {
		return raw, nil
	}
	return raw, &captureReadCloser{ReadCloser: r.Body, raw: raw, max: opts.CaptureMaxBytes}
}

func finalizeCapture(raw rawCapture, contentType string, opts LoggerOptions) recorder.Capture {
	cap := recorder.Capture{
		Headers:     redactHeaders(raw.Headers, opts.RedactKeys),
		ContentType: contentType,
		BodySize:    raw.BodySize,
		Truncated:   raw.Truncated,
		Omitted:     raw.Omitted,
	}
	if !opts.CaptureEnabled {
		return cap
	}
	if len(raw.Body) == 0 {
		return cap
	}
	if !isDisplayableBody(contentType, raw.Body) {
		cap.Binary = true
		cap.Omitted = append(cap.Omitted, "binary body")
		return cap
	}
	body := string(raw.Body)
	if strings.Contains(strings.ToLower(contentType), "json") {
		body = redactJSONBody(body, opts.RedactKeys)
	}
	cap.Body = body
	return cap
}

func redactHeaders(h http.Header, keys []string) map[string]string {
	out := make(map[string]string, len(h))
	for k, values := range h {
		if isSensitiveName(k, keys) {
			out[k] = "***"
			continue
		}
		out[k] = strings.Join(values, ", ")
	}
	return out
}

func isSensitiveName(name string, keys []string) bool {
	lower := strings.ToLower(name)
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" && strings.Contains(lower, key) {
			return true
		}
	}
	return false
}

func isDisplayableBody(contentType string, body []byte) bool {
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "json") ||
		strings.HasPrefix(lower, "text/") ||
		strings.Contains(lower, "javascript") ||
		strings.Contains(lower, "xml") ||
		strings.Contains(lower, "x-www-form-urlencoded") {
		return true
	}
	return contentType == "" && utf8.Valid(body)
}

func redactJSONBody(body string, keys []string) string {
	var value any
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		return body
	}
	redactJSONValue(value, keys)
	data, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return string(data)
}

func redactJSONValue(value any, keys []string) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if isSensitiveName(key, keys) {
				v[key] = "***"
				continue
			}
			redactJSONValue(child, keys)
		}
	case []any:
		for _, child := range v {
			redactJSONValue(child, keys)
		}
	}
}
