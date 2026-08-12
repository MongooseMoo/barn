package compiler

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
)

func TestCompileMOOScopesCacheToRegistry(t *testing.T) {
	source := []string{"return issue_87_registry_target();"}

	first, diagnostics := New(map[string]int{"issue_87_registry_target": 17}).CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("first CompileMOO() diagnostics = %v", diagnostics)
	}
	second, diagnostics := New(map[string]int{"issue_87_registry_target": 23}).CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("second CompileMOO() diagnostics = %v", diagnostics)
	}

	if got := compiledBuiltinID(t, first); got != 17 {
		t.Fatalf("first compiled builtin ID = %d, want 17", got)
	}
	if got := compiledBuiltinID(t, second); got != 23 {
		t.Fatalf("second compiled builtin ID = %d, want 23", got)
	}
}

func TestCompilerSnapshotsRegistryLayout(t *testing.T) {
	source := []string{"return issue_87_mutated_target();"}
	registry := map[string]int{"issue_87_mutated_target": 31}
	beforeCompiler := New(registry)

	before, diagnostics := beforeCompiler.CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("CompileMOO() before mutation diagnostics = %v", diagnostics)
	}
	registry["issue_87_mutated_target"] = 47
	after, diagnostics := New(registry).CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("CompileMOO() after mutation diagnostics = %v", diagnostics)
	}

	if got := compiledBuiltinID(t, before); got != 31 {
		t.Fatalf("builtin ID before mutation = %d, want 31", got)
	}
	if got := compiledBuiltinID(t, after); got != 47 {
		t.Fatalf("builtin ID after mutation = %d, want 47", got)
	}
	unchanged, diagnostics := beforeCompiler.CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("original compiler after mutation diagnostics = %v", diagnostics)
	}
	if got := compiledBuiltinID(t, unchanged); got != 31 {
		t.Fatalf("original compiler builtin ID after external mutation = %d, want immutable 31", got)
	}
}

func compiledBuiltinID(t *testing.T, program *bytecode.Program) byte {
	t.Helper()
	if len(program.Code) < 3 || bytecode.OpCode(program.Code[0]) != bytecode.OP_CALL_BUILTIN {
		t.Fatalf("compiled bytecode = %v, want CALL_BUILTIN as first instruction", program.Code)
	}
	return program.Code[1]
}
