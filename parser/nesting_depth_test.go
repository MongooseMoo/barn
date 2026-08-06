package parser_test

import (
	"barn/bytecode"
	"barn/compiler"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"barn/parser"
	"barn/verb"
)

const testMaxNestingDepth = 256

type nestingTestRegistry struct{}

func (nestingTestRegistry) GetID(string) (int, bool) { return 0, true }

func unarySource(depth int) string {
	return "return " + strings.Repeat("-", depth-2) + "1;"
}

func parenthesizedSource(depth int) string {
	n := depth - 2
	return "return " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + ";"
}

func additiveSource(depth int) string {
	return "return " + strings.Repeat("1 + ", depth-2) + "1;"
}

func listSource(depth int) string {
	n := depth - 2
	return "return " + strings.Repeat("{", n) + "1" + strings.Repeat("}", n) + ";"
}

func mapSource(depth int) string {
	n := depth - 2
	return "return " + strings.Repeat("[\"key\" -> ", n) + "1" + strings.Repeat("]", n) + ";"
}

func powerSource(depth int) string {
	return "return " + strings.Repeat("1 ^ ", depth-2) + "1;"
}

func assignmentSource(depth int) string {
	return strings.Repeat("value = ", depth-2) + "1;"
}

func nestedIfSource(depth int) string {
	n := depth - 2
	return strings.Repeat("if (1)\n", n) + "return 1;\n" + strings.Repeat("endif\n", n)
}

func nestedWhileSource(depth int) string {
	n := depth - 2
	return strings.Repeat("while (1)\n", n) + "return 1;\n" + strings.Repeat("endwhile\n", n)
}

func nestedTryFinallySource(depth int) string {
	n := depth - 2
	return strings.Repeat("try\n", n) + "return 1;\n" + strings.Repeat("finally\n;\nendtry\n", n)
}

func postfixSource(depth int) string {
	return "return value" + strings.Repeat("[1].property", (depth-2)/2) + strings.Repeat("[1]", (depth-2)%2) + ";"
}

func postfixAssignmentSource(depth int) string {
	return "value" + strings.Repeat("[1]", depth-3) + " = 1;"
}

func elseifSource(depth int) string {
	clauses := depth - 2
	var source strings.Builder
	source.WriteString("if (1)\n;\n")
	for i := 1; i < clauses; i++ {
		source.WriteString("elseif (1)\n;\n")
	}
	source.WriteString("else\nreturn 1;\nendif")
	return source.String()
}

func mixedSource(depth int) string {
	nestedStatements := (depth - 2) / 2
	nestedExpressions := depth - 2 - nestedStatements
	return strings.Repeat("if (1)\n", nestedStatements) +
		"return " + strings.Repeat("-", nestedExpressions) + "1;\n" +
		strings.Repeat("endif\n", nestedStatements)
}

func wideSiblingSource(count int) string {
	return "if (1)\n" + strings.Repeat("1;\n", count) + "endif"
}

func TestParserNestingDepthBoundary(t *testing.T) {
	tests := []struct {
		name  string
		build func(int) string
	}{
		{name: "unary", build: unarySource},
		{name: "parentheses", build: parenthesizedSource},
		{name: "nested lists", build: listSource},
		{name: "nested maps", build: mapSource},
		{name: "right associative power", build: powerSource},
		{name: "right associative assignment", build: assignmentSource},
		{name: "nested if", build: nestedIfSource},
		{name: "nested while", build: nestedWhileSource},
		{name: "nested try finally", build: nestedTryFinallySource},
		{name: "flat additive", build: additiveSource},
		{name: "flat postfix index property", build: postfixSource},
		{name: "flat elseif", build: elseifSource},
		{name: "mixed statement expression", build: mixedSource},
	}

	for _, test := range tests {
		for _, depth := range []int{255, 256} {
			t.Run(test.name+"/"+strconv.Itoa(depth), func(t *testing.T) {
				source := test.build(depth)
				program, err := parser.NewParser(source).ParseProgram()
				if err != nil {
					t.Fatalf("ParseProgram() error at depth %d = %v", depth, err)
				}
				if compiled, err := bytecode.NewCompiler().CompileProgram(program); err != nil || compiled == nil {
					t.Fatalf("CompileProgram() at depth %d = (%v, %v), want program", depth, compiled, err)
				}
				assertCanonicalRoundTrip(t, source)
			})
		}
		for _, depth := range []int{257, 2560} {
			t.Run(test.name+"/"+strconv.Itoa(depth), func(t *testing.T) {
				source := test.build(depth)
				program, err := parseWithoutPanic(t, source)
				if err == nil {
					t.Fatalf("ParseProgram() at depth %d = (%#v, nil), want nesting error", depth, program)
				}
				assertNestingError(t, err, depth)
				assertCompileNestingDiagnostic(t, source, depth)
			})
		}
	}
}

func TestParserGuardsPostfixAssignmentLowering(t *testing.T) {
	if _, err := parseWithoutPanic(t, postfixAssignmentSource(256)); err != nil {
		t.Fatalf("ParseProgram(postfix assignment depth 256) error = %v", err)
	}
	for _, depth := range []int{257, 2560} {
		program, err := parseWithoutPanic(t, postfixAssignmentSource(depth))
		if err == nil {
			t.Fatalf("ParseProgram(postfix assignment depth %d) = (%#v, nil), want nesting error", depth, program)
		}
		assertNestingError(t, err, depth)
	}
}

