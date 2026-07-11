package parser

// TestReview_* tests written by the analyst role.
// These are RED tests — they expose confirmed bugs.

import (
	"barn/verb"
	"strings"
	"testing"
)

// TestReview_EIntrptLiteralRejected confirms that E_INTRPT is missing from the
// parser's error-name table. E_INTRPT is a valid ToastStunt error code (code 18)
// but isErrorName() does not recognise it, so the parser rejects `E_INTRPT` as
// an unknown error literal.
func TestReview_EIntrptLiteralRejected(t *testing.T) {
	src := "x = E_INTRPT;"
	p := NewParser(src)
	_, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("BUG: E_INTRPT is a valid MOO error literal but the parser rejected it: %v", err)
	}
}

// TestReview_ListExprAsStatementMistakenForScatter confirms that a list
// expression starting with an identifier used as a statement is incorrectly
// parsed as a scatter assignment. `{x, y};` is a valid MOO statement (list
// expression whose result is discarded), but looksLikeScatter() returns true
// for any `{IDENTIFIER ...}` pattern, causing the parser to attempt scatter
// parsing and then fail because `=` is missing.
func TestReview_ListExprAsStatementMistakenForScatter(t *testing.T) {
	src := "{x, y};"
	p := NewParser(src)
	_, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("BUG: {x, y}; is a valid list expression statement but the parser rejected it: %v", err)
	}
}

// TestReview_UnparseForWithIndexVar confirms that UnparseProgram produces
// incorrect output for a labeled for-in-list loop that also carries an
// index/key variable. The unparser's ForStmt branch for s.Index != "" emits
// `value in [index..len(body)]` using the body statement count as the range
// end — completely wrong for `for label var, key in (container)`.
func TestReview_UnparseForWithIndexVar(t *testing.T) {
	// `for L x, k in (mylist) ... endfor`
	// The label heuristic consumes "L" as label, so ForStmt: Label="L", Value="x", Index="k", Container=mylist.
	src := "for L x, k in (mylist)\nreturn x;\nendfor"
	p := NewParser(src)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	lines := UnparseProgram(stmts)
	got := strings.Join(lines, "\n")

	// The unparser must emit the `for value, index in (container)` surface
	// (ToastStunt parser.y:160-174), preserving the label, not a garbage range.
	if !strings.Contains(got, "for L x, k in (mylist)") {
		t.Fatalf("BUG: UnparseProgram for for-with-index-variable produced wrong output:\n%s\n\nExpected output containing 'for L x, k in (mylist)'", got)
	}

	// Round-trip: the unparsed source must re-parse and re-unparse identically.
	p2 := NewParser(got)
	stmts2, err := p2.ParseProgram()
	if err != nil {
		t.Fatalf("BUG: unparsed for-with-index source failed to re-parse: %v\nsource:\n%s", err, got)
	}
	got2 := strings.Join(UnparseProgram(stmts2), "\n")
	if got != got2 {
		t.Fatalf("BUG: for-with-index round-trip not stable:\nfirst:\n%s\nsecond:\n%s", got, got2)
	}
}

// TestReview_BreakLabelAsIdentExpr confirms the asymmetry between break and
// continue label handling. `break myloop;` inside a labeled while loop should
// break out of the named loop. The parser sets BreakStmt.Label="" and places
// the identifier in BreakStmt.Value instead, leaving disambiguation entirely
// to the compiler. Meanwhile continue correctly populates ContinueStmt.Label.
func TestReview_BreakLabelAsIdentExpr(t *testing.T) {
	breakSrc := "while (1)\nbreak myloop;\nendwhile"
	bp := NewParser(breakSrc)
	bStmts, err := bp.ParseProgram()
	if err != nil {
		t.Fatalf("parse error for break src: %v", err)
	}

	contSrc := "while myloop (1)\ncontinue myloop;\nendwhile"
	cp := NewParser(contSrc)
	cStmts, err := cp.ParseProgram()
	if err != nil {
		t.Fatalf("parse error for continue src: %v", err)
	}

	bWhile, ok := bStmts.Statements[0].(*verb.WhileStmt)
	if !ok {
		t.Fatalf("expected WhileStmt, got %T", bStmts.Statements[0])
	}
	bk, ok := bWhile.Body[0].(*verb.BreakStmt)
	if !ok {
		t.Fatalf("expected BreakStmt, got %T", bWhile.Body[0])
	}

	cWhile, ok := cStmts.Statements[0].(*verb.WhileStmt)
	if !ok {
		t.Fatalf("expected WhileStmt, got %T", cStmts.Statements[0])
	}
	ck, ok := cWhile.Body[0].(*verb.ContinueStmt)
	if !ok {
		t.Fatalf("expected ContinueStmt, got %T", cWhile.Body[0])
	}

	if ck.Label != "myloop" {
		t.Fatalf("continue label: got %q, want %q", ck.Label, "myloop")
	}

	// Break should also populate Label, not Value.
	if bk.Label != "myloop" {
		t.Fatalf("BUG: BreakStmt.Label=%q (empty), but ContinueStmt.Label=%q — break puts the loop name in Value instead of Label", bk.Label, ck.Label)
	}
}
