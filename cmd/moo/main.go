// moo runs MOO code with no network server.
//
//	moo -e '1 + 2'            # evaluate one input and print its result line
//	moo program.moo           # run a file as one program
//	moo                       # read one input per line from stdin
//
// Code runs as the database's first wizard, through the same eval path as
// barn -eval. Without -db the database is an in-memory Minimal.db; with -db it
// is loaded from the named file. Either way nothing is written to disk.
//
// It exits 1 when -e or a file does not compile, raises an uncaught error, or
// panics, and 2 on a usage error.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/dbtool"
)

const (
	exitOK = iota
	exitFailed
	exitUsage
)

const prompt = "moo> "

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole command: it returns the exit status instead of exiting, so
// tests drive it in-process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("moo", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: moo [-db path] [-e source | file]")
		flags.PrintDefaults()
	}
	databasePath := flags.String("db", "", "Database file to load (default: an in-memory Minimal.db); never written back")
	expression := flags.String("e", "", "Evaluate MOO code (statements, or one expression), then exit")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	expressionGiven := false
	flags.Visit(func(f *flag.Flag) { expressionGiven = expressionGiven || f.Name == "e" })
	if flags.NArg() > 1 || (expressionGiven && flags.NArg() == 1) {
		flags.Usage()
		return exitUsage
	}

	// Read the program before building anything, so a missing file costs no
	// database load.
	var program []string
	switch {
	case expressionGiven:
		program = []string{*expression}
	case flags.NArg() == 1:
		data, err := os.ReadFile(flags.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return exitFailed
		}
		program = dbtool.SourceLines(data)
	}

	store, err := openStore(*databasePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return exitFailed
	}
	session, err := dbtool.NewEvalSession(store, config.DefaultOptions())
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return exitFailed
	}
	defer session.Close()

	if program == nil {
		return repl(session, stdin, stdout, stderr)
	}
	if compiled, completed := session.Print(stdout, stderr, program); !compiled || !completed {
		return exitFailed
	}
	return exitOK
}

// openStore returns the database to run in: the file at path, read without
// touching it, or a fresh minimal database when no path is given.
func openStore(path string) (*dbstore.Store, error) {
	if path == "" {
		return dbtool.NewMinimalStore()
	}
	database, err := dbformat.LoadDatabaseReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("load database: %w", err)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		return nil, fmt.Errorf("construct store from database: %w", err)
	}
	return store, nil
}

// repl evaluates one input per line until EOF, every line in the same session.
// A line that fails prints its error and the loop carries on, so the result
// lines all go to stdout and reaching EOF is success.
func repl(session *dbtool.EvalSession, stdin io.Reader, stdout, stderr io.Writer) int {
	interactive := isTerminal(stdin)
	scanner := bufio.NewScanner(stdin)
	// A pasted verb body on one line can exceed the default 64KB line limit.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for {
		if interactive {
			fmt.Fprint(stdout, prompt)
		}
		if !scanner.Scan() {
			break
		}
		if input := strings.TrimSpace(scanner.Text()); input != "" {
			session.Print(stdout, stdout, []string{input})
		}
	}
	if interactive {
		fmt.Fprintln(stdout)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(stderr, "Error: read input: %v\n", err)
		return exitFailed
	}
	return exitOK
}

// isTerminal reports whether input is a character device: someone typing,
// rather than a pipe or a file.
func isTerminal(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
