package fuzzseed

import (
	"math/rand/v2"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/vm"
)

func TestStructuredBuiltinNamesAreRegistered(t *testing.T) {
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, vm.Descriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, shape := range builtinShapes {
		if !registry.Has(shape.name) {
			t.Errorf("builtin %q is not registered", shape.name)
		}
	}
}

func TestStructuredBuiltinsOmitSideEffects(t *testing.T) {
	offered := make(map[string]bool, len(builtinShapes))
	for _, shape := range builtinShapes {
		offered[shape.name] = true
	}
	for _, name := range []string{
		"exec", "shutdown", "dump_database", "file_open", "file_remove",
		"read", "read_http", "call_function", "eval", "getenv", "server_log",
		"reseed_random", "run_gc", "argon2",
	} {
		if offered[name] {
			t.Errorf("structured programs call %q", name)
		}
	}
	for _, name := range []string{
		"create", "recycle", "chr", "sort", "pcre_match", "generate_json",
		"parse_json", "string_hash", "crypt", "kill_task", "notify", "yin",
		"floatstr", "simplex_noise", "disassemble", "length", "suspend",
	} {
		if !offered[name] {
			t.Errorf("structured programs never call %q", name)
		}
	}
}

// Most builtin calls should pass argument admission so they reach the
// builtin's body rather than stopping at E_ARGS or E_TYPE.
func TestStructuredBuiltinCallsMostlyFitTheirSignature(t *testing.T) {
	signatures := make(map[string]*builtins.Signature)
	for _, d := range builtins.Descriptors() {
		signatures[d.Name] = d.Signature
	}
	rng := rand.New(rand.NewPCG(1, 2))
	fit, total := 0, 2000
	for i := 0; i < total; i++ {
		choices := make([]byte, 64)
		for j := range choices {
			choices[j] = byte(rng.Uint32())
		}
		g := &generator{choices: choices, budget: maxNodes, scope: initializedVariables}
		call := g.builtinCall(builtinShapes, 0)
		if fitsSignature(signatures[call.Name], call.Args) {
			fit++
		}
	}
	if fit*2 < total {
		t.Fatalf("%d of %d builtin calls fit their signature, want at least half", fit, total)
	}
}

func fitsSignature(sig *builtins.Signature, args []ir.Expr) bool {
	if int64(len(args)) < sig.MinArgs || sig.MaxArgs >= 0 && int64(len(args)) > sig.MaxArgs {
		return false
	}
	literalTypes := map[ir.LiteralKind]int64{
		ir.LiteralInt: 0, ir.LiteralObj: 1, ir.LiteralString: 2, ir.LiteralErr: 3, ir.LiteralFloat: 9,
	}
	for i, arg := range args {
		if _, splice := arg.(*ir.SpliceExpr); splice {
			return false
		}
		literal, ok := arg.(*ir.LiteralExpr)
		if !ok || i >= len(sig.ArgTypes) {
			continue
		}
		got, want := literalTypes[literal.Kind], sig.ArgTypes[i]
		if want >= 0 && got != want || want == -2 && got != 0 && got != 9 {
			return false
		}
	}
	return true
}

func TestStructuredSourceIsDeterministic(t *testing.T) {
	choices := []byte{2, 3, 4, 200, 17, 9, 5, 6, 7, 8, 99, 1, 0, 12}
	first := StructuredSource(t, choices)
	if second := StructuredSource(t, choices); second != first {
		t.Fatalf("same choices gave different programs:\n%s\n---\n%s", first, second)
	}
	if empty := StructuredSource(t, nil); empty == first {
		t.Fatalf("different choices gave the same program:\n%s", first)
	}
}
