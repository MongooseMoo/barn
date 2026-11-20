package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// An inline SQLite statement that SQLite reports as read-only skips the
// irreversible-effect boundary: no exclusive commit gate, and the attempt stays
// retryable. It is serialized by the store's external write epoch instead
// (db/store/store_external_reads.go), so the tests below also check that a
// reader cannot commit across an SQLite writer.

// sqliteReadOnlyFixture is a runtime with an open in-memory SQLite handle in
// #0.h holding table t(x), and test builtins the task programs call.
type sqliteReadOnlyFixture struct {
	rt    *Runtime
	store *dbstore.Store
}

func newSQLiteReadOnlyFixture(t *testing.T, extra ...builtins.Descriptor) *sqliteReadOnlyFixture {
	t.Helper()
	store := newBoundaryTestStore(t)
	if ec := store.DirectTxn().DefineProperty(0, "h", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	var probe builtins.BuiltinFunc = func(ctx *builtins.Execution, _ []types.Value) types.Result {
		flag := func(b bool) types.Value {
			if b {
				return types.NewInt(1)
			}
			return types.NewInt(0)
		}
		return types.Ok(types.NewList([]types.Value{flag(ctx.StoreTxn.IsCommitGateExempt()), flag(ctx.IrreversibleSideEffect)}))
	}
	descriptors := append([]builtins.Descriptor{testBuiltinSlot("gate_probe", 0, 0, []int64{}, &probe)}, extra...)
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), 4, descriptors...)
	t.Cleanup(rt.Stop)
	f := &sqliteReadOnlyFixture{rt: rt, store: store}
	setup := f.task(t, 96000, `
#0.h = sqlite_open(":memory:");
sqlite_query(#0.h, "CREATE TABLE t(x INTEGER)");
return 1;
`)
	runSQLiteTaskToCompletion(t, rt, setup)
	if setup.Result.Flow != types.FlowReturn {
		t.Fatalf("setup: %+v", setup.Result)
	}
	t.Cleanup(func() { rt.EvalCommandOutput(0, "return sqlite_close(#0.h);") })
	return f
}

func (f *sqliteReadOnlyFixture) task(t *testing.T, id int64, source string) *Task {
	t.Helper()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	tk := NewTaskFull(id, 0, compileTestProgram(t, f.rt.registry, source), ticks, seconds)
	tk.Context.IsWizard = true
	return tk
}

// signalBuiltin is a test builtin whose first call closes reached and then
// waits (bounded) for proceed; later calls return at once.
func signalBuiltin(name string, reached, proceed chan struct{}) builtins.Descriptor {
	calls := 0
	var fn builtins.BuiltinFunc = func(*builtins.Execution, []types.Value) types.Result {
		calls++
		if calls == 1 {
			close(reached)
			if proceed != nil {
				select {
				case <-proceed:
				case <-time.After(5 * time.Second):
				}
			}
		}
		return types.Ok(types.NewInt(int64(calls)))
	}
	return testBuiltinSlot(name, 0, 0, []int64{}, &fn)
}