func parseWithoutPanic(t *testing.T, source string) (program *verb.Program, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("ParseProgram() panicked: %v", recovered)
		}
	}()
	return parser.NewParser(source).ParseProgram()
}

func assertCompileNestingDiagnostic(t *testing.T, source string, depth int) {
	t.Helper()
	var (
		program     *bytecode.Program
		diagnostics []compiler.Diagnostic
	)
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("CompileMOO(depth %d) panicked: %v", depth, recovered)
			}
		}()
		program, diagnostics = compiler.CompileMOO(strings.Split(strings.TrimSuffix(source, "\n"), "\n"), nestingTestRegistry{})
	}()
	if program != nil {
		t.Fatalf("CompileMOO(depth %d) returned partial program", depth)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("CompileMOO(depth %d) diagnostic count = %d, want 1", depth, len(diagnostics))
	}
	diagnostic := diagnostics[0]
	if diagnostic.Stage != compiler.SyntaxStage || diagnostic.Message != "syntax error" {
		t.Fatalf("CompileMOO(depth %d) diagnostic = %+v, want SyntaxStage syntax error", depth, diagnostic)
	}
	var depthErr *verb.NestingDepthError
	if !errors.As(diagnostic.Detail, &depthErr) {
		t.Fatalf("CompileMOO(depth %d) detail = %T %v, want *verb.NestingDepthError", depth, diagnostic.Detail, diagnostic.Detail)
	}
}

func assertNestingError(t *testing.T, err error, depth int) {
	t.Helper()
	var parseErr *parser.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("depth %d error = %T %v, want *parser.ParseError", depth, err, err)
	}
	if parseErr.Msg != "syntax error" {
		t.Fatalf("depth %d outward error = %q, want syntax error", depth, parseErr.Msg)
	}
	var depthErr *verb.NestingDepthError
	if !errors.As(err, &depthErr) {
		t.Fatalf("depth %d error = %T %v, want *verb.NestingDepthError detail", depth, err, err)
	}
	if got, want := depthErr.Error(), "maximum nesting depth exceeded (max 256)"; got != want {
		t.Fatalf("depth error detail = %q, want %q", got, want)
	}
}

func TestParserDepthIsNotNodeCount(t *testing.T) {
	source := wideSiblingSource(2560)
	program, err := parser.NewParser(source).ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram() rejected 2560 shallow siblings: %v", err)
	}
	if compiled, err := bytecode.NewCompiler().CompileProgram(program); err != nil || compiled == nil {
		t.Fatalf("CompileProgram() rejected 2560 shallow siblings: (%v, %v)", compiled, err)
	}
	assertCanonicalRoundTrip(t, source)
}

func TestFormatMOOIsChecked(t *testing.T) {
	var checked func(*verb.Program) ([]string, error) = parser.FormatMOO
	program, err := parser.NewParser(unarySource(testMaxNestingDepth)).ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram() error = %v", err)
	}
	if _, err := formatWithoutPanic(t, checked, program); err != nil {
		t.Fatalf("FormatMOO() error at depth 256 = %v", err)
	}

	for _, depth := range []int{257, 2560} {
		overLimit := &verb.Program{Statements: []verb.Stmt{&verb.ReturnStmt{
			Pos:   verb.Position{Line: 9},
			Value: nestedUnaryIR(depth - 1),
		}}}
		lines, err := formatWithoutPanic(t, checked, overLimit)
		if lines != nil {
			t.Fatalf("FormatMOO(depth %d) returned partial lines: %q", depth, lines)
		}
		var depthErr *verb.NestingDepthError
		if !errors.As(err, &depthErr) {
			t.Fatalf("FormatMOO(depth %d) error = %T %v, want *verb.NestingDepthError", depth, err, err)
		}
	}
}

func formatWithoutPanic(t *testing.T, checked func(*verb.Program) ([]string, error), program *verb.Program) (lines []string, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("FormatMOO() panicked: %v", recovered)
		}
	}()
	return checked(program)
}

func nestedUnaryIR(depth int) verb.Expr {
	var expr verb.Expr = &verb.LiteralExpr{Pos: verb.Position{Line: 9}, Kind: verb.LiteralInt, IntValue: 1}
	for i := 1; i < depth; i++ {
		expr = &verb.UnaryExpr{Pos: verb.Position{Line: 9}, Operator: verb.UnaryNegate, Operand: expr}
	}
	return expr
}

func TestParserNestingErrorUsesOffendingLine(t *testing.T) {
	source := fmt.Sprintf("return 0;\n%s", unarySource(257))
	_, err := parser.NewParser(source).ParseProgram()
	if err == nil {
		t.Fatal("ParseProgram() accepted depth 257 on line 2")
	}
	assertNestingError(t, err, 257)
	var parseErr *parser.ParseError
	errors.As(err, &parseErr)
	if parseErr.Line != 2 {
		t.Fatalf("ParseError.Line = %d, want 2", parseErr.Line)
	}
	var depthErr *verb.NestingDepthError
	errors.As(err, &depthErr)
	if depthErr.Position.Line != 2 {
		t.Fatalf("NestingDepthError.Position.Line = %d, want 2", depthErr.Position.Line)
	}
}
