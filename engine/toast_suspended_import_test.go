package engine

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// This is a native Toast checkpoint, rather than a Barn snapshot with frames
// installed by the test. See testdata/toast-suspended-call.md for the producer.
func TestToastSuspendedCallImportResumesExactlyOnce(t *testing.T) {
	const path = "testdata/toast-suspended-call.db"
	database := loadNativeImportFixture(t, path, "51158bee3d37e0b59225611e3a138dd274261e4c5993a8846f73fc74d229f0d8")
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("native suspended tasks=%d, want 1", len(database.SuspendedTasks))
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	object, code := store.DirectTxn().PropertyValue(1, "conformance_suspended_call_checkpoint")
	if code != types.E_NONE || object.Type() != types.TYPE_OBJ {
		t.Fatalf("native fixture object=%s error=%s", object, code)
	}
	read := func(name string) int64 {
		t.Helper()
		value, code := store.DirectTxn().PropertyValue(object.Obj(), name)
		if code != types.E_NONE || value.Type() != types.TYPE_INT {
			t.Fatalf("%s=%s error=%s", name, value, code)
		}
		return value.Int()
	}
	if before, after, result, finished := read("before"), read("after"), read("result"), read("finished"); before != 1 || after != 0 || result != 0 || finished != 0 {
		t.Fatalf("checkpoint before/after/result/finished=%d/%d/%d/%d, want 1/0/0/0", before, after, result, finished)
	}
	rt := newTestRuntimeWithBuiltins(t, store)
	defer rt.Stop()
	rt.Session().LoadServerOptionsFromStore(store)
	rt.Session().LoadProtectedBuiltinsFromStore(store)
	// Make the continuation due independently of the date the test runs. Its
	// program, native instruction positions, locals and stacks stay untouched.
	saved := database.SuspendedTasks[0]
	saved.Snapshot.StartTime = time.Now().Add(-time.Second)
	if err := rt.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := rt.GetTask(saved.Snapshot.ID)
	if restored == nil {
		t.Fatalf("native suspended task %d was not restored", saved.Snapshot.ID)
	}
	// These variables are implicit call context even when the saved program
	// does not reference them and Barn therefore allocates no named slots.
	for frameIndex, frame := range restored.VM().Frames {
		original := saved.Snapshot.VM.Frames[frameIndex]
		for commandIndex, commandName := range task.CommandVariableNames {
			found := false
			for nativeIndex, nativeName := range original.Program.VarNames {
				if strings.EqualFold(nativeName, commandName) {
					found = true
					want := original.Locals[nativeIndex]
					if got := frame.CommandEnvironment[commandIndex]; !got.Equal(want) {
						t.Errorf("frame %d implicit %s=%s, want native %s", frameIndex, commandName, got, want)
					}
					break
				}
			}
			if !found {
				t.Fatalf("native producer does not cover implicit %s", commandName)
			}
		}
	}
	driveNativeImportedContinuation(t, rt, restored)
	if before, after, result, finished := read("before"), read("after"), read("result"), read("finished"); before != 1 || after != 1 || result != 173 || finished != 1 {
		t.Fatalf("resumed before/after/result/finished=%d/%d/%d/%d, want 1/1/173/1", before, after, result, finished)
	}
}

func driveNativeImportedContinuation(t *testing.T, runtime *Runtime, restored *Task) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		state := restored.GetState()
		if state == task.TaskCompleted || state == task.TaskKilled {
			if state != task.TaskCompleted {
				t.Fatalf("native continuation ended in state %v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native continuation did not settle through the dispatcher")
		}
		if runtime.DispatchReady() == 0 {
			select {
			case <-runtime.ScheduleChanged():
			case <-time.After(time.Millisecond):
			}
		}
	}
	// More scheduling passes must not repeat the driver or the paused verb.
	for range 3 {
		runtime.DispatchReady()
	}
}

func loadNativeImportFixture(t *testing.T, path, expectedHash string) *dbformat.Database {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != expectedHash {
		t.Fatalf("native producer fixture changed: %s", got)
	}
	database, err := dbformat.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	return database
}

