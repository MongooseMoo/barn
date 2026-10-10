package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/mooprof"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/google/pprof/profile"
)

// spinCode is a verb body long enough to cross several tick checkpoints.
const spinCode = `{"x = 0;", "for i in [1..3000]", "x = x + i;", "endfor", "return x;"}`

// profileCall runs setup, then profiles a task running call, whose second line
// starts the verb under test. It returns the profile, setup's result, and the
// ticks the profiled task charged.
func profileCall(t *testing.T, setup, call string) (*profile.Profile, types.Value, int64) {
	t.Helper()
	store := newConflictTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	t.Cleanup(s.Stop)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	run := func(id int64, src string) *task.Task {
		tk := task.NewTaskFull(id, 0, compileTestProgram(t, s.registry, src), ticks, seconds)
		tk.Context.IsWizard = true
		if err := s.runTask(tk); err != nil {
			t.Fatalf("runTask %d failed: %v", id, err)
		}
		runUntilTerminal(t, s, tk)
		if tk.Result.Flow == types.FlowException {
			t.Fatalf("task %d raised %v %v", id, tk.Result.Error, tk.Result.Val)
		}
		return tk
	}
	setupTask := run(5301, setup)

	session, err := mooprof.Start()
	if err != nil {
		t.Fatalf("start profile: %v", err)
	}
	defer session.Stop()
	caller := run(5302, call)
	return session.Stop(), setupTask.Result.Val, caller.TicksLimit - caller.Context.TicksRemaining
}

// requireCallerFrames checks that samples inside fn continue into the calling
// verb's frame at line 2, and that the session's ticks add up to the task's.
func requireCallerFrames(t *testing.T, prof *profile.Profile, fn string, charged int64) {
	t.Helper()
	var inside int
	var total int64
	for _, sample := range prof.Sample {
		total += sample.Value[0]
		if sample.Location[0].Line[0].Function.Name != fn {
			continue
		}
		inside++
		if len(sample.Location) < 2 {
			t.Fatalf("sample inside %s has no caller frames", fn)
		}
		outer := sample.Location[len(sample.Location)-1].Line[0]
		if outer.Line != 2 {
			t.Errorf("caller frame %s line = %d, want 2", outer.Function.Name, outer.Line)
		}
	}
	if inside == 0 {
		t.Fatalf("no sample was taken inside %s", fn)
	}
	if total != charged {
		t.Fatalf("sampled ticks = %d, task charged %d", total, charged)
	}
}

// A waif method call carries the calling verb's frames.
func TestMOOProfileWaifMethodCarriesCallerFrames(t *testing.T) {
	prof, class, charged := profileCall(t, `c = create(-1);
add_verb(c, {#0, "xd", "new"}, {"this", "none", "this"});
set_verb_code(c, "new", {"return new_waif();"});
add_verb(c, {#0, "xd", ":spin"}, {"this", "none", "this"});
set_verb_code(c, ":spin", `+spinCode+`);
add_property(#0, "holder", c:new(), {#0, "rw"});
return c;`, `w = #0.holder;
y = w:spin();
return y;`)
	requireCallerFrames(t, prof, class.String()+"::spin", charged)
}

// create() runs :initialize on a separate pooled VM. Its samples must
// continue into the caller's frames, and its ticks must not be billed again
// to the caller once the caller absorbs them.
func TestMOOProfileNestedVMCarriesCallerFrames(t *testing.T) {
	prof, class, charged := profileCall(t, `c = create(-1);
add_verb(c, {#0, "xd", "initialize"}, {"this", "none", "this"});
set_verb_code(c, "initialize", `+spinCode+`);
return c;`, `c = #0;
o = create(`+"#1"+`);
return o;`)
	if class.Obj() != 1 {
		t.Fatalf("setup created %v, want #1", class)
	}
	requireCallerFrames(t, prof, "#1:initialize", charged)
}
