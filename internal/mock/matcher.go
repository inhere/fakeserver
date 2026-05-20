package mock

import (
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Matcher wraps a compiled when-expression with its source for error reporting.
// design §3.2 introduces cases[i].when as the conditional branch language;
// design §4.5 documents the runtime-error-as-skip downgrade semantics.
//
// A zero-value Matcher (or one with Program == nil) is the "no when clause"
// sentinel: Evaluate always returns (true, nil).
type Matcher struct {
	Source  string
	Program *vm.Program
}

// CompileMatcher precompiles src as a boolean expression. An empty src
// produces a sentinel Matcher whose Evaluate always returns true (the
// "no when clause" semantics from design §3.2).
//
// Compile errors are wrapped with the source for actionable startup-time
// error messages (config.Validate uses the same wrapping for its when
// syntax pre-check in Task 5).
func CompileMatcher(src string) (*Matcher, error) {
	if src == "" {
		return &Matcher{}, nil
	}
	prog, err := expr.Compile(src, expr.AsBool())
	if err != nil {
		return nil, fmt.Errorf("when %q: %w", src, err)
	}
	return &Matcher{Source: src, Program: prog}, nil
}

// Evaluate runs the compiled program against env. Returns:
//
//   - (true,  nil): empty matcher (no when) OR program evaluated true
//   - (false, nil): program evaluated false
//   - (false, err): runtime error — caller treats as "no match" and warns
//                   (design §4.5 cases-error-handling table)
//
// The third return form is the downgrade path: we deliberately do NOT
// surface the error as a 500, because that would let a single bad case
// definition crash the whole route.
func (m *Matcher) Evaluate(env map[string]any) (bool, error) {
	if m == nil || m.Program == nil {
		return true, nil
	}
	out, err := expr.Run(m.Program, env)
	if err != nil {
		return false, fmt.Errorf("when %q: %w", m.Source, err)
	}
	b, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("when %q: expected bool result, got %T", m.Source, out)
	}
	return b, nil
}
