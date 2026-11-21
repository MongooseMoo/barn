// moo-fuzz-source prints the MOO program that the structured fuzz targets
// generate from the choice bytes read on stdin.
//
//	moo-fuzz-source < choices.bin
//	head -c 200 /dev/urandom | moo-fuzz-source
//
// It prints one example program. It is not a grammar of the MOO language.
// It exits 1 when the generated program has no MOO spelling.
//
// With --corpus, stdin instead lists Go fuzz corpus files of a structured
// target, one path per line, and each file gives one JSON line:
// {"path", "choices" (hex), "source"} or {"path", "error"}.
//
// --generator names the generator the choices belong to: structured (the
// default, fuzzseed.Source), suspended-stack (fuzzseed.SuspendedStack) or
// toast-world (fuzzseed.ToastWorld).
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MongooseMoo/barn/internal/cli"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
)

func main() {
	os.Exit(cli.Execute(newRootCommand(), os.Args[1:], os.Stdout, os.Stderr))
}

func newRootCommand() *cobra.Command {
	var corpus bool
	var generator string
	root := cli.NewRoot("moo-fuzz-source", "Print the MOO program a structured fuzz target generates")
	root.Long = `Print the MOO program that the structured fuzz targets generate from the choice
bytes read on stdin. It prints one example program; it is not a grammar of the
MOO language. Exits 1 when the generated program has no MOO spelling.

With --corpus, stdin lists Go fuzz corpus files of a structured target, one path
per line, and each file gives one JSON line: {"path", "choices" (hex),
"source"} or {"path", "error"}.

--generator names the generator the choices belong to: structured, the
FuzzStructured* targets' (the default), or suspended-stack,
FuzzSuspendedStackCheckpoint's, or toast-world, the world
scripts/toast_reads_barn_db.py has Barn write and Toast load (rendered from
FuzzWorldCheckpointEffects inputs).`
	root.Example = `  moo-fuzz-source < choices.bin
  head -c 200 /dev/urandom | moo-fuzz-source
  ls testdata/fuzz/FuzzStructured/* | moo-fuzz-source --corpus
  moo-fuzz-source --generator suspended-stack < choices.bin`
	root.Args = cobra.NoArgs
	root.Flags().BoolVar(&corpus, "corpus", false, "Read corpus file paths on stdin and print one JSON line per file")
	root.Flags().StringVar(&generator, "generator", "structured", "Generator the choices belong to: structured, suspended-stack or toast-world")
	root.RunE = cli.RunE(func(cmd *cobra.Command, _ []string) error {
		render, ok := generators[generator]
		if !ok {
			return cli.UsageErrorf("unknown generator %q: want structured, suspended-stack or toast-world", generator)
		}
		if corpus {
			return renderCorpus(os.Stdin, cmd.OutOrStdout(), render)
		}
		choices, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		source, err := render(choices)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), source)
		return nil
	})
	return root
}

// generators maps a --generator name to the renderer of its choice bytes.
var generators = map[string]func([]byte) (string, error){
	"structured":      fuzzseed.Source,
	"suspended-stack": func(choices []byte) (string, error) { return fuzzseed.SuspendedStack(choices), nil },
	"toast-world":     func(choices []byte) (string, error) { return fuzzseed.ToastWorld(choices), nil },
}

type corpusRecord struct {
	Path    string `json:"path"`
	Choices string `json:"choices,omitempty"`
	Source  string `json:"source,omitempty"`
	Error   string `json:"error,omitempty"`
}

func renderCorpus(paths io.Reader, out io.Writer, render func([]byte) (string, error)) error {
	encoder := json.NewEncoder(out)
	scanner := bufio.NewScanner(paths)
	for scanner.Scan() {
		path := strings.TrimSuffix(scanner.Text(), "\r")
		if path == "" {
			continue
		}
		record := corpusRecord{Path: path}
		choices, err := readChoices(path)
		if err == nil {
			record.Choices = hex.EncodeToString(choices)
			record.Source, err = render(choices)
		}
		if err != nil {
			record.Error = err.Error()
		}
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// readChoices returns the one []byte value of a Go fuzz corpus file. Go writes
// it with fmt's %q (internal/fuzz/encoding.go marshalCorpusFile) and reads it
// back with strconv.Unquote.
func readChoices(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var values []string
	lines := strings.Split(string(data), "\n")
	if strings.TrimSuffix(lines[0], "\r") != "go test fuzz v1" {
		return nil, errors.New("not a go test fuzz v1 file")
	}
	for _, line := range lines[1:] {
		if line = strings.TrimSpace(line); line != "" {
			values = append(values, line)
		}
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("%d values, want one []byte value", len(values))
	}
	literal, prefixed := strings.CutPrefix(values[0], "[]byte(")
	literal, suffixed := strings.CutSuffix(literal, ")")
	if !prefixed || !suffixed {
		return nil, errors.New("value is not []byte(...)")
	}
	choices, err := strconv.Unquote(literal)
	if err != nil {
		return nil, fmt.Errorf("[]byte literal: %w", err)
	}
	return []byte(choices), nil
}
