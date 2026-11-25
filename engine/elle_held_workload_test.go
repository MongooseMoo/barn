package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Each milestone can be announced from an attempt or from its publication
// callback. The latter prevents an observer from running before the write.
func elleHeldSlot() builtins.Descriptor {
	type milestone struct {
		reached chan struct{}
		once    sync.Once
	}
	var milestones [8]milestone
	for i := range milestones {
		milestones[i].reached = make(chan struct{})
	}
	var step builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
		index, action := args[0].Int(), args[1].Str()
		if index < 0 || index >= int64(len(milestones)) {
			return types.Err(types.E_INVARG)
		}
		point := &milestones[index]
		announce := func() { point.once.Do(func() { close(point.reached) }) }
		switch action {
		case "attempt":
			announce()
		case "publish":
			ctx.PendingEffects = append(ctx.PendingEffects, task.PendingEffect{Kind: task.PendingEffectAsyncStart, Start: announce})
		case "wait":
			select {
			case <-point.reached:
			case <-time.After(10 * time.Second):
				return types.Err(types.E_QUOTA)
			}
		default:
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.None)
	}
	return testBuiltinSlot("elle_held", 2, 2, []int64{int64(types.TYPE_INT), int64(types.TYPE_STR)}, &step)
}

// Drive the asynchronous scheduler so one slice can publish and another can
// start while the held readers retain their original snapshots. Done includes
// settling the actual runtime result and running commit callbacks.
func elleDispatchHeld(t *testing.T, rt *Runtime, tasks ...*Task) {
	t.Helper()
	for _, tk := range tasks {
		tk.Done = make(chan struct{})
		rt.QueueTask(tk)
	}
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for _, tk := range tasks {
		for {
			rt.DispatchReady()
			select {
			case <-tk.Done:
				if tk.Result.Flow == types.FlowException || tk.GetState() != task.TaskCompleted {
					t.Fatalf("held task %d: state=%v result=%+v", tk.ID, tk.GetState(), tk.Result)
				}
			case <-rt.ScheduleChanged():
				continue
			case <-deadline.C:
				t.Fatalf("held task %d did not settle", tk.ID)
			}
			break
		}
	}
}

func TestElleHeldHistories(t *testing.T) {
	out := os.Getenv("BARN_ELLE_OUTPUT")
	if out == "" {
		t.Skip("set BARN_ELLE_OUTPUT to collect held histories")
	}
	if !isolationprobe.Enabled {
		t.Fatal("Elle histories require -tags elle")
	}
	for _, scenario := range []struct {
		name    string
		workers int
		run     func(*testing.T, *Runtime, *dbstore.Store) any
	}{
		{"history-prune", 8, elleHeldPrune},
		{"room-membership", 2, elleHeldMembership},
		{"mixed-contention", 8, elleMixedContention},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := elleWorld(t)
			recorder := newElleRecorder()
			var observe builtins.BuiltinFunc = recorder.observe
			rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{AdmissionLimit: scenario.workers}, scenario.workers,
				testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe), elleHeldSlot())
			defer rt.Stop()
			rt.Session().LoadServerOptionsFromStore(store)
			if err := os.MkdirAll(out, 0700); err != nil {
				t.Fatal(err)
			}
			defer func() {
				counts, err := recorder.write(filepath.Join(out, scenario.name+".edn"))
				if err != nil || counts["ok"] == 0 || counts["info"] != 0 || counts["fail"] != 0 {
					t.Errorf("%s publications %v: %v", scenario.name, counts, err)
				}
			}()
			evidence := scenario.run(t, rt, store)
			data, err := json.MarshalIndent(evidence, "", "  ")
			if err == nil {
				err = os.WriteFile(filepath.Join(out, scenario.name+".json"), data, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			compact, _ := json.Marshal(evidence)
			t.Logf("%s: %s", scenario.name, compact)
		})
	}
}

