package config

// Config is the in-memory representation of one or more merged JSON5
// configuration files. Field types intentionally use plain Go primitives
// and maps so that consumers (cli/mock/proxy packages) do not need any
// json5-specific knowledge.
type Config struct {
	Server   ServerOpts     `json:"server"`
	Globals  map[string]any `json:"globals"`
	Fallback string         `json:"fallback"`
	Routes   []Route        `json:"routes"`

	// SourcePaths records, in load order, every config file that
	// contributed to this Config (including @include expansions).
	// Used by validate.go for error messages and (in Phase 5) by the
	// hot-reload watcher to decide which files to subscribe to.
	SourcePaths []string `json:"-"`
}

// ServerOpts mirrors the "server" block in JSON5. Each field's default is
// applied by applyDefaults() in defaults.go when the field is its zero
// value.
type ServerOpts struct {
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	CORS           any      `json:"cors"` // true | false | object — kept as raw any here; Phase 5 parses
	Log            *bool    `json:"log"`  // pointer to detect "unset" vs "false"
	MaxBodySize    string   `json:"maxBodySize"`
	AdminEnabled   bool     `json:"adminEnabled"`
	OSEnvWhitelist []string `json:"osenvWhitelist"`
	FakerSeed      int64    `json:"fakerSeed"`
	HistorySize    int      `json:"historySize"`
	ProjectName    string   `json:"projectName"`
}

// Route describes one declared route in JSON5. The same struct covers
// single-response (status/headers/body/bodyFile/delay), multi-response
// (strategy/cases), and proxy modes. Validate() in validate.go enforces
// the mutual-exclusion rules from design §3.2.
type Route struct {
	// Method may be a string ("GET"), an array (["GET","HEAD"]), or "*".
	// loader.go normalizes whatever the JSON5 yields into a []string.
	Method []string `json:"method"`
	Path   string   `json:"path"`

	// Single-response fields
	Status   int               `json:"status"`
	Delay    string            `json:"delay"`
	Headers  map[string]string `json:"headers"`
	Body     any               `json:"body"`
	BodyFile string            `json:"bodyFile"`

	// Multi-response fields
	Strategy string      `json:"strategy"`
	Cases    []RouteCase `json:"cases"`

	// Proxy mode (mutex with all of the above except method/path)
	Proxy *ProxyConfig `json:"proxy"`

	// SourceFile is the absolute path of the JSON5 file this route was
	// declared in (after @include expansion). Used by Validate() to
	// pinpoint duplicate-route locations in error messages.
	SourceFile string `json:"-"`
}

// RouteCase is one branch inside Route.Cases.
type RouteCase struct {
	When     string            `json:"when"`
	Weight   int               `json:"weight"`
	Status   int               `json:"status"`
	Delay    string            `json:"delay"`
	Headers  map[string]string `json:"headers"`
	Body     any               `json:"body"`
	BodyFile string            `json:"bodyFile"`
}

// ProxyConfig is the "proxy" sub-block of a Route. Only schema/validate is
// implemented in Phase 2 — actual proxying is implemented in Phase 4.
type ProxyConfig struct {
	Target             string            `json:"target"`
	Rewrite            any               `json:"rewrite"` // string or []string
	StripPathPrefix    string            `json:"stripPathPrefix"`
	Headers            map[string]string `json:"headers"`
	ResponseHeaders    map[string]string `json:"responseHeaders"`
	Timeout            string            `json:"timeout"`
	InsecureSkipVerify bool              `json:"insecureSkipVerify"`
	PreserveHost       bool              `json:"preserveHost"`
	BodyLimit          string            `json:"bodyLimit"`
}
