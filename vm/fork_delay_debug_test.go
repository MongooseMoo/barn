package vm

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// A verb without the d flag skips a fork statement whose delay is rejected.
// Toast's OP_FORK reports the delay with RAISE_TYPE_MISMATCH or RAISE_ERROR,
// which push nothing when debug is off (execute.cc; d427b59 added the break
// for the non-numeric case), so the body does not run in the parent and the
// verb continues after endfork.
func TestNonDebugForkWithRejectedDelaySkipsTheFork(t *testing.T) {
	registry := BuildVMRegistry()
	for _, delay := range []string{`"x"`, "{}", "-1", "-0.5"} {
		t.Run(delay, func(t *testing.T) {
			program, diagnostics := registry.Compiler().CompileMOO([]string{
				`r = {"before"};`,
				"for i in [1..3]",
				"fork (" + delay + ")",
				`r = {@r, "body"};`,
				"endfork",
				"endfor",
				`r = {@r, "after"};`,
				"return r;",
			})
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			machine := NewVM(dbstore.NewStore(), newTestSessionWithTaskManager(registry))
			frame := machine.PrepareVerbFrame(program, 0, 0, 0, "test", 0, nil)
			frame.VerbDebug = false
			if machine.CurrentFrame() != frame {
				t.Fatal("test frame is not active")
			}
			base := frame.BasePointer
			result := machine.ExecuteLoop()
			want := types.NewList([]types.Value{types.NewStr("before"), types.NewStr("after")})
			if result.Flow != types.FlowReturn || !result.Val.Equal(want) {
				t.Fatalf("result = %+v; want return %v", result, want)
			}
			if machine.SP != base {
				t.Fatalf("stack pointer after return = %d; want %d (no value left by the skipped forks)", machine.SP, base)
			}
		})
	}
}
