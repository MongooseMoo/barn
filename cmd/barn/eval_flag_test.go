package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/app"
	"github.com/MongooseMoo/barn/types"
)

// evalFixtureDB writes a database whose first wizard is #3, the shape of the
// conformance Test.db the dbtool is normally pointed at.
func evalFixtureDB(t *testing.T) string {
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
		if err := store.Add(b.Build()); err != nil {
			t.Fatalf("add #%d: %v", id, err)
		}
	}
	path := filepath.Join(t.TempDir(), "eval.db")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbformat.NewWriter(f, store.Snapshot()).WriteDatabase(); err != nil {
		f.Close()
		t.Fatalf("write fixture: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func runBarnEval(t *testing.T, cfg app.Config) (string, string, error) {
	t.Helper()
	cfg.LogDir = ""
	cfg.DebugAddr = "off"
	cfg.OperatorAddr = "off"
	var out, errOut bytes.Buffer
	err := app.Run(context.Background(), cfg, &out, &errOut)
	return out.String(), errOut.String(), err
}

// -eval and -eval-file share one implementation: statement lists run every
// statement, a bare expression is evaluated, and both run as the first wizard.
func TestEvalFlagAndEvalFileAgree(t *testing.T) {
	db := evalFixtureDB(t)
	inputs := []struct{ input, want string }{
		{"x = 1; return x + 1;", "=> 2"},
		{"1 + 2", "=> 3"},
		{"player", "=> #3"},
		{"return 1 / 0;", "Error: E_DIV"},
	}

	var fileLines []string
	for _, in := range inputs {
		cfg := app.DefaultConfig()
		cfg.DatabasePath = db
		cfg.Eval = in.input
		out, errOut, err := runBarnEval(t, cfg)
		if err != nil {
			t.Fatalf("-eval %q: %v\nstderr: %s", in.input, err, errOut)
		}
		if out != in.want+"\n" {
			t.Fatalf("-eval %q printed %q, want %q", in.input, out, in.want+"\n")
		}
		fileLines = append(fileLines, in.input)
	}

	filePath := filepath.Join(t.TempDir(), "inputs.moo")
	if err := os.WriteFile(filePath, []byte(strings.Join(fileLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := app.DefaultConfig()
	cfg.DatabasePath = db
	cfg.EvalFile = filePath
	out, errOut, err := runBarnEval(t, cfg)
	if err != nil {
		t.Fatalf("-eval-file: %v\nstderr: %s", err, errOut)
	}
	var want []string
	for _, in := range inputs {
		want = append(want, in.want)
	}
	want = append(want, "-- skipped") // the trailing newline's empty line
	if got := strings.TrimRight(out, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("-eval-file printed:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
}

func TestEvalFlagRejectsUncompilableInputWithAnError(t *testing.T) {
	cfg := app.DefaultConfig()
	cfg.DatabasePath = evalFixtureDB(t)
	cfg.Eval = "1 +"
	out, errOut, err := runBarnEval(t, cfg)
	if err == nil {
		t.Fatal("-eval accepted uncompilable input")
	}
	if out != "" || !strings.HasPrefix(errOut, "Compile error: ") {
		t.Fatalf("stdout = %q, stderr = %q; want an empty stdout and a compile error", out, errOut)
	}
}
