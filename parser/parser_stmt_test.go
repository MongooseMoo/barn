package parser

import (
	"barn/verb"
	"testing"
)

func TestParseExpressionStatementRequiresSemicolonAtEOF(t *testing.T) {
	p := NewParser(`delete_property(#0, "recycle_log")`)
	if _, err := p.ParseProgram(); err == nil {
		t.Fatal("ParseProgram() succeeded, want error for missing trailing semicolon")
	}
}

func TestParseExpressionStatementRequiresSeparatorBetweenStatements(t *testing.T) {
	p := NewParser(`delete_property(#0, "a") delete_property(#0, "b");`)
	if _, err := p.ParseProgram(); err == nil {
		t.Fatal("ParseProgram() succeeded, want error for missing separator")
	}
}

func TestElseIfLowersToNestedSemanticConditionals(t *testing.T) {
	program, err := NewParser("if (a)\nreturn 1;\nelseif (b)\nreturn 2;\nelseif (c)\nreturn 3;\nelse\nreturn 4;\nendif").ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram() error = %v", err)
	}

	root, ok := program.Statements[0].(*verb.IfStmt)
	if !ok {
		t.Fatalf("root = %T, want *verb.IfStmt", program.Statements[0])
	}
	second, ok := root.Else[0].(*verb.IfStmt)
	if !ok {
		t.Fatalf("first else = %T, want nested *verb.IfStmt", root.Else[0])
	}
	third, ok := second.Else[0].(*verb.IfStmt)
	if !ok {
		t.Fatalf("second else = %T, want nested *verb.IfStmt", second.Else[0])
	}
	if len(third.Else) != 1 {
		t.Fatalf("final else length = %d, want 1", len(third.Else))
	}
	if root.Position().Line != 1 || second.Position().Line != 3 || third.Position().Line != 5 {
		t.Fatalf("conditional lines = %d, %d, %d; want 1, 3, 5", root.Position().Line, second.Position().Line, third.Position().Line)
	}
}