func TestNativeCommandEnvironmentReachesCalleeAfterResume(t *testing.T) {
	database := loadNativeImportFixture(t, "testdata/toast-command-environment.db", "b395edf39a8f79b2bb5670d78573b157c1064f90f41a3538bd25301cbff67de1")
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("native suspended tasks=%d, want 1", len(database.SuspendedTasks))
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	object, code := store.DirectTxn().PropertyValue(1, "conformance_native_command_environment")
	if code != types.E_NONE || object.Type() != types.TYPE_OBJ {
		t.Fatalf("native context fixture object=%s error=%s", object, code)
	}
	read := func(name string) types.Value {
		t.Helper()
		value, code := store.DirectTxn().PropertyValue(object.Obj(), name)
		if code != types.E_NONE {
			t.Fatalf("read native context %s: %s", name, code)
		}
		return value
	}
	want := types.NewList([]types.Value{types.NewStr("native command"), types.NewObj(1), types.NewStr("direct"), types.NewStr("with"), types.NewObj(1), types.NewStr("indirect")})
	if before, after, done := read("before"), read("after"), read("done"); !before.Equal(want) || !after.Equal(types.NewInt(0)) || !done.Equal(types.NewInt(0)) {
		t.Fatalf("native context producer state: before=%s after=%s done=%s", before, after, done)
	}
	runtime := newTestRuntimeWithBuiltins(t, store)
	defer runtime.Stop()
	runtime.Session().LoadServerOptionsFromStore(store)
	runtime.Session().LoadProtectedBuiltinsFromStore(store)
	saved := database.SuspendedTasks[0]
	saved.Snapshot.StartTime = time.Now().Add(-time.Second)
	if err := runtime.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Fatal(err)
	}
	restored := runtime.GetTask(saved.Snapshot.ID)
	if restored == nil {
		t.Fatal("native command context task was not restored")
	}
	for _, name := range restored.VM().CurrentFrame().Program.VarNames {
		for _, commandName := range task.CommandVariableNames {
			if strings.EqualFold(name, commandName) {
				t.Fatalf("producer's suspended frame must not name implicit %s", commandName)
			}
		}
	}
	driveNativeImportedContinuation(t, runtime, restored)
	if before, after, done := read("before"), read("after"), read("done"); !before.Equal(want) || !after.Equal(want) || !done.Equal(types.NewInt(1)) {
		t.Fatalf("resumed context helper: before=%s after=%s done=%s", before, after, done)
	}
}

// The production checkpoint remains private and is never committed as testdata.
// Use the existing workload fixture setting to audit all saved continuations.
func TestToastNativeCheckpointRestorationCensus(t *testing.T) {
	path := os.Getenv("BARN_MONGOOSE_DB")
	if path == "" {
		t.Skip("set BARN_MONGOOSE_DB to audit a native checkpoint")
	}
	database, err := dbformat.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	nativeContinuations, nativeFrames := 0, 0
	for _, saved := range database.SuspendedTasks {
		if saved == nil || saved.Snapshot.VM == nil {
			continue
		}
		hasNativeFrame := false
		for _, frame := range saved.Snapshot.VM.Frames {
			if frame.Native != nil {
				nativeFrames++
				hasNativeFrame = true
			}
		}
		if hasNativeFrame {
			nativeContinuations++
		}
	}
	if len(database.QueuedTasks) == 0 || nativeContinuations == 0 || nativeFrames == 0 {
		t.Fatalf("native census coverage requires queued tasks, native continuations and native frames: queued=%d continuations=%d frames=%d", len(database.QueuedTasks), nativeContinuations, nativeFrames)
	}
	t.Logf("native census coverage: queued=%d continuations=%d frames=%d", len(database.QueuedTasks), nativeContinuations, nativeFrames)
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	runtime := newTestRuntimeWithBuiltins(t, store)
	defer runtime.Stop()
	runtime.Session().LoadServerOptionsFromStore(store)
	runtime.Session().LoadProtectedBuiltinsFromStore(store)
	runtime.LoadQueuedTasks(database.QueuedTasks)
	if err := runtime.LoadSuspendedTasks(database.SuspendedTasks); err != nil {
		t.Error(err)
	}
	queued, suspended := 0, 0
	for _, saved := range database.QueuedTasks {
		restored := runtime.GetTask(saved.ID)
		if restored == nil {
			t.Errorf("queued task %d was not restored", saved.ID)
			continue
		}
		if saved.StartTime > 0 && restored.StartTime.Unix() != saved.StartTime {
			t.Errorf("queued task %d deadline=%d want=%d", saved.ID, restored.StartTime.Unix(), saved.StartTime)
		}
		queued++
	}
	for _, saved := range database.SuspendedTasks {
		if runtime.GetTask(saved.Snapshot.ID) == nil {
			t.Errorf("native continuation %d was not restored", saved.Snapshot.ID)
			continue
		}
		suspended++
	}
	t.Logf("checkpoint restored: queued=%d/%d suspended/interrupted=%d/%d", queued, len(database.QueuedTasks), suspended, len(database.SuspendedTasks))
}

