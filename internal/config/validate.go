package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const reservedPrefix = "/__fakeserver/"

var (
	validFallback = map[string]bool{"echo": true, "404": true}
	validStrategy = map[string]bool{
		"":            true, // default → random
		"random":      true,
		"round-robin": true,
		"weighted":    true,
		"first-match": true,
	}
)

// Validate runs every cross-cutting check from design §3.7 and returns the
// full list of problems found. The caller (cli/check.go, cli/serve.go) is
// expected to print all errors and exit non-zero if the slice is non-empty.
//
// Phase 2 does NOT check expr (when:) syntax or template syntax — those
// require their respective libraries which arrive in Phase 4/3.
func Validate(cfg *Config) []error {
	if cfg == nil {
		return []error{fmt.Errorf("validate: config is nil")}
	}

	var errs []error

	// fallback enum
	if !validFallback[cfg.Fallback] {
		errs = append(errs, fmt.Errorf("server.fallback: must be \"echo\" or \"404\", got %q", cfg.Fallback))
	}

	// route-level checks
	type key struct {
		method, path string
	}
	seen := map[key]int{} // value = first index where seen

	for i, r := range cfg.Routes {
		prefix := fmt.Sprintf("routes[%d] (%s %s)", i, strings.Join(r.Method, ","), r.Path)

		if len(r.Method) == 0 {
			errs = append(errs, fmt.Errorf("%s: method is required", prefix))
		}
		if r.Path == "" {
			errs = append(errs, fmt.Errorf("%s: path is required", prefix))
		}

		// reserved prefix
		if strings.HasPrefix(r.Path, reservedPrefix) {
			errs = append(errs, fmt.Errorf("%s: path %q collides with reserved prefix %q", prefix, r.Path, reservedPrefix))
		}

		// strategy enum
		if !validStrategy[r.Strategy] {
			errs = append(errs, fmt.Errorf("%s: strategy %q is not one of random/round-robin/weighted/first-match", prefix, r.Strategy))
		}

		// proxy vs mock mutex
		if r.Proxy != nil {
			if r.Body != nil || r.BodyFile != "" || len(r.Cases) > 0 || r.Status != 0 || len(r.Headers) > 0 || r.Delay != "" {
				errs = append(errs, fmt.Errorf("%s: proxy is mutually exclusive with body/bodyFile/cases/status/headers/delay", prefix))
			}
			if r.Proxy.Target == "" {
				errs = append(errs, fmt.Errorf("%s: proxy.target is required", prefix))
			} else {
				u, err := url.Parse(r.Proxy.Target)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
					errs = append(errs, fmt.Errorf("%s: proxy.target scheme must be http or https, got %q", prefix, r.Proxy.Target))
				}
			}
		} else {
			// mock-side mutex
			if r.Body != nil && r.BodyFile != "" {
				errs = append(errs, fmt.Errorf("%s: body and bodyFile are mutually exclusive", prefix))
			}
			if len(r.Cases) > 0 && (r.Body != nil || r.BodyFile != "") {
				errs = append(errs, fmt.Errorf("%s: cases is mutually exclusive with top-level body/bodyFile", prefix))
			}
			if r.BodyFile != "" {
				resolved := resolveRoutePath(r.BodyFile, r.SourceFile, cfg.SourcePaths)
				if _, err := os.Stat(resolved); err != nil {
					errs = append(errs, fmt.Errorf("%s: bodyFile %q not found (resolved to %q)", prefix, r.BodyFile, resolved))
				}
			}
		}

		// duplicate detection: every (method, path) combination across all
		// listed methods of every route
		for _, m := range r.Method {
			k := key{method: m, path: r.Path}
			if prev, ok := seen[k]; ok {
				errs = append(errs, fmt.Errorf("duplicate route %s %s: defined at routes[%d] and routes[%d]", m, r.Path, prev, i))
				continue
			}
			seen[k] = i
		}
	}

	return errs
}

// resolveRoutePath resolves a relative path against the source file of the
// route (if known) or against the first source path. Absolute paths are
// returned unchanged.
func resolveRoutePath(p, routeSource string, sources []string) string {
	if filepath.IsAbs(p) {
		return p
	}
	base := routeSource
	if base == "" && len(sources) > 0 {
		base = sources[0]
	}
	if base == "" {
		return p
	}
	return filepath.Join(filepath.Dir(base), p)
}
