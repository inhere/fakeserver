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

// Load is the public entry point. Implementations land in subsequent steps;
// this stub exists so json5 dependency resolves before the package has any
// concrete consumer.
func Load(paths []string, envName string, overrides map[string]string) (*Config, error) {
	return nil, nil
}

// Config is forward-declared here so the stub compiles. The real definition
// (with all fields) lands in schema.go in Task 2.
type Config struct{}
