package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/types"
)

// Issue #1083, design C2: a task boundary recycles a WAIF or anonymous object
// it released only when the value is confined to that task. Each test here
// hands a task-created value to another holder through one channel, drops the
// task's own reference, crosses a boundary, and requires that the value was
// not recycled: the other holder still has it. The batched sweep is throttled,
// so a recycle in the window can only come from the boundary decision.

// escapeFixtureSetup builds the fixture in its own task: class c, whose WAIFs
// and anonymous children count their :recycle in c.hits; holder h with a
// property slot; and the flags the multi-task tests coordinate through.
const escapeFixtureSetup = `c = create(#-1);
add_property(c, "hits", 0, {#0, "rw"});
add_property(c, "keeper", c, {#0, "r"});
add_property(c, ":ref", 0, {#0, "rw"});
add_verb(c, {#0, "xd", "new"}, {"this", "none", "this"});
set_verb_code(c, "new", {"w = new_waif();", "return w;"});
add_verb(c, {#0, "xd", ":recycle"}, {"this", "none", "this"});
set_verb_code(c, ":recycle", {"this.class.hits = this.class.hits + 1;"});
add_verb(c, {#0, "rxd", "recycle"}, {"this", "none", "this"});
set_verb_code(c, "recycle", {"k = this.keeper;", "k.hits = k.hits + 1;"});
add_verb(c, {#0, "xd", ":pause"}, {"this", "none", "this"});
set_verb_code(c, ":pause", {"suspend();"});
add_verb(c, {#0, "rxd", "pause"}, {"this", "none", "this"});
set_verb_code(c, "pause", {"suspend();"});
add_verb(c, {#0, "xd", ":pause1"}, {"this", "none", "this"});
set_verb_code(c, ":pause1", {"suspend(0);"});
add_verb(c, {#0, "rxd", "pause1"}, {"this", "none", "this"});
set_verb_code(c, "pause1", {"suspend(0);"});
add_verb(c, {#0, "xd", "make"}, {"this", "none", "this"});
set_verb_code(c, "make", {"return new_waif();"});
add_verb(c, {#0, "xd", "setref"}, {"this", "none", "this"});
set_verb_code(c, "setref", {"w = args[1];", "w.ref = args[2];"});
h = create(#-1);
add_property(h, "slot", 0, {#0, "rw"});
add_property(#0, "c", c, {#0, "rw"});
add_property(#0, "h", h, {#0, "rw"});
add_property(#0, "got", 0, {#0, "rw"});
add_property(#0, "ready", 0, {#0, "rw"});
add_property(#0, "t1", 0, {#0, "rw"});
return 1;`

// The two kinds of finalizable value, made by the task under test.
var escapeMakers = map[string]string{
	"waif": "c:new()",
	"anon": "create(c, 1)",
}

// runEscapeScenario runs body (MAKE replaced by an expression that creates the
// value) as a fresh task after the fixture, with the sweep throttled, and
// returns the task's result.
func runEscapeScenario(t *testing.T, kind, body string) types.Value {
	t.Helper()
	return runEscapeScenarioWith(t, config.DefaultCapabilities(), kind, body)
}

func runEscapeScenarioWith(t *testing.T, capabilities config.Capabilities, kind, body string) types.Value {
	t.Helper()
	store := newConflictTestStore(t)
	options := config.DefaultOptions()
	options.BuiltinCapabilities = &capabilities
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, options, 4)
	runSuspendOrderTask(t, rt, escapeFixtureSetup)
	rt.lifecycle.Mu.Lock()
	rt.lifecycle.LastGCCost = cheapGCSweep
	rt.lifecycle.LastGCSweep = time.Now()
	rt.lifecycle.Mu.Unlock()
	src := "c = #0.c;\nh = #0.h;\n" + strings.ReplaceAll(body, "MAKE", escapeMakers[kind])
	return runSuspendOrderTask(t, rt, src).Result.Val
}

func wantRecycleCount(t *testing.T, got types.Value, want int64, why string) {
	t.Helper()
	if got.Type() != types.TYPE_INT || got.Int() != want {
		t.Fatalf("c.hits = %v, want %d: %s", got, want, why)
	}
}

func forEachKind(t *testing.T, kinds []string, body string, want int64, why string) {
	t.Helper()
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			wantRecycleCount(t, runEscapeScenario(t, kind, body), want, why)
		})
	}
}

