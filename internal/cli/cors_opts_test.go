package cli

import (
	"testing"

	"github.com/inhere/fakeserver/internal/config"
)

func TestCorsOptsFromCfg_NilCfg(t *testing.T) {
	opts, enabled := corsOptsFromCfg(nil)
	if !enabled {
		t.Error("nil cfg → enabled=true (default)")
	}
	if len(opts.Origins) != 0 || len(opts.Methods) != 0 {
		t.Errorf("nil cfg → empty opts, got %+v", opts)
	}
}

func TestCorsOptsFromCfg_BoolTrue(t *testing.T) {
	cfg := &config.Config{Server: config.ServerOpts{CORS: true}}
	_, enabled := corsOptsFromCfg(cfg)
	if !enabled {
		t.Error("cors:true → enabled=true")
	}
}

func TestCorsOptsFromCfg_BoolFalse(t *testing.T) {
	cfg := &config.Config{Server: config.ServerOpts{CORS: false}}
	_, enabled := corsOptsFromCfg(cfg)
	if enabled {
		t.Error("cors:false → enabled=false (CORS disabled)")
	}
}

func TestCorsOptsFromCfg_MapForm(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerOpts{
			CORS: map[string]any{
				"origins":          []any{"http://a.example", "http://b.example"},
				"methods":          []any{"GET", "POST"},
				"headers":          []any{"X-Custom"},
				"allowCredentials": true,
			},
		},
	}
	opts, enabled := corsOptsFromCfg(cfg)
	if !enabled {
		t.Error("map form → enabled=true")
	}
	if len(opts.Origins) != 2 || opts.Origins[0] != "http://a.example" {
		t.Errorf("origins=%v", opts.Origins)
	}
	if len(opts.Methods) != 2 {
		t.Errorf("methods=%v", opts.Methods)
	}
	if len(opts.Headers) != 1 || opts.Headers[0] != "X-Custom" {
		t.Errorf("headers=%v", opts.Headers)
	}
	if !opts.AllowCredentials {
		t.Error("AllowCredentials=true expected")
	}
}

func TestCorsOptsFromCfg_NilCORSField(t *testing.T) {
	// cfg.Server.CORS is nil (not set in JSON5)
	cfg := &config.Config{Server: config.ServerOpts{CORS: nil}}
	_, enabled := corsOptsFromCfg(cfg)
	if !enabled {
		t.Error("nil CORS field → enabled=true (default)")
	}
}
