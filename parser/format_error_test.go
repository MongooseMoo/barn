package parser

import (
	"errors"
	"math"
	"testing"

	"github.com/MongooseMoo/barn/ir"
)

func TestRecursiveFormatterFailuresDiscardWholeOutput(t *testing.T) {
	pos := ir.Position{Line: 8, Column: 3, Offset: 21}
	bad := &ir.LiteralExpr{Pos: pos, Kind: ir.LiteralErr, ErrorName: "E_UNKNOWN"}
	good := &ir.LiteralExpr{Kind: ir.LiteralInt}
	cases := map[string]ir.Stmt{
		"identifier injection": &ir.ReturnStmt{Value: &ir.IdentifierExpr{Pos: pos, Name: "a; return 2"}},
		"builtin keyword":      &ir.ReturnStmt{Value: &ir.BuiltinCallExpr{Pos: pos, Name: "if"}},
		"property name":        &ir.ReturnStmt{Value: &ir.PropertyExpr{Pos: pos, Expr: good, Property: "not a name"}},
		"verb name":            &ir.ReturnStmt{Value: &ir.VerbCallExpr{Pos: pos, Expr: good, Verb: "x)"}},
		"assignment name":      &ir.ExprStmt{Expr: &ir.AssignExpr{Target: &ir.VariableTarget{Pos: pos, Name: "a = b"}, Value: good}},
		"binding name":         &ir.ExprStmt{Expr: &ir.AssignExpr{Target: &ir.DestructuringTarget{Bindings: []ir.Binding{&ir.RequiredBinding{Pos: pos, Name: "a,b"}}}, Value: good}},
		"loop label":           &ir.WhileStmt{Pos: pos, Label: "if", Condition: good},
		"loop binding":         &ir.CollectionLoopStmt{Pos: pos, Value: "if", Collection: good},
		"handler variable":     &ir.TryStmt{Handlers: []ir.ExceptionHandler{{Pos: pos, Variable: "if", IsAny: true}}},
		"fork variable":        &ir.ForkStmt{Pos: pos, VarName: "if", Delay: good},
		"literal":              &ir.ReturnStmt{Value: bad},
		"binary":               &ir.ReturnStmt{Value: &ir.BinaryExpr{Left: good, Right: bad}},
		"argument":             &ir.ReturnStmt{Value: &ir.BuiltinCallExpr{Name: "f", Args: []ir.Expr{good, bad}}},
		"target default":       &ir.ExprStmt{Expr: &ir.AssignExpr{Target: &ir.DestructuringTarget{Bindings: []ir.Binding{&ir.OptionalBinding{Name: "a", Default: bad}}}, Value: good}},
		"nested body":          &ir.IfStmt{Condition: good, Body: []ir.Stmt{&ir.ReturnStmt{Value: bad}}},
		"map value":            &ir.ReturnStmt{Value: &ir.MapExpr{Pairs: []ir.MapPair{{Key: good, Value: bad}}}},
		"catch code":           &ir.ReturnStmt{Value: &ir.CatchExpr{Pos: pos, Expr: good, Codes: []string{"E_UNKNOWN"}}},
		"handler code":         &ir.TryStmt{Handlers: []ir.ExceptionHandler{{Pos: pos, Codes: []string{"E_UNKNOWN"}}}},
		"nonfinite float":      &ir.ReturnStmt{Value: &ir.LiteralExpr{Pos: pos, Kind: ir.LiteralFloat, FloatValue: math.Inf(1)}},
		"NaN":                  &ir.ReturnStmt{Value: &ir.LiteralExpr{Pos: pos, Kind: ir.LiteralFloat, FloatValue: math.NaN()}},
	}
	for name, stmt := range cases {
		t.Run(name, func(t *testing.T) {
			program := &ir.Program{Statements: []ir.Stmt{&ir.ReturnStmt{Value: good}, stmt}}
			for _, checked := range []func(*ir.Program) ([]string, error){FormatMOOChecked, FormatMOOFullyParenthesizedChecked} {
				lines, err := checked(program)
				var failure *FormattingError
				if lines != nil || !errors.As(err, &failure) || failure.Position != pos {
					t.Fatalf("source %q, error %#v (%v)", lines, failure, err)
				}
			}
			if FormatMOO(program) != nil || FormatMOOFullyParenthesized(program) != nil {
				t.Fatal("convenience formatter returned source")
			}
		})
	}
}

func TestEmptyStatementHasExplicitSemanticForm(t *testing.T) {
	program, err := NewParser("; return;").ParseProgram()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := program.Statements[0].(*ir.EmptyStmt); !ok {
		t.Fatalf("empty statement = %T", program.Statements[0])
	}
	for _, checked := range []func(*ir.Program) ([]string, error){FormatMOOChecked, FormatMOOFullyParenthesizedChecked} {
		lines, err := checked(program)
		if err != nil || len(lines) != 2 || lines[0] != ";" || lines[1] != "return;" {
			t.Fatalf("format: %q, %v", lines, err)
		}
	}
}
