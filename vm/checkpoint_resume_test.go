package vm

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// FuzzCheckpointResumeEquivalence requires a generated program to end the
// same way whether or not each of its suspended tasks is written to a
// checkpoint, reloaded and restored before it resumes.
func FuzzCheckpointResumeEquivalence(f *testing.F) {
	fuzzseed.AddChoices(f)
	for _, seed := range checkpointResumeSeeds {
		f.Add(decodeChoices(f, seed))
	}
	f.Fuzz(func(t *testing.T, choices []byte) {
		checkCheckpointResumeEquivalence(t, fuzzseed.StructuredSource(t, choices))
	})
}

// checkpointResumeSeeds are generator choice lists whose programs suspend,
// so the property checks them; none of fuzzseed.AddChoices's do. They are
// the first lists, among pseudo-random 96-byte lists (PCG seeded 7, 11), that
// suspended without reaching the seconds budget. The last also forks. A
// change to the generator changes their programs, so it needs a new scan.
var checkpointResumeSeeds = []string{
	"d7391a52ba95d2be5572c3cb8d7033221298695e3c25a7cc4d78ee798c2ed0f9835c0df753b95ffdecd5b48f89c530ecf06fdd8dc0caf87e8721478eaad9346b7adfda25412c96a5694b025f445df03c54702e2724854aae9e290de1e60e980f",
	"026d28044c6507c82c29d0684f74bfed43b3eea7a452b89b549910d50f9de47f9372706f2331a81a8306d039c6f09d23865930148159cafe8fd8a80c8161c4c6d72964e491149e651b9bbb7a46bd50c9477df964af74a06883de6f2e38a8859e",
	"925aa10c35fe86f1fbd8efbe03866507ac67f82bddf9edbfd09f0c43d099feeb875de0018e3ade09a6998b892f8e0e27433f88ba08aedad8d9db3272bea3436ce2afa21c7edcb426a9d25ca0e13d6442dc696ce349faa02cab5734b93e54c4d3",
	"5abae9ced6e6563e898f1979e23a4b00021e0f6a8b77e5ba7679be4869d74495137a022b5817d4b97db62dfdd2a25746e67cf348cb266307c613a5091083e64f8db6e09cf6c16398ef265048e1cad2814be781dd815b3584b31e6f903ba3ab0a",
}

func decodeChoices(t testing.TB, seed string) []byte {
	t.Helper()
	choices, err := hex.DecodeString(seed)
	if err != nil {
		t.Fatal(err)
	}
	return choices
}

// The seeds must keep reaching a checkpoint, or the property's seed run
// checks nothing.
func TestCheckpointResumeSeedsReachCheckpoint(t *testing.T) {
	for i, seed := range checkpointResumeSeeds {
		source := fuzzseed.StructuredSource(t, decodeChoices(t, seed))
		difference, skip := checkpointResumeDifference(t, source, checkpointSuspendedVM, sourceFuzzTicks, false)
		if skip != "" || difference != "" {
			t.Errorf("seed %d: difference %q, skip %q; want neither\nsource:\n%s", i, difference, skip, source)
		}
	}
}

// checkCheckpointResumeEquivalence fails on a difference and skips, with the
// reason, a case the property cannot check.
func checkCheckpointResumeEquivalence(t *testing.T, source string) {
	t.Helper()
	difference, skip := checkpointResumeDifference(t, source, checkpointSuspendedVM, sourceFuzzTicks, false)
	if skip != "" {
		t.Skip(skip)
	}
	if difference != "" {
		t.Fatalf("checkpointed execution differs: %s\nsource:\n%s", difference, source)
	}
}

// checkpointFunc replaces a task suspended by suspend() with one restored
// from a checkpoint of it and store, and returns the store the rest of the
// execution uses.
type checkpointFunc func(t *testing.T, machine *VM, store *dbstore.Store, queued []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork)

