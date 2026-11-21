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
// default, fuzzseed.Source) or suspended-stack (fuzzseed.SuspendedStack).
//
// With --builtins, stdin instead holds MOO programs, one JSON string per line,
// and each gives one JSON line: {"builtins": [names]} or {"error"}. The names
// are the builtins the parsed program calls, lower-cased and sorted.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MongooseMoo/barn/internal/cli"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
)

func main() {
	os.Exit(cli.Execute(newRootCommand(), os.Args[1:], os.Stdout, os.Stderr))
}

func newRootCommand() *cobra.Command {
	var corpus, builtins bool
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
FuzzSuspendedStackCheckpoint's.

With --builtins, stdin holds MOO programs, one JSON string per line, and each
gives one JSON line: {"builtins": [names]} or {"error"}. The names are the
builtins the parsed program calls, lower-cased and sorted; call_function with a
literal name counts as a call to that name.`
	root.Example = `  moo-fuzz-source < choices.bin
  head -c 200 /dev/urandom | moo-fuzz-source
  ls testdata/fuzz/FuzzStructured/* | moo-fuzz-source --corpus
  moo-fuzz-source --generator suspended-stack < choices.bin
  echo '"return length(\"ab\");"' | moo-fuzz-source --builtins`
	root.Args = cobra.NoArgs
	root.Flags().BoolVar(&corpus, "corpus", false, "Read corpus file paths on stdin and print one JSON line per file")
	root.Flags().BoolVar(&builtins, "builtins", false, "Read JSON-quoted MOO programs on stdin and print the builtins each calls")
	root.Flags().StringVar(&generator, "generator", "structured", "Generator the choices belong to: structured or suspended-stack")
	root.RunE = cli.RunE(func(cmd *cobra.Command, _ []string) error {
		render, ok := generators[generator]
		if !ok {
			return cli.UsageErrorf("unknown generator %q: want structured or suspended-stack", generator)
		}
		if builtins {
			return listBuiltinCalls(os.Stdin, cmd.OutOrStdout())
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

type builtinsRecord struct {
	Builtins []string `json:"builtins"`
	Error    string   `json:"error,omitempty"`
}

func listBuiltinCalls(sources io.Reader, out io.Writer) error {
	encoder := json.NewEncoder(out)
	scanner := bufio.NewScanner(sources)
	scanner.Buffer(nil, 64<<20)
	for scanner.Scan() {
		var source string
		if err := json.Unmarshal(scanner.Bytes(), &source); err != nil {
			return fmt.Errorf("stdin line is not a JSON string: %w", err)
		}
		record := builtinsRecord{Builtins: []string{}}
		names, err := builtinCalls(source)
		if err != nil {
			record.Error = err.Error()
		} else {
			record.Builtins = names
		}
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// builtinCalls returns the sorted lower-cased names of the builtins source
// calls: every name(...) call, and the literal name of call_function("name",
// ...). It walks every field of the parsed ir.Program, so no node kind can
// hide a call.
func builtinCalls(source string) ([]string, error) {
	program, err := parser.NewParser(source).ParseProgram()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var visit func(reflect.Value)
	visit = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				visit(v.Elem())
			}
		case reflect.Pointer:
			if v.IsNil() {
				return
			}
			if call, ok := v.Interface().(*ir.BuiltinCallExpr); ok {
				seen[strings.ToLower(call.Name)] = true
				if strings.EqualFold(call.Name, "call_function") && len(call.Args) > 0 {
					if name, ok := call.Args[0].(*ir.LiteralExpr); ok && name.Kind == ir.LiteralString {
						seen[strings.ToLower(name.StringValue)] = true
					}
				}
			}
			visit(v.Elem())
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				visit(v.Index(i))
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				visit(v.Field(i))
			}
		}
	}
	visit(reflect.ValueOf(program))
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
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
