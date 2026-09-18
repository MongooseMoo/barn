package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestCollectPendingFinalizationValuesKeepsNestedWaifAsOneRoot(t *testing.T) {
	store := dbstore.NewStore()
	parent := types.NewWaif(9, 3)
	child := types.NewWaif(9, 3)
	parent.SetProperty("child", child)
	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{{Locals: []types.Value{child, parent}}}

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 1 || got[0].Type() != types.TYPE_WAIF || !got[0].Equal(parent) {
		t.Fatalf("pending roots = %v, want only the parent WAIF", got)
	}
}

func TestSetVarRetainsOverwrittenWaifForFinalization(t *testing.T) {
	store := dbstore.NewStore()
	waif := types.NewWaif(9, 3)
	exec := NewVM(store, nil)
	frame := &StackFrame{
		Program: &bytecode.Program{Code: []byte{0}},
		Locals:  []types.Value{waif},
	}
	exec.pushFrame(frame)
	exec.Push(types.NewInt(0))
	if err := exec.Execute(bytecode.OP_SET_VAR); err != nil {
		t.Fatalf("set variable: %v", err)
	}
	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 1 || !got[0].Equal(waif) {
		t.Fatalf("pending roots after overwrite = %v, want overwritten WAIF", got)
	}
}

func TestVMRootCollectorsTraverseWaifPropertiesAndTaskContext(t *testing.T) {
	store := dbstore.NewStore()
	anonID, errCode := store.DirectTxn().CreateObject(nil, 3, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}
	parent := types.NewWaif(9, 3)
	child := types.NewWaif(9, 3)
	child.SetProperty("anonymous", types.NewAnon(anonID))
	parent.SetProperty("child", child)
	exec := NewVM(store, nil)
	exec.Context = kernel.NewTaskContext()
	exec.Context.TaskLocal = parent

	var waifs []types.Value
	CollectWaifsFromVM(exec, &waifs)
	if len(waifs) != 2 || !waifValueInList(parent, waifs) || !waifValueInList(child, waifs) {
		t.Fatalf("WAIF roots = %v, want parent and nested child", waifs)
	}
	anons := make(map[types.ObjID]struct{})
	CollectAnonymousRefsFromVM(exec, anons)
	if _, ok := anons[anonID]; !ok {
		t.Fatalf("anonymous roots = %v, want nested anonymous #%d", anons, anonID)
	}
}

func waifValueInList(needle types.Value, values []types.Value) bool {
	for _, value := range values {
		if value.Type() == types.TYPE_WAIF && value.Equal(needle) {
			return true
		}
	}
	return false
}
