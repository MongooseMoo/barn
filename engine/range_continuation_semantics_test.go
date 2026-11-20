package engine

import (
	"fmt"
	"strings"
	"testing"
)

// The native source producer evaluates bounds left-to-right and keeps its
// cursor independent of the user variable. Those states must have a faithful
// Barn representation before a native continuation can be translated.
func TestRangeContinuationSemantics(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
		want string
	}{
		{"bound_order", "x=0; for i in [(x=x*10+1)..(x=x*10+2)] endfor return x;", "{1, 12}"},
		{"binding_after_bounds", "i=99; for i in [1..(bound=i)] break; endfor return bound;", "{1, 99}"},
		{"private_cursor", "seen={}; for i in [1..3] seen={@seen,i}; i=99; endfor return seen;", "{1, {1, 2, 3}}"},
		{"maximum_cursor", "seen={}; for i in [9223372036854775805..9223372036854775807] seen={@seen,i}; i=99; endfor return seen;", "{1, {9223372036854775805, 9223372036854775806, 9223372036854775807}}"},
		{"continue_private_cursor", "seen={}; for i in [1..3] seen={@seen,i}; i=99; continue; endfor return seen;", "{1, {1, 2, 3}}"},
		{"invalid_float_before_binding", "i=99; caught=0; try for i in [1.0..2.0] endfor except (E_TYPE) caught=1; endtry return {caught,i};", "{1, {1, 99}}"},
		{"invalid_empty_float_before_binding", "i=99; caught=0; try for i in [2.0..1.0] endfor except (E_TYPE) caught=1; endtry return {caught,i};", "{1, {1, 99}}"},
		{"invalid_mixed_before_binding", "i=99; caught=0; try for i in [1..2.0] endfor except (E_TYPE) caught=1; endtry return {caught,i};", "{1, {1, 99}}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := newTestRuntimeWithBuiltins(t, newConflictTestStore(t))
			defer runtime.Stop()
			if got := runtime.EvalCommandOutput(0, test.code); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestRangePrivateCursorPreservesLocalCapacity(t *testing.T) {
	var source strings.Builder
	for index := range 253 {
		fmt.Fprintf(&source, "v%d=0;", index)
	}
	source.WriteString("for i in [1..1] endfor return 1;")
	runtime := newTestRuntimeWithBuiltins(t, newConflictTestStore(t))
	defer runtime.Stop()
	if got := runtime.EvalCommandOutput(0, source.String()); got != "{1, 1}" {
		t.Fatalf("previously accepted local capacity changed: %s", got)
	}
}

func TestNamedWhileRetainsConditionValueIncludingFalseExit(t *testing.T) {
	runtime := newTestRuntimeWithBuiltins(t, newConflictTestStore(t))
	defer runtime.Stop()
	got := runtime.EvalCommandOutput(0, "seen={}; i=2; while x(i) seen={@seen,x}; i=i-1; endwhile return {seen,x};")
	if got != "{1, {{2, 1}, 0}}" {
		t.Fatalf("named while condition value: %s", got)
	}
}
