package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// BodyLimit returns a middleware that rejects requests whose body exceeds
// max bytes with a 413 + JSON error. max <= 0 disables the check entirely
// (the middleware degrades to a no-op pass-through).
//
// Implementation note: we read the body up-front into a bounded buffer
// rather than using http.MaxBytesReader, because the latter surfaces its
// error during handler-side body reads — past the point where this
// middleware can cleanly return a 413. The trade-off is that bodies up to
// max bytes are buffered fully; for fakeserver's dev-tooling use case this
// is acceptable (max is typically 1MiB).
func BodyLimit(max int64) func(http.Handler) http.Handler {
	if max <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}
			buf := make([]byte, max+1)
			n, err := readUpTo(r.Body, buf)
			if err != nil {
				writeBodyLimitError(w, max)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(buf[:n]))
			r.ContentLength = int64(n)
			next.ServeHTTP(w, r)
		})
	}
}

// readUpTo reads from r into buf. Returns (n, nil) on EOF within buf;
// returns (n, err) when there's MORE data beyond buf (i.e. body >= len(buf)).
//
// buf must be sized max+1 by the caller so that filling buf exactly means
// "body is at least max+1 bytes" — over the limit.
func readUpTo(r io.ReadCloser, buf []byte) (int, error) {
	defer r.Close()
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err == io.EOF {
			if total >= len(buf) {
				// buf is max+1; filling it exactly means body == max+1 — over limit.
				return total, errors.New("body exceeds limit")
			}
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
	// Loop exited because total == len(buf) == max+1 — buffer filled, over limit.
	return total, errors.New("body exceeds limit")
}

func writeBodyLimitError(w http.ResponseWriter, max int64) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   "request body too large",
		"limit":   max,
		"message": fmt.Sprintf("request body exceeds server.maxBodySize=%d bytes", max),
	})
}
