package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast builds a codes list that starts with @expr as an argument list, so a
// call later in it is paused with that list prefix as an operand. Barn's
// leading OP_SPLICE for codes was not named as that prefix, so a task
// suspended there was refused by ExportNativeSnapshot ("no held value for
// native operand kind 2 from *ir.CatchExpr"), issue #1184. Each suspension
// here is checkpointed through Toast's form, and the restored handlers must
// still catch by their evaluated codes.
func TestCheckpointExportsSplicedExceptionCodes(t *testing.T) {
	for _, tc := range []struct {
		source      []string
		checkpoints int
		want        string
	}{
		{[]string{`l = {E_DIV};`, "return `1 / 0 ! @l, suspend(0) => 5';"}, 1, "5"},
		{[]string{`l = {E_DIV};`, "return `1 / 0 ! @l, E_RANGE, suspend(0) => 5';"}, 1, "5"},
		{[]string{`l = {E_DIV};`, `try`, `  return 1 / 0;`, `except (E_PERM)`, `  return 2;`, `except (@l, E_RANGE, suspend(0))`, `  return 3;`, `endtry`}, 1, "3"},
		{[]string{`l = {E_DIV};`, `try`, `  return 1 / 0;`, `except (@l, suspend(0))`, `  return 2;`, `except (@l, E_RANGE, suspend(0))`, `  return 3;`, `endtry`}, 2, "2"},
	} {
		registry := newCheckpointResumeRegistry(t)
		program, diagnostics := registry.Compiler().CompileMOO(tc.source)
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		results, _, checkpoints := runCheckpointResumeCase(t, registry, program, checkpointSuspendedVM, sourceFuzzTicks, true)
		if checkpoints != tc.checkpoints || len(results) != 1 || results[0].Flow != types.FlowReturn || results[0].Val.String() != tc.want {
			t.Errorf("%q: %d checkpoints, results %v; want %d checkpoints and one task returning %s", tc.source, checkpoints, results, tc.checkpoints, tc.want)
		}
	}
}
