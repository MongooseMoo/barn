// moo runs MOO code with no network server.
//
//	moo -e '1 + 2'            # evaluate one input and print its result line
//	moo program.moo           # run a file as one program
//	moo                       # read one input per line from stdin
//
// Code runs as the database's first wizard, through the same eval path as
// barn eval. Without --db the database is an in-memory Minimal.db; with --db it
// is loaded from the named file. Either way nothing is written to disk.
//
// It exits 1 when -e or a file does not compile, raises an uncaught error, or
// panics, and 2 on a usage error.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/cli"
	"github.com/MongooseMoo/barn/internal/dbtool"
	"github.com/MongooseMoo/barn/logging"
)

const (
	exitOK     = cli.ExitOK
	exitFailed = cli.ExitFailed
)

const prompt = "moo> "

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole command: it returns the exit status instead of exiting, so
// tests drive it in-process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var databasePath, expression string
	root := cli.NewRoot("moo", "Run MOO code with no network server")
	root.Use = "moo [file]"
	root.Long = `Run MOO code as the database's first wizard, through the same eval path as
barn eval, and print each result line ("=> VALUE", "Error: CODE" or
"Compile error: ..."). With -e, evaluate that input; with a file, run the file as
one program; with neither, read one input per line from stdin. Without --db the
database is an in-memory Minimal.db. Nothing is written to disk.`
	root.Example = `  moo -e '1 + 2'
  moo --db Test.db -e 'return #0.name;'
  moo program.moo
  echo '1 + 2' | moo`
	root.Args = cobra.MaximumNArgs(1)
	flags := root.Flags()
	flags.StringVar(&databasePath, "db", "", "Database `file` to load (default: an in-memory Minimal.db); never written back")
	flags.StringVarP(&expression, "eval", "e", "", "Evaluate MOO `code` (statements, or one expression), then exit")
	logs := cli.AddLogFlags(flags, "info")
	root.RunE = cli.RunE(func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("eval") && len(args) == 1 {
			return cli.UsageErrorf("give either -e or a file, not both")
		}
		level, err := logs.Level()
		if err != nil {
			return err
		}
		closeLogs, err := logging.Setup(logging.Options{LevelStr: level, Stderr: stderr})
		if err != nil {
			return err
		}
		defer closeLogs()
		var program []string
		switch {
		case cmd.Flags().Changed("eval"):
			program = []string{expression}
		case len(args) == 1:
			data, err := os.ReadFile(args[0])
			if err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return cli.Exit(exitFailed)
			}
			program = dbtool.SourceLines(data)
		}
		return cli.Exit(runProgram(databasePath, program, stdin, stdout, stderr))
	})
	return cli.Execute(root, args, stdout, stderr)
}

// runProgram runs program (nil reads stdin as a REPL) in databasePath's
// database and returns the exit status.
// The caller reads the program first, so a missing file costs no database load.
func runProgram(databasePath string, program []string, stdin io.Reader, stdout, stderr io.Writer) (exitCode int) {
	store, err := openStore(databasePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return exitFailed
	}
	session, err := dbtool.NewEvalSession(store, config.DefaultOptions())
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return exitFailed
	}
	defer func() {
		if err := session.Close(); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			exitCode = exitFailed
		}
	}()

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
	return dbtool.LoadStore(path, config.DefaultOptions())
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
