package compiler_test

import (
	"testing"

	"github.com/MongooseMoo/barn/compiler"
)

func TestRejectStringAcrossSourceLines(t *testing.T) {
	for _, source := range [][]string{
		{"return \"a", "b\";"},
		{"return \"a\\", "b\";"},
		{"return \"a"},
		{"return \"a\\"},
	} {
		program, diagnostics := compiler.New(nil).CompileMOO(source)
		if program != nil || len(diagnostics) != 2 {
			t.Fatalf("source=%q program=%v diagnostics=%v", source, program, diagnostics)
		}
		for i, want := range []string{"Line 1:  Missing quote", "Line 1:  syntax error"} {
			if diagnostics[i].Error() != want {
				t.Fatalf("source=%q diagnostic=%v want=%q", source, diagnostics[i], want)
			}
		}
	}
	for _, source := range [][]string{
		{"return \"a\" +", "\"b\";"},
		{`return "a\"b\\c";`},
	} {
		if _, diagnostics := compiler.New(nil).CompileMOO(source); len(diagnostics) != 0 {
			t.Fatalf("valid source %q: %v", source, diagnostics)
		}
	}
}
