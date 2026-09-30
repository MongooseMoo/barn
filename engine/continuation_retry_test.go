package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestThreadedContinuationRetriesOnlyUnpublishedSlice(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, "once") }))
	defer server.Close()
	for _, tc := range []struct{ name, setup, operation, want string }{
		{"sort", "", `sort({3, 1, 2})`, "{1, 2, 3}"},
		{"all_members", "", `all_members(1, {1, 2, 1})`, "{1, 3}"},
		{"read", "", `read()`, `"delivered"`},
		{"sqlite", `#0.h = sqlite_open(":memory:"); sqlite_query(#0.h, "CREATE TABLE t(x INTEGER)");`, `sqlite_query(#0.h, "INSERT INTO t VALUES (7)")`, "{}"},
		{"curl", "", fmt.Sprintf(`curl(%q)`, server.URL), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newConflictTestStore(t)
			if code := store.DirectTxn().DefineProperty(0, "h", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
				t.Fatal(code)
			}
			entries, attempts := 0, 0
			entry := builtins.BuiltinFunc(func(_ *builtins.Execution, _ []types.Value) types.Result {
				entries++
				return types.Ok(types.NewInt(0))
			})
			conflict := builtins.BuiltinFunc(func(_ *builtins.Execution, _ []types.Value) types.Result {
				attempts++
				if attempts == 1 {
					if code := store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(50)); code != types.E_NONE {
						return types.Err(code)
					}
				}
				return types.Ok(types.NewInt(0))
			})
			s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{OutboundNetwork: true}, 1,
				testBuiltinSlot("entry", 0, 0, nil, &entry), testBuiltinSlot("conflict", 0, 0, nil, &conflict))
			defer s.Stop()
			if tc.name == "read" {
				configureTestHost(s.session, func(host *builtins.Host) { host.ConnManager = newBenchConnManager([]types.ObjID{0}) })
			}
			ticks, seconds := foregroundTaskLimits(s.session)
			if tc.setup != "" {
				setup := task.NewTaskFull(8100, 0, compileTestProgram(t, s.registry, tc.setup+" return 1;"), ticks, seconds)
				setup.Context.IsWizard = true
				runSQLiteTaskToCompletion(t, s, setup)
				if setup.Result.Flow != types.FlowReturn {
					t.Fatalf("setup=%+v", setup.Result)
				}
				defer s.EvalCommandOutput(0, "return sqlite_close(#0.h);")
			}
			queued := task.NewTaskFull(8101, 0, compileTestProgram(t, s.registry, fmt.Sprintf(`
entry();
#0.v = 1;
sorted = %s;
before = #0.v;
conflict();
#0.v = before + 1;
return sorted;
`, tc.operation)), ticks, seconds)
			queued.Context.IsWizard = true
			if tc.name == "read" {
				s.taskManager.RegisterTask(queued)
				if err := s.runTask(queued); err != nil {
					t.Fatal(err)
				}
				if s.taskManager.FindReadingTask(0) != queued {
					t.Fatal("read did not suspend")
				}
				if !s.ResumeReadingTask(0, "delivered") {
					t.Fatal("input was not delivered")
				}
			} else {
				runSQLiteTaskToCompletion(t, s, queued)
			}
			if entries != 1 || attempts != 2 || store.CommitRetries() != 1 || readRootV(t, store) != 51 || queued.Result.Flow != types.FlowReturn || (tc.want != "" && queued.Result.Val.String() != tc.want) {
				t.Fatalf("entries=%d attempts=%d retries=%d final=%d result=%+v", entries, attempts, store.CommitRetries(), readRootV(t, store), queued.Result)
			}
			if tc.name == "curl" && requests.Load() != 1 {
				t.Fatalf("HTTP requests=%d, want 1", requests.Load())
			}
			if tc.name == "sqlite" && s.EvalCommandOutput(0, "return sqlite_query(#0.h, \"SELECT COUNT(*) FROM t\");") != "{1, {{1}}}" {
				t.Fatal("SQL INSERT was replayed")
			}
		})
	}
}

