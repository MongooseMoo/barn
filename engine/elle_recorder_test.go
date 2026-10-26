package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestElleRecorderCommitsSuspendedAndExceptionSlices(t *testing.T) {
	store := newBoundaryTestStore(t)
	recorder := newElleRecorder()
	var observe builtins.BuiltinFunc = recorder.observe
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2,
		testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe))
	defer rt.Stop()
	tk := task.NewStateFull(412, 0, compileTestProgram(t, rt.registry,
		`#0.write_value = 7; elle_observe("w", "x", 7); suspend(0); #0.write_value = 9; elle_observe("w", "x", 9); raise(E_DIV);`), 1000000, 30)
	rt.populateContextDependencies(tk.Context)
	tk.Context.IsWizard = true
	tk.IntrinsicEval = true
	tk.StartTime = time.Now().Add(-time.Second)
	rt.QueueTask(tk)
	for pass := 0; pass < 5; pass++ {
		if rt.ProcessReadyTasks() == 0 {
			break
		}
	}
	if tk.Result.Error != types.E_DIV {
		t.Fatalf("native exception missing: %+v", tk.Result)
	}
	path := filepath.Join(t.TempDir(), "history.edn")
	counts, err := recorder.write(path)
	if err != nil {
		t.Fatal(err)
	}
	if counts["ok"] != 2 || counts["fail"] != 0 || counts["info"] != 0 {
		t.Fatalf("slice outcomes: %v", counts)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`[:w "x" 7]`, `[:w "x" 9]`, `:type :invoke`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("missing %s: %s", want, content)
		}
	}
}

func TestElleRecorderDiscardsRetryAndSplitsIrreversiblePublication(t *testing.T) {
	store := newBoundaryTestStore(t)
	recorder := newElleRecorder()
	var observe builtins.BuiltinFunc = recorder.observe
	bump, calls := bumpReadValueLiveOnce(t, store)
	var effect builtins.BuiltinFunc = func(*builtins.Execution, []types.Value) types.Result { return types.Ok(types.None) }
	slot := testBuiltinSlot("elle_effect", 0, 0, nil, &effect)
	slot.Effect = builtins.Irreversible
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2,
		testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe), bump, slot)
	defer rt.Stop()
	tk := elleTask(t, rt, 413, `v=#0.read_value; elle_observe("r","y",v); bump_read_value_live_once(); #0.write_value=7; elle_observe("w","x",7); elle_effect(); #0.write_value=9; elle_observe("w","x",9);`)
	elleDrain(t, rt, []*task.State{tk})
	counts, err := recorder.write(filepath.Join(t.TempDir(), "history.edn"))
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 2 || counts["discarded_attempts"] != 1 || counts["ok"] != 2 || counts["r/y"] != 1 || counts["w/x"] != 2 {
		t.Fatalf("boundary/retry outcomes=%v calls=%d", counts, *calls)
	}
}

func TestElleRecorderTerminalCommitFailureIsNotPublished(t *testing.T) {
	if !isolationprobe.Enabled {
		t.Skip("commit failure outcome hook requires -tags elle")
	}
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{0, 1, 2} {
		if err := store.Add(dbstore.NewObject(id, 0)); err != nil {
			t.Fatal(err)
		}
	}
	recorder := newElleRecorder()
	var observe builtins.BuiltinFunc = recorder.observe
	var sabotage builtins.BuiltinFunc = func(ctx *builtins.Execution, _ []types.Value) types.Result {
		if ec := ctx.StoreTxn.MoveObject(1, 2, 0); ec != types.E_NONE {
			return types.Err(ec)
		}
		if err := store.Recycle(2); err != nil {
			t.Error(err)
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.None)
	}
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2,
		testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe), testBuiltinSlot("elle_sabotage", 0, 0, nil, &sabotage))
	defer rt.Stop()
	tk := elleTask(t, rt, 414, `#0.name="private"; elle_observe("w","name","private"); elle_sabotage();`)
	rt.QueueTask(tk)
	rt.ProcessReadyTasks()
	counts, err := recorder.write(filepath.Join(t.TempDir(), "history.edn"))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Result.Error != types.E_INVIND || counts["fail"] != 1 || counts["ok"] != 0 {
		t.Fatalf("result=%+v outcomes=%v", tk.Result, counts)
	}
	name, ec := store.DirectTxn().ObjectName(0)
	if ec != types.E_NONE || name != "" {
		t.Fatalf("failed publication: name=%q error=%v", name, ec)
	}
}

func TestElleRecorderRetainsUnconfirmedAttemptAfterEarlierPublication(t *testing.T) {
	// A publication for this task is insufficient to classify a later attempt.
	recorder := newElleRecorder()
	tk := task.NewStateFull(415, 0, nil, 1000, 30)
	ctx := &builtins.Execution{Context: tk.Context, Task: tk}
	args := []types.Value{types.NewStr("w"), types.NewStr("x"), types.NewInt(7)}
	if result := recorder.observe(ctx, args); result.Flow == types.FlowException {
		t.Fatal(result)
	}
	for _, effect := range ctx.PendingEffects {
		if effect.Kind == task.PendingEffectAsyncStart {
			effect.Start()
		}
	}
	ctx.PendingEffects = nil
	args[2] = types.NewInt(9)
	recorder.observe(ctx, args)
	counts, err := recorder.write(filepath.Join(t.TempDir(), "history.edn"))
	if err != nil {
		t.Fatal(err)
	}
	if counts["ok"] != 1 || counts["info"] != 1 || counts["fail"] != 0 {
		t.Fatalf("outcomes=%v", counts)
	}
}
