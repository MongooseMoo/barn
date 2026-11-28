package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast's OP_ADD reports E_TYPE through PUSH_TYPE_MISMATCH (execute.cc):
// type_mismatch_string's message and the value {{expected codes}, got code},
// with the expected types chosen from the operand pair. Expected values were
// read from the WSL Toast oracle (ToastStunt 2.8.0_7).
func TestAddTypeMismatchError(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		// Nightly fuzz ToastDifferential input (issue #1202) failed on
		// `"ARGSTR:" + args` with args == {}.
		{"fuzz input", `"ARGSTR:" + {}`, `{E_TYPE, "Type mismatch (expected string; got list)", {{130}, 4}}`},
		{"string + list", `"a" + {1}`, `{E_TYPE, "Type mismatch (expected string; got list)", {{130}, 4}}`},
		{"string + integer", `"a" + 1`, `{E_TYPE, "Type mismatch (expected string; got integer)", {{130}, 0}}`},
		{"integer + string", `1 + "a"`, `{E_TYPE, "Type mismatch (expected integer; got string)", {{0}, 2}}`},
		{"object + string", `#1 + "a"`, `{E_TYPE, "Type mismatch (expected string; got object)", {{130}, 1}}`},
		{"integer + float", `1 + 1.0`, `{E_TYPE, "Type mismatch (expected integer; got float)", {{0}, 9}}`},
		{"float + object", `1.0 + #1`, `{E_TYPE, "Type mismatch (expected float; got object)", {{9}, 1}}`},
		{"object + integer", `#1 + 1`, `{E_TYPE, "Type mismatch (expected integer; got object)", {{0}, 1}}`},
		{"map + float", `[] + 1.5`, `{E_TYPE, "Type mismatch (expected float; got map)", {{9}, 10}}`},
		{"object + object", `#1 + #2`, `{E_TYPE, "Type mismatch (expected integer, float or string; got object)", {{0, 9, 130}, 1}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := "try " + test.source + "; except e (ANY) return e[1..3]; endtry"
			result := runBytecodeProgram(t, source, nil, nil)
			if result.Flow != types.FlowReturn || result.Error != types.E_NONE || result.Val.String() != test.want {
				t.Fatalf("result = flow %v, value %v, error %v; want %s", result.Flow, result.Val, result.Error, test.want)
			}
		})
	}
}
