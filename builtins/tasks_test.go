package builtins

import (
	"reflect"
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

type testTaskYielder struct {
	calls int
}

func (y *testTaskYielder) YieldReadyTasks(*kernel.TaskContext) int {
	y.calls++
	return 0
}

func TestYinReleasesOldTransactionWhenRefreshingReadView(t *testing.T) {
	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	ctx.TicksRemaining = 0
	t.Cleanup(func() {
		if ctx.StoreTxn != nil {
			ctx.StoreTxn.Release()
		}
	})

	yielder := &testTaskYielder{}
	SetTaskYielder(yielder)
	t.Cleanup(func() { SetTaskYielder(nil) })

	if got := activeReadRegistrationCount(store); got != 1 {
		t.Fatalf("active read registrations before yin = %d, want 1", got)
	}

	result := builtinYin(ctx, []types.Value{types.NewInt(0), types.NewInt(1)})
	if result.Flow != types.FlowNormal {
		t.Fatalf("yin result flow = %v err=%v, want normal", result.Flow, result.Error)
	}
	if yielder.calls != 1 {
		t.Fatalf("YieldReadyTasks calls = %d, want 1", yielder.calls)
	}
	if ctx.StoreTxn == nil {
		t.Fatal("yin did not install a refreshed read transaction")
	}
	if got := activeReadRegistrationCount(store); got != 1 {
		t.Fatalf("active read registrations after yin = %d, want 1", got)
	}
}

func activeReadRegistrationCount(store *dbstore.Store) int {
	active := reflect.ValueOf(store).Elem().FieldByName("activeReadTS")
	total := 0
	iter := active.MapRange()
	for iter.Next() {
		total += int(iter.Value().Int())
	}
	return total
}
