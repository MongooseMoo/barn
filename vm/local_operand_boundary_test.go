package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func localBoundaryPrefix(count int) string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("v%d", i)
	}
	return strings.Join(names, " = ") + " = 0; "
}

func TestOneBasedLocalOperandsRepresentSlots254And255(t *testing.T) {
	for _, slot := range []int{254, 255} {
		t.Run(fmt.Sprintf("catch/slot_%d", slot), func(t *testing.T) {
			result := runBytecodeProgram(t, localBoundaryPrefix(slot)+"return `1 / 0 ! E_DIV';", nil, nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_ERR || result.Val.ErrCode() != types.E_DIV {
				t.Fatalf("result = flow %v value %v error %v, want returned E_DIV", result.Flow, result.Val, result.Error)
			}
		})

		t.Run(fmt.Sprintf("except/slot_%d", slot), func(t *testing.T) {
			source := localBoundaryPrefix(slot) +
				"try 1 / 0; except captured (E_DIV) return captured[1]; endtry return 0;"
			result := runBytecodeProgram(t, source, nil, nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_ERR || result.Val.ErrCode() != types.E_DIV {
				t.Fatalf("result = flow %v value %v error %v, want returned E_DIV", result.Flow, result.Val, result.Error)
			}
		})

		t.Run(fmt.Sprintf("fork/slot_%d", slot), func(t *testing.T) {
			source := localBoundaryPrefix(slot) +
				"fork task_id (0) return 0; endfork return task_id;"
			result := runBytecodeProgram(t, source, nil, nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 0 {
				t.Fatalf("result = flow %v value %v error %v, want returned task ID 0", result.Flow, result.Val, result.Error)
			}
		})
	}
}