// checkpointResumeDifference runs source twice from the same database: once
// uninterrupted, and once with checkpoint applied at every suspend() yield of
// every task. Both runs use the fuzz driver (sourceFuzzTasks), so forks and
// ticks are handled identically. It compares each task's terminal flow,
// error, exception reason and value, then the persistent state left in the
// store, and returns every difference.
//
// It returns a skip reason instead when no task suspended, so no checkpoint
// was taken, or when a task in either run hit the seconds budget, whose
// timing a checkpoint changes.
//
// Fork yields are not checkpoint boundaries: the runtime queues a forked
// child before its parent continues (engine.Runtime.drainForks), so no
// checkpoint sees a VM stopped at a fork. Already queued children are written
// with the suspended parent and reconstructed from the loaded code/environment.
func checkpointResumeDifference(t *testing.T, source string, checkpoint checkpointFunc, ticks int64, wizard bool) (difference, skip string) {
	t.Helper()
	registry := newCheckpointResumeRegistry(t)
	program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("program does not compile: %v\nsource:\n%s", diagnostics, source)
	}
	plain, plainState, _ := runCheckpointResumeCase(t, registry, program, nil, ticks, wizard)
	resumed, resumedState, suspends := runCheckpointResumeCase(t, registry, program, checkpoint, ticks, wizard)
	for lane, results := range [][]types.Result{plain, resumed} {
		for _, result := range results {
			if wizard && result.Flow != types.FlowReturn {
				t.Fatalf("world history lane %d did not complete: %s; frames=%v\n%s", lane, describeTaskResult(result), result.CallStack, source)
			}
			if result.Flow == types.FlowException && result.Reason == types.ExceptionReasonSecondsBudget {
				return "", "a task hit the seconds budget, which depends on wall-clock time"
			}
		}
	}
	if suspends == 0 {
		return "", "no task suspended, so no checkpoint was taken"
	}

	var differences []string
	if len(plain) != len(resumed) {
		differences = append(differences, fmt.Sprintf("uninterrupted ran %d tasks, checkpointed ran %d", len(plain), len(resumed)))
	}
	for i := 0; i < len(plain) && i < len(resumed); i++ {
		if !sameTaskResult(plain[i], resumed[i]) {
			differences = append(differences, fmt.Sprintf("task %d: uninterrupted %s, checkpointed %s", i+1, describeTaskResult(plain[i]), describeTaskResult(resumed[i])))
		}
	}
	if !sameCheckpointedValue(plainState, resumedState) {
		differences = append(differences, fmt.Sprintf("persistent state: uninterrupted %s, checkpointed %s", plainState, resumedState))
	}
	return strings.Join(differences, "; "), ""
}

// newCheckpointResumeRegistry is newSourceFuzzRegistry, except that a call
// whose result unrepeatableCall names as differing between two runs of the
// same program skips the case in place of the builtin. Such a result differs
// between the uninterrupted and the checkpointed run with or without the
// checkpoint, so it is no evidence about the checkpoint.
func newCheckpointResumeRegistry(t *testing.T) *builtins.Registry {
	t.Helper()
	descriptors := boundedSourceFuzzDescriptors(t.Skipf)
	for i := range descriptors {
		name, call := descriptors[i].Name, descriptors[i].Implementation
		descriptors[i].Implementation = func(ctx *builtins.Execution, args []types.Value) types.Result {
			if reason := unrepeatableCall(name, len(args)); reason != "" {
				t.Skipf("%s() %s, so two runs of the program need not agree; the call was not made, so the case has no result to check", name, reason)
			}
			return call(ctx, args)
		}
	}
	return newRegistryOrFatal(t, descriptors)
}

// unrepeatableCall returns why a call of the named builtin with argc
// arguments can return a different value each time a program runs, or "" if
// its result depends only on the program and the database. A non-wizard's
// call of a wizard-only builtin is E_PERM in both runs, but the world-history
// lane runs as a wizard, so those are named too.
func unrepeatableCall(name string, argc int) string {
	switch name {
	case "time", "ftime", "seconds_left":
		return "reads the clock"
	case "ctime":
		if argc == 0 {
			return "reads the clock"
		}
	case "random", "frandom", "random_bytes":
		return "draws random numbers"
	case "crypt":
		if argc == 1 {
			return "draws a random salt"
		}
	case "queued_tasks":
		return "reports each queued task's start time"
	case "usage":
		return "measures the process"
	case "waif_stats":
		return "counts WAIFs the Go collector has not yet removed"
	case "verb_cache_stats":
		return "measures the verb cache, which a checkpoint reload empties"
	}
	return ""
}

