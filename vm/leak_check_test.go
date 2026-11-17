package vm

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/internal/leakcheck"
)

// TestMemorySourceReplay uses the same source/structured-source execution and
// cleanup as the native fuzzers. It runs only when the campaign supplies an
// explicit input and output, in a fresh test process per input.
func TestMemorySourceReplay(t *testing.T) {
	input, out := os.Getenv("BARN_LEAK_INPUT"), os.Getenv("BARN_LEAK_OUTPUT")
	if input == "" || out == "" {
		t.Skip("set BARN_LEAK_INPUT and BARN_LEAK_OUTPUT for bounded same-process replay")
	}
	runMemorySourceReplay(t, input, out)
}

func TestMemorySourceReplayControl(t *testing.T) {
	for _, name := range []string{"source", "structured"} {
		t.Run(name, func(t *testing.T) {
			runMemorySourceReplay(t, filepath.Join("..", "scripts", "fixtures", "memory", name), t.TempDir())
		})
	}
}

func runMemorySourceReplay(t *testing.T, input, out string) {
	t.Helper()
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 || lines[0] != "go test fuzz v1" {
		t.Fatal("expected one Go fuzz corpus value")
	}
	structured := strings.HasPrefix(lines[1], "[]byte(")
	prefix := "string("
	if structured {
		prefix = "[]byte("
	}
	if !strings.HasPrefix(lines[1], prefix) || !strings.HasSuffix(lines[1], ")") {
		t.Fatal("expected a string or []byte corpus value")
	}
	value, err := strconv.Unquote(strings.TrimSuffix(strings.TrimPrefix(lines[1], prefix), ")"))
	if err != nil {
		t.Fatal(err)
	}
	source := value
	if structured {
		source = fuzzseed.StructuredSource(t, []byte(value))
	}
	world := newSourceFuzzWorld(t)
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "input"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "source.moo"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := leakcheck.Replay(out, func() { executeFuzzSource(t, world, source, structured) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("memory replay: %s; evidence=%s", result.Verdict, out)
	if result.Verdict != "healthy" {
		t.Fatalf("memory replay %s: %v", result.Verdict, result.Reasons)
	}
}
