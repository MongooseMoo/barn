package engine

import (
	"reflect"
	"sync"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func beginShutdownForTest(t *testing.T, runtime *Runtime, exec *vm.VM) <-chan struct{} {
	t.Helper()
	method := reflect.ValueOf(runtime).MethodByName("BeginShutdown")
	if !method.IsValid() {
		t.Fatal("Runtime.BeginShutdown is missing")
	}
	arg := reflect.Zero(method.Type().In(0))
	if exec != nil {
		arg = reflect.ValueOf(exec)
	}
	results := method.Call([]reflect.Value{arg})
	if len(results) != 1 {
		t.Fatalf("BeginShutdown returned %d values, want one readiness channel", len(results))
	}
	ready, ok := results[0].Interface().(<-chan struct{})
	if !ok {
		t.Fatalf("BeginShutdown result has type %T, want <-chan struct{}", results[0].Interface())
	}
	return ready
}

func TestBeginShutdownTransfersUnclaimedDeferredRoots(t *testing.T) {
	store := dbstore.NewStore()
	runtime := NewRuntime(store)
	t.Cleanup(runtime.Stop)
	waif := types.NewWaif(9, 3)
	runtime.pendingWaifBatch = []pendingWaifEntry{{waif: waif, ctx: kernel.NewTaskContext()}}
	var mu sync.Mutex
	var handedOff []types.Value
	runtime.SetPendingFinalizationSink(func(values []types.Value) {
		mu.Lock()
		handedOff = append(handedOff, values...)
		mu.Unlock()
	})

	ready := beginShutdownForTest(t, runtime, nil)
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("shutdown roots were not published")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(handedOff) != 1 || !handedOff[0].Equal(waif) {
		t.Fatalf("shutdown handoff = %v, want WAIF root", handedOff)
	}
}

func TestDeferredWaifRecycleShutdownReturnsBeforePublication(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(9)
	root.SetOwner(3)
	root.SetFlags(dbstore.FlagWizard)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add WAIF class: %v", err)
	}
	wizard := dbstore.NewObjectBuilder(3)
	wizard.SetOwner(3)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("add recycle owner: %v", err)
	}
	verb := dbstore.NewVerb(":recycle", []string{":recycle"}, 3,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"shutdown(); recycle_returned();"})
	if _, errCode := store.AddVerb(9, verb); errCode != types.E_NONE {
		t.Fatalf("add recycle verb: %v", errCode)
	}
	runtime := NewRuntime(store)
	t.Cleanup(runtime.Stop)
	returned := make(chan struct{})
	var returnedOnce sync.Once
	runtime.registry.Register("recycle_returned", func(*kernel.TaskContext, []types.Value) types.Result {
		returnedOnce.Do(func() { close(returned) })
		return types.Ok(types.None)
	})
	readyResult := make(chan (<-chan struct{}), 1)
	runtime.registry.SetShutdownFunc(func(ctx *kernel.TaskContext, _ string, _ bool) error {
		exec, _ := ctx.CallerVM.(*vm.VM)
		readyResult <- beginShutdownForTest(t, runtime, exec)
		return nil
	})
	waif := types.NewWaif(9, 3)
	runtime.pendingWaifBatch = []pendingWaifEntry{{waif: waif, ctx: kernel.NewTaskContext()}}
	var handedOff []types.Value
	runtime.SetPendingFinalizationSink(func(values []types.Value) { handedOff = append(handedOff, values...) })

	runtime.flushDeferredGC()
	select {
	case <-returned:
	default:
		t.Fatal(":recycle did not continue after shutdown() returned")
	}
	var ready <-chan struct{}
	select {
	case ready = <-readyResult:
	default:
		t.Fatal("shutdown hook did not receive a publication boundary")
	}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("shutdown boundary did not publish after recycle returned")
	}
	if len(handedOff) != 0 {
		t.Fatalf("already-recycled loaded root was handed off again: %v", handedOff)
	}
}
