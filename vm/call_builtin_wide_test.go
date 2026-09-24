package vm

import (
	"bytes"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
)

// Builtin IDs above 255 compile to OP_CALL_BUILTIN_WIDE. graphemes() is one
// of the appended builtins past the byte range.
func TestCallBuiltinWideDispatchesHighIDs(t *testing.T) {
	registry := BuildVMRegistry()
	id, ok := registry.GetID("graphemes")
	if !ok || id <= 0xff {
		t.Fatalf("graphemes ID = %d, %v; want a wide ID", id, ok)
	}
	for _, code := range []string{`return graphemes("ab");`, `return graphemes(@{"ab"});`} {
		prog, diagnostics := registry.Compiler().CompileMOO([]string{code})
		if len(diagnostics) > 0 {
			t.Fatal(diagnostics)
		}
		wide := []byte{byte(bytecode.OP_CALL_BUILTIN_WIDE), byte(id >> 8), byte(id)}
		if !bytes.Contains(prog.Code, wide) {
			t.Fatalf("%s: bytecode %v lacks CALL_BUILTIN_WIDE %d", code, prog.Code, id)
		}
		expectMOO(t, code, `{"a", "b"}`)
	}
}
