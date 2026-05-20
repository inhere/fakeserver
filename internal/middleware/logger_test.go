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
	Logger(&buf, false, nil)(h).ServeHTTP(
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
	Logger(&buf, true, nil)(h).ServeHTTP(
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
	Logger(&buf, false, nil)(h).ServeHTTP(
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
	Logger(io.Discard, false, nil)(h).ServeHTTP(
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
	Logger(&buf, false, nil)(h).ServeHTTP(fr, httptest.NewRequest("GET", "/x", nil))
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
	Logger(&buf, false, nil)(h).ServeHTTP(
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
	Logger(&buf, false, ring)(h).ServeHTTP(
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
	Logger(&buf, true, ring)(h).ServeHTTP(
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
