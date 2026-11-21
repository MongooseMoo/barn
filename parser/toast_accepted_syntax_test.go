package parser

import (
	"reflect"
	"testing"
)

// Toast parser.y rule expr '.' ':' tID reads expr.:name as expr.(":name"), and
// unparse.cc prints a name that is not an identifier in the dynamic form.
func TestColonPropertyIsPrefixedDynamicProperty(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"return x.:foo;", `return x.(":foo");`},
		{"return x . : foo;", `return x.(":foo");`},
		{"return #0.:Foo;", `return #0.(":Foo");`},
		{"x.:a = y.:b.:c;", `x.(":a") = y.(":b").(":c");`},
		{"x = a.:b = 3;", `x = a.(":b") = 3;`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			program, err := NewParser(tc.source).ParseProgram()
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			lines, err := FormatMOOChecked(program)
			if err != nil {
				t.Fatalf("format: %v", err)
			}
			if !reflect.DeepEqual(lines, []string{tc.want}) {
				t.Fatalf("formatted %q, want %q", lines, tc.want)
			}
		})
	}
	for _, source := range []string{"return x.:if;", "return x.:;", "return x.:(\"a\");", "return x.:1;"} {
		if _, err := NewParser(source).ParseProgram(); err == nil {
			t.Fatalf("%q parsed; Toast requires an identifier after .:", source)
		}
	}
}

// Toast's list parser passes each byte as a signed char, so a byte above 127
// outside a string or comment ends the program text (parser.y my_getc, yylex).
func TestHighByteEndsProgramText(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   []string
	}{
		{"x = 1; \xc3\xa9 = 2; return x;", []string{"x = 1;"}},
		{"\xc3\xa9 = 1; return \xc3\xa9;", nil},
		{"x = 1;\xc3y = 2; return x;", []string{"x = 1;"}},
		{"return 6;\x80return 7;", []string{"return 6;"}},
		{"return 8;\xffreturn 9;", []string{"return 8;"}},
		{"x = 1;\nreturn x; \xc3\xa9 = 2;\nreturn 3;", []string{"x = 1;", "return x;"}},
		{"return \"a\xc3\xa9b\";", []string{"return \"a\xc3\xa9b\";"}},
		{"/* \xc3\xa9 */ return 3;", []string{"return 3;"}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			program, err := NewParser(tc.source).ParseProgram()
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			lines, err := FormatMOOChecked(program)
			if err != nil {
				t.Fatalf("format: %v", err)
			}
			if len(lines) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(lines, tc.want) {
				t.Fatalf("formatted %q, want %q", lines, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		source string
		line   int
	}{
		{"return 1 +\xc8 2;", 1},
		{"x = 1;\ny = \xc3\xa9;", 2},
		{"return x\xc3;", 1},
	} {
		_, diagnostics := NewParser(tc.source).ParseProgramWithDiagnostics(nil)
		if len(diagnostics) != 1 || diagnostics[0].Msg != "syntax error" || diagnostics[0].Position.Line != tc.line {
			t.Fatalf("%q: diagnostics %+v, want one syntax error on line %d", tc.source, diagnostics, tc.line)
		}
	}
}

// Toast skips C isspace() characters between tokens, form feed and vertical
// tab included.
func TestFormFeedAndVerticalTabSeparateTokens(t *testing.T) {
	for _, source := range []string{"return\v4;", "return\f5;", "\f\vreturn 1;\v"} {
		if _, err := NewParser(source).ParseProgram(); err != nil {
			t.Fatalf("%q: %v", source, err)
		}
	}
}