func TestContinuationRestorationErrorsAreReturned(t *testing.T) {
	runtime := newTestRuntimeWithBuiltins(t, newConflictTestStore(t))
	defer runtime.Stop()
	broken := []*dbformat.SuspendedTask{
		{Snapshot: task.Snapshot{ID: 99, VM: &task.VMSnapshot{}}},
		{Snapshot: task.Snapshot{ID: 100, VM: &task.VMSnapshot{}}},
	}
	var restoreError error
	records := captureRecords(t, func() { restoreError = runtime.LoadSuspendedTasks(broken) })
	if restoreError == nil || !strings.Contains(restoreError.Error(), "99") || !strings.Contains(restoreError.Error(), "100") {
		t.Fatalf("restore failure must identify every failed task: %v", restoreError)
	}
	if len(records) != 2 || runtime.GetTask(99) != nil || runtime.GetTask(100) != nil {
		t.Fatalf("restore warnings=%d; invalid task must not be registered", len(records))
	}
	if err := runtime.LoadSuspendedTasks([]*dbformat.SuspendedTask{nil, {Snapshot: task.Snapshot{ID: 101}}}); err == nil {
		t.Fatal("incomplete saved continuations were accepted")
	}
}

func TestNativeCensusRejectsFixtureWithoutQueuedCoverage(t *testing.T) {
	fixture, err := filepath.Abs("testdata/toast-suspended-call.db")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestToastNativeCheckpointRestorationCensus$", "-test.v")
	command.Env = append(os.Environ(), "BARN_MONGOOSE_DB="+fixture)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("census accepted a fixture with zero queued tasks:\n%s", output)
	}
	if !strings.Contains(string(output), "native census coverage") {
		t.Fatalf("census failed for a reason other than missing workload coverage: %v\n%s", err, output)
	}
}

func TestNativeByteStringIterationRejectsIncompatibleColumns(t *testing.T) {
	encoded, err := os.ReadFile("testdata/toast-string-iteration.db.b64")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "c3e24d9ccbe800f3fc84db3b5b655c827204f0e501bbbe826d38ab14d427659c" {
		t.Fatalf("native binary fixture changed: %s", got)
	}
	path := filepath.Join(t.TempDir(), "native-string.db")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := dbformat.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(database.SuspendedTasks) != 1 {
		t.Fatalf("native suspended tasks=%d, want 1", len(database.SuspendedTasks))
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	runtime := newTestRuntimeWithBuiltins(t, store)
	defer runtime.Stop()
	if err := runtime.LoadSuspendedTasks(database.SuspendedTasks); err == nil || !strings.Contains(err.Error(), "native byte-string iteration") {
		t.Fatalf("incompatible native byte-string iteration must return an explicit restore error: %v", err)
	}
	if runtime.GetTask(database.SuspendedTasks[0].Snapshot.ID) != nil {
		t.Fatal("incompatible native string continuation was registered")
	}
}
