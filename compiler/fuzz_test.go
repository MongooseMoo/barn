package compiler_test

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
)

func FuzzCompileVerifiable(f *testing.F) {
	fuzzseed.Add(f)
	f.Fuzz(func(t *testing.T, source string) {
		// A fresh registry also gives each input a fresh compiler source cache.
		program, diagnostics := builtins.NewRegistry().Compiler().CompileMOO(strings.Split(source, "\n"))
		if len(diagnostics) != 0 {
			return
		}
		if err := bytecode.VerifyProgram(program); err != nil {
			t.Fatalf("compiler emitted invalid bytecode: %v\nsource: %q", err, source)
		}
	})
}