// Issue #908: a queued fork restored from a checkpoint numbered its saved
// body from 1, so its traceback named line 2 where the uninterrupted run named
// verb line 4. Toast numbers it from the header's first line (tasks.cc
// read_task_queue sets first_lineno). In Barn's own frame form the restored
// fork keeps that numbering through a later checkpoint of its own suspension.
func TestCheckpointRestoredForkKeepsVerbLineNumbers(t *testing.T) {
	queued := "x = 1;\nfork (0)\n  x = 2;\n  x = x + \"a\";\nendfork\nsuspend();\nreturn x;"
	resuspended := "x = 1;\nfork (0)\n  x = 2;\n  suspend();\n  x = x + \"a\";\nendfork\nsuspend();\nreturn x;"
	for _, test := range []struct {
		name       string
		source     string
		checkpoint checkpointFunc
	}{
		{"toast form", queued, checkpointSuspendedVM},
		{"barn form", queued, checkpointSuspendedVMInBarnForm},
		{"barn form resuspended", resuspended, checkpointSuspendedVMInBarnForm},
	} {
		t.Run(test.name, func(t *testing.T) {
			difference, skip := checkpointResumeDifference(t, test.source, test.checkpoint, sourceFuzzTicks, false)
			if difference != "" || skip != "" {
				t.Fatalf("difference %q, skip %q; want neither", difference, skip)
			}
		})
	}
}

// Issue #847: a generated program returned ftime() after a suspend, and the
// two runs' clock readings were reported as a checkpoint difference. A call
// that reads the clock or draws random numbers now skips the case.
func TestCheckpointResumeSkipsUnrepeatableCall(t *testing.T) {
	for _, call := range []string{"ftime()", "time()", "ctime()", "random()", "frandom(1.0, 2.0)", "random_bytes(4)", "crypt(\"a\")", "seconds_left()"} {
		var run *testing.T
		t.Run(call, func(t *testing.T) {
			run = t
			difference, skip := checkpointResumeDifference(t, "suspend();\nreturn "+call+";", checkpointSuspendedVM, sourceFuzzTicks, false)
			t.Errorf("the case was not skipped: difference %q, skip %q", difference, skip)
		})
		if !run.Skipped() {
			t.Errorf("%s: a case calling it was checked, not skipped", call)
		}
	}
	difference, skip := checkpointResumeDifference(t, "suspend();\nreturn {ctime(0), crypt(\"a\", \"ab\")};", checkpointSuspendedVM, sourceFuzzTicks, false)
	if difference != "" || skip != "" {
		t.Fatalf("repeatable calls: difference %q, skip %q; want neither", difference, skip)
	}
}

// runCheckpointResumeCase runs program on a fresh copy of the property's
// database with its own session, and returns the task results, the observed
// persistent state, and the number of checkpoints taken.
func runCheckpointResumeCase(t *testing.T, registry *builtins.Registry, program *bytecode.Program, checkpoint checkpointFunc, ticks int64, wizard bool) ([]types.Result, types.Value, int) {
	t.Helper()
	session := newTestSessionWithTaskManager(registry)
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	tasks := &sourceFuzzTasks{nextID: 2, store: newCheckpointResumeStore(t), session: session, checkpoint: checkpoint, ticks: ticks, wizard: wizard}
	results := tasks.runAll(t, program)
	return results, observePersistentState(t, tasks.store, session), tasks.suspends
}

// newCheckpointResumeStore returns #0 and #2, owned by the wizard #2, with a
// readable and writable property p on each, so the generated programs'
// unprivileged property writes have persistent effects.
func newCheckpointResumeStore(t testing.TB) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{0, 2} {
		builder := dbstore.NewObjectBuilder(id)
		builder.SetOwner(2)
		builder.SetLocation(types.ObjNothing)
		builder.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", id, err)
		}
		property := dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)
		if code := store.DirectTxn().DefineProperty(id, "p", property); code != types.E_NONE {
			t.Fatalf("define #%d.p: %v", id, code)
		}
	}
	return store
}

// persistentStateObserver reads, as a wizard, everything a generated program
// can change in the database except property values: each valid object's
// builtin properties, parent, defined properties and verbs. It returns that
// and the {object, name} of every property.
var persistentStateObserver = []string{
	"r = {};",
	"slots = {};",
	"for i in [0..toint(max_object())]",
	"  o = toobj(i);",
	"  if (valid(o))",
	"    v = {o, o.name, o.owner, o.location, o.contents, parent(o), o.wizard, o.programmer, o.r, o.w, o.f};",
	"    for p in (properties(o))",
	"      v = {@v, p, property_info(o, p)};",
	"      slots = {@slots, {o, p}};",
	"    endfor",
	"    for verb in (verbs(o))",
	"      v = {@v, verb, verb_info(o, verb), verb_args(o, verb), verb_code(o, verb)};",
	"    endfor",
	"    r = {@r, v};",
	"  endif",
	"endfor",
	"return {r, slots};",
}

