// Package config loads, merges, and validates fakeserver JSON5 configurations.
//
// Load() walks one or more JSON5 files (or the default search paths when no
// path is given), expands "@include" references, merges results, and produces
// a strongly-typed *Config. Validate() then runs all of design §3.7's checks
// collectively, returning every problem at once.
//
// The schema (struct definitions) lives in schema.go; default values and the
// CWD search list live in defaults.go; cross-checks live in validate.go.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/titanous/json5"
)

// Load reads each path in order, expands @include references relative to
// each file's directory, merges the results, applies defaults, and returns
// a *Config. envName and overrides are accepted but ignored in Phase 2 —
// they exist for v0.2's env-file integration.
//
// On any error (file not found, JSON5 syntax, include cycle, type
// mismatch), Load returns nil + a wrapping error. Validate() in
// validate.go is a separate step the caller must invoke.
func Load(paths []string, envName string, overrides map[string]string) (*Config, error) {
	if len(paths) == 0 {
		return nil, errors.New("config.Load: no paths provided")
	}

	var raws []map[string]any
	var absSources []string
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", p, err)
		}
		raw, err := loadFile(abs)
		if err != nil {
			return nil, err
		}
		rawMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config %q: root must be an object", abs)
		}
		annotateRoutesWithSource(rawMap, abs) // v0.2 lite-tools-gko
		visiting := map[string]bool{abs: true}
		expanded, err := expandIncludes(any(rawMap), filepath.Dir(abs), visiting)
		if err != nil {
			return nil, err
		}
		expandedMap, ok := expanded.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config %q: root must be an object", abs)
		}
		raws = append(raws, expandedMap)
		absSources = append(absSources, abs)
	}

	merged := mergeMaps(raws)

	cfg, routeSources, err := mapToConfig(merged)
	if err != nil {
		return nil, err
	}
	cfg.SourcePaths = absSources
	// v0.2 lite-tools-gko: zip per-route source file back into the
	// struct (lost during the JSON round-trip due to json:"-" tag).
	for i := range cfg.Routes {
		if i < len(routeSources) && routeSources[i] != "" {
			cfg.Routes[i].SourceFile = routeSources[i]
		}
	}

	// v0.2 Phase 1: auto-load fakeserver.env.json5 next to the primary
	// config file. Phase 2: envName is now passed through from CLI/env-var.
	if len(absSources) > 0 {
		envPath := filepath.Join(filepath.Dir(absSources[0]), DefaultEnvFileName)
		envMap, _, eerr := LoadEnvFile(envPath, envName)
		if eerr != nil {
			return nil, fmt.Errorf("env file: %w", eerr)
		}
		cfg.Env = envMap
		// EnvSource is only set when the file existed (LoadEnvFile returns
		// empty map for missing files; we want EnvSource="" in that case).
		// Also append to SourcePaths so the watcher monitors the env file.
		if _, sterr := os.Stat(envPath); sterr == nil {
			cfg.EnvSource = envPath
			cfg.SourcePaths = append(cfg.SourcePaths, envPath)
		}
	}

	// v0.2 Phase 2: --var top-level overrides (design §8.2 deepMerge step).
	// Phase 2 simplifies to top-level key replacement; no dotted-path support.
	if len(overrides) > 0 {
		if cfg.Env == nil {
			cfg.Env = map[string]any{}
		}
		for k, v := range overrides {
			cfg.Env[k] = v
		}
	}

	applyDefaults(cfg)
	return cfg, nil
}

// loadFile reads a single file and parses it as JSON5. For root-level configs
// (called from Load), we expect a map. For included files, any JSON5 value
// is acceptable (map, array, or scalar). The caller distinguishes via context.
func loadFile(absPath string) (any, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", absPath, err)
	}
	var out any
	if err := json5.NewDecoder(strings.NewReader(string(data))).Decode(&out); err != nil {
		return nil, fmt.Errorf("parse %q: %w", absPath, err)
	}
	return out, nil
}

