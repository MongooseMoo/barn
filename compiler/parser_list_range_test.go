package compiler_test

import (
	"testing"

	"github.com/MongooseMoo/barn/compiler"
)

func TestRejectListRangeSource(t *testing.T) {
	for _, source := range []string{"return {0..0};", "return {0..0}[1];"} {
		t.Run(source, func(t *testing.T) {
			program, diagnostics := compiler.New(nil).CompileMOO([]string{source})
			if program != nil || len(diagnostics) != 1 || diagnostics[0].Error() != "Line 1:  syntax error" {
				t.Fatalf("program=%v diagnostics=%v", program, diagnostics)
			}
		})
	}
	for _, source := range []string{"return {0, 1}[1..2];", "for i in [0..1] endfor"} {
		if _, diagnostics := compiler.New(nil).CompileMOO([]string{source}); len(diagnostics) != 0 {
			t.Fatalf("valid source %q: %v", source, diagnostics)
		}
	}
}
