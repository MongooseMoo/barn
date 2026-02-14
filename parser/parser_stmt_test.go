package parser

import "testing"

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
