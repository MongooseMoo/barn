// barn_logs reports what went wrong on a server run.
//
// It reads the JSON log a run leaves behind and prints the records worth
// reading, expanding the two things that matter and that a one-line-per-record
// view would bury: the MOO traceback and, for a panic, the Go stack.
//
//	barn_logs                 # warnings and errors from the last run
//	barn_logs -level error    # only the failures
//	barn_logs -run run-20260711-234422.jsonl
//
// It exits 1 when the run logged an error, so it can be used as a check.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// levels order the records; a record is shown when it is at least as severe as
// the requested level.
var levels = map[string]int{"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3}

// A record is one line of the log. The fields named here are the ones this tool
// formats; everything else is preserved in Extra and printed as attributes.
type record struct {
	Time      string
	Level     string
	Msg       string
	Traceback string
	GoStack   string
	Frames    []frame
	Extra     map[string]any
}

type frame struct {
	VerbLoc int64  `json:"verbloc"`
	Verb    string `json:"verb"`
	This    int64  `json:"this"`
	Line    int    `json:"line"`
	Source  string `json:"source"`
}

func main() {
	dir := flag.String("dir", "logs", "Directory holding the log files")
	run := flag.String("run", "latest.jsonl", "Log file to read, or \"list\" to show available runs")
	level := flag.String("level", "warn", "Minimum level to report: debug, info, warn, error")
	limit := flag.Int("n", 0, "Show only the last N records (0 = all)")
	raw := flag.Bool("json", false, "Print the matching records as JSON instead of formatting them")
	flag.Parse()

	if *run == "list" {
		listRuns(*dir)
		return
	}

	minLevel, ok := levels[strings.ToUpper(*level)]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown level %q (want debug, info, warn, or error)\n", *level)
		os.Exit(2)
	}

	path := filepath.Join(*dir, *run)
	records, err := readRecords(path, minLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}

	if *limit > 0 && len(records) > *limit {
		records = records[len(records)-*limit:]
	}

	if len(records) == 0 {
		fmt.Printf("nothing at or above %s in %s\n", strings.ToUpper(*level), path)
		return
	}

	failed := false
	for _, rec := range records {
		if rec.Level == "ERROR" {
			failed = true
		}
		if *raw {
			line, _ := json.Marshal(rec.Extra)
			fmt.Printf("%s %s %s %s\n", rec.Time, rec.Level, rec.Msg, line)
			continue
		}
		print(rec)
	}

	if failed {
		os.Exit(1)
	}
}

// print renders one record: a headline, its attributes, and then the traceback
// or Go stack indented beneath it — the detail that explains the headline.
func print(rec record) {
	fmt.Printf("%s %-5s %s", shortTime(rec.Time), rec.Level, rec.Msg)
	if attrs := formatAttrs(rec.Extra); attrs != "" {
		fmt.Printf("  %s", attrs)
	}
	fmt.Println()

	if rec.Traceback != "" {
		for _, line := range strings.Split(rec.Traceback, "\n") {
			fmt.Printf("      %s\n", line)
		}
	}
	// The frames carry the source line, which the rendered traceback does not.
	for _, f := range rec.Frames {
		if f.Source != "" {
			fmt.Printf("      #%d:%s line %d => %s\n", f.VerbLoc, f.Verb, f.Line, f.Source)
		}
	}
	if rec.GoStack != "" {
		for _, line := range strings.Split(strings.TrimRight(rec.GoStack, "\n"), "\n") {
			fmt.Printf("      %s\n", line)
		}
	}
}

func readRecords(path string, minLevel int) ([]record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	var records []record
	scanner := bufio.NewScanner(file)
	// A Go stack in a single record can exceed the default 64KB line limit.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		var raw map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue // a truncated final line from a killed server is not an error
		}
		rec := parse(raw)
		if levels[rec.Level] < minLevel {
			continue
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return records, nil
}

func parse(raw map[string]any) record {
	rec := record{Extra: map[string]any{}}
	for key, value := range raw {
		switch key {
		case "time":
			rec.Time, _ = value.(string)
		case "level":
			rec.Level, _ = value.(string)
		case "msg":
			rec.Msg, _ = value.(string)
		case "traceback":
			rec.Traceback, _ = value.(string)
		case "go_stack":
			rec.GoStack, _ = value.(string)
		case "frames":
			if encoded, err := json.Marshal(value); err == nil {
				_ = json.Unmarshal(encoded, &rec.Frames)
			}
		default:
			rec.Extra[key] = value
		}
	}
	return rec
}

func formatAttrs(extra map[string]any) string {
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, extra[key]))
	}
	return strings.Join(parts, " ")
}

// shortTime keeps the clock time and drops the date: a log is read while its run
// is still the thing on your mind.
func shortTime(stamp string) string {
	if _, clock, found := strings.Cut(stamp, "T"); found {
		if hms, _, ok := strings.Cut(clock, "."); ok {
			return hms
		}
		return clock
	}
	return stamp
}

func listRuns(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(matches) == 0 {
		fmt.Printf("no runs in %s\n", dir)
		return
	}
	sort.Strings(matches)
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		fmt.Printf("%s  %s  %d bytes\n",
			info.ModTime().Format("2006-01-02 15:04:05"), filepath.Base(path), info.Size())
	}
}
