package vm

import (
	"fmt"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func mapKeyVMStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	system := dbstore.NewObjectBuilder(0)
	system.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
	m := types.NewMap([][2]types.Value{{types.NewStr("a"), types.NewInt(0)}, {types.NewStr("b"), types.NewInt(0)}})
	system.SetProperty("m", dbstore.NewProperty(m, 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(system.Build()); err != nil {
		t.Fatalf("store.Add: %v", err)
	}
	return store
}

// runOn runs code to completion in txn, as a task of programmer #0.
func runOn(t *testing.T, store *dbstore.Store, txn *dbstore.StoreTxn, code string) {
	t.Helper()
	registry := BuildVMRegistry()
	prog, diagnostics := registry.Compiler().CompileMOO([]string{code})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO: %v", diagnostics[0])
	}
	ctx := task.NewContext()
	ctx.Programmer = 0
	ctx.Store = store
	ctx.StoreTxn = txn
	machine := NewVM(store, newTestSession(registry))
	machine.Context = ctx
	machine.Task = task.NewState(1, 0, 30000, 1)
	machine.PrepareVerbFrame(prog, types.ObjNothing, 0, 0, "", types.ObjNothing, []types.Value{})
	if result := machine.ExecuteLoop(); result.Flow != types.FlowReturn && result.Flow != types.FlowNormal {
		t.Fatalf("%q: flow %v: %v", code, result.Flow, result.Error)
	}
}

func entry(t *testing.T, store *dbstore.Store, key string) int64 {
	t.Helper()
	m, ec := store.DirectTxn().PropertyValue(0, "m")
	if ec != types.E_NONE {
		t.Fatal(ec)
	}
	v, _ := m.MapGet(types.NewStr(key))
	return v.Int()
}

// Two tasks that read-modify-write different entries of one map property,
// written as MOO, both commit from the same snapshot.
func TestKeyedAssignmentsToDifferentEntriesBothCommit(t *testing.T) {
	store := mapKeyVMStore(t)
	ta, tb := store.BeginSnapshot(0), store.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	runOn(t, store, ta, `#0.m["a"] = #0.m["a"] + 1;`)
	runOn(t, store, tb, `#0.m["b"] = #0.m["b"] + 1;`)
	if ec := ta.Commit(); ec != types.E_NONE {
		t.Fatalf("first commit: %v %v", ec, ta.Conflicts())
	}
	if ec := tb.Commit(); ec != types.E_NONE {
		t.Fatalf("second commit: %v %v", ec, tb.Conflicts())
	}
	if a, b := entry(t, store, "a"), entry(t, store, "b"); a != 1 || b != 1 {
		t.Fatalf("a = %d, b = %d; want 1 and 1", a, b)
	}
}

// The same entry, or a whole-value read-modify-write, still conflicts.
func TestKeyedAssignmentConflictsWithTheSameEntryAndWithAWholeWrite(t *testing.T) {
	for _, other := range []string{
		`#0.m["a"] = #0.m["a"] + 1;`,
		`x = #0.m; x["b"] = 5; #0.m = x;`,
	} {
		t.Run(other, func(t *testing.T) {
			store := mapKeyVMStore(t)
			ta, tb := store.BeginSnapshot(0), store.BeginSnapshot(0)
			defer ta.Release()
			defer tb.Release()
			runOn(t, store, ta, `#0.m["a"] = #0.m["a"] + 1;`)
			runOn(t, store, tb, other)
			if ec := ta.Commit(); ec != types.E_NONE {
				t.Fatalf("first commit: %v %v", ec, ta.Conflicts())
			}
			if ec := tb.Commit(); ec == types.E_NONE || !tb.ValidationFailed() {
				t.Fatalf("second commit = %v; want a lost validation", ec)
			}
		})
	}
}

// The map is captured before evaluating an index expression that writes it.
// Those side effects must not hide the captured entry's snapshot dependency.
func TestKeyedReadCapturedBeforeIndexWriteConflicts(t *testing.T) {
	store := mapKeyVMStore(t)
	ta, tb := store.BeginSnapshot(0), store.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	runOn(t, store, tb, `x = #0.m["b"]; #0.m["a"] = 11;`)
	if ec := tb.Commit(); ec != types.E_NONE {
		t.Fatalf("first commit: %v %v", ec, tb.Conflicts())
	}
	runOn(t, store, ta, `x = #0.m[(#0.m["a"] = 2) == 2 ? "a" | "a"]; #0.m["b"] = x + 2;`)
	if ec := ta.Commit(); ec == types.E_NONE || !ta.ValidationFailed() {
		t.Fatalf("second commit = %v; want a lost validation", ec)
	}
}

// An entry written before capture is our own value; a second write during
// index evaluation must not make it depend on the original snapshot entry.
func TestKeyedReadCapturedAfterOwnWriteRemainsIndependent(t *testing.T) {
	store := mapKeyVMStore(t)
	ta, tb := store.BeginSnapshot(0), store.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	runOn(t, store, tb, `x = #0.m["b"]; #0.m["a"] = 11;`)
	if ec := tb.Commit(); ec != types.E_NONE {
		t.Fatalf("first commit: %v %v", ec, tb.Conflicts())
	}
	runOn(t, store, ta, `#0.m["a"] = 2; x = #0.m[(#0.m["a"] = 3) == 3 ? "a" | "a"]; #0.m["b"] = x;`)
	if ec := ta.Commit(); ec != types.E_NONE {
		t.Fatalf("second commit: %v %v", ec, ta.Conflicts())
	}
	if a, b := entry(t, store, "a"), entry(t, store, "b"); a != 3 || b != 2 {
		t.Fatalf("a = %d, b = %d; want 3 and 2", a, b)
	}
}

// A keyed read of one entry does not conflict with a write of another.
func TestKeyedReadOfOneEntryIgnoresAWriteToAnother(t *testing.T) {
	store := mapKeyVMStore(t)
	ta, tb := store.BeginSnapshot(0), store.BeginSnapshot(0)
	defer ta.Release()
	defer tb.Release()
	runOn(t, store, ta, `#0.m["b"] = 7;`)
	runOn(t, store, tb, `x = #0.m["a"]; #0.m["b"] = x + 9;`)
	if ec := ta.Commit(); ec != types.E_NONE {
		t.Fatalf("first commit: %v", ec)
	}
	// tb also wrote b: that is a blind keyed write, so it commits after ta and
	// its value wins, exactly as if it ran second.
	if ec := tb.Commit(); ec != types.E_NONE {
		t.Fatalf("second commit: %v %v", ec, tb.Conflicts())
	}
	if b := entry(t, store, "b"); b != 9 {
		t.Fatalf("b = %d, want 9", b)
	}
}

// Each assignment fits its snapshot's quota, but commit must check the merged
// map too. A rejected merge leaves the first update intact; a serial retry then
// encounters the ordinary VM assignment quota rather than publishing an oversize
// value. This is a value-size contract, independent of Elle serializability.
func TestKeyedMapRebaseRespectsTaskValueQuota(t *testing.T) {
	for _, test := range []struct {
		name  string
		spare int
	}{
		{name: "below-limit", spare: 1},
		{name: "at-limit", spare: 0},
		{name: "above-limit", spare: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := mapKeyVMStore(t)
			initial, ec := store.DirectTxn().PropertyValue(0, "m")
			if ec != types.E_NONE {
				t.Fatal(ec)
			}
			keyA, keyB := types.NewStr("a"), types.NewStr("b")
			valueA, valueB := strings.Repeat("a", 64), strings.Repeat("b", 64)
			first := initial.MapSet(keyA, types.NewStr(valueA))
			second := initial.MapSet(keyB, types.NewStr(valueB))
			merged := first.MapSet(keyB, types.NewStr(valueB))
			limit := types.ValueBytes(merged) + test.spare
			if types.ValueBytes(first) >= limit || types.ValueBytes(second) >= limit {
				t.Fatal("fixture assignments do not individually fit the task quota")
			}
			ta, tb := store.BeginSnapshot(0), store.BeginSnapshot(0)
			defer ta.Release()
			defer tb.Release()
			for _, assignment := range []struct {
				txn   *dbstore.StoreTxn
				key   string
				value string
			}{
				{txn: ta, key: "a", value: valueA},
				{txn: tb, key: "b", value: valueB},
			} {
				ctx := task.NewContext()
				ctx.StoreTxn = assignment.txn
				ctx.ServerOptions = &task.PendingServerOptions{MaxListValueBytes: limit}
				result := runBytecodeProgram(t, fmt.Sprintf(`#0.m[%q]=%q; return 1;`, assignment.key, assignment.value), store, ctx)
				if result.Flow != types.FlowReturn || result.Val.Int() != 1 {
					t.Fatalf("initial %s assignment = %+v; want successful staging", assignment.key, result)
				}
			}
			if ec := ta.Commit(); ec != types.E_NONE {
				t.Fatalf("first commit = %v, conflicts %v", ec, ta.Conflicts())
			}
			want := merged
			if test.spare > 0 {
				if ec := tb.Commit(); ec != types.E_NONE {
					t.Fatalf("below-limit merge = %v, conflicts %v", ec, tb.Conflicts())
				}
			} else {
				if ec := tb.Commit(); ec != types.E_INVARG || !tb.ValidationFailed() {
					t.Fatalf("oversize merge = %v, validationFailed=%v; want retryable E_INVARG", ec, tb.ValidationFailed())
				}
				conflicts := tb.Conflicts()
				if len(conflicts) != 1 || conflicts[0].Kind != dbstore.ConflictMapKey || conflicts[0].ObjID != 0 || conflicts[0].Name != "m" {
					t.Fatalf("oversize merge conflicts = %+v; want one map-key conflict on #0.m", conflicts)
				}
				want = first
				retry := store.BeginSnapshot(0)
				defer retry.Release()
				ctx := task.NewContext()
				ctx.StoreTxn = retry
				ctx.ServerOptions = &task.PendingServerOptions{MaxListValueBytes: limit}
				result := runBytecodeProgram(t, fmt.Sprintf(`#0.m["b"]=%q; return 1;`, valueB), store, ctx)
				if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
					t.Fatalf("serial retry = %+v; want E_QUOTA", result)
				}
				if retry.HasWrites() {
					t.Fatal("quota-rejected serial assignment staged a write")
				}
			}
			actual, ec := store.DirectTxn().PropertyValue(0, "m")
			if ec != types.E_NONE || !actual.Identical(want) || types.ValueBytes(actual) >= limit {
				t.Fatalf("published map = %v, error=%v, bytes=%d, limit=%d; want %v below limit", actual, ec, types.ValueBytes(actual), limit, want)
			}
		})
	}
}