func elleHeldPrune(t *testing.T, rt *Runtime, store *dbstore.Store) any {
	store.MarkHotProperty(11, "data")
	isolationprobe.Reset()
	old := elleTask(t, rt, 70001, `m=#0.elle_map["a"]; elle_observe("r","map-a",m); elle_held(0,"attempt"); elle_held(3,"wait");
		p=#11.data; elle_observe("r","property",p); n=#0.elle_map["a"]; elle_observe("r","map-a",n); return {m,p,n};`)
	middle := elleTask(t, rt, 70002, `elle_held(1,"wait"); suspend(0); m=#0.elle_map["a"]; elle_observe("r","map-a",m);
		elle_held(2,"attempt"); elle_held(3,"wait"); p=#11.data; elle_observe("r","property",p);
		n=#0.elle_map["a"]; elle_observe("r","map-a",n); return {m,p,n};`)
	// Wait for the middle reader in the slice after publishing milestone 1.
	// This keeps the writer independent of either reader's held transaction.
	const writes, midpoint = 24, 8
	var source strings.Builder
	source.WriteString(`elle_held(0,"wait");`)
	for i := 1; i <= writes; i++ {
		source.WriteString(elleAppend(0, i) + elleAppend(9, i))
		if i == midpoint {
			source.WriteString(`elle_held(1,"publish"); suspend(0); elle_held(2,"wait");`)
		} else {
			source.WriteString(`suspend(0);`)
		}
	}
	source.WriteString(`elle_held(3,"publish");`)
	writer := elleTask(t, rt, 70003, source.String())
	renewedBefore, refusedBefore, _ := store.HotReadStats()
	elleDispatchHeld(t, rt, old, middle, writer)
	renewedAfter, refusedAfter, _ := store.HotReadStats()
	fresh := elleTask(t, rt, 70004, `m=#0.elle_map["a"]; p=#11.data; elle_observe("r","map-a",m); elle_observe("r","property",p); return {m,p,m};`)
	elleDrain(t, rt, []*Task{fresh})
	prunes := isolationprobe.Snapshot()["reader_prune"]
	refusals, renewals := refusedAfter-refusedBefore, renewedAfter-renewedBefore
	observations := map[string]any{"reader_prunes": prunes, "renewal_refusals": refusals, "renewals": renewals, "writes": writes}
	fault := isolationprobe.Fault("hot-map-carry")
	allWrites := make([]any, writes)
	for i := range allWrites {
		allWrites[i] = int64(i + 1)
	}
	for _, check := range []struct {
		name  string
		task  *Task
		count int
	}{{"old", old, 0}, {"middle", middle, midpoint}, {"fresh", fresh, writes}} {
		value, err := elleObserved(check.task.Result.Val)
		if err != nil {
			t.Fatal(err)
		}
		observations[check.name] = value
		prefix := allWrites[:check.count]
		want := []any{prefix, prefix, prefix}
		if fault && check.name != "fresh" {
			want = []any{prefix, allWrites, allWrites}
		}
		if !reflect.DeepEqual(value, want) {
			t.Errorf("%s snapshot after history pruning observed %v, want %v", check.name, value, want)
		}
	}
	if prunes == 0 || (!fault && refusals < 2) || (fault && renewals < 2) {
		t.Errorf("pruning/renewal obligations not reached: %v", observations)
	}
	return observations
}

// This small serial model decides whether the two competing admissions could
// have run in either order, starting with an empty room.
func elleSerialAdmission(candidates [2]int64, scans [2][]any, admitted [2]int64, final []any) bool {
	for _, first := range []int{0, 1} {
		second := 1 - first
		if len(scans[first]) == 0 && admitted[first] == 1 && admitted[second] == 0 &&
			reflect.DeepEqual(scans[second], []any{candidates[first]}) && reflect.DeepEqual(final, scans[second]) {
			return true
		}
	}
	return false
}

func TestElleHeldAdmissionModel(t *testing.T) {
	candidates := [2]int64{42, 43}
	winner := []any{int64(42)}
	if !elleSerialAdmission(candidates, [2][]any{{}, winner}, [2]int64{1, 0}, winner) {
		t.Fatal("rejected a legal admission history")
	}
	if elleSerialAdmission(candidates, [2][]any{{}, {}}, [2]int64{1, 1}, []any{int64(42), int64(43)}) {
		t.Fatal("accepted two admissions to an empty singleton room")
	}
}

