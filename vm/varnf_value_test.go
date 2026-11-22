package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast raises E_VARNF through PUSH_X_NOT_FOUND (execute.cc), whose value is
// {#-1, name}: a handler sees it as the error list's third element.
func TestVariableNotFoundErrorValue(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		// Nightly fuzz ToastDifferential input (issue #1113).
		{"fuzz input", "try retur/ 0; except e (ANY) return e[1..3]; endtry", `{E_VARNF, "Variable not found: retur", {#-1, "retur"}}`},
		{"read", "try x; except e (ANY) return e[3]; endtry", `{#-1, "x"}`},
		{"assignment base", "try y[1] = 2; except e (ANY) return e[3]; endtry", `{#-1, "y"}`},
		{"other errors keep 0", "try 1 / 0; except e (ANY) return e[3]; endtry", "0"},
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
