package task

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A state or execution lease written anywhere but its one setter would leave
// the index wrong, and nothing else would notice.
func TestTaskStateIsWrittenInOnePlace(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	write := regexp.MustCompile(`\.(Status|executionActive)\s*=[^=]`)
	var sites []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for number, line := range strings.Split(string(source), "\n") {
			if write.MatchString(line) {
				sites = append(sites, fmt.Sprintf("%s:%d: %s", file, number+1, strings.TrimSpace(line)))
			}
		}
	}
	if len(sites) != 2 || !strings.Contains(sites[0], "t.Status = state") || !strings.Contains(sites[1], "t.executionActive = active") {
		t.Fatalf("task state or execution lease is assigned outside its setter:\n%s", strings.Join(sites, "\n"))
	}
}
