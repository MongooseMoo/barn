package fuzzseed

import (
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/vm"
)

func TestStructuredBuiltinNamesAreRegistered(t *testing.T) {
	registry, err := builtins.NewRegistryFromDescriptors(config.Core|config.SQLite, vm.Descriptors())
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
		"verb_info", "descendants", "parents", "task_perms", "trunc", "spellcheck",
		"sqlite_open", "sqlite_close", "sqlite_query", "sqlite_execute", "sqlite_limit",
		"sqlite_info", "sqlite_interrupt", "sqlite_last_insert_row_id", "sqlite_handles",
	} {
		if !offered[name] {
			t.Errorf("structured programs never call %q", name)
		}
	}
}

// The SQLite builtins follow the core builtins, so a choice byte below the
// core count names the builtin it named before SQLite was offered.
func TestStructuredSQLiteShapesFollowCoreShapes(t *testing.T) {
	core := 0
	for _, d := range builtins.Descriptors() {
		if d.Capability == config.Core && d.Visibility == builtins.Public && !excludedBuiltins[d.Name] && !strings.HasPrefix(d.Name, "file_") {
			core++
		}
	}
	var sqlite []string
	for i, shape := range builtinShapes {
		if (i >= core) != shape.sqlite {
			t.Fatalf("builtinShapes[%d] = %q (sqlite %v), want the %d core builtins first", i, shape.name, shape.sqlite, core)
		}
		if shape.sqlite {
			sqlite = append(sqlite, shape.name)
		}
	}
	want := []string{"sqlite_close", "sqlite_execute", "sqlite_handles", "sqlite_info", "sqlite_interrupt",
		"sqlite_last_insert_row_id", "sqlite_limit", "sqlite_open", "sqlite_query"}
	if !slices.Equal(sqlite, want) {
		t.Fatalf("SQLite shapes = %v, want %v", sqlite, want)
	}
}

// A SQLite call never opens a file and never runs SQL from an expression:
// sqlite_open's path is the literal ":memory:" and sqlite_query's and
// sqlite_execute's SQL is a literal from sqliteStatements.
func TestStructuredSQLiteCallsStayInMemory(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for _, shape := range builtinShapes {
		if !shape.sqlite {
			continue
		}
		for i := 0; i < 500; i++ {
			choices := make([]byte, 64)
			for j := range choices {
				choices[j] = byte(rng.Uint32())
			}
			g := &generator{choices: choices, budget: maxNodes, scope: initializedVariables}
			call := g.builtinCall([]builtinShape{shape}, 0)
			if call.Name != shape.name || !g.sqlite {
				t.Fatalf("builtinCall(%s) = %s, sqlite %v", shape.name, call.Name, g.sqlite)
			}
			literal := func(i int) string {
				if i >= len(call.Args) {
					t.Fatalf("%s has %d arguments, want more than %d", shape.name, len(call.Args), i)
				}
				arg, ok := call.Args[i].(*ir.LiteralExpr)
				if !ok || arg.Kind != ir.LiteralString {
					t.Fatalf("%s argument %d = %#v, want a string literal", shape.name, i+1, call.Args[i])
				}
				return arg.StringValue
			}
			switch shape.name {
			case "sqlite_open":
				if path := literal(0); path != ":memory:" {
					t.Fatalf("sqlite_open path %q, want \":memory:\"", path)
				}
			case "sqlite_query", "sqlite_execute":
				if sql := literal(1); !slices.Contains(sqliteStatements, sql) {
					t.Fatalf("%s SQL %q is not in sqliteStatements", shape.name, sql)
				}
			}
		}
	}
	for _, sql := range sqliteStatements {
		for _, r := range sql {
			if r < 0x20 || r > 0x7e {
				t.Errorf("statement %q is not printable ASCII", sql)
			}
		}
	}
}

// A program that calls a SQLite builtin runs its body inside sqliteWrap; one
// that does not is the program it was before SQLite was offered, with no
// handle variable.
func TestStructuredSQLiteProgramsAreWrapped(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	wrapped := 0
	for i := 0; i < 2000; i++ {
		choices := make([]byte, 300)
		for j := range choices {
			choices[j] = byte(rng.Uint32())
		}
		program := Program(choices)
		source := StructuredSource(t, choices)
		prologue := len(initializedVariables)
		body := program.Statements[prologue:]
		calls := strings.Count(source, "sqlite_")
		if calls == 0 {
			if regexp.MustCompile(`\b` + sqliteHandleVar + `\b`).MatchString(source) {
				t.Fatalf("program without SQLite names %q:\n%s", sqliteHandleVar, source)
			}
			continue
		}
		wrapped++
		first, ok := body[0].(*ir.ExprStmt)
		if !ok {
			t.Fatalf("SQLite program does not begin with set_thread_mode(0):\n%s", source)
		}
		if call, ok := first.Expr.(*ir.BuiltinCallExpr); !ok || call.Name != "set_thread_mode" {
			t.Fatalf("SQLite program does not begin with set_thread_mode(0):\n%s", source)
		}
		if len(body) != 5 {
			t.Fatalf("SQLite program has %d top-level statements after the prologue, want 5:\n%s", len(body), source)
		}
		if try, ok := body[4].(*ir.TryStmt); !ok || try.Finalizer == nil || len(try.Handlers) != 0 {
			t.Fatalf("SQLite program does not end in try ... finally:\n%s", source)
		}
	}
	if wrapped == 0 {
		t.Fatal("no generated program called a SQLite builtin")
	}
}

// ASCIISource is Source with "é", the one non-ASCII string piece, spelled
// "e", and is printable ASCII apart from tab and newline.
func TestStructuredASCIISource(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	changed := 0
	for i := 0; i < 2000; i++ {
		choices := make([]byte, 300)
		for j := range choices {
			choices[j] = byte(rng.Uint32())
		}
		source := StructuredSource(t, choices)
		ascii, err := ASCIISource(choices)
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.ReplaceAll(source, "é", "e"); ascii != want {
			t.Fatalf("ASCIISource differs from Source beyond its string pieces:\n%s\n---\n%s", ascii, source)
		}
		if ascii != source {
			changed++
		}
		for _, r := range ascii {
			if (r < 0x20 && r != '\t' && r != '\n') || r > 0x7e {
				t.Fatalf("ASCIISource has %q:\n%s", r, ascii)
			}
		}
	}
	if changed == 0 {
		t.Fatal("no Source had a non-ASCII string piece")
	}
}

// A flag position (-3, INT or BOOL) gets an INT literal or true/false
// (spelled as the parser spells them, identifiers) unless the first choice
// takes the any-expression escape.
func TestStructuredFlagArgumentIsIntOrBool(t *testing.T) {
	for first := 1; first < 4; first++ {
		for second := 0; second < 256; second++ {
			g := &generator{choices: []byte{byte(first), byte(second), byte(second)}, budget: maxNodes, scope: initializedVariables}
			switch arg := g.typedArgument(-3, 0).(type) {
			case *ir.LiteralExpr:
				if arg.Kind != ir.LiteralInt {
					t.Fatalf("typedArgument(-3) with choices %d, %d = %#v, want an INT literal", first, second, arg)
				}
			case *ir.IdentifierExpr:
				if arg.Name != "true" && arg.Name != "false" {
					t.Fatalf("typedArgument(-3) with choices %d, %d = identifier %q, want true or false", first, second, arg.Name)
				}
			default:
				t.Fatalf("typedArgument(-3) with choices %d, %d = %#v, want an INT literal, true or false", first, second, arg)
			}
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
		if want >= 0 && got != want || want == -2 && got != 0 && got != 9 || want == -3 && got != 0 {
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
