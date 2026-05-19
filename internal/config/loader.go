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
		// Phase 2 Task 4 will expand @include here.
		raws = append(raws, raw)
		absSources = append(absSources, abs)
	}

	merged := mergeMaps(raws)

	cfg, err := mapToConfig(merged)
	if err != nil {
		return nil, err
	}
	cfg.SourcePaths = absSources
	applyDefaults(cfg)
	return cfg, nil
}

// loadFile reads a single file and parses it as JSON5 into a map.
func loadFile(absPath string) (map[string]any, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", absPath, err)
	}
	var out map[string]any
	if err := json5.NewDecoder(strings.NewReader(string(data))).Decode(&out); err != nil {
		return nil, fmt.Errorf("parse %q: %w", absPath, err)
	}
	if out == nil {
		out = map[string]any{}
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

// mapToConfig converts the raw merged map into a strongly-typed *Config.
// We round-trip through encoding/json to leverage struct tags: titanous/json5
// already produced standard Go map/slice/primitive types, so a JSON
// re-encode is lossless. Custom normalization (Route.Method may be string
// or []string) happens after.
func mapToConfig(m map[string]any) (*Config, error) {
	// Normalize Route.Method shapes before re-encoding: JSON's struct tags
	// can't natively accept "string OR []string", so we coerce in-place.
	if routes, ok := m["routes"].([]any); ok {
		for i, r := range routes {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			method := rm["method"]
			switch v := method.(type) {
			case string:
				rm["method"] = []any{v}
			case nil:
				rm["method"] = []any{}
			case []any:
				// already normalized
			default:
				return nil, fmt.Errorf("routes[%d].method: unsupported type %T", i, v)
			}
		}
	}

	buf, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("re-encode to JSON: %w", err)
	}
	cfg := &Config{}
	if err := json.Unmarshal(buf, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal to Config: %w", err)
	}
	return cfg, nil
}
