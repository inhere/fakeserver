package recorder

import (
	"encoding/json"
	"os"
	"sync"
)

// HistoryWriter appends recorder entries to a JSONL file — one JSON object per
// line, same shape as the in-memory Entry (design §11.4 extension: `server.
// historyFile`). The file is opened in append mode at startup so restarts keep
// previous runs' lines.
//
// Write failures are advisory: the caller logs a warning and keeps serving.
// A nil *HistoryWriter is a no-op sink.
type HistoryWriter struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// OpenHistoryFile opens (creating if needed) path for appending.
func OpenHistoryFile(path string) (*HistoryWriter, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &HistoryWriter{f: f, enc: json.NewEncoder(f)}, nil
}

// Write appends one entry as a single JSON line. Safe for concurrent use.
func (w *HistoryWriter) Write(e Entry) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.enc.Encode(e)
}

// Close flushes and closes the underlying file. Safe on a nil writer.
func (w *HistoryWriter) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}