func awaitSignal(t *testing.T, ch chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestInlineReadOnlySQLiteTakesNoGate(t *testing.T) {
	f := newSQLiteReadOnlyFixture(t)
	escalations := f.store.CommitEscalations()
	reader := f.task(t, 96001, `
set_thread_mode(0);
rows = sqlite_query(#0.h, "SELECT total_changes(), (SELECT data_version FROM pragma_data_version), (SELECT schema_version FROM pragma_schema_version)");
probe = gate_probe();
#0.write_value = 1;
return {length(rows), probe};
`)
	runSQLiteTaskToCompletion(t, f.rt, reader)
	if got := reader.Result.Val.String(); reader.Result.Flow != types.FlowReturn || got != "{1, {0, 0}}" {
		t.Fatalf("result = %+v (%s), want {1, {0, 0}}: no gate, attempt still retryable", reader.Result, got)
	}
	if got := f.store.CommitEscalations() - escalations; got != 0 {
		t.Fatalf("exclusive gate acquisitions = %d, want 0 for a read-only statement", got)
	}
	assertCommitGateReleased(t, f.store)
}

func TestInlineWritingSQLiteStillTakesGate(t *testing.T) {
	for _, tc := range []struct{ name, statement string }{
		{"insert", `sqlite_execute(#0.h, "INSERT INTO t VALUES (?)", {1})`},
		{"with_insert", `sqlite_query(#0.h, "WITH v(x) AS (SELECT 7) INSERT INTO t SELECT x FROM v")`},
		{"pragma_write", `sqlite_query(#0.h, "PRAGMA user_version = 3")`},
		{"pragma_setting", `sqlite_query(#0.h, "PRAGMA cache_size = 100")`},
		{"begin", `sqlite_query(#0.h, "BEGIN"); sqlite_query(#0.h, "COMMIT")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSQLiteReadOnlyFixture(t)
			escalations := f.store.CommitEscalations()
			epoch := f.store.ExternalWrites()
			writer := f.task(t, 96002, "set_thread_mode(0);\n"+tc.statement+";\nreturn gate_probe();\n")
			runSQLiteTaskToCompletion(t, f.rt, writer)
			if got := writer.Result.Val.String(); writer.Result.Flow != types.FlowReturn || got != "{1, 1}" {
				t.Fatalf("result = %+v (%s), want {1, 1}: gate held, attempt irreversible", writer.Result, got)
			}
			if got := f.store.CommitEscalations() - escalations; got != 1 {
				t.Fatalf("exclusive gate acquisitions = %d, want 1", got)
			}
			if f.store.ExternalWrites() == epoch {
				t.Fatal("a writing statement did not move the external write epoch")
			}
			assertCommitGateReleased(t, f.store)
		})
	}
}

// The anomaly the gate used to rule out: reader R reads SQLite, writer W then
// inserts a row and commits a store write after reading what R is about to
// write. If R committed on its first attempt, R would precede W in SQLite (it
// did not see the row) and follow W in the store (W did not see R's write): a
// cycle. R must lose validation and re-run, seeing the row.
func TestReadOnlySQLiteLosesToWriterCommittedBetween(t *testing.T) {
	readerRead, writerDone := make(chan struct{}), make(chan struct{})
	attempts := 0
	var attemptNo builtins.BuiltinFunc = func(*builtins.Execution, []types.Value) types.Result {
		attempts++
		return types.Ok(types.NewInt(int64(attempts)))
	}
	f := newSQLiteReadOnlyFixture(t, signalBuiltin("wait_for_writer", readerRead, writerDone), testBuiltinSlot("attempt_no", 0, 0, []int64{}, &attemptNo))
	reader := f.task(t, 96011, `
set_thread_mode(0);
n = attempt_no();
rows = sqlite_query(#0.h, "SELECT count(*) FROM t");
wait_for_writer();
#0.read_value = n;
return {n, rows};
`)
	writer := f.task(t, 96012, `
set_thread_mode(0);
v = #0.read_value;
sqlite_execute(#0.h, "INSERT INTO t VALUES (?)", {1});
#0.write_value = v + 100;
return v;
`)
	go func() {
		defer close(writerDone)
		select {
		case <-readerRead:
		case <-time.After(5 * time.Second):
			return
		}
		runSQLiteTaskToCompletion(t, f.rt, writer)
	}()
	retries := f.store.CommitRetries()
	runSQLiteTaskToCompletion(t, f.rt, reader)
	<-writerDone
	if got := writer.Result.Val.String(); writer.Result.Flow != types.FlowReturn || got != "0" {
		t.Fatalf("writer result = %+v (%s), want 0 (it read #0.read_value before the reader committed)", writer.Result, got)
	}
	if got := reader.Result.Val.String(); reader.Result.Flow != types.FlowReturn || got != "{2, {{1}}}" {
		t.Fatalf("reader result = %+v (%s), want {2, {{1}}}: the first attempt must lose to the writer and the re-run see its row", reader.Result, got)
	}
	if got := f.store.CommitRetries() - retries; got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	assertCommitGateReleased(t, f.store)
}

// A reader with no store writes, running while a writer holds the gate after
// its INSERT, sees the writer's row but not its store write. It must not commit
// that mixed view: its commit waits for the writer, validates, and re-runs.
func TestReadOnlySQLiteDuringWriterHoldRevalidates(t *testing.T) {
	writerHolding, readerDone := make(chan struct{}), make(chan struct{})
	f := newSQLiteReadOnlyFixture(t, signalBuiltin("hold_after_insert", writerHolding, readerDone), signalBuiltin("reader_done", readerDone, nil))
	writer := f.task(t, 96021, `
set_thread_mode(0);
sqlite_execute(#0.h, "INSERT INTO t VALUES (?)", {1});
#0.write_value = 5;
hold_after_insert();
return 1;
`)
	reader := f.task(t, 96022, `
set_thread_mode(0);
v = #0.write_value;
rows = sqlite_query(#0.h, "SELECT count(*) FROM t");
reader_done();
return {v, rows};
`)
	writerFinished := make(chan struct{})
	go func() {
		defer close(writerFinished)
		runSQLiteTaskToCompletion(t, f.rt, writer)
	}()
	awaitSignal(t, writerHolding, "the writer to hold the gate")
	retries := f.store.CommitRetries()
	runSQLiteTaskToCompletion(t, f.rt, reader)
	<-writerFinished
	if got := reader.Result.Val.String(); reader.Result.Flow != types.FlowReturn || got != "{5, {{1}}}" {
		t.Fatalf("reader result = %+v (%s), want {5, {{1}}}: never the row without the writer's store write", reader.Result, got)
	}
	if got := f.store.CommitRetries() - retries; got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	assertCommitGateReleased(t, f.store)
}

// Two read-only statements in concurrent tasks do not exclude each other: both
// read before either commits, and both commit without a retry or the gate.
func TestConcurrentReadOnlySQLiteTasksBothCommit(t *testing.T) {
	firstRead, secondRead := make(chan struct{}), make(chan struct{})
	f := newSQLiteReadOnlyFixture(t, signalBuiltin("first_read", firstRead, secondRead), signalBuiltin("second_read", secondRead, firstRead))
	first := f.task(t, 96031, `
set_thread_mode(0);
rows = sqlite_query(#0.h, "SELECT count(*) FROM t");
first_read();
#0.write_value = 1;
return rows;
`)
	second := f.task(t, 96032, `
set_thread_mode(0);
rows = sqlite_query(#0.h, "SELECT count(*) FROM t");
second_read();
#0.read_value = 1;
return rows;
`)
	escalations, retries := f.store.CommitEscalations(), f.store.CommitRetries()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSQLiteTaskToCompletion(t, f.rt, first)
	}()
	runSQLiteTaskToCompletion(t, f.rt, second)
	<-done
	for _, tk := range []*Task{first, second} {
		if got := tk.Result.Val.String(); tk.Result.Flow != types.FlowReturn || got != "{{0}}" {
			t.Fatalf("task %d result = %+v (%s), want {{0}}", tk.ID, tk.Result, got)
		}
	}
	if got := f.store.CommitRetries() - retries; got != 0 {
		t.Fatalf("commit retries = %d, want 0", got)
	}
	if got := f.store.CommitEscalations() - escalations; got != 0 {
		t.Fatalf("exclusive gate acquisitions = %d, want 0", got)
	}
}
