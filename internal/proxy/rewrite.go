// Package proxy implements fakeserver's reverse-proxy routes (design §9).
// Each route declared with a proxy{} block becomes a httputil.ReverseProxy
// instance with custom Director (path rewrite + header injection),
// ModifyResponse (response header injection), and ErrorHandler (502 on
// dial failure).
package proxy

import (
	"fmt"
	"regexp"
	"strings"
)

// rewriteRule is one compiled "<regex> => <replacement>" pair.
// applyRewrites tries them in order; first match wins.
type rewriteRule struct {
	pattern     *regexp.Regexp
	replacement string
}

// compileRewrites parses the rewrite field from a proxy config block.
// Accepts:
//
//	nil               → []*rewriteRule{} (no rules)
//	string            → one rule
//	[]any / []string  → multiple rules (in order)
//
// Each rule must be "<go-regex> => <replacement>". $1, $2, ... in the
// replacement reference regex capture groups (Go regexp ReplaceAllString
// semantics; design §9.2 explicitly notes these are NOT template variables).
// The first "=>" in src is the separator; later "=>" stay in the replacement
// verbatim (e.g. "^/foo => /bar=>baz" yields pattern "^/foo" and replacement
// "/bar=>baz").
func compileRewrites(raw any) ([]*rewriteRule, error) {
	if raw == nil {
		return nil, nil
	}
	var sources []string
	switch v := raw.(type) {
	case string:
		sources = []string{v}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("rewrite: array element must be string, got %T", item)
			}
			sources = append(sources, s)
		}
	case []string:
		sources = v
	default:
		return nil, fmt.Errorf("rewrite: expected string or []string, got %T", raw)
	}

	out := make([]*rewriteRule, 0, len(sources))
	for _, src := range sources {
		idx := strings.Index(src, "=>")
		if idx < 0 {
			return nil, fmt.Errorf("rewrite %q: missing '=>' separator", src)
		}
		pat := strings.TrimSpace(src[:idx])
		repl := strings.TrimSpace(src[idx+2:])
		if pat == "" {
			return nil, fmt.Errorf("rewrite %q: empty pattern", src)
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("rewrite %q: regex compile: %w", src, err)
		}
		out = append(out, &rewriteRule{pattern: re, replacement: repl})
	}
	return out, nil
}

// applyRewrites runs path through the first matching rule. Returns
// (rewritten, true) on first match or (original, false) when no rule
// matches.
func applyRewrites(rules []*rewriteRule, path string) (string, bool) {
	for _, r := range rules {
		if r.pattern.MatchString(path) {
			return r.pattern.ReplaceAllString(path, r.replacement), true
		}
	}
	return path, false
}
