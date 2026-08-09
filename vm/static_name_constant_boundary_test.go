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

func sourceWithDistinctStaticNames(operation string, count int, tail string) string {
	var source strings.Builder
	source.WriteString("object = caller_perms(); if (0) ")
	for i := 0; i < count-1; i++ {
		name := fmt.Sprintf("boundary_%03d", i)
		switch operation {
		case "get":
			fmt.Fprintf(&source, "object.%s; ", name)
		case "set":
			fmt.Fprintf(&source, "object.%s = 0; ", name)
		case "call":
			fmt.Fprintf(&source, "object:%s(); ", name)
		default:
			panic("unknown static-name operation: " + operation)
		}
	}
	source.WriteString("endif ")
	source.WriteString(tail)
	return source.String()
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

func assertStaticNameEncoding(t *testing.T, source string, constantCount int, wantOpcode string) {
	t.Helper()
	program, diagnostics := compiler.CompileMOO([]string{source}, BuildVMRegistry())
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}
	if got, want := len(program.Constants), constantCount; got != want {
		t.Fatalf("len(Program.Constants) = %d, want %d", got, want)
	}
	last := program.Constants[constantCount-1]
	if last.Type() != types.TYPE_STR || last.Str() != staticNameBoundary {
		t.Fatalf("Program.Constants[%d] = %v, want %q", constantCount-1, last, staticNameBoundary)
	}
	disassembly := strings.Join(bytecode.Disassemble(program), "\n")
	if !strings.Contains(disassembly, wantOpcode) {
		t.Fatalf("Disassemble() omitted %s:\n%s", wantOpcode, disassembly)
	}
	if strings.Contains(disassembly, "_DYNAMIC") {
		t.Fatalf("static-name source emitted a dynamic-name opcode:\n%s", disassembly)
	}
}

func TestStaticNamesAt255And256DistinctNamesExecuteWithoutTakingDynamicPath(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		tail      string
		compactOp string
		wideOp    string
		want      int64
	}{
		{name: "property get", operation: "get", tail: "return object.edge;", compactOp: "GET_PROP", wideOp: "GET_PROP_WIDE", want: 41},
		{name: "property set", operation: "set", tail: "object.edge = 42; return object.edge;", compactOp: "SET_PROP", wideOp: "SET_PROP_WIDE", want: 42},
		{name: "verb call", operation: "call", tail: "return object:edge();", compactOp: "CALL_VERB", wideOp: "CALL_VERB_WIDE", want: 43},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, boundary := range []struct {
				count      int
				wantOpcode string
			}{
				{count: 255, wantOpcode: tc.compactOp},
				{count: 256, wantOpcode: tc.wideOp},
			} {
				t.Run(fmt.Sprintf("%d names", boundary.count), func(t *testing.T) {
					source := sourceWithDistinctStaticNames(tc.operation, boundary.count, tc.tail)
					assertStaticNameEncoding(t, source, boundary.count, boundary.wantOpcode)
					result := runBytecodeProgram(t, source, staticNameBoundaryStore(t), nil)
					if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != tc.want {
						t.Fatalf("Run() = flow %v value %v error %v, want return %d", result.Flow, result.Val, result.Error, tc.want)
					}
				})
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

func TestNestedPropertyAssignmentsKeepDistinctTemporarySlots(t *testing.T) {
	store := dbstore.NewStore()
	for id, property := range []string{"outer", "inner"} {
		object := dbstore.NewObjectBuilder(types.ObjID(id))
		object.SetOwner(0)
		object.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
		object.SetProperty(property, dbstore.NewProperty(
			types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true,
		))
		if err := store.Add(object.Build()); err != nil {
			t.Fatalf("store.Add(#%d) failed: %v", id, err)
		}
	}

	result := runBytecodeProgram(t, `#0.outer = (#1.inner = 99); return {#0.outer, #1.inner};`, store, nil)
	if result.Flow != types.FlowReturn || result.Val.String() != "{99, 99}" {
		t.Fatalf("nested property assignment = flow %v value %v error %v, want {99, 99}", result.Flow, result.Val, result.Error)
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
