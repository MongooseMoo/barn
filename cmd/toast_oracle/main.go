package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	binPath := flag.String("bin", envOr("TOAST_ORACLE_BIN", ""), "path to the ToastStunt moo binary (required; or set $TOAST_ORACLE_BIN)")
	dbPath := flag.String("db", envOr("TOAST_ORACLE_DB", ""), "path to the database file to load (required; or set $TOAST_ORACLE_DB)")
	flag.Parse()

	args := flag.Args()
	if *binPath == "" || *dbPath == "" || len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s -bin PATH -db PATH '<moo-expression>'\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  -bin: path to ToastStunt moo binary (or set $TOAST_ORACLE_BIN)\n")
		fmt.Fprintf(os.Stderr, "  -db:  path to database file (or set $TOAST_ORACLE_DB)\n")
		os.Exit(1)
	}

	expr := args[0]
	result, err := evaluateExpression(*binPath, *dbPath, expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
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
	defer os.Remove(scratchDB)

	outDB := scratchDB + ".out"
	defer os.Remove(outDB)

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
		os.Remove(dst.Name())
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
