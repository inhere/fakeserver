package tpl

import (
	"fmt"
	"regexp"
	"strings"
)

var dashedHeaderAccessRE = regexp.MustCompile(`\.request\.headers\.([A-Za-z][A-Za-z0-9]*-[A-Za-z0-9-]+)`)

// TemplateHint returns a concise fix suggestion for common template authoring
// mistakes. Both strict config validation and runtime mock errors use it so
// users see the same guidance before and after startup.
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
