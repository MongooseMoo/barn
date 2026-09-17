package engine

import (
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A task that calls a verb (resolved through the cross-transaction dispatch
// memo once another task has warmed it) and then runs a coarse builtin such as
// add_verb must complete. The coarse builtin moves the verb-shape clock itself;
// that must not read as a conflict, because a task that has mutated the live
// store cannot retry and the conflict would surface to MOO as E_INVARG. This is
// the shape of every conformance step that programs a verb on #0 or on an
// object it just created.
func TestVerbCallThenCoarseBuiltinCommits(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	probe := dbstore.NewVerb("probe", []string{"probe"}, 0, dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{"return 1;"})
	if _, ec := store.AddVerb(0, probe); ec != types.E_NONE {
		t.Fatalf("AddVerb probe: %v", ec)
	}

	s := NewRuntime(store)
	t.Cleanup(s.Stop)

	run := func(name string, code string) types.Result {
		t.Helper()
		program := compileTestProgram(t, s.registry, code)
		taskID := s.CreateBackgroundTask(0, program, 0)
		running := s.GetTask(taskID)
		running.Context.IsWizard = true
		for pass := 0; pass < 8 && running.GetState() != task.TaskCompleted && running.GetState() != task.TaskKilled; pass++ {
			if processed := s.ProcessReadyTasks(); processed == 0 {
				t.Fatalf("%s: task made no progress in state %v", name, running.GetState())
			}
		}
		return running.Result
	}

	// Warm the dispatch memo for #0:probe.
	if got := run("warm", "return #0:probe();"); got.Flow != types.FlowReturn || got.Val.String() != "1" {
		t.Fatalf("warm-up task = %+v", got)
	}

	cases := []struct {
		name string
		code []string
	}{
		{"add_verb on #0 (no staged topology; the coarse op itself bumps the clock)", []string{
			"#0:probe();",
			`add_verb(#0, {#0, "rxd", "added_on_root"}, {"this", "none", "this"});`,
			`set_verb_code(#0, "added_on_root", {"return 2;"});`,
			"return #0:added_on_root();",
		}},
		{"create then add_verb (staged topology flushed by the coarse op)", []string{
			"#0:probe();",
			"h = create(#-1);",
			`add_verb(h, {#0, "rxd", "on_created"}, {"this", "none", "this"});`,
			`set_verb_code(h, "on_created", {"return 3;"});`,
			"return h:on_created();",
		}},
	}
	for i, tc := range cases {
		got := run(tc.name, strings.Join(tc.code, "\n"))
		want := []string{"2", "3"}[i]
		if got.Flow != types.FlowReturn || got.Val.String() != want {
			t.Fatalf("%s: result = %+v, want return %s", tc.name, got, want)
		}
	}
}
