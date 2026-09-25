package mock

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/inhere/fakeserver/internal/config"
)

// paginateBody implements declarative pagination (design §3.2 extension):
// it slices the list at cfg.ListPath inside the JSON response body by the page
// and page size named in the request (body field first, query parameter second;
// defaults: page 1, size = whole list). The pre-slice length is written into
// cfg.TotalPath when that path already exists in the body. Out-of-range pages
// yield an empty list. Returns the re-marshaled body.
//
// The body may come from a rendered template body or a bodyFile — both arrive
// here as bytes.
func paginateBody(cfg *config.PaginateConfig, body []byte, ctx map[string]any) ([]byte, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("response body is not JSON: %w", err)
	}
	raw, ok := lookupPath(doc, cfg.ListPath)
	if !ok {
		return nil, fmt.Errorf("listPath %q not found in the response body", cfg.ListPath)
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("listPath %q holds %T, not a list", cfg.ListPath, raw)
	}

	page := requestInt(ctx, cfg.PageField, "current")
	size := requestInt(ctx, cfg.SizeField, "size")
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > len(items) {
		// Absent/oversized page size: one page holds the whole list.
		size = len(items)
	}
	if page > len(items)+1 {
		// Clamp so the offset arithmetic stays in range; the page is empty.
		page = len(items) + 1
	}
	start := (page - 1) * size
	pageItems := make([]any, 0, size)
	if start < len(items) {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		pageItems = append(pageItems, items[start:end]...)
	}

	if !setPath(doc, cfg.ListPath, pageItems) {
		return nil, fmt.Errorf("listPath %q cannot be written back", cfg.ListPath)
	}
	if cfg.TotalPath != "" {
		// Only fills an existing slot: pagination never invents structure.
		setPath(doc, cfg.TotalPath, len(items))
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal paginated body: %w", err)
	}
	return out, nil
}

// requestInt resolves a page/size field name against the request body first and
// the query string second, returning 0 when absent or non-numeric. An empty
// field name falls back to defField.
func requestInt(ctx map[string]any, field, defField string) int {
	name := field
	if name == "" {
		name = defField
	}
	req, _ := ctx["request"].(map[string]any)
	if req == nil {
		return 0
	}
	if v, ok := lookupPath(req["body"], name); ok {
		if n, ok := toInt(v); ok {
			return n
		}
	}
	if v, ok := lookupPath(req["query"], name); ok {
		if n, ok := toInt(v); ok {
			return n
		}
	}
	return 0
}

// toInt coerces the JSON/query value shapes a page number can arrive in.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return i, true
	case []any:
		if len(n) > 0 {
			return toInt(n[0])
		}
	case []string:
		if len(n) > 0 {
			return toInt(n[0])
		}
	}
	return 0, false
}

// lookupPath walks a dot path ("data.list", "data.items.0") through maps and
// slices. Returns (nil, false) when any segment is missing.
func lookupPath(doc any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	cur := doc
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// setPath writes value at a dot path whose parent container exists, mutating the
// tree in place. Returns false when the parent path cannot be resolved.
func setPath(doc any, path string, value any) bool {
	segs := strings.Split(path, ".")
	parent := doc
	if len(segs) > 1 {
		p, ok := lookupPath(doc, strings.Join(segs[:len(segs)-1], "."))
		if !ok {
			return false
		}
		parent = p
	}
	last := segs[len(segs)-1]
	switch node := parent.(type) {
	case map[string]any:
		node[last] = value
		return true
	case []any:
		idx, err := strconv.Atoi(last)
		if err != nil || idx < 0 || idx >= len(node) {
			return false
		}
		node[idx] = value
		return true
	}
	return false
}
