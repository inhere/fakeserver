// Package mock turns one config.Route declaration into a live rux handler
// that renders the templated response and writes it back. design §4.5
// defines the rendering order; design §3.2 defines field-level mutex
// rules (already enforced by config.Validate before we get here).
package mock

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/config"
	"github.com/inhere/fakeserver/internal/tpl"
)

// Respond runs the single-response rendering pipeline for one route on
// one rux request. The order matches design §4.5:
//  1. cases selection (Phase 4 — Phase 3 ignores cases)
//  2. headers rendering
//  3. status rendering (constant int for Phase 3)
//  4. body rendering OR bodyFile streaming
//  5. Content-Type inference (only if headers didn't set one)
//  6. delay sleep
//  7. write to ResponseWriter
//
// On any rendering error the response becomes 500 + JSON error body.
func Respond(c *rux.Context, route *config.Route, renderer tpl.Renderer) {
	ctx := tpl.BuildRenderCtx(c.Req, paramsFromContext(c), nil)

	// 2. Render headers
	renderedHeaders, err := renderHeaders(route.Headers, renderer, ctx)
	if err != nil {
		writeError(c.Resp, http.StatusInternalServerError, "template error (headers)", err.Error(), route)
		return
	}

	// 3. Status
	status := route.Status
	if status == 0 {
		status = http.StatusOK
	}

	// 4. Body or bodyFile
	var bodyBytes []byte
	var bodyForCT any
	if route.BodyFile != "" {
		path := resolveBodyFile(route.BodyFile, route.SourceFile)
		b, ferr := os.ReadFile(path)
		if ferr != nil {
			writeError(c.Resp, http.StatusInternalServerError, "bodyFile error", ferr.Error(), route)
			return
		}
		bodyBytes = b
		if renderedHeaders["Content-Type"] == "" {
			renderedHeaders["Content-Type"] = mimeByExt(filepath.Ext(path))
		}
	} else if route.Body != nil {
		rendered, rerr := renderBody(route.Body, renderer, ctx)
		if rerr != nil {
			writeError(c.Resp, http.StatusInternalServerError, "template error (body)", rerr.Error(), route)
			return
		}
		bodyForCT = rendered
		switch v := rendered.(type) {
		case string:
			bodyBytes = []byte(v)
		default:
			b, jerr := json.Marshal(v)
			if jerr != nil {
				writeError(c.Resp, http.StatusInternalServerError, "json marshal error", jerr.Error(), route)
				return
			}
			bodyBytes = b
		}
	}

	// 5. Infer Content-Type when not explicit
	if renderedHeaders["Content-Type"] == "" && bodyForCT != nil {
		switch bodyForCT.(type) {
		case string:
			renderedHeaders["Content-Type"] = "text/plain; charset=utf-8"
		default:
			renderedHeaders["Content-Type"] = "application/json; charset=utf-8"
		}
	}

	// 6. Apply delay
	if d := parseDelay(route.Delay); d > 0 {
		time.Sleep(d)
	}

	// 7. Write headers + status + body
	for k, v := range renderedHeaders {
		c.Resp.Header().Set(k, v)
	}
	c.Resp.WriteHeader(status)
	if len(bodyBytes) > 0 {
		_, _ = c.Resp.Write(bodyBytes)
	} else {
		// rux v2's responseWriter defers WriteHeader until the first Write
		// (or Flush). For bodyless responses make sure the status actually
		// reaches the wire by writing an empty payload.
		_, _ = c.Resp.Write(nil)
	}
}

func renderHeaders(hdrs map[string]string, r tpl.Renderer, ctx map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(hdrs)+1)
	for k, v := range hdrs {
		rendered, err := r.Render(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("header %q: %w", k, err)
		}
		out[k] = rendered
	}
	return out, nil
}

// renderBody walks the (already-decoded JSON5) body value. Strings go
// through the template renderer; maps and slices recurse; anything else
// (numbers, bools, nil) passes through unchanged so json.Marshal preserves
// the original type.
func renderBody(body any, r tpl.Renderer, ctx map[string]any) (any, error) {
	switch v := body.(type) {
	case string:
		return r.Render(v, ctx)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			rendered, err := renderBody(child, r, ctx)
			if err != nil {
				return nil, err
			}
			out[k] = rendered
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			rendered, err := renderBody(item, r, ctx)
			if err != nil {
				return nil, err
			}
			out[i] = rendered
		}
		return out, nil
	default:
		return v, nil
	}
}

// mimeByExt resolves a file extension (".png") to a MIME type using the
// stdlib mime package, falling back to application/octet-stream on miss.
func mimeByExt(ext string) string {
	t := mime.TypeByExtension(ext)
	if t == "" {
		return "application/octet-stream"
	}
	return t
}

// resolveBodyFile turns a route's relative bodyFile path into an absolute
// filesystem path. Relative paths are resolved against the directory of
// the JSON5 file the route was declared in (Route.SourceFile, set by the
// config loader). This matches the rule documented in design §3.2.
func resolveBodyFile(p, routeSource string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if routeSource != "" {
		return filepath.Join(filepath.Dir(routeSource), p)
	}
	return p
}

// parseDelay accepts "120ms" or "100ms~500ms" form (uniform random
// distribution for the range). Returns 0 on parse failure or empty input.
func parseDelay(s string) time.Duration {
	if s == "" {
		return 0
	}
	if i := strings.Index(s, "~"); i >= 0 {
		lo, err1 := time.ParseDuration(s[:i])
		hi, err2 := time.ParseDuration(s[i+1:])
		if err1 != nil || err2 != nil || hi < lo {
			return 0
		}
		if hi == lo {
			return lo
		}
		return lo + time.Duration(rand.Int63n(int64(hi-lo)))
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// paramsFromContext lifts rux v2's inline path params into the flat
// map[string]string shape expected by tpl.BuildRenderCtx.
//
// Probed API (rux v2.0.0 internal/core):
//   - c.Params() returns *core.Params (private [16]Param + uint8 n).
//   - Params.Snapshot() returns a heap copy as []core.Param so we can
//     iterate without touching unexported fields.
//   - rux.Param is a type alias for core.Param{Key,Value string}.
func paramsFromContext(c *rux.Context) map[string]string {
	snap := c.Params().Snapshot()
	out := make(map[string]string, len(snap))
	for _, p := range snap {
		out[p.Key] = p.Value
	}
	return out
}

// writeError emits a uniform 500 error response per design §6.
func writeError(w http.ResponseWriter, status int, short, detail string, route *config.Route) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  short,
		"detail": detail,
		"route":  fmt.Sprintf("%s %s", strings.Join(route.Method, ","), route.Path),
	})
}
