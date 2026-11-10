package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/types"
)

func FuzzWorldCheckpointEffects(f *testing.F) {
	for action := byte(0); action < 8; action++ {
		f.Add([]byte{0, 127, action, 0})
	}
	f.Add([]byte{7, 1, 0, 2, 1, 3, 2, 4, 3, 5, 4, 0, 6, 5, 7, 6, 8, 7})
	f.Fuzz(func(t *testing.T, choices []byte) {
		source := fuzzseed.WorldHistory(choices)
		if difference := worldCheckpointDifference(t, source, checkpointSuspendedVM); difference != "" {
			t.Fatalf("world effects changed: %s\nchoices: %x\nsource:\n%s", difference, choices, source)
		}
	})
}

// Every case must execute valid setup, finish each task and reach a checkpoint.
// Agreement between two rejected setups would not exercise this property.
func worldCheckpointDifference(t *testing.T, source string, checkpoint checkpointFunc) string {
	t.Helper()
	difference, skip := checkpointResumeDifference(t, source, checkpoint, 30000, true)
	if skip != "" {
		t.Fatalf("world history did not evaluate its property: %s\n%s", skip, source)
	}
	return difference
}

func TestWorldHistoryActionsReachCheckpoint(t *testing.T) {
	for action := byte(0); action < 8; action++ {
		source := fuzzseed.WorldHistory([]byte{0, 127, action, 0})
		if difference := worldCheckpointDifference(t, source, checkpointSuspendedVM); difference != "" {
			t.Fatalf("action %d: %s", action, difference)
		}
	}
	for length := byte(1); length <= 8; length++ {
		t.Run(fmt.Sprintf("mixed-prefix-%d", length), func(t *testing.T) {
			choices := []byte{length - 1, 1, 0, 2, 1, 3, 2, 4, 3, 5, 4, 0, 6, 5, 7, 6, 8, 7}
			if difference := worldCheckpointDifference(t, fuzzseed.WorldHistory(choices), checkpointSuspendedVM); difference != "" {
				t.Fatal(difference)
			}
		})
	}
}

func TestWorldHistoryAgreesAcrossTickBudgets(t *testing.T) {
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	source := fuzzseed.WorldHistory([]byte{3, 127, 0, 128, 1, 0, 2, 9, 7})
	program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	baseline, state, _ := runCheckpointResumeCase(t, registry, program, nil, 30000, true)
	for _, ticks := range []int64{30000, 60000} {
		results, observed, checkpoints := runCheckpointResumeCase(t, registry, program, checkpointSuspendedVM, ticks, true)
		if checkpoints == 0 || len(results) != len(baseline) || !sameCheckpointedValue(state, observed) {
			t.Fatalf("budget %d changed state/task count or reached no checkpoints", ticks)
		}
		for i, result := range results {
			if result.Flow != types.FlowReturn || !sameTaskResult(result, baseline[i]) {
				t.Fatalf("budget %d task %d differs: %s / %s", ticks, i, describeTaskResult(result), describeTaskResult(baseline[i]))
			}
		}
	}
}

func TestQueuedCheckpointPreservesCapturedRootsAndRepeatedReload(t *testing.T) {
	// Only the queued child's captured environment retains a after the parent
	// overwrites it. Two suspends require the restored queued source to survive
	// another checkpoint before its first execution.
	setups := []string{
		`a = create(#2, #0, 1);
a.p = 73;
`,
		`class = create(#2);
add_property(class, ":p", 0, {#0, "rw"});
add_verb(class, {#0, "rxd", "make"}, {"this", "none", "this"});
set_verb_code(class, "make", {"return new_waif();"});
a = class:make();
a.p = 73;
`,
	}
	for i, setup := range setups {
		source := setup + `
fork (0)
  #0.p = a.p;
endfork
a = 0;
suspend();
suspend();
return 1;`
		if difference := worldCheckpointDifference(t, source, checkpointSuspendedVM); difference != "" {
			t.Fatalf("root kind %d: %s", i, difference)
		}
	}
}

func TestWorldCheckpointDetectsLostQueuedWork(t *testing.T) {
	source := "fork (0) #0.p = 73; endfork suspend(); return 1;"
	dropQueued := func(t *testing.T, machine *VM, store *dbstore.Store, forks []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork) {
		restored, reloaded, _ := checkpointSuspendedVM(t, machine, store, forks)
		return restored, reloaded, nil
	}
	if difference := worldCheckpointDifference(t, source, dropQueued); difference == "" {
		t.Fatal("observer accepted dropped queued work")
	}
}

func TestWorldCheckpointDetectsChangedVerbCode(t *testing.T) {
	corruptCode := func(t *testing.T, machine *VM, store *dbstore.Store, forks []sourceFuzzFork) (*VM, *dbstore.Store, []sourceFuzzFork) {
		restored, reloaded, queued := checkpointSuspendedVM(t, machine, store, forks)
		program, diagnostics := machine.Builtins.Registry().Compiler().CompileMOO([]string{"return set_verb_code(ancestor, \"probe\", {\"return 99;\"});"})
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		// Use the live setup object's identity captured in the suspended frame.
		probe := newSourceFuzzVM(reloaded, machine.Builtins, 99, 30000)
		probe.Context.IsWizard = true
		probe.Context.Programmer = 0
		frame := probe.PrepareVerbFrame(program, 0, 0, 0, "", 0, nil)
		original := restored.CurrentFrame()
		for i, name := range original.Program.VarNames {
			if name == "ancestor" {
				SetLocalByName(frame, program, name, original.Locals[i])
			}
		}
		if probe.CurrentFrame() != frame {
			t.Fatal("probe frame was not installed")
		}
		if result := probe.ExecuteLoop(); result.Flow != types.FlowReturn {
			t.Fatalf("control did not change verb code: %s", describeTaskResult(result))
		}
		return restored, reloaded, queued
	}
	if difference := worldCheckpointDifference(t, fuzzseed.WorldHistory([]byte{0, 127, 0}), corruptCode); difference == "" {
		t.Fatal("observer accepted changed verb code")
	}
}