// persistentPropertyReader returns the value of property args[2] of args[1].
var persistentPropertyReader = []string{"return args[1].(args[2]);"}

// observePersistentState returns the observer's description of the database
// followed by each property's value. The values are the only part a program
// can make large, and each is read by itself and collected here: a MOO list
// of them is E_QUOTA once together they pass max_list_value_bytes, which is
// no difference between two runs.
func observePersistentState(t *testing.T, store *dbstore.Store, session *builtins.Session) types.Value {
	t.Helper()
	described := runPersistentStateObserver(t, store, session, persistentStateObserver, nil)
	state := []types.Value{described.Get(1)}
	for _, slot := range described.Get(2).Elements() {
		state = append(state, runPersistentStateObserver(t, store, session, persistentPropertyReader, slot.Elements()))
	}
	return types.NewList(state)
}

// runPersistentStateObserver runs lines as a wizard with args and returns
// what they return.
func runPersistentStateObserver(t *testing.T, store *dbstore.Store, session *builtins.Session, lines []string, args []types.Value) types.Value {
	t.Helper()
	program, diagnostics := session.Registry().Compiler().CompileMOO(lines)
	if len(diagnostics) != 0 {
		t.Fatalf("observer does not compile: %v", diagnostics)
	}
	machine := newSourceFuzzVM(store, session, 1, defaultTickLimit)
	machine.Context.IsWizard = true
	result := machine.RunWithVerbContext(program, types.ObjNothing, machine.Context.Player, types.ObjNothing, "", types.ObjNothing, args)
	if result.Flow != types.FlowReturn {
		t.Fatalf("observer ended with flow %v error %v: %v", result.Flow, result.Error, result.Val)
	}
	return result.Val
}

func sameTaskResult(a, b types.Result) bool {
	return a.Flow == b.Flow && a.Error == b.Error && a.Reason == b.Reason && sameCheckpointedValue(a.Val, b.Val)
}

// sameCheckpointedValue is Identical, except that maps compare their pairs in
// traversal order rather than insertion order. A checkpoint writes a map's
// pairs in traversal order and the load inserts them in that order, as Toast's
// dbio_write_var (mapforeach) and dbio_read_var (mapinsert) do, so a map
// survives a checkpoint with its traversal order, not its insertion order.
func sameCheckpointedValue(a, b types.Value) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch a.Type() {
	case types.TYPE_LIST:
		if a.Len() != b.Len() {
			return false
		}
		for i := 1; i <= a.Len(); i++ {
			if !sameCheckpointedValue(a.Get(i), b.Get(i)) {
				return false
			}
		}
		return true
	case types.TYPE_MAP:
		ap, bp := a.Pairs(), b.Pairs()
		if len(ap) != len(bp) {
			return false
		}
		for i := range ap {
			if !sameCheckpointedValue(ap[i][0], bp[i][0]) || !sameCheckpointedValue(ap[i][1], bp[i][1]) {
				return false
			}
		}
		return true
	default:
		return a.Identical(b)
	}
}

func describeTaskResult(result types.Result) string {
	if result.Flow == types.FlowReturn {
		return "return " + result.Val.String()
	}
	return fmt.Sprintf("exception %s reason %v value %s", types.NewErr(result.Error).String(), result.Reason, result.Val.String())
}

// checkpointValueBudget bounds the bytes of task values one checkpoint of
// the property may write: types.ValueBytes summed over every value the
// suspended task and the queued forks persist. A larger checkpoint is
// skipped before it is written.
const checkpointValueBudget = 64 << 20

