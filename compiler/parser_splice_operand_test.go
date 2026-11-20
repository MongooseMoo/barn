package compiler_test

import (
	"testing"

	"github.com/MongooseMoo/barn/compiler"
)

func TestSpliceRequiresArgumentEntry(t *testing.T) {
	for _, source := range []string{
		`return {1/@{}};`, `return {""/@{}};`,
		`return @{};`, `return {(@{})};`, `return {1 + @{}};`,
		`return {1 ? @{} | {}};`, `return {@@{}};`,
	} {
		t.Run(source, func(t *testing.T) {
			program, diagnostics := compiler.New(nil).CompileMOO([]string{source})
			if program != nil || len(diagnostics) != 1 || diagnostics[0].Error() != "Line 1:  syntax error" {
				t.Fatalf("program=%v diagnostics=%v", program, diagnostics)
			}
		})
	}
	for _, source := range []string{
		`return {@x = {1}, @{2}[1..1]};`,
		`{command, @rest, ?last = 9} = {1, 2, 3};`,
		`{@x, ?s} = {0, @x};`,
		`length(@args); #0:foo(@args); #0:("foo")(@args); $foo(@args);`,
		"return `1 ! @{E_TYPE} => 0';",
		`try return 1; except (@{E_TYPE}) return 0; endtry`,
	} {
		if _, diagnostics := compiler.New(map[string]int{"length": 0}).CompileMOO([]string{source}); len(diagnostics) != 0 {
			t.Fatalf("valid source %q: %v", source, diagnostics)
		}
	}
}
