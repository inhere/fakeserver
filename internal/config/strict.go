package config

import (
	"fmt"
	"regexp"
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
// contextual for Validate: parsing route/case header and body templates and
// attaching hints for common authoring mistakes.
func StrictValidate(cfg *Config) []error {
	if cfg == nil {
		return []error{fmt.Errorf("strict: config is nil")}
	}
	funcs := tpl.BaseFuncMap(cfg.Server.OSEnvWhitelist)
	var errs []error
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
				p.Hint = TemplateHint(v, err)
				errs = append(errs, p)
			}
		}
		walkTemplateStrings("body", route.Body, func(field, src string) {
			if err := parseTemplateStrict(src, funcs); err != nil {
				p := base
				p.Field = field
				p.Message = err.Error()
				p.Hint = TemplateHint(src, err)
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
					p.Hint = TemplateHint(v, err)
					errs = append(errs, p)
				}
			}
			walkTemplateStrings(fmt.Sprintf("cases[%d].body", ci), cs.Body, func(field, src string) {
				if err := parseTemplateStrict(src, funcs); err != nil {
					p := base
					p.CaseIndex = ci
					p.Field = field
					p.Message = err.Error()
					p.Hint = TemplateHint(src, err)
					errs = append(errs, p)
				}
			})
		}
	}
	return errs
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

var dashedHeaderAccessRE = regexp.MustCompile(`\.request\.headers\.([A-Za-z][A-Za-z0-9]*-[A-Za-z0-9-]+)`)

// TemplateHint returns a concise fix suggestion for common template authoring
// mistakes. It is exported so runtime mock errors can reuse the same hints.
func TemplateHint(src string, err error) string {
	if m := dashedHeaderAccessRE.FindStringSubmatch(src); len(m) == 2 {
		return fmt.Sprintf(`use {{ index .request.headers "%s" }}`, m[1])
	}
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "function") && strings.Contains(msg, "not defined") {
		return "check function name or docs/fakeserver-design.md template functions"
	}
	if strings.Contains(msg, "unexpected EOF") || strings.Contains(msg, "unclosed") {
		return "check closing braces: {{ ... }}"
	}
	return ""
}