// checkpointSuspendedVM writes the suspended machine and store to a
// checkpoint the way engine.Runtime.TaskSnapshots does, with the task's frames
// exported to Toast's form, loads it, and restores the task on the loaded
// store the way engine.Runtime.loadSuspendedTask does, through the import of
// Toast's frames. A task ExportNativeSnapshot refuses fails the case: the
// runtime keeps such a task in Barn's frame form, and Toast cannot load the
// checkpoint that holds it. The one refusal by design is
// compiler.ErrPathDependentClear (engine TestExportRefusesPathDependentPushClear):
// that task is written in Barn's frame form, as the runtime writes it, and
// must still resume the same. The restored VM keeps the task's state and tick
// count: a snapshot carries no ticks, and the fuzz driver's budget spans the
// whole task.
func checkpointSuspendedVM(t *testing.T, machine *VM, store *dbstore.Store, forks []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork) {
	t.Helper()
	snapshot := suspendedSnapshot(machine)
	snapshot.CallStack = machine.Task.GetCallStack()
	if len(snapshot.CallStack) != len(snapshot.VM.Frames) {
		t.Fatalf("task has %d activations for %d VM frames; the driver must push each task's root activation", len(snapshot.CallStack), len(snapshot.VM.Frames))
	}
	exported, err := ExportNativeSnapshot(snapshot.VM, snapshot.CallStack, machine.Builtins.Registry().Compiler())
	if errors.Is(err, compiler.ErrPathDependentClear) {
		restored, reloadedStore, restoredForks, _ := reloadCheckpoint(t, machine, store, forks, snapshot)
		return restored, reloadedStore, restoredForks
	}
	if err != nil {
		t.Fatalf("ExportNativeSnapshot: %v", err)
	}
	for _, frame := range snapshot.VM.Frames {
		if frame.Program.Origin.FirstLine > 1 {
			t.Skipf("a restored queued fork, numbered from verb line %d, is suspended; Toast writes an activation's program without its first line (execute.cc write_activ, tasks.cc read_task_queue alone sets first_lineno), so after this checkpoint its line numbers count from 1 and need not match the uninterrupted run", frame.Program.Origin.FirstLine)
		}
	}
	snapshot.VM = exported
	restored, reloadedStore, restoredForks, reloaded := reloadCheckpoint(t, machine, store, forks, snapshot)
	for index, frame := range reloaded.Frames {
		if frame.Native == nil {
			t.Fatalf("reloaded frame %d is not in Toast's form", index)
		}
	}
	return restored, reloadedStore, restoredForks
}

// checkpointSuspendedVMInBarnForm is checkpointSuspendedVM with the task
// written in Barn's own frame form, which the runtime keeps for a task
// ExportNativeSnapshot refuses.
func checkpointSuspendedVMInBarnForm(t *testing.T, machine *VM, store *dbstore.Store, forks []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork) {
	t.Helper()
	restored, reloadedStore, restoredForks, _ := reloadCheckpoint(t, machine, store, forks, suspendedSnapshot(machine))
	return restored, reloadedStore, restoredForks
}

// suspendedSnapshot is the suspended machine's task as a checkpoint holds it,
// in Barn's frame form and without its activations.
func suspendedSnapshot(machine *VM) task.Snapshot {
	return task.Snapshot{
		ID:            machine.Task.ID,
		Owner:         machine.Task.Owner,
		State:         task.TaskSuspended,
		WakeValue:     types.NewInt(0),
		TaskLocal:     types.NewEmptyMap(),
		Programmer:    machine.Context.Programmer,
		This:          machine.Context.ThisObj,
		ReadingPlayer: types.ObjNothing,
		VM:            machine.PersistenceVMSnapshot(),
	}
}

