// envfile.go implements design §8 environment-file loading. The file lives
// next to the main config (fakeserver.env.json5 by default) and provides
// per-environment value overrides accessible to mock templates as
// {{ .env.* }} (template wiring lands in v0.2 Phase 2; this Phase only
// produces cfg.Env / cfg.EnvSource).
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultEnvFileName is the basename loader.Load() searches next to the
// main config file. design §8.1.
const DefaultEnvFileName = "fakeserver.env.json5"

// LoadEnvFile parses path as a JSON5 env file and returns the merged
// environment block for envName.
//
// Behavior (design §8):
//   - File missing → ({}, "", nil). Not an error; v0.1 zero-config UX.
//   - Root non-object → error.
//   - Contains @include → error (design §8.2 末尾 explicitly禁止).
//   - $default segment is deep-merged into the chosen segment.
//   - Chosen segment selection (envName == ""):
//     1. file's "$active" field
//     2. first non-$default segment by iteration order
//     3. neither → ({}, "", nil)
//   - envName != "" → that exact segment is chosen; missing → error.
//
// active is the resolved segment name (informational; useful for banners
// and watcher feedback). Phase 1 doesn't render template strings inside
// env values — Phase 2 does.
func LoadEnvFile(path, envName string) (envMap map[string]any, active string, err error) {
	envMap = map[string]any{}

	// Resolve to absolute path so os.Stat and loadFile work consistently.
	abs, aerr := filepath.Abs(path)
	if aerr != nil {
		return nil, "", fmt.Errorf("resolve env file path %q: %w", path, aerr)
	}

	// File missing → ({}, "", nil)
	if _, serr := os.Stat(abs); os.IsNotExist(serr) {
		return envMap, "", nil
	} else if serr != nil {
		return nil, "", fmt.Errorf("stat env file %q: %w", abs, serr)
	}

	raw, rerr := loadFile(abs) // re-use loader.go's JSON5 parser
	if rerr != nil {
		return nil, "", rerr
	}
	rootMap, ok := raw.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("env file %q: root must be an object", abs)
	}

	// Reject @include in env files (design §8.2 末尾). Task 5 fills this in.
	if ierr := rejectIncludesInEnvFile(rootMap, abs); ierr != nil {
		return nil, "", ierr
	}

	// Extract $default and meta $active; everything else is a candidate segment.
	defaults, _ := rootMap["$default"].(map[string]any)
	activeMeta, _ := rootMap["$active"].(string)

	// Determine target segment.
	target := envName
	if target == "" {
		target = activeMeta
	}
	if target == "" {
		// Find first non-$default/$active segment by iteration order.
		for k := range rootMap {
			if k == "$default" || k == "$active" {
				continue
			}
			target = k
			break
		}
	}
	if target == "" {
		// No segments at all → empty result, not an error.
		return envMap, "", nil
	}

	chosen, ok := rootMap[target].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("env file %q: segment %q not found or not an object", abs, target)
	}

	// Deep-merge $default into chosen segment (chosen wins on conflict).
	if defaults != nil {
		envMap = deepMerge(defaults, chosen)
	} else {
		envMap = deepMerge(map[string]any{}, chosen)
	}

	return envMap, target, nil
}

// rejectIncludesInEnvFile walks the root map and errors on any string
// value starting with "@" (and not escaped with "\@"). design §8.2 末尾:
// env files do not support @include. Task 5 fills this in.
func rejectIncludesInEnvFile(node any, path string) error {
	// Task 5 will implement the recursive walk + reject logic.
	return nil
}
