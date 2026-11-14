package vm

import (
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// sourceFuzzTicks is the one tick budget an input's root task, its suspend
// resumes and its forked children share.
const sourceFuzzTicks = 100

// sourceFuzzWorldTicks is the budget of the source-execution lanes, which run
// in the populated world, where a verb call chain spends ticks in each verb.
const sourceFuzzWorldTicks = 100

func FuzzSourceExecution(f *testing.F) {
	fuzzseed.Add(f)
	for _, seed := range sourceFuzzWorldSeeds() {
		f.Add(seed)
	}
	world := newSourceFuzzWorld(f)
	f.Fuzz(func(t *testing.T, source string) {
		executeFuzzSource(t, world, source, false)
	})
}

// FuzzStructuredSourceExecution runs generated programs. Each is valid MOO,
// so every case reaches execution; a compile diagnostic fails the case.
func FuzzStructuredSourceExecution(f *testing.F) {
	fuzzseed.AddChoices(f)
	world := newSourceFuzzWorld(f)
	f.Fuzz(func(t *testing.T, choices []byte) {
		executeFuzzSource(t, world, fuzzseed.StructuredSource(t, choices), true)
	})
}

// executeFuzzSource runs source once as each principal, each time in a fresh
// copy of world.
func executeFuzzSource(t *testing.T, world sourceFuzzWorld, source string, mustCompile bool) {
	registry, program, diagnostics := compileFuzzSource(t, source)
	if len(diagnostics) != 0 {
		if mustCompile {
			t.Fatalf("generated program does not compile: %v\nsource:\n%s", diagnostics, source)
		}
		return
	}
	for _, principal := range sourceFuzzPrincipals {
		runSourceFuzzProgram(t, world, registry, program, principal)
	}
}

// compileFuzzSource compiles source with a new registry of the core
// capabilities, which exclude optional network, SQLite and process input.
// State and compiler caches are private to this input, with no server.
func compileFuzzSource(t *testing.T, source string) (*builtins.Registry, *bytecode.Program, []compiler.Diagnostic) {
	t.Helper()
	registry := newSourceFuzzRegistry(t, t.Skipf)
	program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	return registry, program, diagnostics
}

// runSourceFuzzProgram runs program and its forked children as principal in
// a fresh copy of world, checks a returned value's literal round trip, and
// returns every task's result.
func runSourceFuzzProgram(t *testing.T, world sourceFuzzWorld, registry *builtins.Registry, program *bytecode.Program, principal sourceFuzzPrincipal) []types.Result {
	t.Helper()
	session := newTestSessionWithTaskManager(registry)
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	tasks := &sourceFuzzTasks{nextID: 2, store: world.fresh(t), session: session, ticks: sourceFuzzWorldTicks, wizard: principal.wizard, player: principal.player}
	results := tasks.runAll(t, program)
	if results[0].Flow == types.FlowReturn {
		checkLiteralRoundTrip(t, tasks.store, session, results[0].Val)
	}
	return results
}

// sourceFuzzTasks numbers forked children and queues them to run.
type sourceFuzzTasks struct {
	nextID  int64
	forks   []sourceFuzzFork
	store   *dbstore.Store
	session *builtins.Session
	ticks   int64 // zero selects the raw-source budget
	wizard  bool  // the world-history fixture executes as its wizard #0
	// player is the object the root task runs as when wizard is set or
	// player is not #0. Left #0 with wizard unset, the task runs as #-1.
	player types.ObjID
	// checkpoint, when set, replaces each task suspended by suspend() with
	// one restored from a checkpoint, and returns the store the remaining
	// execution uses. suspends counts the replacements.
	checkpoint checkpointFunc
	suspends   int
}

// runAll runs program as task 1 and then its forked children, and returns
// every run task's terminal result in run order. Errors and tick exhaustion
// are results; a Go panic fails the case.
func (tasks *sourceFuzzTasks) runAll(t *testing.T, program *bytecode.Program) []types.Result {
	t.Helper()
	budget := tasks.ticks
	if budget == 0 {
		budget = sourceFuzzTicks
	}
	machine := newSourceFuzzVM(tasks.store, tasks.session, 1, budget)
	machine.Context.IsWizard = tasks.wizard
	if tasks.wizard || tasks.player != 0 {
		runAs(machine, tasks.player, tasks.wizard)
	}
	result, machine := tasks.run(t, machine, machine.Run(program))
	results := []types.Result{result}
	spent := machine.Ticks
	// Forked children run after their creator, in creation order, on the
	// ticks the earlier tasks left. A child that would start with none left
	// is not run. Children fork further children onto the same queue.
	for len(tasks.forks) > 0 && spent < budget {
		fork := tasks.forks[0]
		tasks.forks = tasks.forks[1:]
		program := fork.program
		if program == nil {
			program = ForkBodyProgram(fork.info)
		}
		child := NewForkVM(tasks.store, tasks.session, fork.id, fork.info, program, budget-spent)
		attachSourceFuzzTask(child, fork.id)
		child.Context.Programmer = fork.programmer
		child.Context.Player = fork.info.Player
		child.Context.ThisObj = fork.info.ThisObj
		child.Context.IsWizard = tasks.wizard
		result, child = tasks.run(t, child, child.ExecuteLoop())
		results = append(results, result)
		spent += child.Ticks
	}
	return results
}

type sourceFuzzFork struct {
	id         int64
	info       *types.ForkInfo
	program    *bytecode.Program // recompiled body of a checkpoint-restored child
	programmer types.ObjID
}

// newSourceFuzzVM returns a VM on store with its own task context and state.
func newSourceFuzzVM(store *dbstore.Store, session *builtins.Session, id, ticks int64) *VM {
	machine := NewVM(store, session)
	machine.TickLimit = ticks
	attachSourceFuzzTask(machine, id)
	return machine
}

// runAs makes player the player, programmer and this of machine's task.
func runAs(machine *VM, player types.ObjID, wizard bool) {
	machine.Context.IsWizard = wizard
	machine.Context.Programmer = player
	machine.Context.Player = player
	machine.Context.ThisObj = player
	machine.Task.Owner = player
	machine.Task.Programmer = player
}

func attachSourceFuzzTask(machine *VM, id int64) {
	context := task.NewContext()
	context.TicksRemaining = machine.TickLimit
	machine.Context = context
	machine.Task = task.NewState(id, context.Player, machine.TickLimit, 1)
	// A named while test charges only an unchecked tick (Toast EOP_WHILE_ID,
	// execute.cc:2286-2288), so such a loop is stopped by the seconds budget
	// at a tick checkpoint, which needs a running deadline.
	machine.Task.SetExecutionDeadline(time.Now().Add(100 * time.Millisecond))
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
// It returns the result and the VM that produced it, which tasks.checkpoint
// may have replaced.
func (tasks *sourceFuzzTasks) run(t *testing.T, machine *VM, result types.Result) (types.Result, *VM) {
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
			tasks.forks = append(tasks.forks, sourceFuzzFork{id: tasks.nextID, info: result.ForkInfo, programmer: machine.Context.Programmer})
			machine.SetForkResult(tasks.nextID)
			tasks.nextID++
		} else if tasks.checkpoint != nil {
			machine, tasks.store, tasks.forks = tasks.checkpoint(t, machine, tasks.store, tasks.forks)
			tasks.suspends++
		}
		result = machine.Resume()
	}
	if machine.IsYielded() {
		t.Fatalf("task ended with flow %v but its VM is still yielded", result.Flow)
	}
	if result.Flow != types.FlowReturn && result.Flow != types.FlowException {
		t.Fatalf("task ended with flow %v, want a return or an exception", result.Flow)
	}
	return result, machine
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