// reloadCheckpoint writes snapshot, the queued forks and store to a
// checkpoint, loads it, and restores the task and the forks on the loaded
// store. It also returns the task's VM snapshot as the reader produced it.
func reloadCheckpoint(t *testing.T, machine *VM, store *dbstore.Store, forks []sourceFuzzFork, snapshot task.Snapshot) (*VM, *dbstore.Store, []sourceFuzzFork, *task.VMSnapshot) {
	t.Helper()
	queued := make([]task.Snapshot, len(forks))
	for i, fork := range forks {
		state := task.NewState(fork.id, fork.info.Player, machine.TickLimit, 1)
		state.ForkInfo = fork.info
		state.Program = fork.program
		state.Programmer = fork.programmer
		state.This = fork.info.ThisObj
		state.VerbLoc = fork.info.VerbLoc
		state.VerbName = fork.info.Verb
		queued[i] = state.PersistenceSnapshotLocked()
	}
	suspended := []task.Snapshot{snapshot}
	if size := persistedValueBytes(queued, suspended); size > checkpointValueBudget {
		t.Skipf("the checkpoint would write %d bytes of task values, over the %d-byte budget", size, checkpointValueBudget)
	}
	path := filepath.Join(t.TempDir(), "checkpoint.db")
	if err := dbformat.WriteCheckpoint(path, store, queued, suspended, nil); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	if got := len(reloaded.SuspendedTasks); got != 1 {
		t.Fatalf("suspended tasks = %d, want 1", got)
	}
	if got := len(reloaded.QueuedTasks); got != len(forks) {
		t.Fatalf("queued tasks = %d, want %d", got, len(forks))
	}
	restoredForks := make([]sourceFuzzFork, len(forks))
	for i, saved := range reloaded.QueuedTasks {
		program, diagnostics := machine.Builtins.Registry().Compiler().CompileMOO(saved.Code)
		if len(diagnostics) != 0 {
			t.Fatalf("queued child %d does not compile: %v", saved.ID, diagnostics)
		}
		program = program.WithFirstLine(saved.FirstLine)
		if saved.ID != forks[i].id {
			t.Fatalf("queued child %d changed ID/order to %d", forks[i].id, saved.ID)
		}
		variables, err := task.RestoreForkEnvironment(program, saved.Variables)
		if err != nil {
			t.Fatal(err)
		}
		restoredForks[i] = sourceFuzzFork{id: saved.ID, program: program, programmer: saved.Programmer, info: &types.ForkInfo{
			Variables: variables, SourceLines: saved.Code,
			ThisObj: saved.This, ThisValue: types.None, Player: saved.Player,
			Caller: saved.Player, Verb: saved.Verb, VerbLoc: saved.VerbLoc,
		}}
	}
	reloadedStore, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		t.Fatalf("NewStoreFromDatabase failed: %v", err)
	}
	context := task.NewContext()
	context.Player = machine.Context.Player
	context.Programmer = machine.Context.Programmer
	context.ThisObj = machine.Context.ThisObj
	context.Verb = machine.Context.Verb
	context.IsWizard = machine.Context.IsWizard
	saved := reloaded.SuspendedTasks[0].Snapshot.VM
	restored, err := RestoreVMSnapshot(saved, reloadedStore, machine.Builtins, context)
	if err != nil {
		t.Fatalf("RestoreVMSnapshot after checkpoint reload: %v", err)
	}
	restored.Task = machine.Task
	restored.TickLimit = machine.TickLimit
	restored.Ticks = machine.Ticks
	if frame := restored.CurrentFrame(); frame == nil {
		t.Fatal("restored suspended VM has no active frame")
	}
	return restored, reloadedStore, restoredForks, saved
}

// A task suspended inside eval() is exported with Toast's eval() return
// continuation (builtin eval, pc 2) on the eval activation, as in the Toast
// checkpoint engine/testdata/toast-suspended-eval.db, and the import of that
// form hands eval()'s caller {1, value}.
func TestCheckpointExportsSuspendedEvalWithToastContinuation(t *testing.T) {
	registry := newCheckpointResumeRegistry(t)
	program, diagnostics := registry.Compiler().CompileMOO([]string{`return eval("suspend(); return 73;");`})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	checkpoint := func(t *testing.T, machine *VM, store *dbstore.Store, queued []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork) {
		exported, err := ExportNativeSnapshot(machine.PersistenceVMSnapshot(), machine.Task.GetCallStack(), registry.Compiler())
		if err != nil {
			t.Fatalf("ExportNativeSnapshot: %v", err)
		}
		top := exported.Frames[len(exported.Frames)-1]
		if len(exported.Frames) != 2 || top.Native == nil || top.Native.Builtin == nil || top.Native.Builtin.Name != "eval" || top.BuiltinPC != 2 {
			t.Fatalf("exported %d frames, top frame %+v; want 2 frames, the top with builtin eval at pc 2", len(exported.Frames), top)
		}
		return checkpointSuspendedVM(t, machine, store, queued)
	}
	results, _, checkpoints := runCheckpointResumeCase(t, registry, program, checkpoint, sourceFuzzTicks, true)
	if checkpoints != 1 || len(results) != 1 || results[0].Flow != types.FlowReturn || results[0].Val.String() != "{1, 73}" {
		t.Fatalf("%d checkpoints, results %v; want 1 checkpoint and one task returning {1, 73}", checkpoints, results)
	}
}

