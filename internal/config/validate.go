package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/expr-lang/expr"

	"github.com/inhere/fakeserver/internal/sizeparse"
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
	switch f := cfg.Fallback.(type) {
	case string:
		if !validFallback[f] {
			errs = append(errs, fmt.Errorf("fallback: must be \"echo\" or \"404\", got %q", f))
		}
	case map[string]any:
		status := 404
		if v, ok := f["status"]; ok {
			n, ok := v.(float64)
			if !ok || n < 100 || n > 599 || n != float64(int(n)) {
				errs = append(errs, fmt.Errorf("fallback: status must be an integer between 100 and 599"))
			} else {
				status = int(n)
			}
		}
		_ = status
		if _, ok := f["body"]; ok {
			if _, both := f["bodyFile"]; both {
				errs = append(errs, fmt.Errorf("fallback: body and bodyFile are mutually exclusive"))
			}
		}
	default:
		errs = append(errs, fmt.Errorf("fallback: must be \"echo\", \"404\", or an object"))
	}

	if cfg.Server.Capture.MaxBodySize != "" {
		if _, err := sizeparse.ParseByteSize(cfg.Server.Capture.MaxBodySize); err != nil {
			errs = append(errs, fmt.Errorf("server.capture.maxBodySize %q: %w", cfg.Server.Capture.MaxBodySize, err))
		}
	}

	if cfg.Server.HistoryBodyMaxSize != "" {
		if _, err := sizeparse.ParseByteSize(cfg.Server.HistoryBodyMaxSize); err != nil {
			errs = append(errs, fmt.Errorf("server.historyBodyMaxSize %q: %w", cfg.Server.HistoryBodyMaxSize, err))
		}
	}

	// route-level checks
	type key struct {
		method, path string
	}
	seen := map[key]int{}                   // value = first index where seen
	routesBySignature := map[string]Route{} // route signature -> route
	caseNamesByRoute := map[string]map[string]bool{}

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

		caseNames := map[string]bool{}
		for ci, cs := range r.Cases {
			if cs.Name == "" {
				continue
			}
			if caseNames[cs.Name] {
				errs = append(errs, fmt.Errorf("%s cases[%d]: case name %q duplicated", prefix, ci, cs.Name))
				continue
			}
			caseNames[cs.Name] = true
		}

		// when expression syntax pre-check (Phase 4)
		for ci, cs := range r.Cases {
			if cs.When == "" {
				continue
			}
			if _, cerr := expr.Compile(cs.When, expr.AsBool()); cerr != nil {
				errs = append(errs, fmt.Errorf("%s %s.when: %w", prefix, caseFieldRef(ci, cs.Name), cerr))
			}
		}

		// proxy vs mock mutex
		if r.Proxy != nil {
			if r.Body != nil || r.BodyFile != "" || len(r.Cases) > 0 || r.Status != 0 || len(r.Headers) > 0 || r.Delay != "" || r.Paginate != nil {
				errs = append(errs, fmt.Errorf("%s: proxy is mutually exclusive with body/bodyFile/cases/status/headers/delay/paginate", prefix))
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

		if err := checkPaginate(r.Paginate); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", prefix, err))
		}
		for ci, cs := range r.Cases {
			if err := checkPaginate(cs.Paginate); err != nil {
				errs = append(errs, fmt.Errorf("%s %s: %w", prefix, caseFieldRef(ci, cs.Name), err))
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
			sig := routeSignature(m, r.Path)
			routesBySignature[sig] = r
			caseNamesByRoute[sig] = caseNames
		}
	}

	if cfg.Server.Scenario != "" {
		if _, ok := cfg.Scenarios[cfg.Server.Scenario]; !ok {
			errs = append(errs, fmt.Errorf("server.scenario %q does not exist in scenarios", cfg.Server.Scenario))
		}
	}

	for scenarioName, scenario := range cfg.Scenarios {
		for sig, caseName := range scenario.Routes {
			route, ok := routesBySignature[sig]
			if !ok || len(route.Cases) == 0 {
				errs = append(errs, fmt.Errorf("scenario %q: route %q does not exist or has no cases", scenarioName, sig))
				continue
			}
			if !caseNamesByRoute[sig][caseName] {
				errs = append(errs, fmt.Errorf("scenario %q: route %q references unknown case %q", scenarioName, sig, caseName))
			}
		}
	}

	return errs
}

// checkPaginate validates one paginate block: listPath is required (it names
// the list to slice). pageField/sizeField may be omitted — the runtime defaults
// them to "current"/"size". A nil block is valid (pagination off).
func checkPaginate(p *PaginateConfig) error {
	if p == nil {
		return nil
	}
	if p.ListPath == "" {
		return fmt.Errorf("paginate.listPath is required (dot path of the list inside the response body, e.g. data.list)")
	}
	return nil
}

func routeSignature(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

// caseFieldRef renders "cases[i]" plus the case name when present, so
// validation errors name the offending case.
func caseFieldRef(idx int, name string) string {
	if name == "" {
		return fmt.Sprintf("cases[%d]", idx)
	}
	return fmt.Sprintf("cases[%d] (%q)", idx, name)
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

// Warn returns non-fatal advisory messages found during validation.
// design §3.7 warning bucket. Caller (cli/serve.go, cli/check.go) is
// expected to print these to stderr without affecting exit code.
//
// Currently checks:
//   - strategy=first-match where every case has a when (no fallback);
//     warns the route can return 500 "no case matched" at runtime.
//   - proxy.target host resolves to localhost/private (design §9.4 info).
func Warn(cfg *Config) []string {
	if cfg == nil {
		return nil
	}
	var warns []string
	for i, r := range cfg.Routes {
		prefix := fmt.Sprintf("routes[%d] (%s %s)", i, strings.Join(r.Method, ","), r.Path)

		// first-match without fallback
		if r.Strategy == "first-match" && len(r.Cases) > 0 {
			hasFallback := false
			for _, c := range r.Cases {
				if c.When == "" {
					hasFallback = true
					break
				}
			}
			if !hasFallback {
				warns = append(warns, fmt.Sprintf("%s: strategy=first-match with no fallback case (all cases have when); runtime requests that match no case will return 500", prefix))
			}
		}

		// proxy.target private host
		if r.Proxy != nil && r.Proxy.Target != "" {
			if isPrivateOrLocalhost(r.Proxy.Target) {
				warns = append(warns, fmt.Sprintf("%s: proxy.target %q resolves to localhost/private network (intentional? double-check)", prefix, r.Proxy.Target))
			}
		}
	}
	return warns
}

// isPrivateOrLocalhost is a cheap heuristic on the host string of a URL.
// It does NOT do DNS resolution — only checks literal hosts. Good enough
// for an advisory warning.
func isPrivateOrLocalhost(target string) bool {
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	for _, p := range []string{"10.", "192.168.", "169.254."} {
		if strings.HasPrefix(host, p) {
			return true
		}
	}
	if strings.HasPrefix(host, "172.") {
		// 172.16.0.0/12
		parts := strings.SplitN(host, ".", 3)
		if len(parts) >= 2 {
			var n int
			_, _ = fmt.Sscanf(parts[1], "%d", &n)
			if n >= 16 && n <= 31 {
				return true
			}
		}
	}
	return false
}
