// Package tpl provides text and HTML template rendering for fakeserver
// mock responses. design §4 covers the contract; this package implements
// it on top of stdlib text/template & html/template (with optional
// easytpl integration for HTML layout in Phase 4+).
package tpl

// Renderer is implemented by both textRenderer and htmlRenderer. Phase 3
// Task 6 fills in concrete types.
type Renderer interface {
	// Render src as a template string using ctx as the data; writes the
	// expanded result to a string. Errors are returned verbatim so the
	// caller can map them to HTTP 500 responses.
	Render(src string, ctx *RenderCtx) (string, error)
}

// RenderCtx is forward-declared here so the stub compiles. Task 5 puts
// the real fields in context.go.
type RenderCtx struct{}
