// Package tpl provides text and HTML template rendering for fakeserver
// mock responses. design §4 covers the contract; this package implements
// it on top of stdlib text/template & html/template (with easytpl's
// tplfunc base FuncMap merged in).
package tpl

import (
	"bytes"
	htmltpl "html/template"
	texttpl "text/template"

	"github.com/brianvoe/gofakeit/v7"
)

// Renderer renders a template source string with the given context. Two
// concrete implementations exist: textRenderer (text/template, default for
// JSON / text responses) and htmlRenderer (html/template, only used when
// the response Content-Type is text/html*).
//
// ctx is map[string]any with lowercase keys (see BuildRenderCtx) so design
// §4.1 access patterns like {{ .request.params.id }} work via Go template
// reflection.
type Renderer interface {
	Render(src string, ctx map[string]any) (string, error)
}

// NewRenderer constructs the default (text-mode) renderer. Pass through
// FuncMap-affecting knobs:
//   globals        — exposed as .config in templates (caller already loaded cfg.Globals)
//   osenvWhitelist — restricts which OS env keys the osenv() func can read
//   fakerSeed      — 0 means random; non-zero seeds gofakeit once for reproducibility
func NewRenderer(globals map[string]any, osenvWhitelist []string, fakerSeed int64) Renderer {
	seedFaker(fakerSeed)
	return &textRenderer{funcs: BaseFuncMap(osenvWhitelist)}
}

// NewHTMLRenderer constructs the html-template-based renderer. Use this
// only when the response Content-Type starts with "text/html".
func NewHTMLRenderer(globals map[string]any, osenvWhitelist []string, fakerSeed int64) Renderer {
	seedFaker(fakerSeed)
	return &htmlRenderer{funcs: htmltpl.FuncMap(BaseFuncMap(osenvWhitelist))}
}

func seedFaker(s int64) {
	if s != 0 {
		gofakeit.Seed(s)
	}
	// s == 0 ≡ "leave gofakeit's default (time-based) seeding alone"
}

type textRenderer struct {
	funcs texttpl.FuncMap
}

func (r *textRenderer) Render(src string, ctx map[string]any) (string, error) {
	tpl, err := texttpl.New("t").Funcs(r.funcs).Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type htmlRenderer struct {
	funcs htmltpl.FuncMap
}

func (r *htmlRenderer) Render(src string, ctx map[string]any) (string, error) {
	tpl, err := htmltpl.New("t").Funcs(r.funcs).Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
