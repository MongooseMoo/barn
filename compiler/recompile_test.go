package compiler_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
)

var recompileBuiltins = map[string]int{"suspend": 1, "length": 2, "notify": 3, "tostr": 4, "raise": 5}

var recompileSources = []string{
	"x = 1; return x + 2 * 3;",
	"for x in ({1, \"a\", 2.5, #3, E_DIV, [1 -> 2]}) y = tostr(x); suspend(0); endfor return y;",
	"try suspend(0); x = 1 / 0; except e (E_DIV, E_TYPE) return e; finally notify(#0, \"done\"); endtry",
	"while loop (1) if (length({1}) > 0) break loop; endif endwhile",
	"fork (0) a = 1; fork t (1) b = 2; endfork endfork fork (2) c = 3; endfork return {a, b, c};",
	"{a, ?b = 3, @c} = {1, 2, 3, 4}; return \"Mixed\" + \"mixed\";",
}

func compileLines(t *testing.T, c *compiler.Compiler, source string) *bytecode.Program {
	t.Helper()
	program, diagnostics := c.CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("%q does not compile: %v", source, diagnostics)
	}
	return program
}

// Restoring a saved task rebuilds its program by compiling the saved source,
// so equal source and builtin layout must give equal programs. Two compilers
// are used because one compiler's cache would return the same object.
func TestCompileMOOIsDeterministic(t *testing.T) {
	for _, source := range recompileSources {
		first := compileLines(t, compiler.New(recompileBuiltins), source)
		second := compileLines(t, compiler.New(recompileBuiltins), source)
		if first == second {
			t.Fatalf("%q: compilers shared a program", source)
		}
		if !slices.Equal(first.Code, second.Code) || !slices.Equal(first.VarNames, second.VarNames) ||
			!slices.Equal(first.LineInfo, second.LineInfo) || first.NumLocals != second.NumLocals {
			t.Fatalf("%q: two compiles differ", source)
		}
		if len(first.Constants) != len(second.Constants) {
			t.Fatalf("%q: constant counts differ", source)
		}
		for i := range first.Constants {
			if bytecode.ConstantKey(first.Constants[i]) != bytecode.ConstantKey(second.Constants[i]) {
				t.Fatalf("%q: constant %d differs: %s vs %s", source, i, first.Constants[i], second.Constants[i])
			}
		}
		if first.Fingerprint() != second.Fingerprint() {
			t.Fatalf("%q: fingerprints differ", source)
		}
	}
}

// Fork bodies are numbered by the code order of their fork instructions in
// the whole compiled program. A runtime fork program is extracted from its
// parent (itself possibly a fork program); it must carry the same number and
// equal the body extracted directly from the whole program by that number.
func TestExtractForkNumbersBodiesInCodeOrder(t *testing.T) {
	root := compileLines(t, compiler.New(recompileBuiltins), recompileSources[4])
	if root.Origin != (bytecode.Origin{}) {
		t.Fatalf("whole program origin = %+v, want zero", root.Origin)
	}
	outer := root.ExtractFork(1)
	nested := outer.ExtractFork(1)
	last := root.ExtractFork(3)
	if outer == nil || nested == nil || last == nil {
		t.Fatalf("missing fork bodies: %v %v %v", outer, nested, last)
	}
	for _, tt := range []struct {
		name    string
		program *bytecode.Program
		index   int
	}{{"outer", outer, 1}, {"nested", nested, 2}, {"last", last, 3}} {
		if tt.program.Origin.Fork != tt.index {
			t.Fatalf("%s: fork index %d, want %d", tt.name, tt.program.Origin.Fork, tt.index)
		}
		direct := root.ExtractFork(tt.index)
		if direct == nil || !slices.Equal(direct.Code, tt.program.Code) || !slices.Equal(direct.LineInfo, tt.program.LineInfo) ||
			direct.Origin != tt.program.Origin || direct.Fingerprint() != tt.program.Fingerprint() {
			t.Fatalf("%s: direct extraction differs from extraction through its parents", tt.name)
		}
	}
	if root.ExtractFork(4) != nil || root.ExtractFork(0) != nil || root.ExtractFork(-1) != nil {
		t.Fatal("out-of-range fork index extracted a program")
	}
	if got := root.ExtractForkBody(0, 1); got != nil && got.Origin.Fork != -1 {
		t.Fatalf("range that is no fork body has fork index %d, want -1", got.Origin.Fork)
	}
}

func TestFingerprintCoversWhatTheFrameRuns(t *testing.T) {
	base := compileLines(t, compiler.New(recompileBuiltins), recompileSources[5])
	mutations := map[string]func(*bytecode.Program){
		"code":      func(p *bytecode.Program) { p.Code[0] ^= 1 },
		"constant":  func(p *bytecode.Program) { p.Constants[len(p.Constants)-1] = p.Constants[len(p.Constants)-2] },
		"names":     func(p *bytecode.Program) { p.VarNames[0] = "z" },
		"lines":     func(p *bytecode.Program) { p.LineInfo[0].Line++ },
		"locals":    func(p *bytecode.Program) { p.NumLocals++ },
		"fork":      func(p *bytecode.Program) { p.Origin.Fork = 1 },
		"native":    func(p *bytecode.Program) { p.Origin.Native = true },
		"layout":    func(p *bytecode.Program) { p.BuiltinLayout[0] ^= 1 },
	}
	for name, mutate := range mutations {
		program := *base
		program.Code = slices.Clone(base.Code)
		program.Constants = slices.Clone(base.Constants)
		program.VarNames = slices.Clone(base.VarNames)
		program.LineInfo = slices.Clone(base.LineInfo)
		mutate(&program)
		if program.Fingerprint() == base.Fingerprint() {
			t.Errorf("%s change keeps the fingerprint", name)
		}
	}
}
