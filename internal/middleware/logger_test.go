package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/recorder"
)

func TestLogger_FormatsAccessLine(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("ok"))
	})
	Logger(&buf, LoggerOptions{}, nil)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("POST", "/users", nil),
	)
	line := buf.String()
	if !strings.Contains(line, "POST") {
		t.Errorf("missing method: %q", line)
	}
	if !strings.Contains(line, "/users") {
		t.Errorf("missing path: %q", line)
	}
	if !strings.Contains(line, "201") {
		t.Errorf("missing status: %q", line)
	}
	if !regexp.MustCompile(`\d+(\.\d+)?(ns|µs|us|ms|s)`).MatchString(line) {
		t.Errorf("missing duration: %q", line)
	}
}

func TestLogger_QuietSuppresses(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	Logger(&buf, LoggerOptions{Quiet: true}, nil)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)
	if buf.Len() != 0 {
		t.Errorf("quiet=true should suppress; got %q", buf.String())
	}
}

func TestLogger_StatusDefaultsTo200(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hi"))
	})
	Logger(&buf, LoggerOptions{}, nil)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)
	if !strings.Contains(buf.String(), "200") {
		t.Errorf("default status 200 not logged: %q", buf.String())
	}
}

func TestLogger_NilOutDoesNotPanic(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	Logger(io.Discard, LoggerOptions{}, nil)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)
}

// flushableRecorder is an http.ResponseWriter that also implements
// http.Flusher, used to verify loggingResponseWriter forwards Flush calls.
type flushableRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (fr *flushableRecorder) Flush() { fr.flushed++ }

func TestLogger_FlushPassesThrough(t *testing.T) {
	var buf bytes.Buffer
	fr := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
			f.Flush()
		} else {
			t.Error("loggingResponseWriter should implement http.Flusher")
		}
	})
	Logger(&buf, LoggerOptions{}, nil)(h).ServeHTTP(fr, httptest.NewRequest("GET", "/x", nil))
	if fr.flushed != 2 {
		t.Errorf("Flush() not propagated to inner: got %d calls want 2", fr.flushed)
	}
}

func TestLogger_5xxStatusLogged(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte("svc unavail"))
	})
	Logger(&buf, LoggerOptions{}, nil)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)
	if !strings.Contains(buf.String(), "503") {
		t.Errorf("5xx not in log: %q", buf.String())
	}
}

// TestLogger_WithRing_AppendsEntry 验证 v0.4 Phase 1：Logger 在 ring != nil
// 时把每个请求落 ring；Path/Method/Status 字段正确，DurationMs 非负。
func TestLogger_WithRing_AppendsEntry(t *testing.T) {
	var buf bytes.Buffer
	ring := recorder.New(10)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("created"))
	})
	Logger(&buf, LoggerOptions{}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("POST", "/users", nil),
	)
	got := ring.Snapshot()
	if len(got) != 1 {
		t.Fatalf("ring has %d entries; want 1", len(got))
	}
	e := got[0]
	if e.Method != "POST" || e.Path != "/users" || e.Status != 201 {
		t.Errorf("entry mismatch: %+v", e)
	}
	if e.DurationMs < 0 {
		t.Errorf("DurationMs should be >= 0; got %f", e.DurationMs)
	}
}

// TestLogger_QuietWithRing_StillAppends 验证 quiet=true + ring != nil
// 不写日志但仍录入 ring（mock 优先 + UI 可用）。
func TestLogger_QuietWithRing_StillAppends(t *testing.T) {
	var buf bytes.Buffer
	ring := recorder.New(10)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	Logger(&buf, LoggerOptions{Quiet: true}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/p", nil),
	)
	if buf.Len() != 0 {
		t.Errorf("quiet mode should write nothing; got %q", buf.String())
	}
	if got := ring.Snapshot(); len(got) != 1 {
		t.Errorf("ring should still record %d entries; want 1", len(got))
	}
}

