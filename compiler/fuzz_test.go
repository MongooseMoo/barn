package compiler_test

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/vm"
)

func FuzzCompileVerifiable(f *testing.F) {
	fuzzseed.Add(f)
	f.Fuzz(func(t *testing.T, source string) {
		compileVerifiable(t, source, false)
	})
}

// FuzzStructuredCompileVerifiable compiles generated programs. Each is valid
// MOO within the compiler's limits, so a diagnostic fails the case.
func FuzzStructuredCompileVerifiable(f *testing.F) {
	fuzzseed.AddChoices(f)
	f.Fuzz(func(t *testing.T, choices []byte) {
		compileVerifiable(t, fuzzseed.StructuredSource(t, choices), true)
	})
}

func compileVerifiable(t *testing.T, source string, mustCompile bool) {
	// A fresh registry also gives each input a fresh compiler source cache.
	// The VM descriptors add pass() and eval(); without them every verb
	// that calls either fails to compile and is never verified.
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, vm.Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	program, diagnostics := registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		if mustCompile {
			t.Fatalf("generated program does not compile: %v\nsource:\n%s", diagnostics, source)
		}
		return
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		t.Fatalf("compiler emitted invalid bytecode: %v\nsource: %q", err, source)
	}
}
