package vm

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
)

// suspendedStackSeeds returns, for each context kind, one-layer stacks of it
// with each variant the kind reads and both forms of the bottom suspension,
// named by kind.
func suspendedStackSeeds() map[string][][]byte {
	seeds := map[string][][]byte{}
	for kind, name := range fuzzseed.SuspendedStackKinds {
		tails := [][2]byte{{0, 0}, {1, 1}}
		switch name {
		case "eval", "scatter", "expression", // call_function("eval", ...)
			"move-accept", "move-exitfunc", "move-enterfunc", // and the hook kinds' builtin
			"create-initialize", "recreate-initialize", "recycle-recycle", "recycle-exitfunc":
			tails = append(tails, [2]byte{2, 0})
		case "in-finally": // the handler entered by a raise and by a break
			tails = append(tails, [2]byte{2, 0}, [2]byte{3, 1})
		}
		for _, tail := range tails {
			// One stack, one layer: kind, variant, bottom form.
			seeds[name] = append(seeds[name], []byte{0, 0, byte(kind), tail[0], tail[1]})
		}
	}
	return seeds
}

// suspendedStackMixedSeeds nest every kind: three stacks of depths six, five
// and six, the last with each call layer's other variant.
var suspendedStackMixedSeeds = [][]byte{
	{
		2,
		5, 0, 0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 0, 1,
		4, 6, 0, 7, 0, 8, 0, 9, 1, 10, 0, 0,
		5, 10, 1, 9, 0, 1, 0, 2, 0, 0, 0, 1, 0, 1,
	},
	// eval() in eval() in a verb in a fork in eval() in a fork.
	{0, 5, 2, 0, 1, 0, 2, 0, 0, 0, 1, 0, 1, 0, 1},
	// A finally handler entered by a raise, in call_function("eval", ...), in
	// accept, in a handler entered by a break, in exitfunc, in a handler
	// entered by a return; then a handler entered by falling through, in
	// enterfunc.
	{
		1,
		5, 11, 1, 13, 0, 11, 3, 12, 0, 1, 2, 11, 2, 1,
		1, 14, 0, 11, 0, 0,
	},
	// Found by the lane: a fork in a fork on the one line of eval()'s source,
	// whose queued child's saved code was cut at the inner endfork. The lane
	// found them as "\x00271z0z0" and "0900y0z0z", when there were 15 kinds;
	// here each kind byte is that kind's index.
	{0, '2', 10, '1', 2, '0', 2, '0'},
	{'0', '9', 3, '0', 1, '0', 2, '0', 2},
}

// FuzzSuspendedStackCheckpoint requires a program suspended at the bottom of a
// generated stack of contexts to end the same way, and leave the same state,
// whether or not every suspension is checkpointed, reloaded and restored.
func FuzzSuspendedStackCheckpoint(f *testing.F) {
	for _, seeds := range suspendedStackSeeds() {
		for _, seed := range seeds {
			f.Add(seed)
		}
	}
	for _, seed := range suspendedStackMixedSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, choices []byte) {
		source := fuzzseed.SuspendedStack(choices)
		if difference := worldCheckpointDifference(t, source, checkpointSuspendedVM); difference != "" {
			t.Fatalf("suspended stack resumed differently: %s\nchoices: %x\nsource:\n%s", difference, choices, source)
		}
	})
}

