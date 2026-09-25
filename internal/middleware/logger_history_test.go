package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/fakeserver/internal/recorder"
)

// openHistory opens a JSONL writer in a temp dir and returns it with a reader
// that decodes the lines written so far.
func openHistory(t *testing.T) (*recorder.HistoryWriter, func() []recorder.Entry) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.jsonl")
	w, err := recorder.OpenHistoryFile(path)
	if err != nil {
		t.Fatalf("OpenHistoryFile: %v", err)
	}
	read := func() []recorder.Entry {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read history: %v", err)
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			return nil
		}
		var out []recorder.Entry
		for _, line := range strings.Split(text, "\n") {
			var e recorder.Entry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				t.Fatalf("history line is not JSON (%q): %v", line, err)
			}
			out = append(out, e)
		}
		return out
	}
	return w, read
}

// TestLogger_HistoryFileSharesEntryIDWithRing 锁定「文件里一行 = 内存 history
// entry 的同一份数据」，包括 ring 分配的稳定 id。
func TestLogger_HistoryFileSharesEntryIDWithRing(t *testing.T) {
	hw, lines := openHistory(t)
	defer hw.Close()
	ring := recorder.New(4)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	Logger(io.Discard, LoggerOptions{Quiet: true, HistoryFile: hw}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/tasks", nil),
	)

	entries := ring.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("ring entries=%d", len(entries))
	}
	got := lines()
	if len(got) != 1 {
		t.Fatalf("history file lines=%d want 1", len(got))
	}
	if got[0].ID != entries[0].ID || got[0].ID == 0 {
		t.Fatalf("file entry id=%d, ring entry id=%d", got[0].ID, entries[0].ID)
	}
	if got[0].Method != "GET" || got[0].Path != "/tasks" {
		t.Fatalf("file entry mismatch: %+v", got[0])
	}
}

// TestLogger_HistoryFileKeepsServingWhenWriteFails 写失败只告警不影响服务。
func TestLogger_HistoryFileKeepsServingWhenWriteFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	hw, err := recorder.OpenHistoryFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Close the underlying file so the append fails.
	if err := hw.Close(); err != nil {
		t.Fatal(err)
	}

	var warn strings.Builder
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	rec := httptest.NewRecorder()
	Logger(&warn, LoggerOptions{HistoryFile: hw}, nil)(h).ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("request must still be served, got %d", rec.Code)
	}
	if !strings.Contains(warn.String(), "history") {
		t.Fatalf("expected a history write warning, got %q", warn.String())
	}
}

// TestLogger_HistoryBodyOff_StripsBodiesFromFile 默认（未开 historyBody）文件里
// 不落请求/响应体，即使 capture 已开启。
func TestLogger_HistoryBodyOff_StripsBodiesFromFile(t *testing.T) {
	hw, lines := openHistory(t)
	defer hw.Close()
	ring := recorder.New(4)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"secret-value"}`))
	})
	Logger(io.Discard, LoggerOptions{Quiet: true, CaptureEnabled: true, CaptureMaxBytes: 256, HistoryFile: hw}, ring)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("GET", "/x", nil),
	)

	if body := ring.Snapshot()[0].Response.Body; body == "" {
		t.Fatal("in-memory capture should still hold the body")
	}
	got := lines()
	if len(got) != 1 {
		t.Fatalf("history lines=%d want 1", len(got))
	}
	if got[0].Response.Body != "" {
		t.Fatalf("history file must not carry bodies without historyBody; got %q", got[0].Response.Body)
	}
}

// TestLogger_HistoryBodyOn_RecordsBodiesAndTruncates 打开 historyBody 后文件里
// 记录请求/响应体，并按上限截断（超出标 truncated）。
func TestLogger_HistoryBodyOn_RecordsBodiesAndTruncates(t *testing.T) {
	hw, lines := openHistory(t)
	defer hw.Close()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 请求体只有在被 handler 读取时才会进入 capture（与真实 mock 一致）
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte("0123456789"))
	})
	Logger(io.Discard, LoggerOptions{Quiet: true, HistoryBody: true, HistoryMaxBytes: 4, HistoryFile: hw}, nil)(h).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest("POST", "/x", strings.NewReader("abcdefg")),
	)

	got := lines()
	if len(got) != 1 {
		t.Fatalf("history lines=%d want 1", len(got))
	}
	if got[0].Request.Body != "abcd" || !got[0].Request.Truncated {
		t.Fatalf("request body capture=%+v, want abcd truncated", got[0].Request)
	}
	if got[0].Response.Body != "0123" || !got[0].Response.Truncated {
		t.Fatalf("response body capture=%+v, want 0123 truncated", got[0].Response)
	}
}

// TestLogger_RedactsCredentialHeadersByDefault 凭据类头（Authorization /
// Cookie / X-*-Key / X-*-Token）无论配置的 redactKeys 是什么都要脱敏。
func TestLogger_RedactsCredentialHeadersByDefault(t *testing.T) {
	hw, lines := openHistory(t)
	defer hw.Close()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer real-token")
	req.Header.Set("Cookie", "sid=real")
	req.Header.Set("X-Api-Key", "real-key")
	req.Header.Set("X-Edge-Token", "real-edge-token")
	req.Header.Set("X-Trace-Id", "keep-me")

	Logger(io.Discard, LoggerOptions{Quiet: true, HistoryBody: true, HistoryMaxBytes: 64, HistoryFile: hw}, nil)(h).ServeHTTP(
		httptest.NewRecorder(), req,
	)

	got := lines()
	if len(got) != 1 {
		t.Fatalf("history lines=%d want 1", len(got))
	}
	headers := got[0].Request.Headers
	for _, key := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Edge-Token"} {
		if headers[key] != "***" {
			t.Errorf("header %s=%q, want ***", key, headers[key])
		}
	}
	if headers["X-Trace-Id"] != "keep-me" {
		t.Errorf("non-credential header should pass through, got %q", headers["X-Trace-Id"])
	}
}
