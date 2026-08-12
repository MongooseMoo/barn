package vm

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestStringIndexPreservesIndividualBytes(t *testing.T) {
	original := "caf\u00e9"

	for index, want := range []byte(original) {
		result := runBytecodeExpr(t, fmt.Sprintf("%q[%d]", original, index+1))
		if result.Flow != types.FlowReturn {
			t.Fatalf("index %d flow = %v, error = %v; want return", index+1, result.Flow, result.Error)
		}
		got := []byte(result.Val.Str())
		if !bytes.Equal(got, []byte{want}) {
			t.Errorf("index %d bytes = %v; want [%d]", index+1, got, want)
		}
	}
}

func TestStringIndexReassemblesNonASCIIStringByteExactly(t *testing.T) {
	result := runBytecodeExpr(t, `"café"[1] + "café"[2] + "café"[3] + "café"[4] + "café"[5]`)
	if result.Flow != types.FlowReturn {
		t.Fatalf("flow = %v, error = %v; want return", result.Flow, result.Error)
	}
	if got, want := []byte(result.Val.Str()), []byte("caf\u00e9"); !bytes.Equal(got, want) {
		t.Fatalf("reassembled bytes = %v; want %v", got, want)
	}
}
