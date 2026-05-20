package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestLogger_FormatsAccessLine(t *testing.T) {
	var buf bytes.Buffer
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("ok"))
	})
	Logger(&buf, false)(h).ServeHTTP(
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
	Logger(&buf, true)(h).ServeHTTP(
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
	Logger(&buf, false)(h).ServeHTTP(
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
	Logger(io.Discard, false)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)
}
