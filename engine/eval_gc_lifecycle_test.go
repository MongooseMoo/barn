package engine

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestEvalRunGCCallbackCreatedAnonymousSurvivesNextEvalYield(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	class, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("create numbered class: %v", errCode)
	}
	for _, name := range []string{"subject", "stash", "recycle_called", "link"} {
		if errCode := store.DirectTxn().DefineProperty(class, name, dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("define #%d.%s: %v", class, name, errCode)
		}
	}
	recycleVerb := dbstore.NewVerb(
		"recycle",
		[]string{"recycle"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{
			fmt.Sprintf("#%d.recycle_called = #%d.recycle_called + 1;", class, class),
			fmt.Sprintf("replacement = create(#%d, 1);", class),
			"replacement.link = replacement;",
			fmt.Sprintf("#%d.stash = replacement;", class),
			"return 0;",
		},
	)
	if _, errCode := store.AddVerb(class, recycleVerb); errCode != types.E_NONE {
		t.Fatalf("add inherited recycle verb: %v", errCode)
	}

	rt := NewRuntime(store)
	defer rt.Stop()
	defer removeTasksForOwner(rt, 0)
	runEvalTask := func(code string) *task.Task {
		program := compileTestProgram(t, rt.registry, code)
		taskID := rt.CreateBackgroundTask(0, program, 0)
		running := rt.GetTask(taskID)
		running.Context.IsWizard = true
		for pass := 0; pass < 8; pass++ {
			state := running.GetState()
			if state == task.TaskCompleted || state == task.TaskKilled {
				break
			}
			if processed := rt.ProcessReadyTasks(); processed == 0 {
				t.Fatalf("eval task %d made no progress in state %v", taskID, state)
			}
		}
		if state := running.GetState(); state != task.TaskCompleted {
			t.Fatalf("eval task %d ended in state %v, want completed", taskID, state)
		}
		return running
	}

	createAndRoot := fmt.Sprintf(
		"subject = create(#%d, 1); subject.link = subject; #%d.subject = subject; return valid(subject);",
		class, class,
	)
	if result := runEvalTask(createAndRoot).Result; result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 1 {
		t.Fatalf("create/root eval task = %+v, want valid anonymous subject", result)
	}
	clearAndCollect := fmt.Sprintf("#%d.subject = 0; run_gc(); return #%d.recycle_called;", class, class)
	if result := runEvalTask(clearAndCollect).Result; result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 1 {
		t.Fatalf("clear/run_gc eval task = %+v, want one recycle hook call", result)
	}
	observeAfterYield := fmt.Sprintf(
		"suspend(0); return {#%d.recycle_called, typeof(#%d.stash), valid(#%d.stash)};",
		class, class, class,
	)
	result := runEvalTask(observeAfterYield).Result
	if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_LIST || result.Val.String() != "{1, 12, 1}" {
		t.Fatalf("post-yield eval task = %+v, want hook count 1 and valid TYPE_ANON stash", result)
	}
}

