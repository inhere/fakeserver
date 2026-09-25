package recorder

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenHistoryFile_AppendsWithoutTruncatingAcrossRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")

	first, err := OpenHistoryFile(path)
	if err != nil {
		t.Fatalf("OpenHistoryFile: %v", err)
	}
	if err := first.Write(Entry{TS: time.Unix(1, 0).UTC(), Method: "GET", Path: "/a", Status: 200}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Second "run": must append, not overwrite.
	second, err := OpenHistoryFile(path)
	if err != nil {
		t.Fatalf("OpenHistoryFile (reopen): %v", err)
	}
	if err := second.Write(Entry{TS: time.Unix(2, 0).UTC(), Method: "POST", Path: "/b", Status: 500}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readHistoryLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("want 2 appended lines, got %d (%v)", len(lines), lines)
	}

	var entry Entry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("line 0 is not a JSON entry (%q): %v", lines[0], err)
	}
	if entry.Method != "GET" || entry.Path != "/a" || entry.Status != 200 {
		t.Fatalf("line 0 round-trip mismatch: %+v", entry)
	}
	if !entry.TS.Equal(time.Unix(1, 0).UTC()) {
		t.Fatalf("line 0 ts=%v", entry.TS)
	}
}

func TestHistoryWriter_RecordsRouteCaseAndWhenError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	w, err := OpenHistoryFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	routeIndex, caseIndex := 3, 1
	if err := w.Write(Entry{
		TS:          time.Unix(100, 0).UTC(),
		Method:      "POST",
		Path:        "/tasks",
		Status:      500,
		DurationMs:  1.5,
		ClientIP:    "127.0.0.1",
		RouteIndex:  &routeIndex,
		CaseIndex:   &caseIndex,
		RouteMode:   "cases",
		CaseName:    "boom",
		WhenError:   "boom: oops",
		ProxyTarget: "http://upstream.test",
	}); err != nil {
		t.Fatal(err)
	}

	lines := readHistoryLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %v", lines)
	}
	var got Entry
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatal(err)
	}
	if got.RouteIndex == nil || *got.RouteIndex != 3 || got.CaseIndex == nil || *got.CaseIndex != 1 {
		t.Fatalf("route/case index lost: %+v", got)
	}
	if got.CaseName != "boom" || got.WhenError != "boom: oops" || got.ProxyTarget != "http://upstream.test" {
		t.Fatalf("match metadata lost: %+v", got)
	}
}

func TestHistoryWriter_NilSafe(t *testing.T) {
	var w *HistoryWriter
	if err := w.Write(Entry{Method: "GET"}); err != nil {
		t.Fatalf("nil Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
}

func readHistoryLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil
	}
	var out []string
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}
