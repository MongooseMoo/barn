package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

func TestRegisterInvalidatesSourceCompiler(t *testing.T) {
	registry := NewRegistry()
	name := "issue_87_mutable_builtin"
	registry.Register(name, func(_ *Execution, _ []types.Value) types.Result {
		return types.Result{}
	})
	source := []string{"return " + name + "();"}

	beforeCompiler := registry.Compiler()
	before, diagnostics := beforeCompiler.CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("compile before re-registration diagnostics = %v", diagnostics)
	}
	beforeID := compiledRegistryBuiltinID(t, before)

	registry.Register(name, func(_ *Execution, _ []types.Value) types.Result {
		return types.Result{}
	})
	afterCompiler := registry.Compiler()
	if afterCompiler == beforeCompiler {
		t.Fatal("Register() did not invalidate the registry source compiler")
	}
	after, diagnostics := afterCompiler.CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("compile after re-registration diagnostics = %v", diagnostics)
	}
	afterID := compiledRegistryBuiltinID(t, after)
	if afterID == beforeID {
		t.Fatalf("builtin ID after re-registration = %d, want a new ID", afterID)
	}
	if want, ok := registry.GetID(name); !ok || int(afterID) != want {
		t.Fatalf("compiled builtin ID = %d, registry ID = %d, found = %v", afterID, want, ok)
	}
}

func compiledRegistryBuiltinID(t *testing.T, program *bytecode.Program) byte {
	t.Helper()
	if len(program.Code) < 3 || bytecode.OpCode(program.Code[0]) != bytecode.OP_CALL_BUILTIN {
		t.Fatalf("compiled bytecode = %v, want CALL_BUILTIN as first instruction", program.Code)
	}
	return program.Code[1]
}
