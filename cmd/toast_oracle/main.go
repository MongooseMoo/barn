// toast_oracle evaluates one MOO expression in ToastStunt's emergency wizard
// mode against a scratch copy of a database and prints the result.
//
//	toast_oracle --bin /path/to/moo --db Test.db '1 + 2'
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MongooseMoo/barn/internal/cli"
)

func main() {
	os.Exit(cli.Execute(newRootCommand(), os.Args[1:], os.Stdout, os.Stderr))
}

func newRootCommand() *cobra.Command {
	var binPath, dbPath string
	root := cli.NewRoot("toast_oracle", "Evaluate a MOO expression on ToastStunt")
	root.Use = "toast_oracle <moo-expression>"
	root.Long = `Evaluate one MOO expression in ToastStunt's emergency wizard mode (-e) against a
scratch copy of the database, and print the result. The database itself is
never touched.`
	root.Example = `  toast_oracle --bin /root/src/toaststunt/build/moo --db Test.db '1 + 2'
  TOAST_ORACLE_BIN=... TOAST_ORACLE_DB=... toast_oracle 'server_version()'`
	root.Args = cobra.ExactArgs(1)
	root.Flags().StringVar(&binPath, "bin", envOr("TOAST_ORACLE_BIN", ""), "ToastStunt moo `binary` (required; or set $TOAST_ORACLE_BIN)")
	root.Flags().StringVar(&dbPath, "db", envOr("TOAST_ORACLE_DB", ""), "Database `file` to load (required; or set $TOAST_ORACLE_DB)")
	root.RunE = cli.RunE(func(cmd *cobra.Command, args []string) error {
		if binPath == "" || dbPath == "" {
			return cli.UsageErrorf("--bin and --db are required (or set $TOAST_ORACLE_BIN and $TOAST_ORACLE_DB)")
		}
		result, err := evaluateExpression(binPath, dbPath, args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), result)
		return nil
	})
	return root
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// evaluateExpression runs the given MOO expression against a *copy* of dbPath
// in ToastStunt's emergency wizard mode (-e) and returns the printed result.
// Emergency mode loads the db, runs commands as #2, and on `quit` re-dumps the
// (possibly mutated) database to the output path — so this always operates on
// a scratch copy, never the caller's original file.
func evaluateExpression(binPath, dbPath, expr string) (string, error) {
	scratchDB, err := copyToScratch(dbPath)
	if err != nil {
		return "", fmt.Errorf("failed to copy db to scratch: %w", err)
	}
	defer func() { _ = os.Remove(scratchDB) }()

	outDB := scratchDB + ".out"
	defer func() { _ = os.Remove(outDB) }()

	cmd := exec.Command(binPath, "-e", scratchDB, outDB)

	// Prepare stdin: ";EXPR" evaluates a single MOO expression and prints its
	// result as "=> RESULT"; "quit" exits emergency mode, saving the db.
	input := fmt.Sprintf(";%s\nquit\n", expr)
	cmd.Stdin = strings.NewReader(input)

	// Capture combined stdout and stderr (Toast writes interactive output to
	// stderr in this build, banner/log lines to stdout).
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run moo: %w\noutput: %s", err, output.String())
	}

	result, err := parseToastOutput(output.String())
	if err != nil {
		return "", fmt.Errorf("failed to parse output: %w\noutput:\n%s", err, output.String())
	}

	return result, nil
}

// copyToScratch copies dbPath into a temp file and returns its path, so the
// emergency-mode session (which may mutate and re-save the db) never touches
// the caller's original file.
func copyToScratch(dbPath string) (string, error) {
	src, err := os.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.CreateTemp("", "toast_oracle_*.db")
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := dst.ReadFrom(src); err != nil {
		_ = os.Remove(dst.Name())
		return "", err
	}
	return dst.Name(), nil
}

func parseToastOutput(output string) (string, error) {
	// Emergency mode output (this ToastStunt build) looks like:
	// ** Now running emergency commands as #2 ...
	//
	// => RESULT
	// Bye.  (saving database)
	//
	// A parse/compile error instead looks like:
	// ** N errors during parsing:
	//   Line N:  syntax error

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var parseErrors []string
	inParseErrors := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "=> ") {
			return strings.TrimPrefix(line, "=> "), nil
		}

		if strings.Contains(line, "errors during parsing") {
			inParseErrors = true
			continue
		}
		if inParseErrors {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || trimmed == "Bye.  (saving database)" {
				inParseErrors = false
				continue
			}
			parseErrors = append(parseErrors, trimmed)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scanner error: %w", err)
	}

	if len(parseErrors) > 0 {
		return "", fmt.Errorf("compile error: %s", strings.Join(parseErrors, "; "))
	}

	return "", fmt.Errorf("could not find result in output")
}