// mergeMaps merges a slice of root maps left-to-right per design §3.4:
//   - server / globals / fallback: deep-merge (later overrides earlier)
//   - routes: append in order
//
// Empty input returns a fresh empty map.
func mergeMaps(raws []map[string]any) map[string]any {
	if len(raws) == 0 {
		return map[string]any{}
	}
	if len(raws) == 1 {
		return raws[0]
	}
	out := map[string]any{}
	for _, m := range raws {
		for k, v := range m {
			if k == "routes" {
				existing, _ := out["routes"].([]any)
				if newRoutes, ok := v.([]any); ok {
					out["routes"] = append(existing, newRoutes...)
				}
				continue
			}
			if existing, ok := out[k].(map[string]any); ok {
				if newMap, ok := v.(map[string]any); ok {
					out[k] = deepMerge(existing, newMap)
					continue
				}
			}
			out[k] = v
		}
	}
	return out
}

// deepMerge merges right into left recursively. Maps are merged key by
// key; non-map values from right replace left.
func deepMerge(left, right map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range left {
		out[k] = v
	}
	for k, v := range right {
		if existingMap, ok := out[k].(map[string]any); ok {
			if newMap, ok := v.(map[string]any); ok {
				out[k] = deepMerge(existingMap, newMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// annotateRoutesWithSource walks node and tags every route-shaped map
// with a `__source_file__` sentinel key pointing to sourceFile. This
// preserves the per-route origin file path across mapToConfig's JSON
// round-trip — mapToConfig extracts and clears the sentinel before
// running encoding/json, then Load zips routeSources into Route.SourceFile.
//
// Called at two loadFile callsites: Load (top-level cfg) and
// resolveInclude (each @included file). Each route map is annotated
// exactly once (at the file boundary before expandIncludes recurses),
// so no "first wins" guard is needed — and we deliberately overwrite
// any pre-existing `__source_file__` from the user's JSON5 to prevent
// a malicious config from spoofing the resolved path.
//
// Recognizes three shapes:
//   - map[string]any with a "routes" []any → annotate each map element of routes
//   - []any → annotate each map element (each is a route)
//   - map[string]any without "routes" → annotate self (single-route include)
func annotateRoutesWithSource(node any, sourceFile string) {
	switch v := node.(type) {
	case map[string]any:
		if routes, ok := v["routes"].([]any); ok {
			for _, r := range routes {
				if rm, ok := r.(map[string]any); ok {
					rm["__source_file__"] = sourceFile
				}
			}
			return
		}
		// Single-route map (from a one-route @include file)
		v["__source_file__"] = sourceFile
	case []any:
		for _, r := range v {
			if rm, ok := r.(map[string]any); ok {
				rm["__source_file__"] = sourceFile
			}
		}
	}
}

// mapToConfig converts the raw merged map into a strongly-typed *Config.
// We round-trip through encoding/json to leverage struct tags: titanous/json5
// already produced standard Go map/slice/primitive types, so a JSON
// re-encode is lossless. Custom normalization (Route.Method may be string
// or []string) happens after.
// Returns the config and a parallel slice of per-route source file paths
// (extracted from the __source_file__ sentinel before the JSON round-trip).
func mapToConfig(m map[string]any) (*Config, []string, error) {
	var routeSources []string
	if routes, ok := m["routes"].([]any); ok {
		routeSources = make([]string, len(routes))
		for i, r := range routes {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			if sf, ok := rm["__source_file__"].(string); ok {
				routeSources[i] = sf
				delete(rm, "__source_file__")
			}
			// Method shape normalization (preserve Phase 2 logic)
			method := rm["method"]
			switch v := method.(type) {
			case string:
				rm["method"] = []any{v}
			case nil:
				rm["method"] = []any{}
			case []any:
				// already normalized
			default:
				return nil, nil, fmt.Errorf("routes[%d].method: unsupported type %T", i, v)
			}
		}
	}

	buf, err := json.Marshal(m)
	if err != nil {
		return nil, nil, fmt.Errorf("re-encode to JSON: %w", err)
	}
	cfg := &Config{}
	if err := json.Unmarshal(buf, cfg); err != nil {
		return nil, nil, fmt.Errorf("unmarshal to Config: %w", err)
	}
	return cfg, routeSources, nil
}

// expandIncludes walks the JSON5-decoded map, replacing any string value
// starting with "@" (literal) with the JSON5 content from the referenced
// file. Behavior per design §3.3:
//   - Path resolution is relative to the including file's directory
//   - Globs ("*", "**") are expanded; zero matches is an error
//   - Recursion is allowed; cycles are detected via the visiting set
//   - Only .json/.json5 extensions are accepted
//   - "\@..." escapes a literal leading "@"
//
// In Phase 2 the only call site that interprets include results
// structurally is the top-level routes array — strings appearing
// elsewhere are still expanded (per design §3.3 second row) but the
// resulting value replaces the string in place.
//
// visiting holds the absolute paths currently on the recursion stack so
// we can detect cycles before re-reading the same file.
func expandIncludes(node any, baseDir string, visiting map[string]bool) (any, error) {
	switch v := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			expanded, err := expandIncludes(child, baseDir, visiting)
			if err != nil {
				return nil, err
			}
			out[k] = expanded
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			expanded, err := expandIncludes(item, baseDir, visiting)
			if err != nil {
				return nil, err
			}
			// If a string @include resolved to a slice (e.g. a routes
			// array), flatten it into the parent slice.
			if subSlice, ok := expanded.([]any); ok && isIncludeString(item) {
				out = append(out, subSlice...)
			} else {
				out = append(out, expanded)
			}
		}
		return out, nil
	case string:
		if !strings.HasPrefix(v, "@") {
			return v, nil
		}
		if strings.HasPrefix(v, "\\@") {
			return v[1:], nil // unescape literal leading @
		}
		target := v[1:]
		return resolveInclude(target, baseDir, visiting)
	default:
		return v, nil
	}
}

// isIncludeString reports whether the original node is a "@..." reference
// (not a literal). Used so a slice include can flatten into its parent.
func isIncludeString(node any) bool {
	s, ok := node.(string)
	if !ok {
		return false
	}
	return strings.HasPrefix(s, "@") && !strings.HasPrefix(s, "\\@")
}

// resolveInclude reads and recursively expands the file (or glob) named
// by spec, relative to baseDir.
func resolveInclude(spec, baseDir string, visiting map[string]bool) (any, error) {
	// Resolve absolute target(s)
	absSpec := spec
	if !filepath.IsAbs(absSpec) {
		absSpec = filepath.Join(baseDir, spec)
	}

	var matches []string
	if strings.ContainsAny(absSpec, "*?[") {
		m, err := filepath.Glob(absSpec)
		if err != nil {
			return nil, fmt.Errorf("@include glob %q: %w", spec, err)
		}
		if len(m) == 0 {
			return nil, fmt.Errorf("@include %q: no files matched", spec)
		}
		matches = m
	} else {
		matches = []string{absSpec}
	}

	var results []any
	for _, p := range matches {
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".json" && ext != ".json5" {
			return nil, fmt.Errorf("@include %q: unsupported extension %q (only .json/.json5)", p, ext)
		}
		if visiting[p] {
			return nil, fmt.Errorf("@include cycle detected: %s", cyclePath(visiting, p))
		}
		raw, err := loadFile(p)
		if err != nil {
			return nil, err
		}
		annotateRoutesWithSource(raw, p) // v0.2 lite-tools-gko

		// Recurse into the loaded content
		visiting[p] = true
		expanded, err := expandIncludes(any(raw), filepath.Dir(p), visiting)
		delete(visiting, p)
		if err != nil {
			return nil, err
		}

		// The included file's root may be a route object (map), a slice
		// of routes, or a top-level config map. Convert to "any" and
		// append.
		results = append(results, expanded)
	}

	// Single match? Return the single result so the caller can decide
	// whether to flatten. Multiple matches always flatten as a slice.
	if len(results) == 1 {
		return results[0], nil
	}
	// Multiple matches: convert each to its inner shape and flatten
	flat := make([]any, 0, len(results))
	for _, r := range results {
		if slice, ok := r.([]any); ok {
			flat = append(flat, slice...)
		} else {
			flat = append(flat, r)
		}
	}
	return flat, nil
}

// cyclePath formats the visiting set into a "a → b → c → a" chain for
// error messages.
func cyclePath(visiting map[string]bool, dup string) string {
	keys := make([]string, 0, len(visiting)+1)
	for k := range visiting {
		keys = append(keys, k)
	}
	keys = append(keys, dup)
	return strings.Join(keys, " → ")
}

// LoadDefault searches cwd for the conventional config files (see
// DefaultPaths) and loads the first one that exists. Returns (nil, nil)
// — not an error — when none exist, so the caller (cli/serve.go) can
// degrade to echo-only mode silently.
func LoadDefault(cwd, envName string, overrides map[string]string) (*Config, error) {
	for _, rel := range DefaultPaths() {
		abs := filepath.Join(cwd, rel)
		if _, err := os.Stat(abs); err == nil {
			return Load([]string{abs}, envName, overrides)
		}
	}
	return nil, nil
}
