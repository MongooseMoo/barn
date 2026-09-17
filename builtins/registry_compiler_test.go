package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/config"
)

func TestRegistryRejectsDuplicateCompilerNames(t *testing.T) {
	d := testDescriptor("issue_87_mutable_builtin")
	if _, err := NewRegistryFromDescriptors(config.Core, []Descriptor{d, d}); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	r, err := NewRegistryFromDescriptors(config.Core, []Descriptor{d})
	if err != nil {
		t.Fatal(err)
	}
	compiler := r.Compiler()
	program, diagnostics := compiler.CompileMOO([]string{"return issue_87_mutable_builtin();"})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if compiledRegistryBuiltinID(t, program) != 0 {
		t.Fatal("wrong builtin ID")
	}
	if r.Compiler() != compiler {
		t.Fatal("immutable registry replaced compiler")
	}
}

func compiledRegistryBuiltinID(t *testing.T, program *bytecode.Program) byte {
	t.Helper()
	if len(program.Code) < 3 || bytecode.OpCode(program.Code[0]) != bytecode.OP_CALL_BUILTIN {
		t.Fatalf("compiled bytecode = %v, want CALL_BUILTIN as first instruction", program.Code)
	}
	return program.Code[1]
}
