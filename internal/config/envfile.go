// envfile.go implements design §8 environment-file loading. The file lives
// next to the main config (fakeserver.env.json5 by default) and provides
// per-environment value overrides accessible to mock templates as
// {{ .env.* }} (template wiring lands in v0.2 Phase 2; this Phase only
// produces cfg.Env / cfg.EnvSource).
package config

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
	// Tasks 4-5 will fill this in; Task 3 just establishes the signature.
	return map[string]any{}, "", nil
}