var bothKinds = []string{"waif", "anon"}

func TestConfinedEscapeByNumberedProperty(t *testing.T) {
	forEachKind(t, bothKinds, `v = MAKE;
h.slot = v;
v = 0;
suspend(0);
r = c.hits;
h.slot = 0;
return r;`, 0, "h.slot still holds the value")
}

// A value published to a numbered object stays escaped after the holder lets
// it go: other tasks could read the property in between.
func TestConfinedEscapeByNumberedPropertyIsPermanent(t *testing.T) {
	forEachKind(t, bothKinds, `v = MAKE;
h.slot = v;
v = 0;
suspend(0);
h.slot = 0;
suspend(0);
return c.hits;`, 0, "the value was readable through h.slot by other tasks")
}

// Under the slice's exclusive commit grant a publication is tentative; one the
// holder still has when the slice commits escapes.
func TestConfinedEscapeByNumberedPropertyUnderGrant(t *testing.T) {
	forEachKind(t, bothKinds, `v = MAKE;
add_verb(h, {#0, "xd", "spare"}, {"this", "none", "this"});
h.slot = v;
v = 0;
suspend(0);
r = c.hits;
h.slot = 0;
return r;`, 0, "h.slot still holds the value after the slice's commit")
}

// The C2 case: published under the grant, its holder destroyed before the
// slice commits. Nobody else can have it, so it is recycled before resume.
func TestConfinedTentativePublicationRecycledBeforeResume(t *testing.T) {
	forEachKind(t, bothKinds, `v = MAKE;
add_verb(h, {#0, "xd", "spare"}, {"this", "none", "this"});
g = create(#-1);
add_property(g, "slot", v, {#0, "rw"});
recycle(g);
v = 0;
suspend(0);
return c.hits;`, 1, "the only holder was recycled within the slice")
}

// The holder a escapes through h.slot; the value is then reachable only
// through a's property.
func TestConfinedEscapeByAnonymousProperty(t *testing.T) {
	forEachKind(t, bothKinds, `a = create(h, 1);
h.slot = a;
v = MAKE;
a.slot = v;
v = 0;
a = 0;
suspend(0);
r = c.hits;
h.slot.slot = 0;
h.slot = 0;
return r;`, 0, "anonymous object a, which h holds, still holds the value")
}

// The holder escapes through h.slot; the value is then reachable only through
// the holder's property. The write is made in c:setref, whose frame (and the
// assignment's hidden temporary) is gone by the boundary.
func TestConfinedEscapeByWaifProperty(t *testing.T) {
	forEachKind(t, bothKinds, `h.slot = c:new();
v = MAKE;
c:setref(h.slot, v);
v = 0;
suspend(0);
r = c.hits;
h.slot = 0;
return r;`, 0, "the WAIF in h.slot still holds the value")
}

// A tentative publication still held when its slice ended is not confined in
// a later slice, after the holder lets go: other tasks could read it between.
// c:make() keeps the WAIF out of the first slice's releases, so the first
// boundary does not decide it.
func TestConfinedTentativeFromAnEarlierSliceStaysEscaped(t *testing.T) {
	wantRecycleCount(t, runEscapeScenario(t, "waif", `v = c:make();
add_verb(h, {#0, "xd", "spare"}, {"this", "none", "this"});
h.slot = v;
suspend(0);
h.slot = 0;
v = 0;
suspend(0);
return c.hits;`), 0, "the value was readable through h.slot between the slices")
}

func TestConfinedEscapeByFork(t *testing.T) {
	forEachKind(t, bothKinds, `v = MAKE;
fork tid (30)
  x = v;
endfork
v = 0;
suspend(0);
r = c.hits;
kill_task(tid);
return r;`, 0, "the forked task holds the value")
}

func TestConfinedEscapeByResume(t *testing.T) {
	forEachKind(t, bothKinds, `fork tid (0)
  y = suspend();
  #0.got = 1;
  suspend(30);
endfork
suspend(0);
v = MAKE;
resume(tid, v);
v = 0;
while (!#0.got) suspend(0); endwhile
r = c.hits;
kill_task(tid);
return r;`, 0, "the resumed task holds the value")
}

