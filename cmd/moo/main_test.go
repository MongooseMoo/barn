package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/internal/dbtool"
)

// runMoo drives the command in-process: no stdin unless a test supplies one.
func runMoo(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func writeSource(t *testing.T, name, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExpressionFlagPrintsResultLine(t *testing.T) {
	cases := []struct{ input, want string }{
		{"1 + 2", "=> 3\n"},
		{"x = 1; return x + 1;", "=> 2\n"},
		{"x = 1;", "=> 0\n"},
		{`{player, player.name, player.location.name}`, "=> {#3, \"Wizard\", \"The First Room\"}\n"},
	}
	for _, c := range cases {
		code, out, errOut := runMoo(t, "", "-e", c.input)
		if code != 0 || out != c.want || errOut != "" {
			t.Errorf("-e %q: exit %d, stdout %q, stderr %q; want exit 0 and %q", c.input, code, out, errOut, c.want)
		}
	}
}

func TestExpressionFlagFailsOnUncaughtError(t *testing.T) {
	code, out, _ := runMoo(t, "", "-e", "return 1 / 0;")
	if code == 0 {
		t.Fatal("an uncaught error exited 0")
	}
	if out != "Error: E_DIV\n" {
		t.Fatalf("stdout = %q, want \"Error: E_DIV\\n\"", out)
	}
}

func TestExpressionFlagFailsOnCompileError(t *testing.T) {
	code, out, errOut := runMoo(t, "", "-e", "1 +")
	if code == 0 {
		t.Fatal("a compile error exited 0")
	}
	if out != "" || !strings.HasPrefix(errOut, "Compile error: ") {
		t.Fatalf("stdout = %q, stderr = %q; want an empty stdout and a compile error", out, errOut)
	}
}

// A file is one program: a statement that spans lines, and variables set on one
// line and read on another, only work when every line is compiled together.
func TestFileRunsAsOneProgram(t *testing.T) {
	path := writeSource(t, "sum.moo", "total = 0;\r\nfor i in [1..4]\n  total = total + i;\nendfor\nreturn total;\n")
	code, out, errOut := runMoo(t, "", path)
	if code != 0 || out != "=> 10\n" || errOut != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q; want exit 0 and \"=> 10\\n\"", code, out, errOut)
	}
}

// A file holding one expression gets the same fallback as -e, across lines.
func TestFileHoldingAnExpressionIsEvaluated(t *testing.T) {
	path := writeSource(t, "expr.moo", "{1,\n 2 + 3}\n")
	code, out, errOut := runMoo(t, "", path)
	if code != 0 || out != "=> {1, 5}\n" || errOut != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q; want exit 0 and \"=> {1, 5}\\n\"", code, out, errOut)
	}
}

func TestFileFailsOnUncaughtError(t *testing.T) {
	path := writeSource(t, "div.moo", "x = 0;\nreturn 1 / x;\n")
	code, out, _ := runMoo(t, "", path)
	if code == 0 {
		t.Fatal("an uncaught error exited 0")
	}
	if out != "Error: E_DIV\n" {
		t.Fatalf("stdout = %q, want \"Error: E_DIV\\n\"", out)
	}
}

func TestFileFailsOnCompileError(t *testing.T) {
	path := writeSource(t, "broken.moo", "x = 1;\nif (x)\nreturn x;\n")
	code, out, errOut := runMoo(t, "", path)
	if code == 0 {
		t.Fatal("a compile error exited 0")
	}
	if out != "" || !strings.HasPrefix(errOut, "Compile error: ") {
		t.Fatalf("stdout = %q, stderr = %q; want an empty stdout and a compile error", out, errOut)
	}
}

func TestMissingFileFails(t *testing.T) {
	code, out, errOut := runMoo(t, "", filepath.Join(t.TempDir(), "absent.moo"))
	if code == 0 || out != "" || errOut == "" {
		t.Fatalf("exit %d, stdout %q, stderr %q; want a failure reported on stderr", code, out, errOut)
	}
}

// With no source argument each stdin line is one input. One session serves them
// all, so a property added on one line is there on the next; a failing line
// does not end the session or the exit status.
func TestStdinIsAReplOverOneSession(t *testing.T) {
	stdin := strings.Join([]string{
		`add_property(#0, "counter", 41, {player, "r"}); return 1;`,
		"",
		"$counter + 1",
		"1 +",
		"return 1 / 0;",
		"x = 2; return x * x;",
	}, "\n") + "\n"
	code, out, errOut := runMoo(t, stdin)
	if code != 0 {
		t.Fatalf("exit %d at EOF, want 0\nstderr: %s", code, errOut)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 5 || lines[0] != "=> 1" || lines[1] != "=> 42" || !strings.HasPrefix(lines[2], "Compile error: ") || lines[3] != "Error: E_DIV" || lines[4] != "=> 4" {
		t.Fatalf("REPL output lines = %q", lines)
	}
}

// A pipe is not a terminal, so nothing but result lines reaches stdout.
func TestReplPrintsNoPromptWithoutATerminal(t *testing.T) {
	_, out, _ := runMoo(t, "1\n2\n")
	if out != "=> 1\n=> 2\n" {
		t.Fatalf("stdout = %q, want only the two result lines", out)
	}
}

// minimalDBFile writes the minimal database to disk, with a marker property so
// a test can tell it from the built-in one.
func minimalDBFile(t *testing.T) string {
	t.Helper()
	store, err := dbtool.NewMinimalStore()
	if err != nil {
		t.Fatal(err)
	}
	session, err := dbtool.NewEvalSession(store, config.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	compiled, completed := session.Print(&out, &errOut, []string{`add_property(#0, "from_file", "yes", {player, "r"});`})
	session.Close()
	if !compiled || !completed {
		t.Fatalf("marking the fixture: %s%s", out.String(), errOut.String())
	}
	path := filepath.Join(t.TempDir(), "fixture.db")
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

func TestDBFlagLoadsDatabaseAndNeverWritesIt(t *testing.T) {
	path := minimalDBFile(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runMoo(t, "", "-db", path, "-e", `$from_file = "changed"; return $from_file;`)
	if code != 0 || out != "=> \"changed\"\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the database file changed on disk")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("the database directory holds %d entries after the run, want only the database", len(entries))
	}
	if code, out, _ := runMoo(t, "", "-db", path, "-e", "$from_file"); code != 0 || out != "=> \"yes\"\n" {
		t.Fatalf("second run: exit %d, stdout %q; want the value on disk", code, out)
	}
}

func TestWithoutDBFlagTheDatabaseIsTheMinimalOne(t *testing.T) {
	code, out, errOut := runMoo(t, "", "-e", `{max_object(), properties(#0), verbs(#0)}`)
	if code != 0 || out != "=> {#3, {}, {}}\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestUnloadableDatabaseFails(t *testing.T) {
	code, out, errOut := runMoo(t, "", "-db", filepath.Join(t.TempDir(), "absent.db"), "-e", "1")
	if code == 0 || out != "" || errOut == "" {
		t.Fatalf("exit %d, stdout %q, stderr %q; want a failure reported on stderr", code, out, errOut)
	}
}

func TestUsageErrors(t *testing.T) {
	path := writeSource(t, "one.moo", "return 1;\n")
	for _, args := range [][]string{
		{"-e", "1", path},
		{path, path},
		{"-nonsense"},
	} {
		code, out, errOut := runMoo(t, "", args...)
		if code != 2 || out != "" || errOut == "" {
			t.Errorf("moo %q: exit %d, stdout %q, stderr %q; want exit 2 and a usage message", args, code, out, errOut)
		}
	}
}
