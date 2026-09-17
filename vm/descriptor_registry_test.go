package vm

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestVMDescriptorsAndDisabledLookup(t *testing.T) {
	for _, caps := range []config.Capabilities{0, config.Core, config.DefaultCapabilities()} {
		r, err := builtins.NewRegistryFromDescriptors(caps, Descriptors())
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"eval", "pass"} {
			if r.Has(name) != (caps&config.Core != 0) {
				t.Fatalf("%s capability", name)
			}
			_, diagnostics := r.Compiler().CompileMOO([]string{"return " + name + "();"})
			if (len(diagnostics) == 0) != r.Has(name) {
				t.Fatalf("%s compiler availability", name)
			}
		}
	}
}

func TestDefaultDescriptorIDsPreserveExistingBytecode(t *testing.T) {
	r := BuildVMRegistry()
	program, diagnostics := r.Compiler().CompileMOO([]string{"return typeof(1);"})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	// Computed independently from the 252 registrations at pre-migration 4eacf59.
	const legacyLayout = "73cd9d376515c5d1fb6682bb2c1aa213fd3d6e54a450a5d322dc3980b9cc272d"
	if got := fmt.Sprintf("%x", program.BuiltinLayout); got != legacyLayout {
		t.Fatalf("default builtin IDs changed: %s", got)
	}
	core, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range Descriptors() {
		if !core.Has(d.Name) {
			continue
		}
		before, _ := r.GetID(d.Name)
		after, _ := core.GetID(d.Name)
		if before != after {
			t.Fatalf("filter reassigned %s from %d to %d", d.Name, before, after)
		}
	}
}

func TestSavedProgramRetainsAndChecksRegistryLayout(t *testing.T) {
	full := BuildVMRegistry()
	program, diagnostics := full.Compiler().CompileMOO([]string{"return typeof(1);"})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	cloned := cloneProgram(program)
	if cloned.BuiltinLayout != program.BuiltinLayout {
		t.Fatal("snapshot lost layout")
	}
	core, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{{Program: cloned, Locals: make([]types.Value, cloned.NumLocals)}}}
	if _, err := RestoreVMSnapshot(snapshot, dbstore.NewStore(), builtins.NewSession(core, builtins.NoHost()), kernel.NewTaskContext()); err == nil {
		t.Fatal("foreign saved layout accepted")
	}
	if _, err := RestoreVMSnapshot(snapshot, dbstore.NewStore(), builtins.NewSession(full, builtins.NoHost()), kernel.NewTaskContext()); err != nil {
		t.Fatal(err)
	}
}

func TestVMRejectsForeignBuiltinLayout(t *testing.T) {
	full := BuildVMRegistry()
	core, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"return typeof(1);", "return pass();"} {
		program, diagnostics := full.Compiler().CompileMOO([]string{source})
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		result := NewVM(dbstore.NewStore(), builtins.NewSession(core, builtins.NoHost())).Run(program)
		if result.Flow != types.FlowException || result.Error != types.E_INVARG {
			t.Fatalf("foreign layout executed: %+v", result)
		}
	}
}

func TestVMForeignLayoutNonDebugConsumesCall(t *testing.T) {
	full := BuildVMRegistry()
	core, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{"typeof(1)", "typeof(@{1})", "pass()", "pass(1, 2)", "pass(@{1, 2})"} {
		t.Run(call, func(t *testing.T) {
			program, diagnostics := full.Compiler().CompileMOO([]string{"return {99, " + call + ", 77};"})
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			machine := NewVM(dbstore.NewStore(), builtins.NewSession(core, builtins.NoHost()))
			frame := machine.PrepareVerbFrame(program, 0, 0, 0, "test", 0, nil)
			frame.VerbDebug = false
			if machine.CurrentFrame() != frame {
				t.Fatal("test frame is not active")
			}
			result := machine.ExecuteLoop()
			want := types.NewList([]types.Value{types.NewInt(99), types.NewErr(types.E_INVARG), types.NewInt(77)})
			if result.Flow != types.FlowReturn || !result.Val.Equal(want) {
				t.Fatalf("call corrupted continuation: %+v, want %v", result, want)
			}
		})
	}
}

func TestVMOwnedDescriptorAdmissionParity(t *testing.T) {
	descriptors := Descriptors()
	for i := range descriptors {
		if descriptors[i].Name == "eval" || descriptors[i].Name == "pass" {
			descriptors[i].Implementation = func(*builtins.Execution, []types.Value) types.Result { return types.Ok(types.NewInt(42)) }
		}
	}
	r, err := builtins.NewRegistryFromDescriptors(config.DefaultCapabilities(), descriptors)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"eval", "pass"} {
		session := builtins.NewSession(r, builtins.NoHost())
		ctx := session.NewExecution(kernel.NewTaskContext(), nil)
		id, _ := r.GetID(name)
		for _, args := range [][]types.Value{nil, {types.NewInt(1)}, {types.NewStr("return 1;")}} {
			byID := session.CallByIDWithExecution(id, ctx, args)
			byName, _ := session.CallByNameWithExecution(name, ctx, args)
			dynamic, _ := r.Get("call_function")
			byDynamic := dynamic(ctx, append([]types.Value{types.NewStr(name)}, args...))
			if byID.Error != byName.Error || byID.Error != byDynamic.Error || !byID.Val.Equal(byDynamic.Val) {
				t.Fatalf("%s admission differs", name)
			}
		}
	}
}
