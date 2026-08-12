package compiler_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/types"
)

// TestMOOCompilationContractCharacterizesEveryNodeFamily fixes the current
// source -> semantic verb IR -> bytecode behavior while compilation ownership moves to
// its semantic owner. It intentionally compiles through both current entry
// points: Phase 5 will delete CompileVerb, and this test must then move to the
// single source-compilation owner rather than preserving the old API.
func TestMOOCompilationContractCharacterizesEveryNodeFamily(t *testing.T) {
	source := []string{
		"integer = 1;",
		"floating = 2.5;",
		"string = \"value\";",
		"boolean = true;",
		"object = #0;",
		"err = E_TYPE;",
		"unary = -integer;",
		"binary = integer + 2 * 3;",
		"ternary = boolean ? integer | 0;",
		"list = {integer, @{2, 3}};",
		"list_range = {1..3};",
		"map = [\"key\" -> integer];",
		"indexed = list[1];",
		"ranged = list[^..$];",
		"property = object.name;",
		"dynamic_property = object.(\"name\");",
		"verb_result = object:look();",
		"dynamic_verb_result = object:(\"look\")();",
		"caught = `integer / 0 ! E_DIV => 0';",
		"if (boolean)",
		"  integer = integer + 1;",
		"elseif (integer == 2)",
		"  integer = 3;",
		"else",
		"  integer = 4;",
		"endif",
		"while named_loop (integer < 5)",
		"  integer = integer + 1;",
		"  break named_loop;",
		"endwhile",
		"for value, index in (list)",
		"  continue value;",
		"endfor",
		"for value in [1..3]",
		"  break value;",
		"endfor",
		"try",
		"  integer = integer / 1;",
		"except detail (E_DIV)",
		"  integer = 0;",
		"finally",
		"  boolean = false;",
		"endtry",
		"{required, ?optional = 2, @rest} = list;",
		"fork task_id (0)",
		"  integer = integer + 1;",
		"endfork",
		"return {integer, floating, string, boolean, object, err};",
	}

	compiled, diagnostics := compiler.New(nil).CompileMOO(source)
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO failed: %v", diagnostics)
	}
	if !reflect.DeepEqual(compiled.Source, source) {
		t.Fatalf("compiled source changed:\n got: %#v\nwant: %#v", compiled.Source, source)
	}
	if len(compiled.Code) == 0 {
		t.Fatal("compiled program contains no bytecode")
	}
	if len(compiled.LineInfo) == 0 {
		t.Fatal("compiled program contains no source-line mapping")
	}
	if got := compiled.LineForIP(0); got != 1 {
		t.Fatalf("first instruction maps to line %d, want 1", got)
	}
}

func TestMOOCompilationContractPreservesEmptyProgram(t *testing.T) {
	compiled, diagnostics := compiler.New(nil).CompileMOO([]string{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO(empty) failed: %v", diagnostics)
	}
	if len(compiled.Source) != 0 {
		t.Fatalf("empty program source = %#v, want empty", compiled.Source)
	}
	if len(compiled.Code) != 1 || bytecode.OpCode(compiled.Code[0]) != bytecode.OP_RETURN_NONE {
		t.Fatalf("empty program bytecode = %v, want only OP_RETURN_NONE", compiled.Code)
	}
}

func TestMOOCompilationContractKeepsAdjacentFloatConstantsDistinct(t *testing.T) {
	compiled, diagnostics := compiler.New(nil).CompileMOO([]string{
		"return {1.0, 1.0000000000000002};",
	})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO failed: %v", diagnostics)
	}

	wantBits := map[uint64]bool{
		math.Float64bits(1.0):                  false,
		math.Float64bits(math.Nextafter(1, 2)): false,
	}
	for _, constant := range compiled.Constants {
		if constant.Type() != types.TYPE_FLOAT {
			continue
		}
		bits := math.Float64bits(constant.Float())
		if _, ok := wantBits[bits]; ok {
			wantBits[bits] = true
		}
	}
	for bits, found := range wantBits {
		if !found {
			t.Fatalf("compiled constants omit float bits %016x: %#v", bits, compiled.Constants)
		}
	}
}

func TestMOOCompilationContractReportsParseLine(t *testing.T) {
	_, diagnostics := compiler.New(nil).CompileMOO([]string{
		"value = 1;",
		"value = ;",
	})
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %v, want exactly one", diagnostics)
	}
	if !strings.HasPrefix(diagnostics[0].Error(), "Line 2:  ") {
		t.Fatalf("diagnostic = %q, want Line 2 prefix", diagnostics[0].Error())
	}
}
