package compiler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/verb"
)

func TestIndependentCompilationDiagnostics(t *testing.T) {
	cases := []struct {
		name     string
		source   []string
		messages []string
	}{
		{"loop exits", []string{"break;", "continue;"}, []string{"No enclosing loop for `break' statement", "No enclosing loop for `continue' statement"}},
		{"unknown builtins", []string{"missing_one();", "missing_two();"}, []string{"Unknown built-in function: missing_one", "Unknown built-in function: missing_two"}},
		{"float overflow", []string{"a = 1e400;", "b = 2e400;"}, []string{"Floating-point literal out of range", "Floating-point literal out of range"}},
		{"scatter", []string{"{1} = {};", "{@a, @b} = {};", "{} = {};"}, []string{"Scattering assignment targets must be simple variables.", "More than one `@' target in scattering assignment.", "Empty list in scattering assignment."}},
		{"index boundaries", []string{"return ^;", "return $;"}, []string{"Illegal context for `^' expression.", "Illegal context for `$' expression."}},
		{"ordinary syntax", []string{"a = ;", "break;", "continue;"}, []string{"syntax error"}},
		{"semantic before syntax", []string{"break;", "a = ;", "continue;"}, []string{"No enclosing loop for `break' statement", "syntax error"}},
		{"scatter avoids cascades", []string{"{@a, @b, 1} = {};", "{1, ?a} = {};", "break;"}, []string{"Scattering assignment targets must be simple variables.", "Scattering assignment targets must be simple variables.", "No enclosing loop for `break' statement"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New(nil)
			var previous []Diagnostic
			for attempt := range 2 {
				program, diagnostics := c.CompileMOO(tc.source)
				if program != nil || len(diagnostics) != len(tc.messages) {
					t.Fatalf("program %v, diagnostics %v; want %v", program, diagnostics, tc.messages)
				}
				for i, want := range tc.messages {
					if diagnostics[i].Message != want || diagnostics[i].Position.Line != i+1 {
						t.Errorf("diagnostic %d = %+v; want line %d %q", i, diagnostics[i], i+1, want)
					}
				}
				if _, ok := c.cache.get(sourcekey.Of(tc.source)); ok {
					t.Fatal("failed source entered program cache")
				}
				if attempt != 0 {
					for i := range previous {
						if previous[i].Position != diagnostics[i].Position || previous[i].Message != diagnostics[i].Message {
							t.Fatal("diagnostics changed between attempts")
						}
					}
				}
				previous = diagnostics
			}
		})
	}
}

func TestCompilationDiagnosticRetainsFullOffendingPosition(t *testing.T) {
	for _, tc := range []struct {
		source string
		marker string
		stage  DiagnosticStage
	}{
		{"return 1;\n  answer = ;\n  missing();", ";\n  missing", SyntaxStage},
		{"return 1;\n  /* unfinished", "/*", SyntaxStage},
		{"return 1;\n  \x00", "\x00", SyntaxStage},
		{"  MissingFn();", ")", BytecodeStage},
	} {
		c := New(nil)
		program, diagnostics := c.CompileMOO(strings.Split(tc.source, "\n"))
		if program != nil || len(diagnostics) != 1 {
			t.Fatalf("%q: %v / %v", tc.source, program, diagnostics)
		}
		offset := strings.Index(tc.source, tc.marker)
		line := 1 + strings.Count(tc.source[:offset], "\n")
		column := offset - strings.LastIndex(tc.source[:offset], "\n")
		want := verb.Position{Line: line, Column: column, Offset: offset}
		if !reflect.DeepEqual(diagnostics[0].Position, want) || diagnostics[0].Stage != tc.stage {
			t.Errorf("%q: diagnostic %+v; want position %+v, stage %v", tc.source, diagnostics[0], want, tc.stage)
		}
		if strings.Contains(diagnostics[0].Error(), "Column") || !strings.HasPrefix(diagnostics[0].Error(), "Line ") {
			t.Fatalf("MOO-facing format changed: %q", diagnostics[0].Error())
		}
	}
}

func TestConcurrentIndependentDiagnosticsRemainCallerOwned(t *testing.T) {
	c := New(nil)
	lines := []string{"break;", "missing_one();", "continue;"}
	key := sourcekey.Of(lines)
	results := compileBurst([]*Compiler{c, c, c, c}, [][]string{lines, lines, lines, lines}, []sourcekey.Key{key, key, key, key})
	_, expected := New(nil).CompileMOO(lines)
	for i, result := range results {
		if result.program != nil || len(result.diagnostics) != 3 {
			t.Fatalf("caller %d: %+v", i, result)
		}
		for j, diagnostic := range result.diagnostics {
			if diagnostic.Position != expected[j].Position || diagnostic.Stage != expected[j].Stage || diagnostic.Message != expected[j].Message {
				t.Fatalf("caller %d diagnostic %d changed: %+v", i, j, diagnostic)
			}
		}
	}
	results[0].diagnostics[1].Message = "caller edit"
	if results[1].diagnostics[1].Message != expected[1].Message {
		t.Fatal("diagnostic batches share mutable storage")
	}
	program, retried := c.CompileMOO(lines)
	if program != nil || len(retried) != 3 || retried[1].Message != expected[1].Message {
		t.Fatalf("retry changed diagnostics: %v / %v", program, retried)
	}
	if _, found := c.cache.get(key); found {
		t.Fatal("failed source entered the program cache")
	}
}
