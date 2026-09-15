package dbtool

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// evalStore is a Test.db-shaped fixture: #0 is the system object with
// $server_options -> #1, #2 is a programmer and #3 is the first wizard, so the
// tool must evaluate as #3 just as Toast's emergency mode would.
func evalStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	flags := map[types.ObjID]dbstore.ObjectFlags{
		0: dbstore.FlagRead,
		1: dbstore.FlagRead,
		2: dbstore.FlagProgrammer | dbstore.FlagUser | dbstore.FlagRead,
		3: dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser | dbstore.FlagRead,
	}
	for id := types.ObjID(0); id <= 3; id++ {
		b := dbstore.NewObjectBuilder(id)
		b.SetOwner(3)
		b.SetLocation(types.ObjNothing)
		b.SetFlags(flags[id])
		if id == 0 {
			b.SetProperty("server_options", dbstore.NewProperty(types.NewObj(1), 3, dbstore.PropRead, false, true))
		}
		if err := store.Add(b.Build()); err != nil {
			t.Fatalf("add #%d: %v", id, err)
		}
	}
	return store
}

func runEval(t *testing.T, store *dbstore.Store, input string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := EvalExpression(&out, &errOut, store, input, config.DefaultOptions())
	return out.String(), errOut.String(), err
}

func TestEvalExpressionRunsStatementList(t *testing.T) {
	out, errOut, err := runEval(t, evalStore(t), "x = 1; return x + 1;")
	if err != nil {
		t.Fatalf("EvalExpression: %v\nstderr: %s", err, errOut)
	}
	if out != "=> 2\n" {
		t.Fatalf("statement list printed %q, want \"=> 2\\n\"", out)
	}
}

func TestEvalExpressionEvaluatesBareExpression(t *testing.T) {
	out, errOut, err := runEval(t, evalStore(t), "1 + 2")
	if err != nil {
		t.Fatalf("EvalExpression: %v\nstderr: %s", err, errOut)
	}
	if out != "=> 3\n" {
		t.Fatalf("expression printed %q, want \"=> 3\\n\"", out)
	}
}

func TestEvalExpressionRunsAsFirstWizardWithEvalFrame(t *testing.T) {
	out, errOut, err := runEval(t, evalStore(t), "{player, this, verb, caller_perms(), length(callers())}")
	if err != nil {
		t.Fatalf("EvalExpression: %v\nstderr: %s", err, errOut)
	}
	if want := "=> {#3, #-1, \"\", #3, 2}\n"; out != want {
		t.Fatalf("eval frame printed %q, want %q", out, want)
	}
}

func TestEvalExpressionReportsUncaughtError(t *testing.T) {
	out, _, err := runEval(t, evalStore(t), "return 1 / 0;")
	if err != nil {
		t.Fatalf("EvalExpression: %v", err)
	}
	if out != "Error: E_DIV\n" {
		t.Fatalf("uncaught error printed %q, want \"Error: E_DIV\\n\"", out)
	}
}

func TestEvalExpressionRejectsUncompilableInput(t *testing.T) {
	out, errOut, err := runEval(t, evalStore(t), "1 +")
	if err == nil {
		t.Fatal("EvalExpression accepted uncompilable input")
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
	if !strings.HasPrefix(errOut, "Compile error: ") {
		t.Fatalf("stderr = %q, want a compile error", errOut)
	}
}

func TestEvalExpressionNeedsAWizard(t *testing.T) {
	_, errOut, err := runEval(t, dbstore.NewStore(), "1")
	if err == nil {
		t.Fatal("EvalExpression ran against a database with no wizard")
	}
	if !strings.Contains(errOut, "no wizard") {
		t.Fatalf("stderr = %q, want the missing-wizard error", errOut)
	}
}

// Nested verbs see the eval activation the way a live ";" does: the eval'd
// user-code frame plus the two synthetic eval wrapper frames.
func TestEvalFileNestedVerbSeesEvalCallers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callers.moo")
	if err := os.WriteFile(path, []byte(
		`o = create(#-1); add_verb(o, {player, "rxd", "c"}, {"this", "none", "this"}); set_verb_code(o, "c", {"return callers();"}); r = o:c(); recycle(o); return r;`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := EvalFile(&out, &errOut, evalStore(t), path, config.DefaultOptions()); err != nil {
		t.Fatalf("EvalFile: %v\nstderr: %s", err, errOut.String())
	}
	want := "=> {{#-1, \"\", #3, #-1, #3}, {#-1, \"eval\", #-1, #-1, #3}, {#-1, \"eval\", #3, #-1, #3}}\n-- skipped\n"
	if got := out.String(); got != want {
		t.Fatalf("callers() from a nested verb:\n%s\nwant:\n%s", got, want)
	}
}

// A protected builtin called from a frame whose `this` is not #0 is redirected
// to #0:bf_<name>, and load_server_options() on an earlier line governs later
// lines of the same file.
func TestEvalFileRedirectsProtectedBuiltins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.moo")
	lines := []string{
		`add_property($server_options, "protect_valid", 1, {player, "r"}); add_verb(#0, {player, "rxd", "bf_valid"}, {"this", "none", "this"}); set_verb_code(#0, "bf_valid", {"return valid(args[1]) + 10;"}); load_server_options(); return 1;`,
		"",
		"## comment",
		`{valid(#0), valid(#-1)}`,
		`o = create(#-1); add_verb(o, {player, "rxd", "probe"}, {"this", "none", "this"}); set_verb_code(o, "probe", {"return valid(args[1]);"}); r = {o:probe(o), o:probe(#-1)}; recycle(o); return r;`,
		`delete_verb(#0, "bf_valid"); load_server_options(); return valid(#0);`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := EvalFile(&out, &errOut, evalStore(t), path, config.DefaultOptions()); err != nil {
		t.Fatalf("EvalFile: %v\nstderr: %s", err, errOut.String())
	}
	want := strings.Join([]string{
		"=> 1",
		"-- skipped",
		"-- skipped",
		"=> {11, 10}",
		"=> {11, 10}",
		"=> 1", // wizard without a wrapper falls through to the real builtin
		"-- skipped",
	}, "\n") + "\n"
	if got := out.String(); got != want {
		t.Fatalf("EvalFile output:\n%s\nwant:\n%s", got, want)
	}
}

func TestEvalFileReportsCompileErrorsPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.moo")
	if err := os.WriteFile(path, []byte("1 +\n2 + 2\nx = 1; return x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := EvalFile(&out, &errOut, evalStore(t), path, config.DefaultOptions()); err != nil {
		t.Fatalf("EvalFile: %v\nstderr: %s", err, errOut.String())
	}
	got := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(got) != 4 || !strings.HasPrefix(got[0], "Compile error: ") || got[1] != "=> 4" || got[2] != "=> 1" || got[3] != "-- skipped" {
		t.Fatalf("EvalFile output lines = %q", got)
	}
}
