package parser

import (
	"testing"

	"github.com/MongooseMoo/barn/ir"
)

// Toast has no Boolean literal: true and false are built-in variables
// (sym_table.cc SLOT_TRUE/SLOT_FALSE) bound when a frame starts
// (eval_env.cc), so they parse as identifiers and may be assigned.
func TestParseTrueAndFalseAsIdentifiers(t *testing.T) {
	for _, input := range []string{"true", "false", "TRUE", "False"} {
		t.Run(input, func(t *testing.T) {
			expr, err := NewParser(input).ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("ParseExpression(%q) error = %v", input, err)
			}
			identifier, ok := expr.(*ir.IdentifierExpr)
			if !ok || identifier.Name != input {
				t.Fatalf("ParseExpression(%q) = %#v, want identifier %q", input, expr, input)
			}
		})
	}
}

func TestParseAssignmentToTrue(t *testing.T) {
	if _, err := NewParser("x = true = {};").ParseProgram(); err != nil {
		t.Fatalf("ParseProgram() error = %v", err)
	}
}
