package vm

import (
	"strings"
	"testing"

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
		case "eval", "scatter", "expression": // call_function("eval", ...)
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
	// whose queued child's saved code was cut at the inner endfork.
	[]byte("\x00271z0z0"),
	[]byte("0900y0z0z"),
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
		"eval": {"return {{1, {1, 0}}}", "return {{1, {1, {0, 0}}}}", "return {{1, {1, 0}}}"},
		// The handler was entered by falling through (the statement after
		// endtry ran), by a return (the verb's {args[1], 1} arrived, not its
		// later return 0), by a raise (the outer except got code, message and
		// value), by a break (one iteration counted, not two or three).
		"in-finally": {
			"return {{1, 0}}", "return {{{1, 1}, {0, 0}}}",
			`return {{E_PERM, "1", 1, 0}}`, "return {{1, {0, 0}}}",
		},
		// move()'s 0, {layer, inner value, the hook's argument was what},
		// what ended in where, and for exitfunc where:enterfunc then ran.
		"move-accept":    {"return {{0, {1, 0, 1}, 1}}", "return {{0, {1, {0, 0}, 1}, 1}}"},
		"move-exitfunc":  {"return {{0, {1, 0, 1}, 1, 1}}", "return {{0, {1, {0, 0}, 1}, 1, 1}}"},
		"move-enterfunc": {"return {{0, {1, 0, 1}, 1}}", "return {{0, {1, {0, 0}, 1}, 1}}"},
		"fork":           {"return {1}", "return {1}"},
		"except":         {`return {{E_RANGE, "1", 0}}`, `return {{E_RANGE, "1", {0, 0}}}`},
		"finally":        {"return {{1, 0}}", "return {{1, {0, 0}}}"},
		"for-list":       {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"for-range":      {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"for-map":        {"return {{{1, -1}, 0, {3, 1}}}", "return {{{1, -1}, {0, 0}, {3, 1}}}"},
		"while":          {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"scatter":        {"return {{1, {1, 0}, {}}}", "return {{1, {1, {1, {0, 0}}}, {}}}", "return {{1, {1, {1, 0}}, {}}}"},
		"expression":     {"return {{1, {1, 0}, 2}}", "return {{1, {1, {1, {0, 0}}}, 2}}", "return {{1, {1, {1, 0}}, 2}}"},
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
				continue
			}
			if got := state.Get(state.Len()).String(); len(results) != 2 || got != wantForked[i] {
				t.Errorf("fork seed %d: %d tasks, f1 = %s; want 2 tasks, f1 = %s\n%s", i, len(results), got, wantForked[i], source)
			}
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
	for _, seed := range suspendedStackMixedSeeds {
		source := fuzzseed.SuspendedStack(seed)
		if difference := worldCheckpointDifference(t, source, checkpointSuspendedVM); difference != "" {
			t.Fatalf("choices %x: %s\nsource:\n%s", seed, difference, source)
		}
	}
}
