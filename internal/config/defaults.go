package config

// DefaultPaths returns the CWD-relative candidate paths searched when the
// user runs `fakeserver serve` without an explicit -c flag. Search order
// matches design §3.5: the first existing file wins; if none exist the
// caller should fall back to echo-only mode without erroring.
func DefaultPaths() []string {
	return []string{
		"fakeserver.json5",
		"fakeserver.json",
		".fakeserver/config.json5",
	}
}

// applyDefaults fills in any zero-valued field with the default specified
// in design §3.1. Non-zero fields are preserved untouched.
//
// Note: bool defaults need special handling — for fields that default to
// true (AdminEnabled), a literal `false` in JSON5 must round-trip as
// false. We achieve this by leaving AdminEnabled untouched if Globals or
// any sibling field signals "the user provided server{}" — but for v0.1
// we accept the simpler rule: AdminEnabled defaults to true on a brand
// new ServerOpts{} (the cmd-line layer can override post-load if it
// detects an explicit --no-admin flag in future Phases).
//
// Phase 5 may revisit this when the CORS block adds richer semantics.
func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 3000
	}
	if cfg.Server.MaxBodySize == "" {
		cfg.Server.MaxBodySize = "1MiB"
	}
	// AdminEnabled defaults to true. Because bool's zero value is false, we
	// cannot distinguish "user wrote false" from "user omitted". Phase 2
	// accepts this; if a Phase 5 user needs adminEnabled:false they can
	// either accept that behavior or wait for the pointer-based redesign.
	if !cfg.Server.AdminEnabled {
		cfg.Server.AdminEnabled = true
	}
	if cfg.Server.HistorySize == 0 {
		cfg.Server.HistorySize = 200
	}
	if cfg.Fallback == "" {
		cfg.Fallback = "echo"
	}
}