func TestLogger_AppendsTraceFields(t *testing.T) {
	ring := recorder.New(10)
	routeIndex := 2
	caseIndex := 1
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.SetRouteMatch(r.Context(), recorder.RequestTrace{
			RouteIndex:  &routeIndex,
			CaseIndex:   &caseIndex,
			RouteMode:   "cases",
			RouteSource: "routes/users.json5",
			ProxyTarget: "https://example.test",
		})
		w.WriteHeader(202)
	})

	Logger(io.Discard, LoggerOptions{Quiet: true}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/trace", nil),
	)

	e := ring.Snapshot()[0]
	if e.RouteIndex == nil || *e.RouteIndex != routeIndex {
		t.Fatalf("RouteIndex=%v, want %d", e.RouteIndex, routeIndex)
	}
	if e.CaseIndex == nil || *e.CaseIndex != caseIndex {
		t.Fatalf("CaseIndex=%v, want %d", e.CaseIndex, caseIndex)
	}
	if e.RouteMode != "cases" || e.RouteSource != "routes/users.json5" || e.ProxyTarget != "https://example.test" {
		t.Fatalf("trace fields not copied: %+v", e)
	}
}

func TestLogger_AppendsScenarioTraceFields(t *testing.T) {
	ring := recorder.New(10)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.SetRouteMatch(r.Context(), recorder.RequestTrace{
			Scenario:       "emptyUsers",
			CaseName:       "empty",
			OverrideSource: "scenario",
		})
		w.WriteHeader(http.StatusOK)
	})
	wrapped := Logger(io.Discard, LoggerOptions{Quiet: true}, ring)(handler)
	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/users", nil))

	entries := ring.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("entries=%d", len(entries))
	}
	if entries[0].Scenario != "emptyUsers" || entries[0].CaseName != "empty" || entries[0].OverrideSource != "scenario" {
		t.Fatalf("scenario trace not recorded: %#v", entries[0])
	}
}

func TestLogger_CapturesTextRequestAndResponseBody(t *testing.T) {
	ring := recorder.New(10)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	Logger(io.Discard, LoggerOptions{Quiet: true, CaptureEnabled: true, CaptureMaxBytes: 64}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("POST", "/capture", strings.NewReader(`{"name":"alice"}`)),
	)

	e := ring.Snapshot()[0]
	if e.Request.Body != `{"name":"alice"}` {
		t.Fatalf("request body=%q", e.Request.Body)
	}
	if e.Response.Body != `{"ok":true}` {
		t.Fatalf("response body=%q", e.Response.Body)
	}
}

func TestLogger_RedactsSensitiveHeaders(t *testing.T) {
	ring := recorder.New(10)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	req := httptest.NewRequest("GET", "/redact", nil)
	req.Header.Set("Authorization", "Bearer abc")

	Logger(io.Discard, LoggerOptions{Quiet: true, CaptureEnabled: true, CaptureMaxBytes: 64, RedactKeys: []string{"authorization"}}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		req,
	)

	if got := ring.Snapshot()[0].Request.Headers["Authorization"]; got != "***" {
		t.Fatalf("Authorization header=%q, want ***", got)
	}
}

func TestLogger_RedactsSensitiveJSONBody(t *testing.T) {
	ring := recorder.New(10)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"server-secret","ok":true}`))
	})
	req := httptest.NewRequest("POST", "/redact-body", strings.NewReader(`{"password":"abc","name":"alice"}`))
	req.Header.Set("Content-Type", "application/json")

	Logger(io.Discard, LoggerOptions{Quiet: true, CaptureEnabled: true, CaptureMaxBytes: 256, RedactKeys: []string{"password", "token"}}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		req,
	)

	e := ring.Snapshot()[0]
	if strings.Contains(e.Request.Body, "abc") || !strings.Contains(e.Request.Body, `"password":"***"`) {
		t.Fatalf("request body not redacted: %s", e.Request.Body)
	}
	if strings.Contains(e.Response.Body, "server-secret") || !strings.Contains(e.Response.Body, `"token":"***"`) {
		t.Fatalf("response body not redacted: %s", e.Response.Body)
	}
}

func TestLogger_TruncatesBody(t *testing.T) {
	ring := recorder.New(10)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	})

	Logger(io.Discard, LoggerOptions{Quiet: true, CaptureEnabled: true, CaptureMaxBytes: 8}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/truncate", nil),
	)

	resp := ring.Snapshot()[0].Response
	if resp.Body != "hello wo" || !resp.Truncated {
		t.Fatalf("response capture=%+v, want truncated first 8 bytes", resp)
	}
}

func TestLogger_DoesNotRenderBinaryBody(t *testing.T) {
	ring := recorder.New(10)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0, 1, 2, 3})
	})

	Logger(io.Discard, LoggerOptions{Quiet: true, CaptureEnabled: true, CaptureMaxBytes: 64}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/bin", nil),
	)

	resp := ring.Snapshot()[0].Response
	if !resp.Binary || resp.Body != "" {
		t.Fatalf("response capture=%+v, want binary without body", resp)
	}
}
