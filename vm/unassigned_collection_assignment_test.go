package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast pushes an index or range assignment's base variable before it
// generates any index, bound or value (code_gen.cc push_lvalue, EXPR_ASGN), so
// an unassigned base raises E_VARNF first and nothing after it runs.
func TestUnassignedCollectionAssignmentBase(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"index", "return `y[1] = 1 ! ANY';", "E_VARNF"},
		{"range", "return `y[1..2] = 1 ! ANY';", "E_VARNF"},
		{"bad index", "return `y[~\"\"] = 1 ! ANY';", "E_VARNF"},
		{"index error", "return `y[1 / 0] = 1 ! ANY';", "E_VARNF"},
		{"value error", "return `y[1] = 1 / 0 ! ANY';", "E_VARNF"},
		{"range bound error", "return `y[1..1 / 0] = {1} ! ANY';", "E_VARNF"},
		{"range start error before dollar", "return `y[1 / 0..$] = {1} ! ANY';", "E_VARNF"},
		{"nested outer index error", "return `y[1 / 0][2] = 1 ! ANY';", "E_VARNF"},
		{"nested range index error", "return `y[1 / 0][1..2] = {} ! ANY';", "E_VARNF"},
		{
			"index and value not run",
			"log = {}; r = `y[(log = {@log, \"index\"}) && 1] = (log = {@log, \"value\"}) && 1 ! ANY'; return {r, log};",
			"{E_VARNF, {}}",
		},
		{
			"bounds and value not run",
			"log = {}; r = `y[(log = {@log, \"start\"}) && 1..(log = {@log, \"end\"}) && 1] = (log = {@log, \"value\"}) && {1} ! ANY'; return {r, log};",
			"{E_VARNF, {}}",
		},
		{
			"nested indexes and value not run",
			"log = {}; r = `y[(log = {@log, \"outer\"}) && 1][(log = {@log, \"inner\"}) && 2] = (log = {@log, \"value\"}) && 1 ! ANY'; return {r, log};",
			"{E_VARNF, {}}",
		},
		{
			"error names the base",
			"try y[x] = 1; except e (ANY) return e[2]; endtry",
			`"Variable not found: y"`,
		},
		{
			"range error names the base",
			"try y[x..z] = {1}; except e (ANY) return e[2]; endtry",
			`"Variable not found: y"`,
		},
		{
			"assigned base runs index then value",
			"y = {0}; log = {}; y[(log = {@log, \"index\"}) && 1] = (log = {@log, \"value\"}) && 5; return {y, log};",
			`{{5}, {"index", "value"}}`,
		},
		{
			"assigned base runs start, end, then value",
			"y = {0, 0}; log = {}; y[(log = {@log, \"start\"}) && 1..(log = {@log, \"end\"}) && 2] = (log = {@log, \"value\"}) && {7}; return {y, log};",
			`{{7}, {"start", "end", "value"}}`,
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
