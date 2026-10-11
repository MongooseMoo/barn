package engine

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

var elleRows = []string{"property", "contents", "children", "property-definitions", "verb-definitions", "verb-code", "name", "inherited-property"}

func elleObject(kind int) int {
	if kind == 7 {
		return 18
	}
	return 11 + kind
}

func elleWorld(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	for _, id := range []int{0, 11, 12, 13, 14, 15, 16, 17, 18, 30, 31, 32, 40, 41, 42, 43, 44, 45, 46, 47} {
		b := dbstore.NewObjectBuilder(types.ObjID(id))
		b.SetOwner(0)
		b.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagFertile)
		if id == 0 {
			b.SetProperty(":data", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			b.SetProperty("child", dbstore.NewProperty(types.NewObj(types.ObjNothing), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			b.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagRead | dbstore.FlagWrite)
		}
		if id == 11 || id == 18 || id == 0 {
			b.SetProperty("data", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		if id == 0 {
			b.SetProperty("token", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			b.SetProperty("recycled", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			b.SetProperty("waif", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		if id == 30 {
			b.SetProperty("data", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		if id == 13 {
			b.SetProperty("token", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
		if id == 32 {
			b.SetName("0")
		}
		if id >= 42 && id <= 45 {
			b.SetProperty("scratch", dbstore.NewProperty(types.NewInt(1), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
		}
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
	if fault := os.Getenv("BARN_ELLE_FAULT"); fault != "" && fault != "property-reads" {
		t.Fatal("unknown BARN_ELLE_FAULT")
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
	manifest := map[string]any{"seed": seed, "rounds": rounds, "fault": isolationprobe.DisablePropertyReads, "settings": paths, "models": map[string]any{}}
	t.Logf("Elle seed=%d rounds=%d settings=%v fault=%t", seed, rounds, paths, isolationprobe.DisablePropertyReads)
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
		var invariantMark builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
			ops := args[0].Elements()
			ctx.PendingEffects = append(ctx.PendingEffects, kernel.PendingEffect{Kind: kernel.PendingEffectAsyncStart, Start: func() {
				for _, op := range ops {
					isolationprobe.Note("operation/" + op.Str())
				}
			}})
			return types.Ok(types.NewInt(0))
		}
		var barrierMu sync.Mutex
		arrived := map[int64]bool{}
		release := make(chan struct{})
		var barrier builtins.BuiltinFunc = func(ctx *builtins.Execution, _ []types.Value) types.Result {
			barrierMu.Lock()
			if arrived[ctx.TaskID] {
				barrierMu.Unlock()
				return types.Ok(types.NewInt(0))
			}
			arrived[ctx.TaskID] = true
			if len(arrived) == 4 {
				close(release)
			}
			barrierMu.Unlock()
			select {
			case <-release:
				return types.Ok(types.NewInt(0))
			case <-time.After(10 * time.Second):
				return types.Err(types.E_QUOTA)
			}
		}
		rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{AdmissionLimit: 4}, 4,
			testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe), effectSlot,
			testBuiltinSlot("elle_barrier", 0, 0, nil, &barrier),
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
			}()
			isolationprobe.Reset()
			id := int64(100000)
			// Single-threaded append/order preflight exercises every row twice. Its
			// recorded reads must contain exactly the declared append tokens in order.
			if model == "list-append" {
				for kind := range elleRows {
					source := elleAppend(kind, kind*2+1) + elleAppend(kind, kind*2+2) + fmt.Sprintf(`if (!equal(#0:read_row(%d,#%d), {%d,%d})) raise(E_INVARG); endif`, kind, elleObject(kind), kind*2+1, kind*2+2)
					elleDrain(t, rt, []*task.Task{elleTask(t, rt, id, source)})
					id++
				}
			} else {
				elleDrain(t, rt, []*task.Task{elleTask(t, rt, id, `#30.data=0; #32.name="0"; set_verb_code(#31,1,{"0;"}); elle_observe("w","property",0); elle_observe("w","name",0); elle_observe("w","verb-code",0);`)})
				id++
			}
			// Four first attempts read the same value before any may commit. Healthy
			// validation retries; the property-read fault deterministically loses appends.
			if model == "list-append" && paths["parallel"] {
				var batch []*task.Task
				for worker := 0; worker < 4; worker++ {
					source := fmt.Sprintf(`v=#11.data; elle_observe("r","property",v); elle_barrier(); #11.data={@v,%d}; elle_observe("append","property",%d);`, 100+worker, 100+worker)
					batch = append(batch, elleTask(t, rt, id, source))
					id++
				}
				elleDrain(t, rt, batch)
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
				elleDrain(t, rt, batch)
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
