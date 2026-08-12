package compiler_test

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/compiler"
)

func TestBuiltinNamesAreCaseInsensitive(t *testing.T) {
	for _, name := range []string{"LENGTH", "LeNgTh"} {
		_, diagnostics := compiler.New(map[string]int{"length": 1}).CompileMOO(
			[]string{"return " + name + "({1, 2});"},
		)
		if len(diagnostics) > 0 {
			t.Fatalf("builtin %q failed to compile: %v", name, diagnostics)
		}
	}
}

func TestPassNameIsCaseInsensitive(t *testing.T) {
	for _, name := range []string{"PASS", "PaSs"} {
		_, diagnostics := compiler.New(nil).CompileMOO([]string{"return " + name + "();"})
		if len(diagnostics) > 0 {
			t.Fatalf("native builtin %q failed to compile: %v", name, diagnostics)
		}
	}
}

func TestUnknownBuiltinDiagnosticPreservesSourceSpelling(t *testing.T) {
	_, diagnostics := compiler.New(nil).CompileMOO([]string{"return MiSsInG();"})
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "MiSsInG") {
		t.Fatalf("diagnostics = %v, want original builtin spelling", diagnostics)
	}
}
