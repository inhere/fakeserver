package mock

import (
	"github.com/expr-lang/expr/vm"
)

// Matcher wraps a compiled when-expression with its source for error reporting.
// design §3.2 introduces cases[i].when as the conditional branch language;
// design §4.5 documents the runtime-error-as-skip downgrade semantics.
//
// A zero-value Matcher (or one with Program == nil) is the "no when clause"
// sentinel: Task 2 will honour this convention in Evaluate by returning
// (true, nil) so missing-when cases always match.
//
// Task 2 fills in CompileMatcher and Evaluate.
type Matcher struct {
	Source  string
	Program *vm.Program
}
