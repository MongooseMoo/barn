package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast's OP_CALL_VERB raises E_VERBNF through PUSH_X_NOT_FOUND (execute.cc):
// the message names the receiver and verb, and the value is {receiver, verb}.
func TestVerbNotFoundErrorValue(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		// Nightly fuzz ToastDifferential input (issue #1203).
		{"fuzz input", "try return #0:look(); except e (ANY) return e[1..3]; endtry", `{E_VERBNF, "Verb not found: #0:look()", {#0, "look"}}`},
		{"computed name", `try v = "x\"y"; #0:(v)(1); except e (ANY) return e[1..3]; endtry`, `{E_VERBNF, "Verb not found: #0:x\"y()", {#0, "x\"y"}}`},
		{"repeated call site", "r = {}; for i in [1..2] try #0:nope(); except e (ANY) r = {@r, e[3]}; endtry endfor return r;", `{{#0, "nope"}, {#0, "nope"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runBytecodeProgram(t, test.source, newBytecodeVerbStore(), nil)
			if result.Flow != types.FlowReturn || result.Error != types.E_NONE || result.Val.String() != test.want {
				t.Fatalf("result = flow %v, value %v, error %v; want %s", result.Flow, result.Val, result.Error, test.want)
			}
		})
	}
}
