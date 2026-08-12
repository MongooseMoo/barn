package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestVariableNamesAreCaseInsensitiveAcrossTypeConstantSpellings(t *testing.T) {
	registry := builtins.NewRegistry()
	program, diagnostics := registry.Compiler().CompileMOO([]string{
		"waif = 41;",
		"ANON = 42;",
		"return {WAIF, anon};",
	})
	if len(diagnostics) != 0 {
		t.Fatalf("compile diagnostics = %v", diagnostics)
	}
	if got, want := program.VarNames, []string{"WAIF", "ANON"}; !equalStrings(got, want) {
		t.Fatalf("variable names = %v, want %v", got, want)
	}

	machine := NewVM(dbstore.NewStore(), registry)
	result := machine.Run(program)
	want := types.NewList([]types.Value{types.NewInt(41), types.NewInt(42)})
	if result.Flow != types.FlowReturn || !result.Val.Equal(want) {
		t.Fatalf("mixed-case variable result = %+v, want %v", result, want)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
