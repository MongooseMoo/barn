package vm

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestCompilerTemporariesDoNotAliasSourceVariables(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "generated loop result",
			source: `__loop_result_1__ = 99; while (0) endwhile return __loop_result_1__;`,
		},
		{
			name:   "generated map literal",
			source: `__maplit_1__ = 99; m = ["a" -> 1]; return __maplit_1__;`,
		},
		{
			name:   "fixed nested assignment",
			source: `__nested_val = 99; x = {{1}}; x[1][1] = 2; return __nested_val;`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runBytecodeProgram(t, test.source, nil, nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 99 {
				t.Fatalf("result = flow %v, value %v, error %v; want integer 99", result.Flow, result.Val, result.Error)
			}
		})
	}
}

func TestCompilerTemporariesAreHiddenFromVariableNames(t *testing.T) {
	registry := BuildVMRegistry()
	program, diagnostics := registry.Compiler().CompileMOO([]string{
		`visible = ["a" -> 1];`,
		`while (0) endwhile`,
		`return visible;`,
	})
	if len(diagnostics) != 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	if !slices.Equal(program.VarNames, []string{"visible"}) {
		t.Fatalf("VarNames = %q; want only source variable visible", program.VarNames)
	}
	if program.NumLocals <= len(program.VarNames) {
		t.Fatalf("NumLocals = %d, VarNames = %d; want separate internal storage", program.NumLocals, len(program.VarNames))
	}
}
