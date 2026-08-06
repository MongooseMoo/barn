package vm

import (
	"fmt"
	"strings"
	"testing"

	"barn/compiler"
	dbstore "barn/db/store"
	"barn/types"
)

const staticNameBoundary = "edge"

func sourceWithStaticNameAtConstant255(tail string) string {
	fillers := make([]string, 255)
	for i := range fillers {
		fillers[i] = fmt.Sprintf("%q", fmt.Sprintf("constant-%03d", i))
	}
	return "fillers = {" + strings.Join(fillers, ", ") + "}; " + tail
}

func staticNameBoundaryStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := newBytecodeVerbStore()
	if errCode := store.DefineProperty(0, staticNameBoundary, dbstore.NewProperty(
		types.NewInt(41), 0, dbstore.PropRead|dbstore.PropWrite, false, true,
	)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty(%q) = %v, want E_NONE", staticNameBoundary, errCode)
	}
	if _, errCode := store.AddVerb(0, dbstore.NewVerb(
		staticNameBoundary,
		[]string{staticNameBoundary},
		0,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute,
		dbstore.VerbArgs{},
		[]string{"return 43;"},
	)); errCode != types.E_NONE {
		t.Fatalf("AddVerb(%q) = %v, want E_NONE", staticNameBoundary, errCode)
	}
	return store
}

func assertStaticNameUsesConstant255(t *testing.T, source string) {
	t.Helper()
	program, diagnostics := compiler.CompileMOO([]string{source}, BuildVMRegistry())
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}
	if got, want := len(program.Constants), 256; got != want {
		t.Fatalf("len(Program.Constants) = %d, want %d", got, want)
	}
	last := program.Constants[255]
	if last.Type() != types.TYPE_STR || last.Str() != staticNameBoundary {
		t.Fatalf("Program.Constants[255] = %v, want %q", last, staticNameBoundary)
	}
}

func TestStaticNamesAtConstant255ExecuteWithoutTakingDynamicPath(t *testing.T) {
	tests := []struct {
		name string
		tail string
		want int64
	}{
		{name: "property get", tail: "object = caller_perms(); return object.edge;", want: 41},
		{name: "property set", tail: "object = caller_perms(); object.edge = 42; return object.edge;", want: 42},
		{name: "verb call", tail: "object = caller_perms(); return object:edge();", want: 43},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := sourceWithStaticNameAtConstant255(tc.tail)
			assertStaticNameUsesConstant255(t, source)
			result := runBytecodeProgram(t, source, staticNameBoundaryStore(t), nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != tc.want {
				t.Fatalf("Run() = flow %v value %v error %v, want return %d", result.Flow, result.Val, result.Error, tc.want)
			}
		})
	}
}

func TestDynamicPropertyAndVerbNamesRemainExecutable(t *testing.T) {
	tests := []struct {
		name string
		code string
		want int64
	}{
		{name: "property get", code: `object = caller_perms(); name = "edge"; return object.(name);`, want: 41},
		{name: "property set", code: `object = caller_perms(); name = "edge"; object.(name) = 44; return object.edge;`, want: 44},
		{name: "verb call", code: `object = caller_perms(); name = "edge"; return object:(name)();`, want: 43},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runBytecodeProgram(t, tc.code, staticNameBoundaryStore(t), nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != tc.want {
				t.Fatalf("Run() = flow %v value %v error %v, want return %d", result.Flow, result.Val, result.Error, tc.want)
			}
		})
	}
}
