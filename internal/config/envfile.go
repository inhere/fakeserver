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
	"sort"
	"strings"
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

	// Extract and validate $default (must be a map if present)
	var defaults map[string]any
	if raw, present := rootMap["$default"]; present {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("env file %q: $default must be an object, got %T", abs, raw)
		}
		defaults = m
	}

	// Extract and validate $active (must be a string if present)
	var activeMeta string
	if raw, present := rootMap["$active"]; present {
		s, ok := raw.(string)
		if !ok {
			return nil, "", fmt.Errorf("env file %q: $active must be a string, got %T", abs, raw)
		}
		activeMeta = s
	}

	// Determine target segment + remember its source for error messages.
	target := envName
	targetSource := "envName arg"
	if target == "" && activeMeta != "" {
		target = activeMeta
		targetSource = "$active field"
	}
	if target == "" {
		// Find first non-$default/$active segment by iteration order.
		// NOTE: Go map iteration is randomized — "first segment" is
		// non-deterministic when multiple non-meta keys exist. Users who
		// need a stable default should set $active explicitly.
		for k := range rootMap {
			if k == "$default" || k == "$active" {
				continue
			}
			target = k
			targetSource = "first non-$default segment"
			break
		}
	}
	if target == "" {
		// No segments at all → empty result, not an error.
		return envMap, "", nil
	}

	raw, present := rootMap[target]
	if !present {
		return nil, "", fmt.Errorf("env file %q: segment %q not found (selected via %s)", abs, target, targetSource)
	}
	chosen, ok := raw.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("env file %q: segment %q must be an object, got %T (selected via %s)", abs, target, raw, targetSource)
	}

	// Deep-merge $default into chosen segment (chosen wins on conflict).
	if defaults != nil {
		envMap = deepMerge(defaults, chosen)
	} else {
		envMap = deepMerge(map[string]any{}, chosen)
	}

	return envMap, target, nil
}

// ExtractEnvNames 返回 path 指向的 env 文件中所有可选 env 段名（按字母序，
// 排除 `$default` / `$active` 元字段）。供 v0.3 项目注册时填充
// Project.Envs 字段使用（design §10.2）。
//
// 容错策略（与 LoadEnvFile 对齐）：文件不存在 / 解析失败 / root 非 object
// 均返回 nil（不报错）——envs 字段是辅助信息，不应阻塞 serve 启动。
func ExtractEnvNames(path string) []string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	if _, err := os.Stat(abs); err != nil {
		return nil
	}
	raw, err := loadFile(abs)
	if err != nil {
		return nil
	}
	root, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(root))
	for k := range root {
		if k == "$default" || k == "$active" {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// rejectIncludesInEnvFile walks the root map and errors on any string
// value starting with "@" (and not escaped with "\@"). design §8.2 末尾:
// env files do not support @include — keeps recursion complexity bounded
// and avoids env-vs-route include semantics ambiguity.
func rejectIncludesInEnvFile(node any, path string) error {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if err := rejectIncludesInEnvFile(child, path); err != nil {
				return fmt.Errorf("env file %q at key %q: %w", path, k, err)
			}
		}
	case []any:
		for i, child := range v {
			if err := rejectIncludesInEnvFile(child, path); err != nil {
				return fmt.Errorf("env file %q at index %d: %w", path, i, err)
			}
		}
	case string:
		if strings.HasPrefix(v, "@") && !strings.HasPrefix(v, "\\@") {
			return fmt.Errorf("@include is not supported in env files (use main config @include instead): %q", v)
		}
	}
	return nil
}
