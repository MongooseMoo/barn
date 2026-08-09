package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
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

func assertStaticNameUsesConstant255(t *testing.T, source, wantOpcode string) {
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
	disassembly := strings.Join(bytecode.Disassemble(program), "\n")
	if !strings.Contains(disassembly, wantOpcode) {
		t.Fatalf("Disassemble() omitted %s:\n%s", wantOpcode, disassembly)
	}
}

func TestStaticNamesAtConstant255ExecuteWithoutTakingDynamicPath(t *testing.T) {
	tests := []struct {
		name       string
		tail       string
		wantOpcode string
		want       int64
	}{
		{name: "property get", tail: "object = caller_perms(); return object.edge;", wantOpcode: "GET_PROP_WIDE", want: 41},
		{name: "property set", tail: "object = caller_perms(); object.edge = 42; return object.edge;", wantOpcode: "SET_PROP_WIDE", want: 42},
		{name: "verb call", tail: "object = caller_perms(); return object:edge();", wantOpcode: "CALL_VERB_WIDE", want: 43},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := sourceWithStaticNameAtConstant255(tc.tail)
			assertStaticNameUsesConstant255(t, source, tc.wantOpcode)
			result := runBytecodeProgram(t, source, staticNameBoundaryStore(t), nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != tc.want {
				t.Fatalf("Run() = flow %v value %v error %v, want return %d", result.Flow, result.Val, result.Error, tc.want)
			}
		})
	}
}

func TestDynamicPropertyAndVerbNamesRemainExecutable(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		wantOpcode string
		want       int64
	}{
		{name: "property get", code: `object = caller_perms(); name = "edge"; return object.(name);`, wantOpcode: "GET_PROP_DYNAMIC", want: 41},
		{name: "property set", code: `object = caller_perms(); name = "edge"; object.(name) = 44; return object.edge;`, wantOpcode: "SET_PROP_DYNAMIC", want: 44},
		{name: "verb call", code: `object = caller_perms(); name = "edge"; return object:(name)();`, wantOpcode: "CALL_VERB_DYNAMIC", want: 43},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program, diagnostics := compiler.CompileMOO([]string{tc.code}, BuildVMRegistry())
			if len(diagnostics) > 0 {
				t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
			}
			disassembly := strings.Join(bytecode.Disassemble(program), "\n")
			if !strings.Contains(disassembly, tc.wantOpcode) {
				t.Fatalf("Disassemble() omitted %s:\n%s", tc.wantOpcode, disassembly)
			}
			result := runBytecodeProgram(t, tc.code, staticNameBoundaryStore(t), nil)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != tc.want {
				t.Fatalf("Run() = flow %v value %v error %v, want return %d", result.Flow, result.Val, result.Error, tc.want)
			}
		})
	}
}

func TestLegacyDynamicNameSentinelsRemainExecutableAfterPersistence(t *testing.T) {
	object := types.NewObj(0)
	name := types.NewStr(staticNameBoundary)
	value, ok := bytecode.MakeImmediateOpcode(44)
	if !ok {
		t.Fatal("MakeImmediateOpcode(44) failed")
	}
	tests := []struct {
		name    string
		program *bytecode.Program
		want    int64
	}{
		{
			name: "property get",
			program: &bytecode.Program{
				Constants: []types.Value{object, name},
				Code: []byte{
					byte(bytecode.OP_PUSH), 0,
					byte(bytecode.OP_PUSH), 1,
					byte(bytecode.OP_GET_PROP), 0xFF,
					byte(bytecode.OP_RETURN),
				},
			},
			want: 41,
		},
		{
			name: "property set",
			program: &bytecode.Program{
				Constants: []types.Value{object, name},
				Code: []byte{
					byte(value),
					byte(bytecode.OP_PUSH), 0,
					byte(bytecode.OP_PUSH), 1,
					byte(bytecode.OP_SET_PROP), 0xFF,
					byte(bytecode.OP_PUSH), 0,
					byte(bytecode.OP_PUSH), 1,
					byte(bytecode.OP_GET_PROP), 0xFF,
					byte(bytecode.OP_RETURN),
				},
			},
			want: 44,
		},
		{
			name: "verb call",
			program: &bytecode.Program{
				Constants: []types.Value{object, name},
				Code: []byte{
					byte(bytecode.OP_PUSH), 0,
					byte(bytecode.OP_PUSH), 1,
					byte(bytecode.OP_CALL_VERB), 0xFF, 0,
					byte(bytecode.OP_RETURN),
				},
			},
			want: 43,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := staticNameBoundaryStore(t)
			ctx := kernel.NewTaskContext()
			ctx.Player = 0
			ctx.Programmer = 0
			ctx.Store = store
			ctx.Task = task.NewTask(1, 0, ctx.TicksRemaining, 1)
			registry := BuildVMRegistry()
			ctx.Registry = registry
			machine := NewVM(store, registry)
			machine.Context = ctx
			result := machine.Run(tc.program)
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != tc.want {
				t.Fatalf("Run(persisted Program) = flow %v value %v error %v, want return %d", result.Flow, result.Val, result.Error, tc.want)
			}
		})
	}
}
