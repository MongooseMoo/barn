package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/internal/buildinfo"
	"github.com/MongooseMoo/barn/internal/cli"
)

// barn runs the real command tree with args and returns its exit status and
// output. --log-dir= keeps runs from writing logs/ into the package directory.
func barn(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Execute(newRootCommand(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionSpellings(t *testing.T) {
	want := buildinfo.Current().Line("barn") + "\n"
	for _, args := range [][]string{{"--version"}, {"-V"}, {"version"}} {
		code, out, errOut := barn(t, args...)
		if code != cli.ExitOK || out != want {
			t.Fatalf("%v: code %d out %q err %q, want %q", args, code, out, errOut, want)
		}
	}
}

func TestEvalCommand(t *testing.T) {
	db := evalFixtureDB(t)
	code, out, errOut := barn(t, "eval", "--db", db, "--log-dir=", "-q", "1 + 2")
	if code != cli.ExitOK || out != "=> 3\n" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}

	file := filepath.Join(t.TempDir(), "inputs.moo")
	if err := os.WriteFile(file, []byte("player\nreturn 1 / 0;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = barn(t, "--db", db, "--log-dir=", "-q", "eval", "-f", file)
	if code != cli.ExitOK || !strings.HasPrefix(out, "=> #3\nError: E_DIV\n") {
		t.Fatalf("--file: code %d out %q err %q", code, out, errOut)
	}
}

func TestEvalCommandFailureExitsOne(t *testing.T) {
	code, out, errOut := barn(t, "eval", "--db", evalFixtureDB(t), "--log-dir=", "-q", "1 +")
	if code != cli.ExitFailed || out != "" || !strings.HasPrefix(errOut, "Compile error: ") {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

func TestCommandLineMistakesExitTwo(t *testing.T) {
	cases := [][]string{
		{"eval"},                     // neither code nor --file
		{"eval", "--file", "x", "1"}, // both
		{"verb-code"},                // missing argument
		{"-db", "x", "eval", "1"},    // old single-dash spelling
		{"--eval", "1"},              // old mode flag
		{"serve", "--nope"},          // unknown flag
		{"eval", "-v", "-q", "1"},    // conflicting verbosity
		{"frobnicate"},               // unknown command
		{"dump-obj-raw", "#1", "#2"}, // too many arguments
		{"memory-report", "--memory-profile", "x"},
		{"toast-check", "--db", "x"}, // no --builtins
	}
	for _, args := range cases {
		code, _, errOut := barn(t, args...)
		if code != cli.ExitUsage || !strings.Contains(errOut, "--help' for usage.") {
			t.Errorf("%v: code %d stderr %q, want usage error", args, code, errOut)
		}
	}
}

func TestBareBarnPrintsHelpAndStartsNothing(t *testing.T) {
	code, out, _ := barn(t)
	if code != cli.ExitOK || !strings.Contains(out, "serve") || !strings.Contains(out, "Inspect a database:") {
		t.Fatalf("code %d out %q", code, out)
	}
}

func TestServeRejectsPortWithListen(t *testing.T) {
	code, _, _ := barn(t, "serve", "--log-dir=", "-q", "--db", evalFixtureDB(t),
		"-p", "7777", "--listen", "tcp://127.0.0.1:0", "--debug-addr", "off", "--operator-addr", "off")
	if code != cli.ExitFailed {
		t.Fatalf("code %d, want %d", code, cli.ExitFailed)
	}
}

func TestListProfilesCommand(t *testing.T) {
	code, out, errOut := barn(t, "list-profiles", "--registry", "../../profiles/barn/profiles.json", "--log-dir=", "-q")
	if code != cli.ExitOK || !strings.Contains(out, "barn-linux-testdb-outbound-off") {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}
