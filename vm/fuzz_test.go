package vm

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// sourceFuzzTicks is the one tick budget an input's root task, its suspend
// resumes and its forked children share.
const sourceFuzzTicks = 100

func FuzzSourceExecution(f *testing.F) {
	fuzzseed.Add(f)
	f.Fuzz(func(t *testing.T, source string) {
		// Core capabilities exclude optional network, SQLite and process input.
		// State and compiler caches are private to this input, with no server.
		registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
		if err != nil {
			t.Fatal(err)
		}
		program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
		if len(diagnostics) != 0 {
			return
		}
		session := newTestSessionWithTaskManager(registry)
		defer func() {
			if err := session.Close(); err != nil {
				t.Error(err)
			}
		}()
		store := dbstore.NewStore()
		tasks := &sourceFuzzTasks{nextID: 2}
		machine := newSourceFuzzVM(store, session, 1, sourceFuzzTicks)
		// Errors and tick exhaustion are results; a Go panic fails the case.
		result := tasks.run(t, machine, machine.Run(program))
		spent := machine.Ticks
		if result.Flow == types.FlowReturn {
			checkLiteralRoundTrip(t, store, session, result.Val)
		}
		// Forked children run after their creator, in creation order, on the
		// ticks the earlier tasks left. A child that would start with none left
		// is not run. Children fork further children onto the same queue.
		for len(tasks.forks) > 0 && spent < sourceFuzzTicks {
			fork := tasks.forks[0]
			tasks.forks = tasks.forks[1:]
			child := NewForkVM(store, session, fork.id, fork.info, ForkBodyProgram(fork.info), sourceFuzzTicks-spent)
			attachSourceFuzzTask(child, fork.id)
			tasks.run(t, child, child.ExecuteLoop())
			spent += child.Ticks
		}
	})
}

// sourceFuzzTasks numbers forked children and queues them to run.
type sourceFuzzTasks struct {
	nextID int64
	forks  []sourceFuzzFork
}

type sourceFuzzFork struct {
	id   int64
	info *types.ForkInfo
}

// newSourceFuzzVM returns a VM on store with its own task context and state.
func newSourceFuzzVM(store *dbstore.Store, session *builtins.Session, id, ticks int64) *VM {
	machine := NewVM(store, session)
	machine.TickLimit = ticks
	attachSourceFuzzTask(machine, id)
	return machine
}

func attachSourceFuzzTask(machine *VM, id int64) {
	context := task.NewContext()
	context.TicksRemaining = machine.TickLimit
	machine.Context = context
	machine.Task = task.NewState(id, context.Player, machine.TickLimit, 1)
}

// run drives one task to its end the way the runtime does: a fork yield
// queues the child under a new task id (SetForkResult, then Resume, as
// engine.Runtime.drainForks) and a suspend() yield resumes at once with the
// default value (as engine.Runtime.launchSlice). A delay is scheduler timing,
// so it is not waited out.
//
// The task must end: finished or raised, never left yielded. Every yield
// charges at least one tick, as in Toast, where OP_FORK, OP_FORK_WITH_ID and
// OP_BI_FUNC_CALL each cost one (src/include/opcode.h:44-50, COUNT_TICK at
// opcode.h:129), so a task cannot yield more often than its budget allows.
func (tasks *sourceFuzzTasks) run(t *testing.T, machine *VM, result types.Result) types.Result {
	t.Helper()
	before := int64(-1)
	for result.Flow == types.FlowFork || result.Flow == types.FlowSuspend {
		if !machine.IsYielded() {
			t.Fatalf("flow %v returned without a yielded VM", result.Flow)
		}
		if machine.Ticks <= before {
			t.Fatalf("task yielded again at tick %d without charging a tick", machine.Ticks)
		}
		before = machine.Ticks
		if result.Flow == types.FlowFork {
			if ForkBodyProgram(result.ForkInfo) == nil {
				t.Fatalf("fork yield without a fork body: %+v", result.ForkInfo)
			}
			tasks.forks = append(tasks.forks, sourceFuzzFork{id: tasks.nextID, info: result.ForkInfo})
			machine.SetForkResult(tasks.nextID)
			tasks.nextID++
		}
		result = machine.Resume()
	}
	if machine.IsYielded() {
		t.Fatalf("task ended with flow %v but its VM is still yielded", result.Flow)
	}
	if result.Flow != types.FlowReturn && result.Flow != types.FlowException {
		t.Fatalf("task ended with flow %v, want a return or an exception", result.Flow)
	}
	return result
}

// checkLiteralRoundTrip requires toliteral(value), compiled and evaluated
// again, to equal value under equal(). Toast's toliteral is unparse_value
// (list.cc:1189 bf_toliteral, list.cc:449 unparse_value). Its output is a value
// expression for INT, OBJ, ERR, STR, LIST, MAP and BOOL. It is not for:
//   - FLOAT: printed with %.*g and DBL_DIG, 15 significant digits
//     (list.cc:460-465), so 0.1 + 0.2 prints 0.3 and reads back unequal.
//   - ANON: printed as *anonymous*, not an expression.
//   - WAIF: printed as [[class = #c, owner = #o]], not an expression.
//
// A value containing any of these, at any depth, is skipped.
func checkLiteralRoundTrip(t *testing.T, store *dbstore.Store, session *builtins.Session, value types.Value) {
	t.Helper()
	if !literalReadsBack(value) {
		return
	}
	machine := newSourceFuzzVM(store, session, 1, defaultTickLimit)
	execution := session.NewExecution(machine.Context, machine.Task)
	literal, ok := session.CallByNameWithExecution("toliteral", execution, []types.Value{value})
	if !ok {
		t.Fatal("toliteral is not registered")
	}
	if literal.Flow == types.FlowException {
		return // e.g. E_QUOTA past the string limit; a result, not a round trip
	}
	source := "return " + literal.Val.Str() + ";"
	program, diagnostics := session.Registry().Compiler().CompileMOO([]string{source})
	if len(diagnostics) != 0 {
		t.Fatalf("toliteral of %s does not compile: %v", literal.Val.Str(), diagnostics)
	}
	reread := machine.Run(program)
	if reread.Flow != types.FlowReturn {
		t.Fatalf("toliteral of %s evaluated to flow %v error %v", literal.Val.Str(), reread.Flow, reread.Error)
	}
	same, ok := session.CallByNameWithExecution("equal", execution, []types.Value{value, reread.Val})
	if !ok {
		t.Fatal("equal is not registered")
	}
	if !same.Val.Truthy() {
		t.Fatalf("toliteral round trip changed %s: %s reads back as %s", value.String(), literal.Val.Str(), reread.Val.String())
	}
}

func literalReadsBack(value types.Value) bool {
	switch value.Type() {
	case types.TYPE_INT, types.TYPE_OBJ, types.TYPE_ERR, types.TYPE_STR, types.TYPE_BOOL:
		return true
	case types.TYPE_LIST:
		for _, element := range value.Elements() {
			if !literalReadsBack(element) {
				return false
			}
		}
		return true
	case types.TYPE_MAP:
		for _, pair := range value.Pairs() {
			if !literalReadsBack(pair[0]) || !literalReadsBack(pair[1]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
