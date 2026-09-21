package vm

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestWaifWizardAndProgrammerIntrinsicsAreFalse(t *testing.T) {
	waif := types.NewWaif(10, 1)
	for _, property := range []string{"wizard", "programmer"} {
		t.Run(property, func(t *testing.T) {
			machine := NewVM(nil, nil)
			if err := machine.getWaifProp(waif, property); err != nil {
				t.Fatalf("getWaifProp(%q) failed: %v", property, err)
			}
			value := machine.Pop()
			if value.Type() != types.TYPE_INT || value.Int() != 0 {
				t.Fatalf("waif.%s = %v, want integer 0", property, value)
			}
		})
	}
}

func TestWaifContainmentSeesPrivateWrites(t *testing.T) {
	s := dbstore.NewStore()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	machine := NewVM(s, nil)
	machine.Context = &kernel.TaskContext{Store: s, StoreTxn: tx}
	a, b := types.NewWaif(0, 0), types.NewWaif(0, 0)
	if err := machine.setWaifProp(a, "child", b); err != nil {
		t.Fatal(err)
	}
	if err := machine.setWaifProp(b, "child", a); err == nil {
		t.Fatal("staged recursive containment was accepted")
	}
	if _, exists := a.GetProperty("child"); exists {
		t.Fatal("private containment write escaped before commit")
	}
}

func TestWaifRootCaptureIncludesCachedAndStagedImages(t *testing.T) {
	s := dbstore.NewStore()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	oldChild, newChild := types.NewWaif(0, 0), types.NewWaif(0, 0)
	parent := types.NewWaif(0, 0).SetProperty("child", oldChild)
	if ec := tx.SetWaifProperty(parent, "child", types.NewList([]types.Value{newChild, types.NewAnon(77)})); ec != types.E_NONE {
		t.Fatal(ec)
	}
	machine := NewVM(s, nil)
	machine.Context = &kernel.TaskContext{Store: s, StoreTxn: tx}
	var roots []types.Value
	CollectWaifsFromVM(machine, &roots)
	seen := types.NewWaifSet(roots)
	for _, value := range []types.Value{parent, oldChild, newChild} {
		if !seen.Has(value) {
			t.Fatalf("missing transaction root %v", value)
		}
	}
	anon := make(map[types.ObjID]struct{})
	CollectAnonymousRefsFromVM(machine, anon)
	if _, found := anon[77]; !found {
		t.Fatal("staged anonymous root missing")
	}
}