// A subject task creates the value and suspends holding it (hold); this task,
// the spy, reads the subject's state once the subject's suspending slice has
// committed, keeps what it read, and resumes the subject, which drops the
// value and crosses a boundary. read names the subject's task id as sub.
func spyScenario(hold, read string) string {
	return `fork sub (0)
  v = MAKE;
  #0.ready = 1;
  ` + hold + `;
  v = 0;
  suspend(0);
  #0.got = c.hits + 1;
  suspend(30);
endfork
while (!#0.ready) suspend(0); endwhile
seen = ` + read + `;
while (typeof(` + "`resume(sub) ! ANY'" + `) == ERR) suspend(0); endwhile
while (!#0.got) suspend(0); endwhile
kill_task(sub);
return #0.got - 1;`
}

func TestConfinedEscapeByTaskStack(t *testing.T) {
	forEachKind(t, bothKinds, spyScenario("suspend()", "task_stack(sub, 1, 1)"), 0, "the spy holds the value task_stack returned")
}

// queued_tasks() shows a suspended task's "this". Without the Barn extensions
// there is no finished-tasks record, which would hold the value too.
func TestConfinedEscapeByQueuedTasks(t *testing.T) {
	for _, kind := range bothKinds {
		t.Run(kind, func(t *testing.T) {
			got := runEscapeScenarioWith(t, config.DefaultCapabilities()&^config.BarnExtensions, kind,
				spyScenario("v:pause()", "queued_tasks()"))
			wantRecycleCount(t, got, 0, "the spy holds the value queued_tasks returned")
		})
	}
}

// A slice that ran on the value records it as finished_tasks()'s "this".
func TestConfinedEscapeByFinishedTasksRecord(t *testing.T) {
	forEachKind(t, bothKinds, `v = MAKE;
v:pause1();
v = 0;
suspend(0);
return c.hits;`, 0, "the finished-tasks record holds the value")
}

func TestConfinedEscapeBySetConnectionOption(t *testing.T) {
	forEachKind(t, bothKinds, `v = MAKE;
`+"`set_connection_option(#0, \"x\", v) ! ANY'"+`;
v = 0;
suspend(0);
return c.hits;`, 0, "a connection would keep the option value")
}

// Without any channel the value is recycled at the boundary: the controls for
// the tests above.
func TestConfinedValueRecycledAtSuspend(t *testing.T) {
	forEachKind(t, bothKinds, `v = MAKE;
v = 0;
suspend(0);
return c.hits;`, 1, "the value was confined and released before suspend(0)")
}

func TestConfinedValueRecycledBeforeCompletion(t *testing.T) {
	for _, kind := range bothKinds {
		t.Run(kind, func(t *testing.T) {
			store := newConflictTestStore(t)
			rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), 4)
			runSuspendOrderTask(t, rt, escapeFixtureSetup)
			rt.lifecycle.Mu.Lock()
			rt.lifecycle.LastGCCost = cheapGCSweep
			rt.lifecycle.LastGCSweep = time.Now()
			rt.lifecycle.Mu.Unlock()
			runSuspendOrderTask(t, rt, "c = #0.c;\nv = "+escapeMakers[kind]+";\nv = 0;\nreturn 0;")
			// The next command sees the recycle the completed one queued.
			wantRecycleCount(t, runSuspendOrderTask(t, rt, "return #0.c.hits;").Result.Val, 1,
				"a completed task's confined garbage is recycled before its completion is delivered")
		})
	}
}

// Server hooks receive their arguments in a task of their own.
func TestConfinedEscapeByHookArguments(t *testing.T) {
	store := newConflictTestStore(t)
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), 4)
	runSuspendOrderTask(t, rt, escapeFixtureSetup+"\n")
	runSuspendOrderTask(t, rt, `add_verb(#0, {#0, "xd", "hook"}, {"this", "none", "this"});
set_verb_code(#0, "hook", {"return 0;"});
return 0;`)
	waif := types.NewWaif(0, 0)
	store.StampFinalizerCreator(waif, 777)
	if _, err := rt.RunServerVerbTaskWithArgstr(0, "hook", []types.Value{types.NewList([]types.Value{waif})}, 0, "", nil); err != nil {
		t.Fatal(err)
	}
	if tag := store.FinalizerTag(waif); tag.ConfinedTo(777, 0) {
		t.Fatalf("a WAIF passed to a server hook is still confined to its creator")
	}
}
