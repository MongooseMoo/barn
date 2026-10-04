package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/verb"
)

func TestActiveDepthRejectsBeforeDescendingBeyondBudget(t *testing.T) {
	for name, source := range map[string]string{
		"unary":      strings.Repeat("!", 2560) + "1;",
		"list":       strings.Repeat("{", 2560) + "1" + strings.Repeat("}", 2560) + ";",
		"map":        strings.Repeat("[1 -> ", 2560) + "1" + strings.Repeat("]", 2560) + ";",
		"power":      strings.Repeat("1 ^ ", 2560) + "1;",
		"assignment": strings.Repeat("a = ", 2560) + "1;",
	} {
		t.Run(name, func(t *testing.T) {
			p := NewParser(source)
			program, err := p.ParseProgram()
			if program != nil || !errors.Is(err, verb.ErrMaxNestingDepth) {
				t.Fatalf("program %v / error %v", program, err)
			}
			if p.current.Position.Offset > len(source)/5 {
				t.Fatalf("parsed attacker-controlled tail before rejecting depth: cursor %+v, source bytes %d", p.current.Position, len(source))
			}
		})
	}
}

func TestActiveDepthCombinesStatementsAndParentheses(t *testing.T) {
	for _, depth := range []int{255, 256, 257, 2560} {
		blocks := depth / 2
		parentheses := depth - blocks
		source := strings.Repeat("if (1)\n", blocks) + strings.Repeat("(", parentheses) + "1" + strings.Repeat(")", parentheses) + ";\n" + strings.Repeat("endif\n", blocks)
		program, err := NewParser(source).ParseProgram()
		if depth <= MaxNestingDepth {
			if err != nil || program == nil {
				t.Fatalf("combined depth %d unexpectedly rejected: %v", depth, err)
			}
		} else if program != nil || !errors.Is(err, verb.ErrMaxNestingDepth) {
			t.Fatalf("combined depth %d accepted: %v / %v", depth, program, err)
		}
	}
}