// call_function("eval", ...) pushes the same eval activation from a call site
// not named eval. ExportNativeSnapshot marks that activation as eval()'s own,
// and the import refused the frame it had written ("native builtin
// continuation requires conversion"), so Barn could not reload its own
// checkpoint of such a task.
func TestCheckpointReloadsEvalCalledThroughCallFunction(t *testing.T) {
	source := `return call_function("eval", "suspend(); return 73;");`
	registry := newCheckpointResumeRegistry(t)
	program, diagnostics := registry.Compiler().CompileMOO([]string{source})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	results, _, checkpoints := runCheckpointResumeCase(t, registry, program, checkpointSuspendedVM, sourceFuzzTicks, true)
	if checkpoints != 1 || len(results) != 1 || results[0].Flow != types.FlowReturn || results[0].Val.String() != "{1, 73}" {
		t.Fatalf("%d checkpoints, results %v; want 1 checkpoint and one task returning {1, 73}", checkpoints, results)
	}
}

// A checkpoint writes every variable of the suspended task and of each queued
// fork in full, as Toast's write_forked_task and write_activ do (tasks.cc:1911,
// execute.cc:3979), while in memory they share one value. This program holds
// one 6 MB string in four variables and forks ten times in 87 ticks; its
// checkpoint is 276,827,071 bytes. Thirty forks of a 12 MB string in eight
// variables wrote 9.4 GB, and a fuzz worker that reloaded such checkpoints
// was killed by the OOM killer at 25 GB. The property skips the case instead.
func TestCheckpointResumeSkipsOversizedCheckpoint(t *testing.T) {
	source := strings.Join([]string{
		`s = "aaaaaa";`,
		"for x in [1..20]",
		"  s = s + s;",
		"endfor",
		"y = s;",
		"z = s;",
		"acc = s;",
		"for n in [1..10]",
		"  fork (0)",
		"  endfork",
		"endfor",
		"suspend();",
		// No builtin is called with s, so the call bound of
		// newSourceFuzzRegistry does not skip the case first.
		"return s == y;",
	}, "\n")
	var run *testing.T
	reached := false
	checkpoint := func(t *testing.T, machine *VM, store *dbstore.Store, queued []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork) {
		reached = true
		return checkpointSuspendedVM(t, machine, store, queued)
	}
	t.Run("case", func(t *testing.T) {
		run = t
		checkpointResumeDifference(t, source, checkpoint, sourceFuzzTicks, false)
	})
	if !reached {
		t.Fatal("the case ended before its checkpoint")
	}
	if !run.Skipped() {
		t.Fatalf("a checkpoint over the %d-byte budget was written and reloaded, not skipped", checkpointValueBudget)
	}
}

// persistedValueBytes sums types.ValueBytes over every value the snapshots
// persist, each occurrence counted in full as the checkpoint writes it.
func persistedValueBytes(snapshotLists ...[]task.Snapshot) int {
	total := 0
	count := func(value types.Value) types.Value {
		total += types.ValueBytes(value)
		return value
	}
	for _, snapshots := range snapshotLists {
		for i := range snapshots {
			snapshots[i].TransformPersistenceValues(count)
		}
	}
	return total
}

// The property fails, naming the task and the state, when a checkpoint
// loses a persisted write.
func TestCheckpointResumeEquivalenceDetectsDroppedWrite(t *testing.T) {
	source := "#0.p = 5;\nsuspend();\nreturn #0.p;"
	difference, skip := checkpointResumeDifference(t, source, checkpointSuspendedVM, sourceFuzzTicks, false)
	if difference != "" || skip != "" {
		t.Fatalf("unperturbed checkpoint: difference %q, skip %q; want neither", difference, skip)
	}

	dropWrite := func(t *testing.T, machine *VM, store *dbstore.Store, queued []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork) {
		restored, reloaded, forks := checkpointSuspendedVM(t, machine, store, queued)
		if code := reloaded.DirectTxn().SetPropertyValue(0, "p", types.NewInt(0)); code != types.E_NONE {
			t.Fatalf("reset #0.p: %v", code)
		}
		return restored, reloaded, forks
	}
	difference, skip = checkpointResumeDifference(t, source, dropWrite, sourceFuzzTicks, false)
	want := "task 1: uninterrupted return 5, checkpointed return 0; persistent state: "
	if skip != "" || !strings.HasPrefix(difference, want) {
		t.Fatalf("dropped write: difference\n%s\nskip %q; want difference\n%s", difference, skip, want)
	}
}
