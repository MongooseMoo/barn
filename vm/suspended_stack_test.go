package vm

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/internal/fuzzseed"
)

// suspendedStackSeeds returns, for each context kind, one-layer stacks of it
// with either variant and either form of the bottom suspension, named by kind.
func suspendedStackSeeds() map[string][][]byte {
	seeds := map[string][][]byte{}
	for kind, name := range fuzzseed.SuspendedStackKinds {
		for _, tail := range [][2]byte{{0, 0}, {1, 1}} {
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
	// The root task's result for each kind's two seeds. The second seed's
	// bottom value is {0, 0} and its scatter and expression calls use eval().
	want := map[string][2]string{
		"verb":       {"return {{1, 0}}", "return {{1, {0, 0}}}"},
		"eval":       {"return {{1, {1, 0}}}", "return {{1, {1, {0, 0}}}}"},
		"fork":       {"return {1}", "return {1}"},
		"except":     {`return {{E_RANGE, "1", 0}}`, `return {{E_RANGE, "1", {0, 0}}}`},
		"finally":    {"return {{1, 0}}", "return {{1, {0, 0}}}"},
		"for-list":   {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"for-range":  {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"for-map":    {"return {{{1, -1}, 0, {3, 1}}}", "return {{{1, -1}, {0, 0}, {3, 1}}}"},
		"while":      {"return {{1, 0, 3}}", "return {{1, {0, 0}, 3}}"},
		"scatter":    {"return {{1, {1, 0}, {}}}", "return {{1, {1, {1, {0, 0}}}, {}}}"},
		"expression": {"return {{1, {1, 0}, 2}}", "return {{1, {1, {1, {0, 0}}}, 2}}"},
	}
	// The fork layer's child stores its value in f1, the property defined last.
	wantForked := [2]string{"{1, 0}", "{1, {0, 0}}"}
	registry := newCheckpointResumeRegistry(t)
	seeds := suspendedStackSeeds()
	for _, kind := range fuzzseed.SuspendedStackKinds {
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