func TestRunGCValidationConflictDoesNotRecycleNewPersistentRoot(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	class, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("create numbered class: %v", errCode)
	}
	for _, name := range []string{"subject", "recycle_called", "link"} {
		if errCode := store.DirectTxn().DefineProperty(class, name, dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("define #%d.%s: %v", class, name, errCode)
		}
	}
	recycleVerb := dbstore.NewVerb(
		"recycle",
		[]string{"recycle"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{fmt.Sprintf("#%d.recycle_called = #%d.recycle_called + 1;", class, class)},
	)
	if _, errCode := store.AddVerb(class, recycleVerb); errCode != types.E_NONE {
		t.Fatalf("add inherited recycle verb: %v", errCode)
	}
	rootA, errCode := store.DirectTxn().CreateObject([]types.ObjID{class}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous root A: %v", errCode)
	}
	rootB, errCode := store.DirectTxn().CreateObject([]types.ObjID{class}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous root B: %v", errCode)
	}
	for _, id := range []types.ObjID{rootA, rootB} {
		if errCode := store.DirectTxn().SetPropertyValue(id, "link", types.NewAnon(id)); errCode != types.E_NONE {
			t.Fatalf("set anonymous #%d self-cycle: %v", id, errCode)
		}
	}
	if errCode := store.DirectTxn().SetPropertyValue(class, "subject", types.NewAnon(rootA)); errCode != types.E_NONE {
		t.Fatalf("persist anonymous root A: %v", errCode)
	}

	rt := NewRuntime(store)
	t.Cleanup(rt.Stop)
	t.Cleanup(func() { removeTasksForOwner(rt, 0) })
	rt.registry.Register("commit_subject_b", func(_ *builtins.Execution, args []types.Value) types.Result {
		if len(args) != 0 {
			return types.Err(types.E_ARGS)
		}
		tx := store.BeginReadOnly(0)
		defer tx.Release()
		if errCode := tx.SetPropertyValue(class, "subject", types.NewAnon(rootB)); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if errCode := tx.Commit(); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	program := compileTestProgram(t, rt.registry, fmt.Sprintf(
		"#%d.subject = 0; commit_subject_b(); run_gc(); return 1;",
		class,
	))
	taskID := rt.CreateBackgroundTask(0, program, 0)
	running := rt.GetTask(taskID)
	running.Context.IsWizard = true
	if err := rt.runTask(running); err != nil {
		t.Fatalf("runTask: %v", err)
	}
	if running.Result.Flow != types.FlowException || running.Result.Error != types.E_INVARG {
		t.Fatalf("run_gc conflict result = %+v, want E_INVARG exception", running.Result)
	}
	subject, errCode := store.DirectTxn().PropertyValue(class, "subject")
	if errCode != types.E_NONE || subject.Type() != types.TYPE_ANON || subject.Obj() != rootB {
		t.Fatalf("persisted subject = %v (%v), want anonymous root B #%d", subject, errCode, rootB)
	}
	recycleCalled, errCode := store.DirectTxn().PropertyValue(class, "recycle_called")
	if errCode != types.E_NONE || recycleCalled.Type() != types.TYPE_INT || recycleCalled.Int() != 0 {
		t.Fatalf("persisted recycle count = %v (%v), want 0", recycleCalled, errCode)
	}
	for _, id := range []types.ObjID{rootA, rootB} {
		if !store.DirectTxn().Valid(id) {
			t.Errorf("anonymous object #%d recycled despite validation conflict", id)
		}
	}
}

func TestRunGCCommitsStagedAnonymousEdgeBeforeLiveSweep(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	class, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("create numbered class: %v", errCode)
	}
	for _, name := range []string{"subject", "link"} {
		if errCode := store.DirectTxn().DefineProperty(class, name, dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("define #%d.%s: %v", class, name, errCode)
		}
	}
	anchor, errCode := store.DirectTxn().CreateObject([]types.ObjID{class}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous anchor: %v", errCode)
	}
	left, errCode := store.DirectTxn().CreateObject([]types.ObjID{class}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous cycle left: %v", errCode)
	}
	right, errCode := store.DirectTxn().CreateObject([]types.ObjID{class}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous cycle right: %v", errCode)
	}
	if errCode := store.DirectTxn().SetPropertyValue(left, "link", types.NewAnon(right)); errCode != types.E_NONE {
		t.Fatalf("set cycle left -> right: %v", errCode)
	}
	if errCode := store.DirectTxn().SetPropertyValue(right, "link", types.NewAnon(left)); errCode != types.E_NONE {
		t.Fatalf("set cycle right -> left: %v", errCode)
	}
	if errCode := store.DirectTxn().SetPropertyValue(class, "subject", types.NewAnon(anchor)); errCode != types.E_NONE {
		t.Fatalf("persist anonymous anchor: %v", errCode)
	}

	rt := NewRuntime(store)
	t.Cleanup(rt.Stop)
	t.Cleanup(func() { removeTasksForOwner(rt, 0) })
	rt.registry.Register("stage_cycle_edge", func(ctx *builtins.Execution, args []types.Value) types.Result {
		if len(args) != 0 || ctx.StoreTxn == nil {
			return types.Err(types.E_INVARG)
		}
		if errCode := ctx.StoreTxn.SetPropertyValue(anchor, "link", types.NewAnon(left)); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	program := compileTestProgram(t, rt.registry, "stage_cycle_edge(); run_gc(); return 1;")
	taskID := rt.CreateBackgroundTask(0, program, 0)
	running := rt.GetTask(taskID)
	running.Context.IsWizard = true
	if err := rt.runTask(running); err != nil {
		t.Fatalf("runTask: %v", err)
	}
	if running.Result.Flow != types.FlowReturn || running.Result.Val.Type() != types.TYPE_INT || running.Result.Val.Int() != 1 {
		t.Fatalf("staged-edge run_gc result = %+v, want return 1", running.Result)
	}
	linked, errCode := store.DirectTxn().PropertyValue(anchor, "link")
	if errCode != types.E_NONE || linked.Type() != types.TYPE_ANON || linked.Obj() != left {
		t.Fatalf("persisted anchor edge = %v (%v), want anonymous left #%d", linked, errCode, left)
	}
	for _, id := range []types.ObjID{anchor, left, right} {
		if !store.DirectTxn().Valid(id) {
			t.Errorf("anonymous object #%d recycled despite committed persistent edge", id)
		}
	}
}
