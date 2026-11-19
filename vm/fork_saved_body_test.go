package vm

import (
	"slices"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A queued forked task is checkpointed as the unparse of its fork body, as
// Toast's write_forked_task writes it (tasks.cc, dbio_write_forked_program):
// the saved code must compile and run to the uninterrupted run's result
// wherever the fork statement's text sits in its source line.
func TestForkBodyOnOneLineSurvivesCheckpoint(t *testing.T) {
	cases := map[string]string{
		"nested": `#0.p = {};
fork (0) fork (0) #0.p = {@#0.p, 2}; endfork suspend(); #0.p = {@#0.p, 1}; endfork
suspend();
return #0.p;`,
		"nested two deep": `#0.p = {};
fork (0) fork (0) fork (0) #0.p = {@#0.p, 3}; endfork suspend(); #0.p = {@#0.p, 2}; endfork suspend(); #0.p = {@#0.p, 1}; endfork
suspend();
return #0.p;`,
		"endfork in a string": `fork (0) #0.p = "endfork"; suspend(); #0.p = {#0.p, "an endfork; "}; endfork
suspend();
return #0.p;`,
		"fork in a string before the fork": `#2.p = "fork (0) "; fork (0) #0.p = {#2.p, 1}; suspend(); #0.p = {@#0.p, 2}; endfork
suspend();
return #0.p;`,
		"second fork on the line": `#0.p = {}; fork (0) #0.p = {@#0.p, 1}; endfork fork (0) suspend(); #0.p = {@#0.p, 2}; endfork
suspend();
return #0.p;`,
		// The frame that forks was itself restored from the checkpoint.
		"forked by a restored frame": `#0.p = {};
suspend();
fork (0) fork (0) #0.p = {@#0.p, 2}; endfork suspend(); #0.p = {@#0.p, 1}; endfork
suspend();
return #0.p;`,
		"nested in a body of several lines": `#0.p = {};
fork (0)
  fork (0) #0.p = {@#0.p, 2}; endfork suspend();
  #0.p = {@#0.p, 1};
endfork
suspend();
return #0.p;`,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if difference := worldCheckpointDifference(t, source, checkpointSuspendedVM); difference != "" {
				t.Fatalf("%s\nsource:\n%s", difference, source)
			}
		})
	}
}

// The saved code is Toast's unparse of the fork body: fully parenthesized, not
// indented, one statement per line (db_io.cc dbio_write_forked_program).
func TestForkSavedCodeIsUnparsedBody(t *testing.T) {
	registry := newCheckpointResumeRegistry(t)
	source := `x = 1; fork (0) fork t (x) y = x + 2 * 3; endfork if (x) return "endfork"; endif endfork return 0;`
	program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	session := newTestSessionWithTaskManager(registry)
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	machine := newSourceFuzzVM(newCheckpointResumeStore(t), session, 1, defaultTickLimit)
	result := machine.RunWithVerbContext(program, types.ObjNothing, machine.Context.Player, types.ObjNothing, "", types.ObjNothing, nil)
	if result.Flow != types.FlowFork {
		t.Fatalf("flow %v, want a fork", result.Flow)
	}
	want := []string{
		"fork t (x)",
		"y = x + (2 * 3);",
		"endfork",
		"if (x)",
		`return "endfork";`,
		"endif",
	}
	if got := result.ForkInfo.SourceLines; !slices.Equal(got, want) {
		t.Fatalf("saved code %q, want %q", got, want)
	}
}
