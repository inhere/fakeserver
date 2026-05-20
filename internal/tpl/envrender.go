package tpl

import (
	"bytes"
	"fmt"
	"text/template"
)

// RenderEnvValues walks env (a map decoded from JSON5) and renders every
// string leaf as a text/template using fakeserver's BaseFuncMap. The same
// osenvWhitelist that gates {{ osenv }} in mock body templates also gates
// it here (design §4.6: env file is not an escape hatch).
//
// globals is exposed as ".config" inside env templates (matches BuildRenderCtx
// shape). nil is treated as empty map.
//
// Mutates env in place. Returns the first render error encountered.
//
// Note: env values cannot reference each other (.env not exposed inside).
// Phase 2 v0.2 keeps simple; v0.3 may allow cross-key via topo-sort.
func RenderEnvValues(env map[string]any, osenvWhitelist []string, globals map[string]any) error {
	funcs := BaseFuncMap(osenvWhitelist)
	ctx := map[string]any{
		"config": globals,
	}
	return renderEnvNode(env, funcs, ctx)
}

func renderEnvNode(node any, funcs template.FuncMap, ctx map[string]any) error {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if s, ok := child.(string); ok {
				rendered, err := renderEnvString(s, funcs, ctx)
				if err != nil {
					return fmt.Errorf("env value at key %q: %w", k, err)
				}
				v[k] = rendered
			} else {
				if err := renderEnvNode(child, funcs, ctx); err != nil {
					return fmt.Errorf("env at key %q: %w", k, err)
				}
			}
		}
	case []any:
		for i, child := range v {
			if s, ok := child.(string); ok {
				rendered, err := renderEnvString(s, funcs, ctx)
				if err != nil {
					return fmt.Errorf("env value at index %d: %w", i, err)
				}
				v[i] = rendered
			} else {
				if err := renderEnvNode(child, funcs, ctx); err != nil {
					return fmt.Errorf("env at index %d: %w", i, err)
				}
			}
		}
	}
	return nil
}

func renderEnvString(src string, funcs template.FuncMap, ctx map[string]any) (string, error) {
	tpl, err := template.New("env").Funcs(funcs).Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
