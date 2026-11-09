package format

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// taskRoundTripRaw writes, through the checkpoint's task-root snapshot, a
// database holding a queued fork, a suspended VM and an interrupted VM. Anonymous
// #37 is reachable only from the queued task and #38 only from the suspended
// task; one WAIF is shared by an object property and both tasks.
func taskRoundTripRaw(t testing.TB) []byte {
	t.Helper()
	s := store.NewStore()
	shared := types.NewWaif(0, 0)
	root := store.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetLocation(types.ObjNothing)
	root.SetPropOrder([]string{"shared"})
	root.SetPropDefsCount(1)
	root.SetProperty("shared", store.NewProperty(shared, 0, 1, false, true))
	if err := s.Add(root.Build()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []types.ObjID{37, 38} {
		anon := store.NewObjectBuilder(id)
		anon.SetAnonymous(true)
		anon.SetName("Task-only anonymous")
		anon.SetLocation(types.ObjNothing)
		anon.SetPropOrder([]string{"self"})
		anon.SetPropDefsCount(1)
		anon.SetProperty("self", store.NewProperty(types.NewAnon(id), 0, 1, false, true))
		s.AddAnonymous(anon.Build())
	}
	frame := func(source string, locals ...types.Value) task.VMFrameSnapshot {
		names := []string{"x", "w"}[:len(locals)]
		return task.VMFrameSnapshot{
			Program: bytecode.Program{Source: []string{source}, VarNames: names, NumLocals: len(names)},
			Locals:  locals, Stack: []types.Value{types.NewStr("operand")},
			This: 0, ThisValue: types.NewObj(0), Player: 0, Verb: "run", StoredVerb: "run", VerbLoc: 0,
			Args: []types.Value{types.NewInt(1)},
		}
	}
	queued := []task.Snapshot{{
		ID: 5, Owner: 0, State: task.TaskQueued, StartTime: time.Unix(-3, 0), VerbName: "fork_verb",
		ReadingPlayer: types.ObjNothing,
		Fork: &task.ForkSnapshot{
			// Case-distinct spellings stay distinct: persistence does not
			// apply engine's case-folded local matching.
			Variables: map[string]types.Value{
				"a": types.NewAnon(37), "w": shared, "N": types.NewInt(1), "n": types.NewStr("x"),
			},
			VariableNames: []string{"n", "a", "w", "N"},
			SourceLines:   []string{"return a;"},
			FirstLine:     2,
		},
	}}
	suspended := []task.Snapshot{{
		ID: 6, State: task.TaskSuspended, StartTime: time.Unix(100, 0), ReadingPlayer: types.ObjNothing,
		WakeValue: types.NewInt(0), TaskLocal: types.NewStr("local"),
		VM: &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{
			frame("return 1;", types.NewAnon(38), shared),
		}},
	}, {
		ID: 7, State: task.TaskSuspended, StartTime: time.Unix(200, 0), ReadingPlayer: 0,
		TaskLocal: types.NewInt(7),
		VM:        &task.VMSnapshot{MaxStackDepth: 50, Frames: []task.VMFrameSnapshot{frame("return 7;")}},
	}}
	snapshot := snapshotWithTaskRoots(s, queued, suspended)
	var raw bytes.Buffer
	writer := NewWriter(&raw, snapshot)
	writer.SetTaskSnapshots(queued, suspended)
	if err := writer.WriteDatabase(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func parseRaw(t *testing.T, raw []byte) *Database {
	t.Helper()
	database, err := parseDatabaseWithWaifIdentities(bufio.NewReader(bytes.NewReader(raw)), nil)
	if err != nil {
		t.Fatal(err)
	}
	return database
}

func TestTaskRoundTripRetainsQueuedAndSuspendedTasks(t *testing.T) {
	raw := taskRoundTripRaw(t)
	for _, section := range []string{"\n1 queued tasks\n", "\n1 suspended tasks\n", "\n1 interrupted tasks\n"} {
		if !bytes.Contains(raw, []byte(section)) {
			t.Fatalf("fixture lacks %q", section)
		}
	}
	database := parseRaw(t, raw)
	plain, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	// A store-only snapshot has no task roots, so the old fuzz helper wrote
	// neither the tasks nor the anonymous objects only they reach.
	if got := len(plain.Snapshot().AnonymousObjects); got != 0 {
		t.Fatalf("store-only snapshot anonymous objects = %d, want 0", got)
	}
	first, second, err := checkpointTwice(database)
	if err != nil {
		t.Fatal(err)
	}
	if err := compareRoundTripPersistence(first, second); err != nil {
		t.Fatal(err)
	}
	for name, state := range map[string]persistedState{"first": first, "second": second} {
		if len(state.queued) != 1 || len(state.suspended) != 2 || len(state.snapshot.AnonymousObjects) != 2 {
			t.Fatalf("%s reload: queued %d, suspended %d, anonymous %d; want 1, 2, 2",
				name, len(state.queued), len(state.suspended), len(state.snapshot.AnonymousObjects))
		}
	}
	queued, suspended := first.queued[0], first.suspended[0]
	if queued.StartTime.Unix() != -3 || queued.Fork.FirstLine != 1 ||
		!queued.Fork.Variables["N"].Identical(types.NewInt(1)) || !queued.Fork.Variables["n"].Identical(types.NewStr("x")) {
		t.Fatalf("queued task after first reload: start %d, first line %d, variables %v",
			queued.StartTime.Unix(), queued.Fork.FirstLine, queued.Fork.Variables)
	}
	anons := anonymousObjectsByID(first.snapshot)
	queuedAnon, suspendedAnon := queued.Fork.Variables["a"], suspended.VM.Frames[0].Locals[0]
	if queuedAnon.Type() != types.TYPE_ANON || suspendedAnon.Type() != types.TYPE_ANON ||
		anons[queuedAnon.Obj()] == nil || anons[suspendedAnon.Obj()] == nil || queuedAnon.Obj() == suspendedAnon.Obj() {
		t.Fatalf("task-only anonymous objects lost: queued %v, suspended %v", queuedAnon, suspendedAnon)
	}
	property := first.snapshot.Objects[0].Properties["shared"].Value.WaifIdentity()
	if queued.Fork.Variables["w"].WaifIdentity() != property || suspended.VM.Frames[0].Locals[1].WaifIdentity() != property {
		t.Fatal("WAIF shared by a property and both tasks was split by the round trip")
	}
	// readInterruptedTask resumes an interrupted task with E_INTRPT, so the
	// first dump writes it as a ready suspended task.
	interrupted := first.suspended[1]
	if interrupted.ID != 7 || interrupted.State != task.TaskQueued || interrupted.IsInterruptedForPersistence() ||
		!interrupted.WakeValue.Identical(types.NewErr(types.E_INTRPT)) {
		t.Fatalf("interrupted task after first reload: id %d, state %v, wake %v", interrupted.ID, interrupted.State, interrupted.WakeValue)
	}
}

// An empty fork body ("fork (n) endfork") is a valid program, and Toast writes
// it as the "." terminator alone (db_io.cc dbio_write_forked_program), so the
// queued task must survive the checkpoint with an empty body.
func TestTaskRoundTripRetainsEmptyQueuedBody(t *testing.T) {
	raw := taskRoundTripRaw(t)
	body := []byte("return a;\n.\n")
	if bytes.Count(raw, body) != 1 {
		t.Fatal("queued body is not unique")
	}
	database := parseRaw(t, bytes.Replace(raw, body, []byte(".\n"), 1))
	if len(database.QueuedTasks) != 1 || len(database.QueuedTasks[0].Code) != 0 {
		t.Fatal("reader did not accept the empty queued body")
	}
	first, second, err := checkpointTwice(database)
	if err != nil {
		t.Fatalf("empty queued body: %v", err)
	}
	if err := compareRoundTripPersistence(first, second); err != nil {
		t.Fatal(err)
	}
	for _, state := range []persistedState{first, second} {
		if len(state.queued) != 1 || state.queued[0].ID != 5 || state.queued[0].Fork == nil || len(state.queued[0].Fork.SourceLines) != 0 {
			t.Fatalf("empty queued body not retained: %+v", state.queued)
		}
	}
}

func TestCompareRoundTripPersistenceDetectsTaskMutation(t *testing.T) {
	fork := func(s *persistedState) *task.ForkSnapshot { return s.queued[0].Fork }
	vm := func(s *persistedState) *task.VMFrameSnapshot { return &s.suspended[0].VM.Frames[0] }
	cases := []struct {
		name, witness string
		mutate        func(*persistedState)
	}{
		{"queued count", "queued tasks", func(s *persistedState) { s.queued = nil }},
		{"queued id", "queued task 5 identity/time", func(s *persistedState) { s.queued[0].ID++ }},
		{"queued time", "queued task 5 identity/time", func(s *persistedState) { s.queued[0].StartTime = time.Unix(-2, 0) }},
		{"queued verb", "queued task 5 descriptor", func(s *persistedState) { s.queued[0].VerbName = "Fork_verb" }},
		{"queued programmer", "queued task 5 descriptor", func(s *persistedState) { s.queued[0].Programmer = 1 }},
		{"queued source", "queued task 5 source", func(s *persistedState) { fork(s).SourceLines = []string{"return A;"} }},
		{"queued variable type", "queued task 5 variables", func(s *persistedState) { fork(s).Variables["N"] = types.NewStr("1") }},
		{"queued variable case", "queued task 5 variables", func(s *persistedState) {
			fork(s).Variables["A"] = fork(s).Variables["a"]
			delete(fork(s).Variables, "a")
		}},
		// The two anonymous objects have the same shape, so the merge is caught
		// where the shared identity context sees #38 claimed a second time.
		{"queued anonymous alias merge", "suspended task 6 frame 0 locals", func(s *persistedState) {
			fork(s).Variables["a"] = vm(s).Locals[0]
		}},
		{"queued waif alias split", "queued task 5 variables", func(s *persistedState) { fork(s).Variables["w"] = types.NewWaif(0, 0) }},
		{"suspended count", "suspended tasks", func(s *persistedState) { s.suspended = s.suspended[:1] }},
		{"category ready", "suspended task 6 category", func(s *persistedState) { s.suspended[0].State = task.TaskQueued }},
		{"category interrupted", "suspended task 6 category", func(s *persistedState) {
			s.suspended[0].IsExecSuspended, s.suspended[0].ExecCommandName = true, "exec"
		}},
		{"suspended id", "suspended task 6 identity/time", func(s *persistedState) { s.suspended[0].ID++ }},
		{"suspended time", "suspended task 6 identity/time", func(s *persistedState) { s.suspended[0].StartTime = time.Unix(101, 0) }},
		{"wake value", "suspended task 6 wake/task-local value", func(s *persistedState) { s.suspended[0].WakeValue = types.NewInt(1) }},
		{"task local", "suspended task 6 wake/task-local value", func(s *persistedState) { s.suspended[0].TaskLocal = types.NewStr("Local") }},
		{"task descriptor", "suspended task 6 task descriptor", func(s *persistedState) { s.suspended[0].Programmer = 1 }},
		{"max stack depth", "suspended task 6 activation count", func(s *persistedState) { s.suspended[0].VM.MaxStackDepth++ }},
		{"activation count", "suspended task 6 activation count", func(s *persistedState) {
			s.suspended[0].VM.Frames = append(s.suspended[0].VM.Frames, *vm(s))
		}},
		{"frame source", "frame 0 source/IP/local names", func(s *persistedState) { vm(s).Program.Source = []string{"return 2;"} }},
		{"frame IP", "frame 0 source/IP/local names", func(s *persistedState) { vm(s).IP++ }},
		{"local name case", "frame 0 source/IP/local names", func(s *persistedState) { vm(s).Program.VarNames = []string{"X", "w"} }},
		{"local value", "frame 0 locals", func(s *persistedState) { vm(s).Locals[0] = types.NewInt(0) }},
		{"local waif alias split", "frame 0 locals", func(s *persistedState) { vm(s).Locals[1] = types.NewWaif(0, 0) }},
		{"stack value", "frame 0 stack", func(s *persistedState) { vm(s).Stack[0] = types.NewStr("Operand") }},
		{"frame verb", "frame 0 call descriptor", func(s *persistedState) { vm(s).Verb = "Run" }},
		{"frame args", "frame 0 call descriptor", func(s *persistedState) { vm(s).Args = nil }},
		{"frame this value", "frame 0 call descriptor", func(s *persistedState) { vm(s).ThisValue = types.NewObj(1) }},
		{"frame programmer", "frame 0 call descriptor", func(s *persistedState) { s.suspended[0].CallStack[0].Programmer = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b, err := checkpointTwice(parseRaw(t, taskRoundTripRaw(t)))
			if err != nil {
				t.Fatal(err)
			}
			if err := compareRoundTripPersistence(a, b); err != nil {
				t.Fatalf("unmodified fixture: %v", err)
			}
			tc.mutate(&b)
			if err := compareRoundTripPersistence(a, b); err == nil || !strings.Contains(err.Error(), tc.witness) {
				t.Fatalf("mutation %q: got %v, want witness %q", tc.name, err, tc.witness)
			}
		})
	}
}
