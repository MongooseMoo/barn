package parser

import (
	"errors"
	"math"
	"testing"

	"github.com/MongooseMoo/barn/verb"
)

func TestRecursiveFormatterFailuresDiscardWholeOutput(t *testing.T) {
	pos := verb.Position{Line: 8, Column: 3, Offset: 21}
	bad := &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralErr, ErrorName: "E_UNKNOWN"}
	good := &verb.LiteralExpr{Kind: verb.LiteralInt}
	cases := map[string]verb.Stmt{
		"identifier injection": &verb.ReturnStmt{Value: &verb.IdentifierExpr{Pos: pos, Name: "a; return 2"}},
		"builtin keyword":      &verb.ReturnStmt{Value: &verb.BuiltinCallExpr{Pos: pos, Name: "if"}},
		"property name":        &verb.ReturnStmt{Value: &verb.PropertyExpr{Pos: pos, Expr: good, Property: "not a name"}},
		"verb name":            &verb.ReturnStmt{Value: &verb.VerbCallExpr{Pos: pos, Expr: good, Verb: "x)"}},
		"assignment name":      &verb.ExprStmt{Expr: &verb.AssignExpr{Target: &verb.VariableTarget{Pos: pos, Name: "a = b"}, Value: good}},
		"binding name":         &verb.ExprStmt{Expr: &verb.AssignExpr{Target: &verb.DestructuringTarget{Bindings: []verb.Binding{&verb.RequiredBinding{Pos: pos, Name: "a,b"}}}, Value: good}},
		"loop label":           &verb.WhileStmt{Pos: pos, Label: "if", Condition: good},
		"loop binding":         &verb.CollectionLoopStmt{Pos: pos, Value: "if", Collection: good},
		"handler variable":     &verb.TryStmt{Handlers: []verb.ExceptionHandler{{Pos: pos, Variable: "if", IsAny: true}}},
		"fork variable":        &verb.ForkStmt{Pos: pos, VarName: "if", Delay: good},
		"literal":              &verb.ReturnStmt{Value: bad},
		"binary":               &verb.ReturnStmt{Value: &verb.BinaryExpr{Left: good, Right: bad}},
		"argument":             &verb.ReturnStmt{Value: &verb.BuiltinCallExpr{Name: "f", Args: []verb.Expr{good, bad}}},
		"target default":       &verb.ExprStmt{Expr: &verb.AssignExpr{Target: &verb.DestructuringTarget{Bindings: []verb.Binding{&verb.OptionalBinding{Name: "a", Default: bad}}}, Value: good}},
		"nested body":          &verb.IfStmt{Condition: good, Body: []verb.Stmt{&verb.ReturnStmt{Value: bad}}},
		"map value":            &verb.ReturnStmt{Value: &verb.MapExpr{Pairs: []verb.MapPair{{Key: good, Value: bad}}}},
		"catch code":           &verb.ReturnStmt{Value: &verb.CatchExpr{Pos: pos, Expr: good, Codes: []string{"E_UNKNOWN"}}},
		"handler code":         &verb.TryStmt{Handlers: []verb.ExceptionHandler{{Pos: pos, Codes: []string{"E_UNKNOWN"}}}},
		"nonfinite float":      &verb.ReturnStmt{Value: &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralFloat, FloatValue: math.Inf(1)}},
		"NaN":                  &verb.ReturnStmt{Value: &verb.LiteralExpr{Pos: pos, Kind: verb.LiteralFloat, FloatValue: math.NaN()}},
	}
	for name, stmt := range cases {
		t.Run(name, func(t *testing.T) {
			program := &verb.Program{Statements: []verb.Stmt{&verb.ReturnStmt{Value: good}, stmt}}
			for _, checked := range []func(*verb.Program) ([]string, error){FormatMOOChecked, FormatMOOFullyParenthesizedChecked} {
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
	if _, ok := program.Statements[0].(*verb.EmptyStmt); !ok {
		t.Fatalf("empty statement = %T", program.Statements[0])
	}
	for _, checked := range []func(*verb.Program) ([]string, error){FormatMOOChecked, FormatMOOFullyParenthesizedChecked} {
		lines, err := checked(program)
		if err != nil || len(lines) != 2 || lines[0] != ";" || lines[1] != "return;" {
			t.Fatalf("format: %q, %v", lines, err)
		}
	}
}
