package tpl

import (
	"errors"
	"strings"
	"testing"
)

func TestTemplateHint_HeaderKeyWithDash(t *testing.T) {
	hint := TemplateHint(`{{ .request.headers.User-Agent }}`, errors.New("bad character '-'"))
	if !strings.Contains(hint, `index .request.headers "User-Agent"`) {
		t.Fatalf("hint=%q", hint)
	}
}

func TestTemplateHint_UnclosedAction(t *testing.T) {
	hint := TemplateHint(`{{ .unclosed`, errors.New("unclosed action"))
	if !strings.Contains(hint, "closing braces") {
		t.Fatalf("hint=%q", hint)
	}
}
