package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast pushes an index or range assignment's base variable before its
// indexes, bounds and value (code_gen.cc push_lvalue), updates that pushed
// value, and stores the result into the variable last. An index, bound or
// value that assigns the variable therefore neither changes which value is
// updated nor survives the assignment. Every expectation here is the result
// Toast 2.8.0_7 returns for the same program (issue #1124).
func TestCollectionAssignmentBaseReassignedByOperand(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			"range start assigns base (issue 1124)",
			"l = {1, l = 2, 3, 4}; return `l[l = 22..3] = {\"x\"} ! ANY';",
			"E_RANGE",
		},
		{"range start assigns base", "l = {1, 2, 3, 4}; l[l = 2..3] = {\"x\"}; return l;", `{1, "x", 4}`},
		{"index assigns base", "l = {1, 2, 3}; l[l = 2] = 9; return l;", "{1, 9, 3}"},
		{"string range assigns base", "l = \"abcd\"; l[l = 2..3] = \"x\"; return l;", `"axd"`},
		{"value assigns base", "l = {1, 2, 3}; l[2] = (l = 5); return l;", "{1, 5, 3}"},
		{"value index-assigns base", "l = {1, 2, 3}; l[1] = (l[2] = 7); return l;", "{7, 2, 3}"},
		{"nested index assigns base", "l = {{1, 2}, {3, 4}}; l[1][l = 2] = 9; return l;", "{{1, 9}, {3, 4}}"},
		{"nested range assigns base", "l = {{1, 2, 3}, 4}; l[1][l = 2..3] = {\"y\"}; return l;", `{{1, "y"}, 4}`},
		{"dollar uses held base", "l = {1, 2, 3}; l[$ - (l = 1)] = 0; return l;", "{1, 0, 3}"},
		{"map key assigns base", "l = [\"a\" -> 1]; l[(l = 3) && \"b\"] = 2; return l;", `["a" -> 1, "b" -> 2]`},
		{"other case spelling", "l = {1, 2, 3}; l[L = 2] = 4; return L;", "{1, 4, 3}"},
		{
			"index assignment ticks",
			"l = {1, 2, 3}; t = ticks_left(); l[l = 2] = 9; return {l, t - ticks_left()};",
			"{{1, 9, 3}, 6}",
		},
		{
			"range assignment ticks",
			"l = {1, 2, 3}; t = ticks_left(); l[2..2] = {(l = 4)}; return {l, t - ticks_left()};",
			"{{1, 4, 3}, 6}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runBytecodeProgram(t, test.source, nil, nil)
			if result.Flow != types.FlowReturn || result.Error != types.E_NONE || result.Val.String() != test.want {
				t.Fatalf("result = flow %v, value %v, error %v; want %s", result.Flow, result.Val, result.Error, test.want)
			}
		})
	}
}
