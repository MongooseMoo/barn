package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A handler whose codes are not all error literals holds its evaluated codes
// in a compiler temporary while its guarded code runs, where Toast holds them
// as an operand. A task suspended there was refused by ExportNativeSnapshot
// ("computed exception code has no Toast operand Barn holds"), issue #907.
// Each suspension here is checkpointed through Toast's form, and the restored
// handlers must still catch by their evaluated codes.
func TestCheckpointExportsComputedExceptionCodes(t *testing.T) {
	registry := newCheckpointResumeRegistry(t)
	program, diagnostics := registry.Compiler().CompileMOO([]string{
		`c = E_DIV;`,
		"r = `{suspend(0), 1 / 0} ! c, E_RANGE => 5';",
		`try`,
		`  suspend(0);`,
		`  1 / 0;`,
		`except (c, E_PERM)`,
		`  return {r, 7};`,
		`endtry`,
	})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	results, _, checkpoints := runCheckpointResumeCase(t, registry, program, checkpointSuspendedVM, sourceFuzzTicks, true)
	if checkpoints != 2 || len(results) != 1 || results[0].Flow != types.FlowReturn || results[0].Val.String() != "{5, 7}" {
		t.Fatalf("%d checkpoints, results %v; want 2 checkpoints and one task returning {5, 7}", checkpoints, results)
	}
}
