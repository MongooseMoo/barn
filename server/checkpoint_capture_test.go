package server

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestCheckpointCapturedTaskGraphsSurviveResumeKillAndCollection(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard|dbstore.FlagProgrammer)
	for _, name := range []string{":marker", ":self", ":anonymous"} {
		if code := store.DirectTxn().DefineProperty(0, name, dbstore.NewProperty(types.None, 0, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
			t.Fatal(code)
		}
	}
	anon, code := store.DirectTxn().CreateObject(nil, 0, true)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	pendingAnon, code := store.DirectTxn().CreateObject(nil, 0, true)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	waif := types.NewWaif(0, 0).SetProperty("marker", types.NewInt(1))
	waif.SetProperty("self", waif)
	waif.SetProperty("anonymous", types.NewAnon(anon))
	for id, values := range map[types.ObjID]map[string]types.Value{
		anon: {"marker": types.NewInt(1), "cycle": waif}, pendingAnon: {"marker": types.NewInt(7)},
	} {
		for name, value := range values {
			if code := store.DirectTxn().DefineProperty(id, name, dbstore.NewProperty(value, 0, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
				t.Fatal(code)
			}
		}
	}
	graph := types.NewMap([][2]types.Value{{waif, types.NewAnon(anon)}, {types.NewAnon(anon), waif}})
	if code := store.DirectTxn().DefineProperty(0, "graph", dbstore.NewProperty(graph, 0, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	rt := engine.NewRuntimeWithOptions(store, config.Options{AdmissionLimit: 2})
	defer rt.Stop()
	program, diagnostics := rt.Registry().Compiler().CompileMOO([]string{"held = #0.graph; suspend(); return held;"})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	taskIDs := []int64{rt.CreateBackgroundTask(0, program, 0), rt.CreateBackgroundTask(0, program, 0)}
	if processed := rt.ProcessReadyTasks(); processed != 2 {
		t.Fatalf("initial task slices=%d, want two holders", processed)
	}
	for _, id := range taskIDs {
		if rt.GetTask(id).GetState() != task.TaskSuspended {
			t.Fatal("holder did not suspend")
		}
	}
	if outcome := rt.Eval(0, []string{"#0.graph = 0; return 1;"}); outcome.Result.Val.Int() != 1 {
		t.Fatal(outcome)
	}
	store.SetPendingFinalizations([]types.Value{waif, types.NewAnon(anon), types.NewAnon(pendingAnon)})
	server := &Server{store: store, runtime: rt, connManager: NewConnectionManager(0), dbPath: filepath.Join(t.TempDir(), "task-capture.db")}
	held, serializationReleased := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(serializationReleased) })
	defer release()
	checkpoint := make(chan error, 1)
	go func() {
		checkpoint <- server.checkpointWith(checkpointProbeFactory(nil, func() {
			close(held)
			<-serializationReleased
		}, nil), true, checkpointRunning)
	}()
	select {
	case <-held:
	case err := <-checkpoint:
		t.Fatalf("checkpoint failed before serialization hold: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("checkpoint did not reach serialization")
	}
	if code := store.DirectTxn().SetWaifProperty(waif, "marker", types.NewInt(2)); code != types.E_NONE {
		t.Error(code)
	}
	if code := store.DirectTxn().SetPropertyValue(anon, "marker", types.NewInt(2)); code != types.E_NONE {
		t.Error(code)
	}
	if err := rt.ResumeTask(taskIDs[0], types.NewInt(0)); err != nil {
		t.Error(err)
	}
	if processed := rt.ProcessReadyTasks(); processed != 1 {
		t.Errorf("resumed task slices=%d, want 1", processed)
	}
	if err := rt.KillTask(taskIDs[1], 0); err != nil {
		t.Error(err)
	}
	store.TakePendingFinalizations()
	if outcome := rt.Eval(0, []string{"run_gc(); return 1;"}); outcome.Result.Val.Int() != 1 {
		t.Error(outcome)
	}
	if store.DirectTxn().Valid(anon) || store.DirectTxn().Valid(pendingAnon) {
		t.Error("live anonymous roots survived resume/kill and collection")
	}
	release()
	if err := <-checkpoint; err != nil {
		t.Fatal(err)
	}
	loaded, err := dbformat.LoadDatabase(server.dbPath + ".new")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SuspendedTasks) != 2 || len(loaded.AnonymousObjs) != 2 || len(loaded.PendingFinalizations) != 1 {
		t.Fatalf("restored ownership: suspended=%d anonymous=%d pending=%d", len(loaded.SuspendedTasks), len(loaded.AnonymousObjs), len(loaded.PendingFinalizations))
	}
	restored, err := loaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var capturedWaif types.Value
	for _, suspended := range loaded.SuspendedTasks {
		frame := suspended.Snapshot.VM.Frames[0]
		var heldGraph types.Value
		for index, name := range frame.Program.VarNames {
			if name == "held" {
				heldGraph = frame.Locals[index]
			}
		}
		pairs := heldGraph.PairsInInsertionOrder()
		if len(pairs) != 2 {
			t.Fatalf("captured graph=%v", heldGraph)
		}
		var frozenWaif, frozenAnon, reverseAnon, reverseWaif types.Value
		for _, pair := range pairs {
			if pair[0].Type() == types.TYPE_WAIF {
				frozenWaif, frozenAnon = pair[0], pair[1]
			} else if pair[0].Type() == types.TYPE_ANON {
				reverseAnon, reverseWaif = pair[0], pair[1]
			}
		}
		if frozenWaif.Type() != types.TYPE_WAIF || frozenAnon.Type() != types.TYPE_ANON {
			t.Fatalf("captured map key types changed: %v", heldGraph)
		}
		marker, ok := frozenWaif.GetProperty("marker")
		self, selfOK := frozenWaif.GetProperty("self")
		waifAnon, anonOK := frozenWaif.GetProperty("anonymous")
		if !ok || marker.Int() != 1 || !selfOK || !self.Identical(frozenWaif) || !anonOK || !waifAnon.Identical(frozenAnon) || !reverseAnon.Identical(frozenAnon) || !reverseWaif.Identical(frozenWaif) {
			t.Fatal("captured WAIF values, aliases or cycles changed")
		}
		if frozenWaif.WaifIdentity() != waif.WaifIdentity() {
			t.Fatal("WAIF sidecar identity changed")
		}
		if capturedWaif.Type() == types.TYPE_WAIF && !capturedWaif.Identical(frozenWaif) {
			t.Fatal("task snapshots no longer share one WAIF")
		}
		capturedWaif = frozenWaif
		marker, code := restored.DirectTxn().PropertyValue(frozenAnon.ID(), "marker")
		if code != types.E_NONE || marker.Int() != 1 {
			t.Fatalf("captured anonymous marker=%v code=%v", marker, code)
		}
		cycle, code := restored.DirectTxn().PropertyValue(frozenAnon.ID(), "cycle")
		if code != types.E_NONE || !cycle.Identical(frozenWaif) {
			t.Fatal("anonymous-to-WAIF cycle changed")
		}
	}
	pendingMarker, code := restored.DirectTxn().PropertyValue(loaded.PendingFinalizations[0].ID(), "marker")
	if code != types.E_NONE || pendingMarker.Int() != 7 {
		t.Fatalf("pending-only anonymous marker=%v code=%v", pendingMarker, code)
	}
}