func TestContinuationRetriesDiscardEffectsAndTaskLocalChanges(t *testing.T) {
	store := newConflictTestStore(t)
	attempts, effects, completions := 0, 0, 0
	conflict := builtins.BuiltinFunc(func(ctx *builtins.Execution, _ []types.Value) types.Result {
		attempts++
		if attempts <= 3 {
			if code := store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(int64(attempts*10))); code != types.E_NONE {
				return types.Err(code)
			}
		}
		ctx.PendingEffects = append(ctx.PendingEffects, kernel.PendingEffect{Kind: kernel.PendingEffectAsyncStart, Start: func() { effects++ }})
		return types.Ok(types.NewInt(0))
	})
	rt := newTestRuntimeWithBuiltins(t, store, testBuiltinSlot("conflict", 0, 0, nil, &conflict))
	defer rt.Stop()
	ticks, seconds := foregroundTaskLimits(rt.session)
	queued := task.NewTaskFull(8110, 0, compileTestProgram(t, rt.registry, `set_task_local(7); suspend(0); old = task_local(); set_task_local(old + 1); before = #0.v; conflict(); #0.v = before + 1; return task_local();`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.SetOnComplete(func(types.Result) { completions++ })
	runSQLiteTaskToCompletion(t, rt, queued)
	if attempts != 4 || effects != 1 || completions != 1 || queued.Result.Val.Int() != 8 || readRootV(t, store) != 31 {
		t.Fatalf("attempts=%d effects=%d completions=%d result=%+v v=%d", attempts, effects, completions, queued.Result, readRootV(t, store))
	}
}

func TestContinuationRetryPreservesErrorWakeMode(t *testing.T) {
	for _, errorAsValue := range []bool{false, true} {
		t.Run(map[bool]string{false: "raised", true: "value"}[errorAsValue], func(t *testing.T) {
			store := newConflictTestStore(t)
			attempts := 0
			conflict := builtins.BuiltinFunc(func(_ *builtins.Execution, _ []types.Value) types.Result {
				attempts++
				if attempts == 1 {
					store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(50))
				}
				return types.Ok(types.NewInt(0))
			})
			s := newTestRuntimeWithBuiltins(t, store, testBuiltinSlot("conflict", 0, 0, nil, &conflict))
			defer s.Stop()
			code := `value = suspend(0); before = #0.v; conflict(); #0.v = before + 1; return value;`
			if !errorAsValue {
				code = `try suspend(0); except (E_TYPE) before = #0.v; conflict(); #0.v = before + 1; return E_TYPE; endtry`
			}
			ticks, seconds := foregroundTaskLimits(s.session)
			queued := task.NewTaskFull(8102, 0, compileTestProgram(t, s.registry, code), ticks, seconds)
			queued.Context.IsWizard = true
			if err := s.runTask(queued); err != nil {
				t.Fatal(err)
			}
			queued.WakeValue = types.NewErr(types.E_TYPE)
			queued.WakeErrorAsValue = errorAsValue
			if err := s.runTask(queued); err != nil {
				t.Fatal(err)
			}
			if attempts != 2 || store.CommitRetries() != 1 || queued.Result.Flow != types.FlowReturn || queued.Result.Val.ErrCode() != types.E_TYPE {
				t.Fatalf("attempts=%d retries=%d result=%+v", attempts, store.CommitRetries(), queued.Result)
			}
		})
	}
}

func TestResumedContinuationsJoinConcurrentBatch(t *testing.T) {
	store := newConflictTestStore(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	barrier := builtins.BuiltinFunc(func(_ *builtins.Execution, _ []types.Value) types.Result {
		entered <- struct{}{}
		<-release
		return types.Ok(types.NewInt(0))
	})
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2, testBuiltinSlot("barrier", 0, 0, nil, &barrier))
	defer s.Stop()
	defer releaseOnce()
	ticks, seconds := foregroundTaskLimits(s.session)
	for _, owner := range []types.ObjID{11, 12} {
		queued := task.NewTaskFull(int64(owner), owner, compileTestProgram(t, s.registry, `suspend(0); barrier(); return 1;`), ticks, seconds)
		queued.Context.IsWizard = true
		s.taskManager.RegisterTask(queued)
		if err := s.runTask(queued); err != nil {
			t.Fatal(err)
		}
	}
	finished := make(chan struct{})
	go func() { s.ProcessReadyTasks(); close(finished) }()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("continuations were serialized instead of sharing a batch")
		}
	}
	releaseOnce()
	<-finished
	if store.CommitEscalations() != 0 {
		t.Fatal("ordinary resumed slices took the exclusive commit gate")
	}
}
