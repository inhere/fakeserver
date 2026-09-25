package config

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/inhere/fakeserver/internal/tpl"
)

// StrictProblem describes one best-effort static validation issue found by
// StrictValidate. It is intentionally richer than a plain error so callers can
// later render source/route/field/hint separately.
type StrictProblem struct {
	Source     string
	RouteIndex int
	CaseIndex  int
	Method     string
	Path       string
	Field      string
	Message    string
	Hint       string
}

func (p StrictProblem) Error() string {
	var sb strings.Builder
	if p.Source != "" {
		sb.WriteString(p.Source)
		sb.WriteString(" ")
	}
	sb.WriteString(fmt.Sprintf("routes[%d] %s %s %s: %s", p.RouteIndex, p.Method, p.Path, p.Field, p.Message))
	if p.CaseIndex >= 0 {
		sb.WriteString(fmt.Sprintf(" (case %d)", p.CaseIndex))
	}
	if p.Hint != "" {
		sb.WriteString("\nhint: ")
		sb.WriteString(p.Hint)
	}
	return sb.String()
}

// StrictValidate performs best-effort checks that are too expensive or too
// contextual for Validate: case-level bodyFile existence (StrictBodyFiles) and
// parsing route/case header and body templates, attaching hints for common
// authoring mistakes.
func StrictValidate(cfg *Config) []error {
	if cfg == nil {
		return []error{fmt.Errorf("strict: config is nil")}
	}
	errs := StrictBodyFiles(cfg)
	funcs := tpl.BaseFuncMap(cfg.Server.OSEnvWhitelist)
	for ri, route := range cfg.Routes {
		method := strings.Join(route.Method, ",")
		base := StrictProblem{
			Source:     route.SourceFile,
			RouteIndex: ri,
			CaseIndex:  -1,
			Method:     method,
			Path:       route.Path,
		}
		for k, v := range route.Headers {
			if err := parseTemplateStrict(v, funcs); err != nil {
				p := base
				p.Field = "headers." + k
				p.Message = err.Error()
				p.Hint = tpl.TemplateHint(v, err)
				errs = append(errs, p)
			}
		}
		walkTemplateStrings("body", route.Body, func(field, src string) {
			if err := parseTemplateStrict(src, funcs); err != nil {
				p := base
				p.Field = field
				p.Message = err.Error()
				p.Hint = tpl.TemplateHint(src, err)
				errs = append(errs, p)
			}
		})
		for ci, cs := range route.Cases {
			for k, v := range cs.Headers {
				if err := parseTemplateStrict(v, funcs); err != nil {
					p := base
					p.CaseIndex = ci
					p.Field = fmt.Sprintf("cases[%d].headers.%s", ci, k)
					p.Message = err.Error()
					p.Hint = tpl.TemplateHint(v, err)
					errs = append(errs, p)
				}
			}
			walkTemplateStrings(fmt.Sprintf("cases[%d].body", ci), cs.Body, func(field, src string) {
				if err := parseTemplateStrict(src, funcs); err != nil {
					p := base
					p.CaseIndex = ci
					p.Field = field
					p.Message = err.Error()
					p.Hint = tpl.TemplateHint(src, err)
					errs = append(errs, p)
				}
			})
		}
	}
	return errs
}

// StrictBodyFiles reports case-level bodyFile targets that cannot be stat'ed.
// Route-level bodyFile is already a hard error in Validate; case-level ones are
// only checked here (check --strict, doctor) so that a config which used to
// serve fine keeps working outside strict mode.
//
// Relative paths resolve against the source file of the route's own JSON5 file
// (Route.SourceFile), so a case declared in an @included file resolves against
// that include's directory — same rule as the route-level check.
func StrictBodyFiles(cfg *Config) []error {
	if cfg == nil {
		return []error{fmt.Errorf("strict: config is nil")}
	}
	var errs []error
	for ri, route := range cfg.Routes {
		if len(route.Cases) == 0 {
			continue
		}
		for ci, cs := range route.Cases {
			if cs.BodyFile == "" {
				continue
			}
			resolved := resolveRoutePath(cs.BodyFile, route.SourceFile, cfg.SourcePaths)
			if _, err := os.Stat(resolved); err != nil {
				errs = append(errs, StrictProblem{
					Source:     route.SourceFile,
					RouteIndex: ri,
					CaseIndex:  ci,
					Method:     strings.Join(route.Method, ","),
					Path:       route.Path,
					Field:      fmt.Sprintf("cases[%d].bodyFile", ci),
					Message:    fmt.Sprintf("case %s bodyFile %q not found (resolved to %q)", caseLabel(cs.Name, ci), cs.BodyFile, resolved),
				})
			}
		}
	}
	return errs
}

// caseLabel renders a case name for diagnostics, falling back to its index for
// unnamed cases.
func caseLabel(name string, idx int) string {
	if name == "" {
		return fmt.Sprintf("#%d", idx)
	}
	return fmt.Sprintf("%q", name)
}

func parseTemplateStrict(src string, funcs template.FuncMap) error {
	if !strings.Contains(src, "{{") {
		return nil
	}
	_, err := template.New("strict").Funcs(funcs).Parse(src)
	return err
}

func walkTemplateStrings(field string, node any, visit func(field string, src string)) {
	switch v := node.(type) {
	case nil:
		return
	case string:
		visit(field, v)
	case map[string]any:
		for k, child := range v {
			walkTemplateStrings(field+"."+k, child, visit)
		}
	case []any:
		for i, child := range v {
			walkTemplateStrings(fmt.Sprintf("%s[%d]", field, i), child, visit)
		}
	}
}
