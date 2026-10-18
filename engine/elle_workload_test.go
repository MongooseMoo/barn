package engine

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

var elleRows = []string{"property", "contents", "children", "property-definitions", "verb-definitions", "verb-code", "name", "inherited-property", "waif", "map-a", "map-b"}

func elleObject(kind int) int {
	switch kind {
	case 7:
		return 18
	case 8:
		return 0 // the row is a property of the WAIF held in #0.elle_waif
	case 9, 10:
		return 0 // the rows are entries "a" and "b" of the map in #0.elle_map
	}
	return 11 + kind
}

func elleWorld(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	for _, id := range []int{0, 11, 12, 13, 14, 15, 16, 17, 18, 30, 31, 32, 40, 41, 42, 43, 44, 45, 46, 47} {
		b := dbstore.NewObjectBuilder(types.ObjID(id))
		b.SetOwner(0)
		b.SetLocation(types.ObjNothing)
		var localNames []string
		define := func(name string, property dbstore.Property) {
			b.SetProperty(name, property)
			if property.View(name).Defined {
				localNames = append(localNames, name)
			}
		}
		b.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagFertile)
		if id == 0 {
			define(":data", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			define("child", dbstore.NewProperty(types.NewObj(types.ObjNothing), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			b.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagRead | dbstore.FlagWrite)
		}
		if id == 11 || id == 18 || id == 0 {
			// #18 later inherits #0.data and owns an override of that slot.
			define("data", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, id != 18))
		}
		if id == 0 {
			define("token", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			define("recycled", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			define("waif", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			define("elle_waif", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			entries := types.NewMap([][2]types.Value{{types.NewStr("a"), types.NewList(nil)}, {types.NewStr("b"), types.NewList(nil)}})
			define("elle_map", dbstore.NewProperty(entries, 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		if id == 30 {
			define("data", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		if id == 13 {
			define("token", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		if id == 32 {
			b.SetName("0")
		}
		if id >= 42 && id <= 45 {
			define("scratch", dbstore.NewProperty(types.NewInt(1), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		// These are fixture-local definitions. Runtime-created inherited/clear
		// slots below remain inherited; portable reload needs real local names.
		b.SetPropOrder(localNames)
		b.SetPropDefsCount(len(localNames))
		if err := store.Add(b.Build()); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []types.ObjID{18, 46, 47} {
		if code := store.ChangeParents(id, []types.ObjID{0}); code != types.E_NONE {
			t.Fatal(code)
		}
	}
	// Create a real clear inherited slot rather than emulating inheritance.
	child, code := store.DirectTxn().CreateObject([]types.ObjID{18}, 0, false)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	if code := store.DirectTxn().SetPropertyValue(0, "child", types.NewObj(child)); code != types.E_NONE {
		t.Fatal(code)
	}
	for _, id := range []types.ObjID{16, 31} {
		elleAddVerb(t, store, id, "code", []string{"0;"})
	}
	elleAddVerb(t, store, 0, "read_row", strings.Split(`
{kind, o} = args;
if (kind == 0) return o.data;
elseif (kind == 7) return #0.child.data;
elseif (kind == 8) return #0.elle_waif.data;
elseif (kind == 9) return #0.elle_map["a"];
elseif (kind == 10) return #0.elle_map["b"];
elseif (kind == 1 || kind == 2)
 result = {}; ids = kind == 1 ? o.contents | children(o);
 for id in (ids) result = {@result, id.token}; endfor
 return result;
elseif (kind == 3 || kind == 4)
 result = {}; names = kind == 3 ? properties(o) | verbs(o);
 for name in (names) result = {@result, toint(name[2..$])}; endfor
 return result;
elseif (kind == 5)
 result = {}; lines = verb_code(o, 1);
 for line in (lines[2..$]) result = {@result, toint(line[1..$-1])}; endfor
 return result;
elseif (kind == 6)
 result = {}; for word in (explode(o.name)) result = {@result, toint(word)}; endfor
 return result;
endif
raise(E_INVARG);`, "\n"))
	elleAddVerb(t, store, 0, "make_waif", []string{"return new_waif();"})
	elleAddVerb(t, store, 0, "memo", []string{"return 1;"})
	// Stable invariant population, disjoint from the append/register keys.
	for id := types.ObjID(42); id <= 45; id++ {
		if code := store.ChangeParents(id, []types.ObjID{0}); code != types.E_NONE {
			t.Fatal(code)
		}
		if code := store.DirectTxn().MoveObject(id, 40, 0); code != types.E_NONE {
			t.Fatal(code)
		}
		if code := store.DirectTxn().SetPropertyValue(id, "data", types.NewInt(1)); code != types.E_NONE {
			t.Fatal(code)
		}
		elleAddVerb(t, store, id, "scratchV", []string{"return 0;"})
	}
	elleAddVerb(t, store, 0, "invariants", strings.Split(`
rooms = {#40, #41}; tokens = {#42, #43, #44, #45}; seen = {};
for room in (rooms)
 for obj in (room.contents)
  if (!valid(obj) || obj.location != room || obj in seen) raise(E_INVARG); endif
  seen = {@seen, obj};
 endfor
endfor
for token in (tokens)
 if (!(token in seen)) raise(E_INVARG); endif
 for p in (parents(token)) if (!(token in children(p))) raise(E_INVARG); endif endfor
endfor
if (length(seen) != length(tokens)) raise(E_INVARG); endif
for p in ({#0, #46, #47})
 for c in (children(p))
  if (!valid(c) || !(p in parents(c))) raise(E_INVARG); endif
 endfor
endfor
for dead in (#0.recycled)
 if (valid(dead)) raise(E_INVARG); endif
 for room in (rooms) if (dead in room.contents) raise(E_INVARG); endif endfor
 for p in ({#0, #46, #47}) if (dead in children(p)) raise(E_INVARG); endif endfor
endfor
return 1;`, "\n"))
	return store
}

func elleAddVerb(t *testing.T, store *dbstore.Store, id types.ObjID, name string, code []string) {
	t.Helper()
	v := dbstore.NewVerb(name, []string{name}, 0, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, code)
	if _, err := store.AddVerb(id, v); err != types.E_NONE {
		t.Fatal(err)
	}
}

func elleTask(t *testing.T, rt *Runtime, id int64, code string) *task.Task {
	t.Helper()
	tk := task.NewTaskFull(id, 0, compileTestProgram(t, rt.registry, code), 10000000, 30)
	rt.populateTaskContextDependencies(tk.Context)
	tk.Context.IsWizard = true
	tk.IntrinsicEval = true
	tk.ForkCreator = rt
	tk.StartTime = time.Now().Add(-time.Second)
	return tk
}

func elleDrain(t *testing.T, rt *Runtime, tasks []*task.Task) {
	t.Helper()
	for _, tk := range tasks {
		rt.QueueTask(tk)
	}
	for pass := 0; pass < 1000; pass++ {
		if rt.ProcessReadyTasks() == 0 {
			for _, tk := range rt.taskManager.GetAllTasks() {
				state := tk.GetState()
				if state != task.TaskCompleted && state != task.TaskKilled {
					t.Fatalf("unsettled task %d: %s", tk.ID, state)
				}
				if tk.Result.Flow == types.FlowException && (tk.Result.Error != types.E_DIV || !elleExpectedExceptions[tk.ID]) {
					t.Fatalf("task %d native error %v stack=%+v", tk.ID, tk.Result.Error, tk.Result.CallStack)
				}
			}
			return
		}
	}
	t.Fatal("Elle workload did not drain")
}

var elleExpectedExceptions = map[int64]bool{}

// elleFaults are the BARN_ELLE_FAULT values. Each removes one protection that a
// part of the workload makes Elle reject every run: a synchronized wave for
// each validation stage, and the write-skew pair for commit exclusion.
var elleFaults = []string{"property-reads", "scalar-reads", "verb-reads", "relationship-reads", "waif-reads", "map-key-reads", "map-key-rebase", "map-key-capture", "commit-exclusion", "checkpoint-live-reads"}

// elleWaves are the read / barrier / append bodies, one per validation stage:
// property value, object scalar (name), verb code, relationship (contents),
// WAIF property, one map entry, two map entries at once, and a map read whose
// index expression writes the same entry. In the two-entry wave, odd
// tokens append to entry "a" and even ones to "b": commits to different entries
// of one map each keep the other's appends only because the map they publish is
// rebuilt on the live one.
var elleWaves = []string{
	`v=#11.data; elle_observe("r","property",v); elle_barrier(WAVE); #11.data={@v,TOKEN}; elle_observe("append","property",TOKEN);`,
	`v=#0:read_row(6,#17); elle_observe("r","name",v); s=#17.name; elle_barrier(WAVE); #17.name=s+" TOKEN"; elle_observe("append","name",TOKEN);`,
	`v=#0:read_row(5,#16); elle_observe("r","verb-code",v); c=verb_code(#16,1); elle_barrier(WAVE); set_verb_code(#16,1,{@c,"TOKEN;"}); elle_observe("append","verb-code",TOKEN);`,
	`v=#0:read_row(1,#12); elle_observe("r","contents",v); elle_barrier(WAVE); x=create(#0); x.token=TOKEN; move(x,#12); elle_observe("append","contents",TOKEN);`,
	`v=#0:read_row(8,#0); elle_observe("r","waif",v); w=#0.elle_waif; elle_barrier(WAVE); w.data={@v,TOKEN}; elle_observe("append","waif",TOKEN);`,
	`v=#0:read_row(9,#0); elle_observe("r","map-a",v); elle_barrier(WAVE); #0.elle_map["a"]={@v,TOKEN}; elle_observe("append","map-a",TOKEN);`,
	`if (TOKEN % 2) v=#0:read_row(9,#0); elle_observe("r","map-a",v); elle_barrier(WAVE); #0.elle_map["a"]={@v,TOKEN}; elle_observe("append","map-a",TOKEN); else v=#0:read_row(10,#0); elle_observe("r","map-b",v); elle_barrier(WAVE); #0.elle_map["b"]={@v,TOKEN}; elle_observe("append","map-b",TOKEN); endif`,
	// Capture the original map before the index writes an empty list. Only the
	// final original-list-plus-token write commits, matching the append ledger.
	`v=#0.elle_map[(#0.elle_map["a"]={}) == {} ? "a" | "a"]; elle_observe("r","map-a",v); elle_barrier(WAVE); #0.elle_map["a"]={@v,TOKEN}; elle_observe("append","map-a",TOKEN);`,
}

func elleRegisterReads() string {
	return `elle_observe("r","property",#30.data); elle_observe("r","name",toint(#32.name)); lines=verb_code(#31,1); line=lines[1]; elle_observe("r","verb-code",toint(line[1..$-1]));`
}

func elleAppend(kind int, n int) string {
	o := elleObject(kind)
	key := elleRows[kind]
	read := fmt.Sprintf(`v = #0:read_row(%d, #%d); elle_observe("r", "%s", v);`, kind, o, key)
	var write string
	switch kind {
	case 0, 7:
		write = fmt.Sprintf(`#%d.data = {@v, %d};`, o, n)
	case 1:
		write = fmt.Sprintf(`x = create(#0); x.token = %d; move(x, #%d);`, n, o)
	case 2:
		write = fmt.Sprintf(`x = create(#%d); x.token = %d;`, o, n)
	case 3:
		write = fmt.Sprintf(`add_property(#%d, "p%d", 0, {#0, "rw"});`, o, n)
	case 4:
		write = fmt.Sprintf(`add_verb(#%d, {#0, "rxd", "v%d"}, {"this", "none", "none"});`, o, n)
	case 5:
		write = fmt.Sprintf(`set_verb_code(#%d, 1, {@verb_code(#%d, 1), "%d;"});`, o, o, n)
	case 6:
		write = fmt.Sprintf(`#%d.name = #%d.name + " %d";`, o, o, n)
	case 8:
		write = fmt.Sprintf(`w = #0.elle_waif; w.data = {@v, %d};`, n)
	case 9, 10:
		write = fmt.Sprintf(`#0.elle_map["%s"] = {@v, %d};`, key[len("map-"):], n)
	}
	return read + write + fmt.Sprintf(`elle_observe("append", "%s", %d);`, key, n)
}

func elleReadAll() string {
	var code strings.Builder
	for kind, key := range elleRows {
		fmt.Fprintf(&code, `elle_observe("r", "%s", #0:read_row(%d, #%d));`, key, kind, elleObject(kind))
	}
	return code.String()
}

func elleCheckpointMark(ctx *builtins.Execution, args []types.Value) types.Result {
	marks := append([]types.Value(nil), args[0].Elements()...)
	ctx.PendingEffects = append(ctx.PendingEffects, kernel.PendingEffect{Kind: kernel.PendingEffectAsyncStart, Start: func() {
		for _, mark := range marks {
			isolationprobe.Note(mark.Str())
		}
	}})
	return types.Ok(types.NewInt(0))
}

func TestElleCheckpointFixtureRoundTrip(t *testing.T) {
	store := elleWorld(t)
	recorder := newElleRecorder()
	var observe builtins.BuiltinFunc = recorder.observe
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{AdmissionLimit: 4}, 4,
		testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe))
	defer rt.Stop()
	source := `w=#0:make_waif(); w.data={}; #0.elle_waif=w;`
	for kind := range elleRows {
		source += elleAppend(kind, 100+2*kind) + elleAppend(kind, 101+2*kind)
	}
	elleDrain(t, rt, []*task.Task{elleTask(t, rt, 1, source)})
	elleCheckpointPreflight(t, rt, store, recorder, "list-append", 2)
}

func elleReturnRows(model string) string {
	if model == "rw-register" {
		return `lines=verb_code(#31,1); line=lines[1]; return {#30.data,toint(#32.name),toint(line[1..$-1])};`
	}
	var rows []string
	for kind := range elleRows {
		rows = append(rows, fmt.Sprintf(`#0:read_row(%d,#%d)`, kind, elleObject(kind)))
	}
	return "return {" + strings.Join(rows, ",") + "};"
}

func elleCheckpointPreflight(t *testing.T, rt *Runtime, store *dbstore.Store, recorder *elleRecorder, model string, id int64) {
	t.Helper()
	original := elleTask(t, rt, id, elleReturnRows(model))
	elleDrain(t, rt, []*task.Task{original})
	capture := store.CaptureCheckpoint(nil)
	defer capture.Close()
	isolationprobe.Note("checkpoint_capture")
	snapshot, _, err := capture.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	capture.Close()
	reloaded := elleCheckpointRead(t, snapshot, recorder, model, id+1)
	if !original.Result.Val.Equal(reloaded) {
		t.Fatalf("portable fixture rows differ: original=%v reloaded=%v", original.Result.Val, reloaded)
	}
}

func elleCheckpointRead(t *testing.T, snapshot dbstore.Snapshot, recorder *elleRecorder, model string, id int64) types.Value {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "checkpoint-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := dbformat.NewWriter(file, snapshot).WriteDatabase(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, err := dbformat.LoadDatabaseReadOnly(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	store, err := loaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var observe builtins.BuiltinFunc = recorder.observe
	var mark builtins.BuiltinFunc = elleCheckpointMark
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{AdmissionLimit: 4}, 4,
		testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe),
		testBuiltinSlot("elle_checkpoint_mark", 1, 1, []int64{int64(types.TYPE_LIST)}, &mark))
	defer rt.Stop()
	read, keys := elleReadAll(), elleRows
	if model == "rw-register" {
		read, keys = elleRegisterReads(), []string{"property", "name", "verb-code"}
	}
	marks := []string{`"checkpoint_reload_read"`}
	for _, key := range keys {
		marks = append(marks, strconv.Quote("checkpoint_read/"+key))
	}
	// Assertions precede the commit-deferred coverage markers. The recorder's
	// normal pending effect publishes these MOO reads as one real transaction.
	source := read + `if (#0:invariants() != 1) raise(E_INVARG); endif elle_checkpoint_mark({` + strings.Join(marks, ",") + `});` + elleReturnRows(model)
	reader := elleTask(t, rt, id, source)
	elleDrain(t, rt, []*task.Task{reader})
	return reader.Result.Val
}

func elleHeldCheckpoint(t *testing.T, capture *dbstore.CheckpointCapture, writer func()) dbstore.Snapshot {
	t.Helper()
	held, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var release sync.Once
	var snapshot dbstore.Snapshot
	var materializeErr error
	isolationprobe.SetPause(func(point string) {
		if point == "checkpoint/object-captured/11" {
			close(held)
			<-resume
		}
	})
	defer isolationprobe.SetPause(nil)
	go func() {
		snapshot, _, materializeErr = capture.Materialize()
		close(done)
	}()
	defer func() { release.Do(func() { close(resume) }); <-done }()
	select {
	case <-held:
	case <-done:
		t.Fatalf("checkpoint completed before held object: %v", materializeErr)
	case <-time.After(10 * time.Second):
		t.Fatal("checkpoint did not reach held numbered object")
	}
	writer()
	release.Do(func() { close(resume) })
	<-done
	if materializeErr != nil {
		t.Fatal(materializeErr)
	}
	return snapshot
}

// Invoked with a new output directory and -tags elle. Small default worlds are
// sufficient for CI; seed and rounds can be increased for local stress/replay.
func TestElleIsolationHistories(t *testing.T) {
	out := os.Getenv("BARN_ELLE_OUTPUT")
	if out == "" {
		t.Skip("set BARN_ELLE_OUTPUT to collect isolation histories")
	}
	if !isolationprobe.Enabled {
		t.Fatal("Elle histories require -tags elle")
	}
	fault := isolationprobe.ActiveFault()
	if fault != "" && !slices.Contains(elleFaults, fault) {
		t.Fatalf("unknown BARN_ELLE_FAULT %q; known: %v", fault, elleFaults)
	}
	elleExpectedExceptions = map[int64]bool{}
	paths := map[string]bool{}
	for _, key := range []string{"parallel", "exclusive", "exclusive_gate", "hot_renewal", "verb_memo", "suspend", "fork"} {
		paths[key] = true
	}
	if selection := os.Getenv("BARN_ELLE_PATHS"); selection != "" {
		for key := range paths {
			paths[key] = false
		}
		for _, key := range strings.Split(selection, ",") {
			if _, ok := paths[key]; !ok {
				t.Fatalf("unknown BARN_ELLE_PATHS entry %q", key)
			}
			paths[key] = true
		}
	}
	if runtime.GOMAXPROCS(0) < 4 {
		t.Fatal("Elle histories require GOMAXPROCS >= 4")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	seed := int64(412)
	rounds := 40
	if value := os.Getenv("BARN_ELLE_SEED"); value != "" {
		var err error
		seed, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
	}
	if value := os.Getenv("BARN_ELLE_ROUNDS"); value != "" {
		var err error
		rounds, err = strconv.Atoi(value)
		if err != nil || rounds < 8 || rounds > 10000 {
			t.Fatal("BARN_ELLE_ROUNDS must be 8..10000")
		}
	}
	if fault != "" {
		// A fault run is the preflight, the synchronized waves, the held pair
		// and the final read, with no random rounds: the anomaly Elle must find
		// is then the one the fault's interleaving made, and its cycle graph
		// stays small enough to plot. The rounds' coverage is not required.
		rounds = 0
	}
	manifest := map[string]any{"seed": seed, "rounds": rounds, "fault": fault, "settings": paths, "models": map[string]any{}}
	t.Logf("Elle seed=%d rounds=%d settings=%v fault=%q", seed, rounds, paths, fault)
	coverage := map[string]uint64{}
	defer func() {
		content, err := json.MarshalIndent(manifest, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(out, "manifest.json"), content, 0600)
		}
		if err != nil {
			t.Error(err)
		}
	}()
	rng := rand.New(rand.NewSource(seed))
	for _, model := range []string{"list-append", "rw-register"} {
		elleExpectedExceptions = map[int64]bool{}
		recorder := newElleRecorder()
		store := elleWorld(t)
		if paths["hot_renewal"] {
			store.MarkHotProperty(11, "data")
		}
		var observe builtins.BuiltinFunc = recorder.observe
		var checkpointMark builtins.BuiltinFunc = elleCheckpointMark
		var effect builtins.BuiltinFunc = func(*builtins.Execution, []types.Value) types.Result { return types.Ok(types.NewInt(0)) }
		effectSlot := testBuiltinSlot("elle_effect", 0, 0, nil, &effect)
		effectSlot.Effect = builtins.Irreversible
		hotReady, hotCommitted := make(chan struct{}), make(chan struct{})
		var hotWait builtins.BuiltinFunc = func(_ *builtins.Execution, _ []types.Value) types.Result {
			close(hotReady)
			select {
			case <-hotCommitted:
				return types.Ok(types.NewInt(0))
			case <-time.After(10 * time.Second):
				return types.Err(types.E_QUOTA)
			}
		}
		var hotRelease builtins.BuiltinFunc = func(ctx *builtins.Execution, _ []types.Value) types.Result {
			select {
			case <-hotReady:
			case <-time.After(10 * time.Second):
				return types.Err(types.E_QUOTA)
			}
			ctx.PendingEffects = append(ctx.PendingEffects, kernel.PendingEffect{Kind: kernel.PendingEffectAsyncStart, Start: func() { close(hotCommitted) }})
			return types.Ok(types.NewInt(0))
		}
		// elle_torn(step) sequences the torn-snapshot pair. 0: the reader waits
		// until the writer's commit holds a timestamp. 1: the reader has read
		// its first row; let the commit publish and wait until it has.
		// 2: the writer announces its publication through a commit effect.
		tornStamped, tornRead, tornPublished := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var tornReadOnce sync.Once
		var torn builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
			wait := tornStamped
			switch args[0].Int() {
			case 1:
				tornReadOnce.Do(func() { close(tornRead) })
				wait = tornPublished
			case 2:
				ctx.PendingEffects = append(ctx.PendingEffects, kernel.PendingEffect{Kind: kernel.PendingEffectAsyncStart, Start: func() { close(tornPublished) }})
				return types.Ok(types.NewInt(0))
			}
			select {
			case <-wait:
				return types.Ok(types.NewInt(0))
			case <-time.After(10 * time.Second):
				return types.Err(types.E_QUOTA)
			}
		}
		var invariantMark builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
			ops := args[0].Elements()
			ctx.PendingEffects = append(ctx.PendingEffects, kernel.PendingEffect{Kind: kernel.PendingEffectAsyncStart, Start: func() {
				for _, op := range ops {
					isolationprobe.Note("operation/" + op.Str())
				}
			}})
			return types.Ok(types.NewInt(0))
		}
		// elle_barrier(wave) holds the first attempt of each of a wave's four
		// tasks until all four have arrived. A retry passes straight through.
		type elleWave struct {
			arrived map[int64]bool
			release chan struct{}
		}
		var barrierMu sync.Mutex
		waves := map[int64]*elleWave{}
		var barrier builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
			barrierMu.Lock()
			wave := waves[args[0].Int()]
			if wave == nil {
				wave = &elleWave{arrived: map[int64]bool{}, release: make(chan struct{})}
				waves[args[0].Int()] = wave
			}
			if wave.arrived[ctx.TaskID] {
				barrierMu.Unlock()
				return types.Ok(types.NewInt(0))
			}
			wave.arrived[ctx.TaskID] = true
			if len(wave.arrived) == 4 {
				close(wave.release)
			}
			barrierMu.Unlock()
			select {
			case <-wave.release:
				return types.Ok(types.NewInt(0))
			case <-time.After(10 * time.Second):
				return types.Err(types.E_QUOTA)
			}
		}
		rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{AdmissionLimit: 4}, 4,
			testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe), effectSlot,
			testBuiltinSlot("elle_checkpoint_mark", 1, 1, []int64{int64(types.TYPE_LIST)}, &checkpointMark),
			testBuiltinSlot("elle_barrier", 1, 1, []int64{int64(types.TYPE_INT)}, &barrier),
			testBuiltinSlot("elle_torn", 1, 1, []int64{int64(types.TYPE_INT)}, &torn),
			testBuiltinSlot("elle_hot_wait", 0, 0, nil, &hotWait),
			testBuiltinSlot("elle_hot_release", 0, 0, nil, &hotRelease),
			testBuiltinSlot("elle_invariant_mark", 1, 1, []int64{int64(types.TYPE_LIST)}, &invariantMark))
		func() {
			defer rt.Stop()
			defer func() {
				counts, err := recorder.write(filepath.Join(out, model+".edn"))
				paths := isolationprobe.Snapshot()
				for key, value := range paths {
					coverage[key] += value
				}
				manifest["models"].(map[string]any)[model] = map[string]any{"outcomes": counts, "paths": paths}
				if err != nil {
					t.Error(err)
				}
				t.Logf("%s outcomes=%v paths=%v", model, counts, isolationprobe.Snapshot())
				if counts["info"] != 0 || counts["fail"] != 0 {
					t.Errorf("unconfirmed publications: %v", counts)
				}
				keys, operation := elleRows, "append/"
				if model == "rw-register" {
					keys, operation = []string{"property", "name", "verb-code"}, "w/"
				}
				for _, key := range keys {
					if counts[operation+key] == 0 {
						t.Errorf("missing committed %s%s", operation, key)
					}
				}
				if fault == "" {
					required := []string{"checkpoint_capture", "checkpoint_writer_overlap", "checkpoint_reload_read"}
					for _, key := range keys {
						required = append(required, "checkpoint_read/"+key)
					}
					for _, key := range required {
						if paths[key] == 0 {
							t.Errorf("%s missing actual committed path: %s", model, key)
						}
					}
				}
			}()
			isolationprobe.Reset()
			id := int64(100000)
			// Single-threaded append/order preflight exercises every row twice. Its
			// recorded reads must contain exactly the declared append tokens in order.
			if model == "list-append" {
				// One WAIF, held by a property, carries the "waif" row for the run.
				elleDrain(t, rt, []*task.Task{elleTask(t, rt, id, `w=#0:make_waif(); w.data={}; #0.elle_waif=w;`)})
				id++
				for kind := range elleRows {
					source := elleAppend(kind, kind*2+1) + elleAppend(kind, kind*2+2) + fmt.Sprintf(`if (!equal(#0:read_row(%d,#%d), {%d,%d})) raise(E_INVARG); endif`, kind, elleObject(kind), kind*2+1, kind*2+2)
					elleDrain(t, rt, []*task.Task{elleTask(t, rt, id, source)})
					id++
				}
			} else {
				elleDrain(t, rt, []*task.Task{elleTask(t, rt, id, `#30.data=0; #32.name="0"; set_verb_code(#31,1,{"0;"}); elle_observe("w","property",0); elle_observe("w","name",0); elle_observe("w","verb-code",0);`)})
				id++
			}
			// Compare every normalized MOO row across a portable reload before
			// concurrency, so invalid fixture metadata cannot masquerade as an
			// isolation failure in the later checkpoint observations.
			elleCheckpointPreflight(t, rt, store, recorder, model, id)
			id += 2
			// Per wave, four first attempts read the same value before any may
			// commit. Healthy validation retries three of them; the fault that
			// removes that row's validation deterministically loses their appends.
			if model == "list-append" && paths["parallel"] {
				for wave, source := range elleWaves {
					var batch []*task.Task
					for worker := 0; worker < 4; worker++ {
						token := 100 + wave*10 + worker
						batch = append(batch, elleTask(t, rt, id, strings.NewReplacer("WAVE", strconv.Itoa(wave), "TOKEN", strconv.Itoa(token)).Replace(source)))
						id++
					}
					elleDrain(t, rt, batch)
				}
				// Write skew: each task reads the row the other appends to. Both are
				// held after validation until both have validated. A healthy commit
				// excludes the second for as long as the first is held, so the first
				// times out, publishes, and the second loses validation and retries.
				// Under the commit-exclusion fault both validate and both publish.
				var skewMu sync.Mutex
				validated, both := 0, make(chan struct{})
				isolationprobe.SetPause(func(point string) {
					if point != "commit/validated" {
						return
					}
					skewMu.Lock()
					validated++
					n := validated
					if n == 2 {
						close(both)
					}
					skewMu.Unlock()
					if n > 2 {
						return
					}
					select {
					case <-both:
					case <-time.After(2 * time.Second):
					}
				})
				elleDrain(t, rt, []*task.Task{
					elleTask(t, rt, id, `v=#0:read_row(7,#18); elle_observe("r","inherited-property",v);`+elleAppend(0, 190)),
					elleTask(t, rt, id+1, `v=#0:read_row(0,#11); elle_observe("r","property",v);`+elleAppend(7, 191)),
				})
				isolationprobe.SetPause(nil)
				id += 2
				// Torn snapshot: one commit appends to two rows and is held once it
				// has its timestamp, before it publishes. The reader's second slice
				// begins its snapshot inside that hold, reads the first row, lets
				// the commit publish, and then reads the second row. It must see
				// both appends or neither.
				var stampedOnce sync.Once
				isolationprobe.SetPause(func(point string) {
					if point != "commit/timestamped" {
						return
					}
					stampedOnce.Do(func() {
						close(tornStamped)
						select {
						case <-tornRead:
						case <-time.After(2 * time.Second):
						}
					})
				})
				// The reader's second slice must start while the writer is still
				// held, so this pair is dispatched without waiting for the batch,
				// as the server loop does; elleDrain then checks how it settled.
				pair := []*task.Task{
					elleTask(t, rt, id, elleAppend(0, 195)+elleAppend(7, 196)+`elle_torn(2);`),
					elleTask(t, rt, id+1, `elle_torn(0); suspend(0); v=#0:read_row(0,#11); elle_observe("r","property",v); elle_torn(1); w=#0:read_row(7,#18); elle_observe("r","inherited-property",w);`),
				}
				for _, tk := range pair {
					rt.QueueTask(tk)
				}
				for deadline := time.Now().Add(30 * time.Second); ; {
					settled := true
					for _, tk := range pair {
						if state := tk.GetState(); state != task.TaskCompleted && state != task.TaskKilled {
							settled = false
						}
					}
					if settled {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("torn-snapshot pair did not settle")
					}
					if rt.DispatchReady() == 0 {
						select {
						case <-rt.ScheduleChanged():
						case <-time.After(5 * time.Millisecond):
						}
					}
				}
				elleDrain(t, rt, nil)
				isolationprobe.SetPause(nil)
				id += 2
				// A real checkpoint is held after the #11 property image is copied.
				// One live transaction then appends both #11 and inherited #18 rows.
				// Timestamp resolution must keep both old images in the reloaded read.
				func() {
					capture := store.CaptureCheckpoint(nil)
					defer capture.Close()
					isolationprobe.Note("checkpoint_capture")
					snapshot := elleHeldCheckpoint(t, capture, func() {
						elleDrain(t, rt, []*task.Task{elleTask(t, rt, id, elleAppend(0, 197)+elleAppend(7, 198)+`elle_checkpoint_mark({"checkpoint_writer_overlap"});`)})
					})
					id++
					capture.Close()
					elleCheckpointRead(t, snapshot, recorder, model, id)
					id++
				}()
			}
			if model == "list-append" && paths["hot_renewal"] {
				// Hold an old reader until a newer hot-key publication has completed.
				elleDrain(t, rt, []*task.Task{
					elleTask(t, rt, id, `elle_hot_wait();`+elleReadAll()),
					elleTask(t, rt, id+1, elleAppend(0, 200)+`elle_hot_release();`),
				})
				id += 2
			}
			for round := 0; round < rounds; round++ {
				var batch []*task.Task
				for worker := 0; worker < 4; worker++ {
					var source string
					for slice := 0; slice < 2; slice++ {
						if round%4 == 0 && paths["exclusive_gate"] {
							source += `elle_effect();`
						}
						if model == "list-append" {
							kind := rng.Intn(len(elleRows))
							token := 1000 + round*100 + worker*10 + slice
							source += elleAppend(kind, token)
							source += elleAppend(rng.Intn(len(elleRows)), 2000000+token)
							if round%5 == 0 && paths["exclusive"] {
								source += `w=#0:make_waif(); w.data=1; #0.waif=w;`
							}
							source += elleReadAll()
						} else {
							token := 1000 + round*100 + worker*10 + slice
							switch rng.Intn(3) {
							case 0:
								source += fmt.Sprintf(`#30.data=%d; elle_observe("w","property",%d);`, token, token)
							case 1:
								source += fmt.Sprintf(`#32.name="%d"; elle_observe("w","name",%d);`, token, token)
							case 2:
								source += fmt.Sprintf(`set_verb_code(#31,1,{"%d;"}); elle_observe("w","verb-code",%d);`, token, token)
							}
							source += elleRegisterReads()
						}
						if slice == 0 && paths["suspend"] {
							source += `suspend(0);`
						}
					}
					if paths["verb_memo"] {
						source += `if (#42:memo() != 1) raise(E_INVARG); endif `
					}
					if round%4 == 0 {
						source += `elle_checkpoint_mark({"checkpoint_writer_overlap"});`
					}
					if round%7 == 0 {
						source += `raise(E_DIV);`
						elleExpectedExceptions[id] = true
					}
					batch = append(batch, elleTask(t, rt, id, source))
					id++
					if worker == 0 {
						// Co-schedule a read-only invariant snapshot with the first mutator.
						batch = append(batch, elleTask(t, rt, id, `ok=#0:invariants(); elle_invariant_mark({"invariant"}); return ok;`))
						id++
					}
					// Concurrent invariant mutator: moves/reparents and nonmonotone writes
					// operate on a fixed population separate from the Elle append keys.
					tok := 42 + worker
					mutation := fmt.Sprintf(`move(#%[1]d,#%[2]d); chparent(#%[1]d,#%[3]d); delete_property(#%[1]d,"scratch"); add_property(#%[1]d,"scratch",1,{#0,"rw"}); set_property_info(#%[1]d,"scratch",{#0,"r"}); delete_verb(#%[1]d,"scratchV"); add_verb(#%[1]d,{#0,"rxd","scratchV"},{"this","none","none"}); clear_property(#%[1]d,"data"); if (!equal(#%[1]d.data,{})) raise(E_INVARG); endif #%[1]d.data=1; #%[1]d.r=%[4]d; #%[1]d.owner=#%[5]d; if (#%[1]d.r != %[4]d || #%[1]d.owner != #%[5]d) raise(E_INVARG); endif x=create(#46); chparent(x,#47); recycle(x); #0.recycled={@#0.recycled,x}; ok=#0:invariants(); elle_invariant_mark({"move","chparent","create","recycle","delete_property","delete_verb","clear_property","flags","owner","permissions","invariant"}); return ok;`, tok, 40+round%2, 46+round%2, round%2, 47*(1-round%2))
					mutation = fmt.Sprintf(`add_verb(#0,{#0,"rxd","ancestor%d"},{"this","none","none"}); `, id) + mutation
					mutation = strings.Replace(mutation, `clear_property(`, fmt.Sprintf(`if (length(verbs(#%d)) != 1) raise(E_INVARG); endif clear_property(`, tok), 1)
					batch = append(batch, elleTask(t, rt, id, strings.ReplaceAll(mutation, "; ", ";\n")))
					id++
				}
				if round%3 == 0 && model == "list-append" && paths["fork"] {
					source := fmt.Sprintf(`fork (0) %s endfork return 1;`, elleAppend(0, 4000000+round))
					batch = append(batch, elleTask(t, rt, id, source))
					id++
				}
				if round%4 == 0 {
					func() {
						capture := store.CaptureCheckpoint(nil)
						defer capture.Close()
						isolationprobe.Note("checkpoint_capture")
						elleDrain(t, rt, batch)
						snapshot, _, err := capture.Materialize()
						if err != nil {
							t.Fatal(err)
						}
						capture.Close()
						elleCheckpointRead(t, snapshot, recorder, model, id)
						id++
					}()
				} else {
					elleDrain(t, rt, batch)
				}
			}
			final := `return #0:invariants();`
			if model == "list-append" {
				final = elleReadAll() + final
			} else {
				final = elleRegisterReads() + final
			}
			elleDrain(t, rt, []*task.Task{elleTask(t, rt, id, final)})
		}()
	}
	manifest["coverage"] = coverage
	if fault != "" {
		return
	}
	for key, selected := range paths {
		if selected && coverage[key] == 0 {
			t.Errorf("missing selected path: %s", key)
		}
	}
	for _, key := range []string{"native_exception", "read_only", "operation/move", "operation/chparent", "operation/create", "operation/recycle", "operation/delete_property", "operation/delete_verb", "operation/clear_property", "operation/flags", "operation/owner", "operation/permissions", "operation/invariant"} {
		if coverage[key] == 0 {
			t.Errorf("missing actual coverage: %s", key)
		}
	}
}
