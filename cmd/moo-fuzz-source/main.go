// moo-fuzz-source prints the MOO program that the structured fuzz targets
// generate from the choice bytes read on stdin.
//
//	moo-fuzz-source < choices.bin
//	head -c 200 /dev/urandom | moo-fuzz-source
//
// It prints one example program. It is not a grammar of the MOO language.
// It exits 1 when the generated program has no MOO spelling.
//
// With -corpus, stdin instead lists Go fuzz corpus files of a structured
// target, one path per line, and each file gives one JSON line:
// {"path", "choices" (hex), "source"} or {"path", "error"}.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/internal/fuzzseed"
)

func main() {
	corpus := flag.Bool("corpus", false, "read corpus file paths on stdin and print one JSON line per file")
	flag.Parse()
	if *corpus {
		if err := renderCorpus(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	choices, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	source, err := fuzzseed.Source(choices)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(source)
}

type corpusRecord struct {
	Path    string `json:"path"`
	Choices string `json:"choices,omitempty"`
	Source  string `json:"source,omitempty"`
	Error   string `json:"error,omitempty"`
}

func renderCorpus(paths io.Reader, out io.Writer) error {
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
			record.Source, err = fuzzseed.Source(choices)
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
