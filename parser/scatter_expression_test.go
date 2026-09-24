package parser

import (
	"strings"
	"testing"
)

// A scatter with optional or rest targets is an expression in Toast's grammar,
// so it must parse and round-trip wherever '=' may appear, not only as a
// statement. ChatMUD's #0:core_objects drains a queue this way.
func TestScatterWithOptionalAndRestTargetsParsesAsExpression(t *testing.T) {
	for _, src := range []string{
		"while ({?sfc, @todo} = todo)\nendwhile",
		"if ({?a = 5, b} = x)\nendif",
		"y = ({a, @b} = x);",
		"y = z = {?a, b} = x;",
		"return {a, @b} = x;",
	} {
		stmts, err := NewParser(src).ParseProgram()
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		got := strings.Join(FormatMOO(stmts), "\n")
		again, err := NewParser(got).ParseProgram()
		if err != nil {
			t.Errorf("%q: unparsed form %q failed to re-parse: %v", src, got, err)
			continue
		}
		if got2 := strings.Join(FormatMOO(again), "\n"); got2 != got {
			t.Errorf("%q: round-trip not stable:\n%s\n---\n%s", src, got, got2)
		}
	}
}

// Toast's dedicated scatter production needs a '?' item and is an operand at
// any precedence. Without one the target is a list lowered by '=', so it
// cannot sit under a tighter operator.
func TestScatterExpressionPrecedenceFollowsToastProductions(t *testing.T) {
	if _, err := NewParser("return 0 || {?a, b} = x;").ParseProgram(); err != nil {
		t.Errorf("0 || {?a, b} = x: %v", err)
	}
	for _, src := range []string{"return 1 + {a, @b} = x;", "return 0 || {a, b} = x;"} {
		if _, err := NewParser(src).ParseProgram(); err == nil {
			t.Errorf("%s parsed; a list target binds with assignment precedence", src)
		}
	}
	if _, err := NewParser("return {a ? b | c} = x;").ParseProgram(); err == nil {
		t.Error("a ternary element was taken as an optional scatter target")
	}
}
