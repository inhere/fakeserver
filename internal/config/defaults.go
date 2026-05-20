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
// v0.4 Phase 1：AdminEnabled 改 *bool 后语义清晰：
//   - nil（用户未写）→ 设为 &true
//   - 非 nil → 保留用户写的 true / false
//
// 这允许 design §11.6 的 `adminEnabled: false` 真正生效。
func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 5090
	}
	if cfg.Server.MaxBodySize == "" {
		cfg.Server.MaxBodySize = "1MiB"
	}
	if cfg.Server.AdminEnabled == nil {
		t := true
		cfg.Server.AdminEnabled = &t
	}
	if cfg.Server.HistorySize == 0 {
		cfg.Server.HistorySize = 200
	}
	if cfg.Fallback == "" {
		cfg.Fallback = "echo"
	}
}