// Each layer's mark on the value must be in the checkpointed run's results:
// a generator whose layers left no trace would compare equal and check nothing.
func TestSuspendedStackLayersMarkResult(t *testing.T) {
	// The root task's result for each of the kind's seeds. The second seed's
	// bottom value is {0, 0} and its scatter and expression calls use eval();
	// a third calls eval() through call_function.
	want := map[string][]string{
		"verb": {"return {{1, 0}}", "return {{1, {0, 0}}}"},
		// eval()'s {1, {layer, inner value, the name callers() gave its builtin
		// entry}}: eval, and call_function for the third seed.
		"eval": {`return {{1, {1, 0, "eval"}}}`, `return {{1, {1, {0, 0}, "eval"}}}`, `return {{1, {1, 0, "call_function"}}}`},
		// The handler was entered by falling through (the statement after
		// endtry ran), by a return (the verb's {args[1], 1} arrived, not its
		// later return 0), by a raise (the outer except got code, message and
		// value), by a break (one iteration counted, not two or three).
		"in-finally": {
			"return {{1, 0}}", "return {{{1, 1}, {0, 0}}}",
			`return {{E_PERM, "1", 1, 0}}`, "return {{1, {0, 0}}}",
		},
		// move()'s 0, {layer, inner value, the hook's argument was what, the
		// name of the builtin entry below the hook in callers()}, what ended in
		// where, and for exitfunc the name enterfunc's builtin entry then had,
		// which move() took from the resumed exitfunc's frame. The third seed
		// calls move() through call_function, so Toast names that entry
		// call_function (execute.cc OP_BI_FUNC_CALL, unwind_stack).
		"move-accept":    {`return {{0, {1, 0, 1, "move"}, 1}}`, `return {{0, {1, {0, 0}, 1, "move"}, 1}}`, `return {{0, {1, 0, 1, "call_function"}, 1}}`},
		"move-exitfunc":  {`return {{0, {1, 0, 1, "move"}, 1, "move"}}`, `return {{0, {1, {0, 0}, 1, "move"}, 1, "move"}}`, `return {{0, {1, 0, 1, "call_function"}, 1, "call_function"}}`},
		"move-enterfunc": {`return {{0, {1, 0, 1, "move"}, 1}}`, `return {{0, {1, {0, 0}, 1, "move"}, 1}}`, `return {{0, {1, 0, 1, "call_function"}, 1}}`},
		// create() and recreate() returned the object, which has parent, and
		// {layer, inner value, initialize's this has parent, builtin entry
		// name}; recycle()'s 0, {layer, inner value, the hook's this or
		// argument was what, builtin entry name}, what is no longer valid. The
		// third seed calls the builtin through call_function.
		"create-initialize":   {`return {{1, {1, 0, 1, "create"}}}`, `return {{1, {1, {0, 0}, 1, "create"}}}`, `return {{1, {1, 0, 1, "call_function"}}}`},
		"recreate-initialize": {`return {{1, {1, 0, 1, "recreate"}}}`, `return {{1, {1, {0, 0}, 1, "recreate"}}}`, `return {{1, {1, 0, 1, "call_function"}}}`},
		"recycle-recycle":     {`return {{0, {1, 0, 1, "recycle"}, 0}}`, `return {{0, {1, {0, 0}, 1, "recycle"}, 0}}`, `return {{0, {1, 0, 1, "call_function"}, 0}}`},
		"recycle-exitfunc":    {`return {{0, {1, 0, 1, "recycle"}, 0}}`, `return {{0, {1, {0, 0}, 1, "recycle"}, 0}}`, `return {{0, {1, 0, 1, "call_function"}, 0}}`},
		"fork":                {"return {1}", "return {1}"},
		"except":              {`return {{E_RANGE, "1", 0}}`, `return {{E_RANGE, "1", {0, 0}}}`},
		"finally":             {"return {{1, 0}}", "return {{1, {0, 0}}}"},
		"for-list":            {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"for-range":           {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"for-map":             {"return {{{1, -1}, 0, {3, 1}}}", "return {{{1, -1}, {0, 0}, {3, 1}}}"},
		"while":               {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"scatter":             {"return {{1, {1, 0}, {}}}", `return {{1, {1, {1, {0, 0}, "eval"}}, {}}}`, `return {{1, {1, {1, 0, "call_function"}}, {}}}`},
		"expression":          {"return {{1, {1, 0}, 2}}", `return {{1, {1, {1, {0, 0}, "eval"}}, 2}}`, `return {{1, {1, {1, 0, "call_function"}}, 2}}`},
	}
	// The fork layer's child stores its value in f1, the property defined last.
	wantForked := [2]string{"{1, 0}", "{1, {0, 0}}"}
	registry := newCheckpointResumeRegistry(t)
	seeds := suspendedStackSeeds()
	for _, kind := range fuzzseed.SuspendedStackKinds {
		if len(want[kind]) != len(seeds[kind]) {
			t.Fatalf("%s: %d pinned results for %d seeds", kind, len(want[kind]), len(seeds[kind]))
		}
		for i, seed := range seeds[kind] {
			// A subtest per seed: checkpointSuspendedVM skips a seed whose
			// suspended fork has no comparable line numbers, which must not
			// skip the seeds after it.
			t.Run(fmt.Sprintf("%s/%d", kind, i), func(t *testing.T) {
				source := fuzzseed.SuspendedStack(seed)
				program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
				if len(diagnostics) != 0 {
					t.Fatalf("%s: %v\n%s", kind, diagnostics, source)
				}
				results, state, _ := runCheckpointResumeCase(t, registry, program, checkpointSuspendedVM, 30000, true)
				if got := describeTaskResult(results[0]); got != want[kind][i] {
					t.Errorf("%s seed %d: root task %s, want %s\n%s", kind, i, got, want[kind][i], source)
				}
				if kind != "fork" {
					return
				}
				if got := state.Get(state.Len()).String(); len(results) != 2 || got != wantForked[i] {
					t.Errorf("fork seed %d: %d tasks, f1 = %s; want 2 tasks, f1 = %s\n%s", i, len(results), got, wantForked[i], source)
				}
			})
		}
	}
}

func TestSuspendedStackKindsReachCheckpoint(t *testing.T) {
	seeds := suspendedStackSeeds()
	for _, kind := range fuzzseed.SuspendedStackKinds {
		t.Run(kind, func(t *testing.T) {
			if len(seeds[kind]) == 0 {
				t.Fatal("no seed")
			}
			for _, seed := range seeds[kind] {
				source := fuzzseed.SuspendedStack(seed)
				if difference := worldCheckpointDifference(t, source, checkpointSuspendedVM); difference != "" {
					t.Fatalf("choices %x: %s\nsource:\n%s", seed, difference, source)
				}
			}
		})
	}
	for i, seed := range suspendedStackMixedSeeds {
		// A subtest per seed, so one seed's skip does not skip the others.
		t.Run(fmt.Sprintf("mixed/%d", i), func(t *testing.T) {
			source := fuzzseed.SuspendedStack(seed)
			if difference := worldCheckpointDifference(t, source, checkpointSuspendedVM); difference != "" {
				t.Fatalf("choices %x: %s\nsource:\n%s", seed, difference, source)
			}
		})
	}
}

// A task suspended in a verb that create(), recreate() or recycle() called,
// directly or through call_function, is refused at that verb's frame with
// ErrToastLosesObjectContinuation, before any other frame is exported. The
// refusal names the frame's builtin: call_function when the builtin was called
// through it, as Toast names that frame.
func TestExportRefusesObjectContinuation(t *testing.T) {
	want := map[string][2]string{
		"create-initialize":   {"frame 1 (#4:initialize)", "create"},
		"recreate-initialize": {"frame 1 (#4:initialize)", "recreate"},
		"recycle-recycle":     {"frame 1 (#4:recycle)", "recycle"},
		"recycle-exitfunc":    {"frame 1 (#5:exitfunc)", "recycle"},
	}
	seeds := suspendedStackSeeds()
	for kind, frame := range want {
		// The first seed calls the builtin directly, the third through
		// call_function.
		for i, seed := range [][]byte{seeds[kind][0], seeds[kind][2]} {
			builtin := frame[1]
			if i == 1 {
				builtin = "call_function"
			}
			reason := frame[0] + ": " + builtin + ": " + ErrToastLosesObjectContinuation.Error()
			t.Run(kind, func(t *testing.T) {
				var refusals []string
				record := func(t *testing.T, machine *VM, store *dbstore.Store, forks []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork) {
					snapshot := suspendedSnapshot(machine)
					_, err := ExportNativeSnapshot(snapshot.VM, machine.Task.GetCallStack(), machine.Builtins.Registry().Compiler())
					if !errors.Is(err, ErrToastLosesObjectContinuation) {
						t.Fatalf("ExportNativeSnapshot: %v, want %v", err, ErrToastLosesObjectContinuation)
					}
					refusals = append(refusals, err.Error())
					return checkpointSuspendedVMInBarnForm(t, machine, store, forks)
				}
				source := fuzzseed.SuspendedStack(seed)
				if difference := worldCheckpointDifference(t, source, record); difference != "" {
					t.Fatalf("choices %x: %s\nsource:\n%s", seed, difference, source)
				}
				if len(refusals) != 1 || refusals[0] != reason {
					t.Fatalf("refusals %q, want [%q]\nsource:\n%s", refusals, reason, source)
				}
			})
		}
	}
}

// A host too starved to finish a task inside its wall-clock budget must make
// the world-history lane skip the case, not report that the program failed to
// complete. The input is the one recorded by issue #902, which a loaded fuzz
// runner stopped at the seconds budget; here every task's deadline has passed
// before it starts, in both the uninterrupted and the checkpointed run.
func TestWorldHistoryLaneSkipsSecondsBudget(t *testing.T) {
	saved := sourceFuzzTaskBudget
	sourceFuzzTaskBudget = 0
	defer func() { sourceFuzzTaskBudget = saved }()
	cases := map[string]string{
		// Recorded with 15 kinds, when a move kind ignored its variant; each
		// kind byte here is that kind's index, and the move-exitfunc layer's
		// variant is 0 (direct move()) where it was 2.
		"issue-902": fuzzseed.SuspendedStack([]byte("\x92\x0b\x0d\x20\x05\xff\x00\x05\x0d\x1c\x0d\x1c\x0d\x1c\x1c\x1c\x0d\x1c\x0d\x85\x0d\x20\x0d\x85\x0d\x1c\x1c\x20\x0a\x8d\x0d\x85\x0d\x1c\x1c\x1c\x1c\x1c\x8d\xf7\x35\xf1\xa3\x50\xff")),
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			difference, skip := checkpointResumeDifference(t, source, checkpointSuspendedVM, 30000, true)
			if skip != secondsBudgetSkip || difference != "" {
				t.Fatalf("difference %q, skip %q; want skip %q", difference, skip, secondsBudgetSkip)
			}
		})
	}
}