func elleHeldMembership(t *testing.T, rt *Runtime, _ *dbstore.Store) any {
	const scan = `seen={}; for o in (#12.contents) seen={@seen,toint(o)}; endfor `
	var tasks [2]*Task
	candidates := [2]int64{42, 43}
	for i, candidate := range candidates {
		body := scan + fmt.Sprintf(`elle_held(%d,"attempt"); elle_held(%d,"wait");
			if (!seen) move(#%d,#12); elle_observe("append","room-admission",%d); return {seen,1}; endif return {seen,0};`, i, 1-i, candidate, candidate)
		tasks[i] = elleTask(t, rt, 71001+int64(i), body)
	}
	elleDispatchHeld(t, rt, tasks[:]...)
	var scans [2][]any
	var admitted [2]int64
	for i, tk := range tasks {
		value, err := elleObserved(tk.Result.Val)
		if err != nil {
			t.Fatal(err)
		}
		result := value.([]any)
		scans[i], admitted[i] = result[0].([]any), result[1].(int64)
	}
	final := elleTask(t, rt, 71003, scan+`elle_observe("r","room-admission",seen); return seen;`)
	elleDrain(t, rt, []*Task{final})
	value, err := elleObserved(final.Result.Val)
	if err != nil {
		t.Fatal(err)
	}
	occupants := value.([]any)
	if !elleSerialAdmission(candidates, scans, admitted, occupants) {
		t.Errorf("room admission has no serial order: candidates=%v scans=%v admitted=%v final=%v", candidates, scans, admitted, occupants)
	}
	if len(occupants) != 1 {
		t.Fatalf("cannot exercise membership changes with occupant set %v", occupants)
	}
	occupant := occupants[0].(int64)
	deletion := elleTask(t, rt, 71004, fmt.Sprintf(`elle_held(2,"wait"); move(#%d,#41); elle_held(3,"publish");`, occupant))
	reader := elleTask(t, rt, 71005, scan+`first=seen; elle_held(2,"attempt"); elle_held(3,"wait"); `+scan+`return {first,seen};`)
	elleDispatchHeld(t, rt, reader, deletion)
	insertion := elleTask(t, rt, 71006, fmt.Sprintf(`elle_held(4,"wait"); move(#%d,#12); elle_held(5,"publish");`, occupant))
	emptyReader := elleTask(t, rt, 71007, scan+`first=seen; elle_held(4,"attempt"); elle_held(5,"wait"); `+scan+`return {first,seen};`)
	elleDispatchHeld(t, rt, emptyReader, insertion)
	deleted, deleteErr := elleObserved(reader.Result.Val)
	inserted, insertErr := elleObserved(emptyReader.Result.Val)
	reopened := elleTask(t, rt, 71008, scan+`return seen;`)
	elleDrain(t, rt, []*Task{reopened})
	reopenedContents, reopenedErr := elleObserved(reopened.Result.Val)
	if deleteErr != nil || insertErr != nil || !reflect.DeepEqual(deleted, []any{occupants, occupants}) ||
		!reflect.DeepEqual(inserted, []any{[]any{}, []any{}}) || reopenedErr != nil || !reflect.DeepEqual(reopenedContents, occupants) {
		t.Errorf("membership changed within held snapshot: deletion=%v (%v) insertion=%v (%v)", deleted, deleteErr, inserted, insertErr)
	}
	return map[string]any{"candidates": candidates, "admission_scans": scans, "admitted": admitted,
		"final_room": occupants, "held_deletion": deleted, "held_insertion": inserted, "after_insertion": reopenedContents}
}

// One burst guarantees actual overlap at both extremes: two writers depend on
// the same value, and six writers stage different entries of one map.
func elleMixedContention(t *testing.T, rt *Runtime, _ *dbstore.Store) any {
	const wait = `for i in [0..7] elle_held(i,"wait"); endfor `
	var tasks [8]*Task
	var final strings.Builder
	final.WriteString(`p=#11.data; elle_observe("r","property",p); if (length(p) != 2 || !(1 in p) || !(2 in p)) raise(E_INVARG); endif `)
	for worker := range tasks {
		ready := fmt.Sprintf(`elle_held(%d,"attempt");`, worker) + wait
		if worker < 2 {
			source := `p=#11.data; elle_observe("r","property",p);` + ready +
				fmt.Sprintf(`#11.data={@p,%d}; elle_observe("append","property",%d);`, worker+1, worker+1)
			tasks[worker] = elleTask(t, rt, 72001+int64(worker), source)
			continue
		}
		key := fmt.Sprintf("distinct-%d", worker)
		read := fmt.Sprintf("p=`#0.elle_map[%q] ! E_RANGE => {}'; elle_observe(\"r\",%q,p);", key, key)
		keyed := fmt.Sprintf(`#0.elle_map[%q]={@p,1}; elle_observe("append",%q,1);`, key, key)
		whole := fmt.Sprintf(`m=#0.elle_map; m[%q]={@p,2}; #0.elle_map=m; elle_observe("append",%q,2);`, key, key)
		tasks[worker] = elleTask(t, rt, 72001+int64(worker), read+ready+keyed+`suspend(0);`+read+whole)
		final.WriteString(read + `if (!equal(p,{1,2})) raise(E_INVARG); endif `)
	}
	elleDispatchHeld(t, rt, tasks[:]...)
	elleDrain(t, rt, []*Task{elleTask(t, rt, 72009, final.String())})
	return map[string]int{"overlapping_workers": len(tasks), "shared_key_writers": 2, "distinct_keys": 6}
}
