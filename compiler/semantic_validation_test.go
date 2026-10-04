package compiler

import (
	"errors"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/verb"
)

type unknownExpression struct{ verb.Expr }
type unknownStatement struct{ verb.Stmt }
type unknownTarget struct{ verb.CollectionTarget }
type unknownBinding struct{ verb.Binding }

type nonSemanticCycle struct{ Next *nonSemanticCycle }
type cyclicUnknownExpression struct {
	verb.Expr
	Extra *nonSemanticCycle
}

func TestUnknownSemanticFamilyDoesNotTraverseForeignCycles(t *testing.T) {
	cycle := &nonSemanticCycle{}
	cycle.Next = cycle
	finished := make(chan error, 1)
	go func() { finished <- verb.ValidateNode(&cyclicUnknownExpression{Extra: cycle}) }()
	select {
	case err := <-finished:
		assertSemanticValidationError(t, err)
	case <-time.After(time.Second):
		t.Fatal("validation followed a foreign non-semantic cycle")
	}
}

func assertSemanticValidationError(t *testing.T, err error) {
	t.Helper()
	var shape *verb.ValidationError
	if !errors.Is(err, verb.ErrInvalidProgram) || !errors.As(err, &shape) {
		t.Errorf("error %v is not a typed semantic validation error", err)
	}
}

func TestSemanticValidationDiagnosticPreservesPosition(t *testing.T) {
	pos := verb.Position{Line: 3, Column: 4, Offset: 17}
	err := verb.ValidateNode(&verb.UnaryExpr{Pos: pos})
	diagnostic := compileDiagnostic(err)
	if diagnostic.Position != pos || diagnostic.Stage != BytecodeStage || !errors.Is(diagnostic.Detail, verb.ErrInvalidProgram) {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestMalformedSemanticProgramsFailClosed(t *testing.T) {
	lit := func() verb.Expr { return &verb.LiteralExpr{Kind: verb.LiteralInt, IntValue: 1} }
	expr := func(e verb.Expr) *verb.Program {
		return &verb.Program{Statements: []verb.Stmt{&verb.ExprStmt{Expr: e}}}
	}
	stmt := func(s verb.Stmt) *verb.Program { return &verb.Program{Statements: []verb.Stmt{s}} }
	target := func(v verb.Target) *verb.Program { return expr(&verb.AssignExpr{Target: v, Value: lit()}) }
	var typedNil *verb.LiteralExpr
	cases := map[string]*verb.Program{
		"literal high kind":            expr(&verb.LiteralExpr{Kind: 99}),
		"unary negative operator":      expr(&verb.UnaryExpr{Operator: -1, Operand: lit()}),
		"binary negative operator":     expr(&verb.BinaryExpr{Left: lit(), Right: lit(), Operator: -1}),
		"boundary negative enum":       expr(&verb.IndexBoundaryExpr{Boundary: -1}),
		"typed nil binding":            target(&verb.DestructuringTarget{Bindings: []verb.Binding{(*verb.RequiredBinding)(nil)}}),
		"typed nil collection target":  target(&verb.IndexTarget{Collection: (*verb.VariableTarget)(nil), Index: lit()}),
		"unknown expression":           expr(&unknownExpression{}),
		"unknown statement":            stmt(&unknownStatement{}),
		"unknown target":               target(&unknownTarget{}),
		"unknown binding":              target(&verb.DestructuringTarget{Bindings: []verb.Binding{&unknownBinding{}}}),
		"nil program":                  nil,
		"nil statement":                stmt(nil),
		"typed nil statement":          stmt((*verb.ReturnStmt)(nil)),
		"empty expression sentinel":    stmt(&verb.ExprStmt{}),
		"typed nil expression":         expr(typedNil),
		"literal kind":                 expr(&verb.LiteralExpr{Kind: -1}),
		"literal inactive integer":     expr(&verb.LiteralExpr{Kind: verb.LiteralString, IntValue: 1}),
		"literal inactive float":       expr(&verb.LiteralExpr{Kind: verb.LiteralInt, FloatValue: 1}),
		"literal inactive string":      expr(&verb.LiteralExpr{Kind: verb.LiteralInt, StringValue: "x"}),
		"literal inactive bool":        expr(&verb.LiteralExpr{Kind: verb.LiteralInt, BoolValue: true}),
		"literal inactive object":      expr(&verb.LiteralExpr{Kind: verb.LiteralInt, ObjID: 1}),
		"literal inactive error":       expr(&verb.LiteralExpr{Kind: verb.LiteralInt, ErrorName: "E_TYPE"}),
		"literal missing error":        expr(&verb.LiteralExpr{Kind: verb.LiteralErr}),
		"identifier name":              expr(&verb.IdentifierExpr{}),
		"unary operator":               expr(&verb.UnaryExpr{Operator: 99, Operand: lit()}),
		"unary operand":                expr(&verb.UnaryExpr{}),
		"binary operator":              expr(&verb.BinaryExpr{Left: lit(), Right: lit(), Operator: 99}),
		"binary left":                  expr(&verb.BinaryExpr{Right: lit()}),
		"binary right":                 expr(&verb.BinaryExpr{Left: lit()}),
		"ternary condition":            expr(&verb.TernaryExpr{ThenExpr: lit(), ElseExpr: lit()}),
		"ternary then":                 expr(&verb.TernaryExpr{Condition: lit(), ElseExpr: lit()}),
		"ternary else":                 expr(&verb.TernaryExpr{Condition: lit(), ThenExpr: lit()}),
		"boundary enum":                expr(&verb.IndexBoundaryExpr{Boundary: 99}),
		"index base":                   expr(&verb.IndexExpr{Index: lit()}),
		"index value":                  expr(&verb.IndexExpr{Expr: lit()}),
		"range base":                   expr(&verb.RangeExpr{Start: lit(), End: lit()}),
		"range start":                  expr(&verb.RangeExpr{Expr: lit(), End: lit()}),
		"range end":                    expr(&verb.RangeExpr{Expr: lit(), Start: lit()}),
		"property base":                expr(&verb.PropertyExpr{Property: "p"}),
		"property neither name":        expr(&verb.PropertyExpr{Expr: lit()}),
		"property both names":          expr(&verb.PropertyExpr{Expr: lit(), Property: "p", PropertyExpr: lit()}),
		"verb base":                    expr(&verb.VerbCallExpr{Verb: "v"}),
		"verb neither name":            expr(&verb.VerbCallExpr{Expr: lit()}),
		"verb both names":              expr(&verb.VerbCallExpr{Expr: lit(), Verb: "v", VerbExpr: lit()}),
		"verb nil argument":            expr(&verb.VerbCallExpr{Expr: lit(), Verb: "v", Args: []verb.Expr{nil}}),
		"builtin name":                 expr(&verb.BuiltinCallExpr{}),
		"builtin nil argument":         expr(&verb.BuiltinCallExpr{Name: "f", Args: []verb.Expr{nil}}),
		"splice value":                 expr(&verb.SpliceExpr{}),
		"catch expression":             expr(&verb.CatchExpr{IsAny: true}),
		"catch no selector":            expr(&verb.CatchExpr{Expr: lit()}),
		"catch both selectors":         expr(&verb.CatchExpr{Expr: lit(), IsAny: true, Codes: []string{"E_TYPE"}}),
		"catch empty code":             expr(&verb.CatchExpr{Expr: lit(), Codes: []string{""}}),
		"catch typed nil default":      expr(&verb.CatchExpr{Expr: lit(), IsAny: true, Default: typedNil}),
		"assign target":                expr(&verb.AssignExpr{Value: lit()}),
		"assign value":                 expr(&verb.AssignExpr{Target: &verb.VariableTarget{Name: "a"}}),
		"list element":                 expr(&verb.ListExpr{Elements: []verb.Expr{nil}}),
		"list range start":             expr(&verb.ListRangeExpr{End: lit()}),
		"list range end":               expr(&verb.ListRangeExpr{Start: lit()}),
		"map key":                      expr(&verb.MapExpr{Pairs: []verb.MapPair{{Value: lit()}}}),
		"map value":                    expr(&verb.MapExpr{Pairs: []verb.MapPair{{Key: lit()}}}),
		"variable target name":         target(&verb.VariableTarget{}),
		"property target object":       target(&verb.PropertyTarget{Name: "p"}),
		"property target neither name": target(&verb.PropertyTarget{Object: lit()}),
		"property target both names":   target(&verb.PropertyTarget{Object: lit(), Name: "p", NameExpr: lit()}),
		"index target collection":      target(&verb.IndexTarget{Index: lit()}),
		"index target index":           target(&verb.IndexTarget{Collection: &verb.VariableTarget{Name: "a"}}),
		"range target collection":      target(&verb.RangeTarget{Start: lit(), End: lit()}),
		"range target start":           target(&verb.RangeTarget{Collection: &verb.VariableTarget{Name: "a"}, End: lit()}),
		"range target end":             target(&verb.RangeTarget{Collection: &verb.VariableTarget{Name: "a"}, Start: lit()}),
		"nil binding":                  target(&verb.DestructuringTarget{Bindings: []verb.Binding{nil}}),
		"required binding name":        target(&verb.DestructuringTarget{Bindings: []verb.Binding{&verb.RequiredBinding{}}}),
		"optional binding name":        target(&verb.DestructuringTarget{Bindings: []verb.Binding{&verb.OptionalBinding{}}}),
		"optional typed nil default":   target(&verb.DestructuringTarget{Bindings: []verb.Binding{&verb.OptionalBinding{Name: "a", Default: typedNil}}}),
		"rest binding name":            target(&verb.DestructuringTarget{Bindings: []verb.Binding{&verb.RestBinding{}}}),
		"multiple rest bindings":       target(&verb.DestructuringTarget{Bindings: []verb.Binding{&verb.RestBinding{Name: "a"}, &verb.RestBinding{Name: "b"}}}),
		"if condition":                 stmt(&verb.IfStmt{}),
		"if body":                      stmt(&verb.IfStmt{Condition: lit(), Body: []verb.Stmt{nil}}),
		"if else":                      stmt(&verb.IfStmt{Condition: lit(), Else: []verb.Stmt{nil}}),
		"while condition":              stmt(&verb.WhileStmt{}),
		"while body":                   stmt(&verb.WhileStmt{Condition: lit(), Body: []verb.Stmt{nil}}),
		"collection binding":           stmt(&verb.CollectionLoopStmt{Collection: lit()}),
		"collection value":             stmt(&verb.CollectionLoopStmt{Value: "a"}),
		"collection body":              stmt(&verb.CollectionLoopStmt{Value: "a", Collection: lit(), Body: []verb.Stmt{nil}}),
		"range binding":                stmt(&verb.RangeLoopStmt{Start: lit(), End: lit()}),
		"range loop start":             stmt(&verb.RangeLoopStmt{Value: "a", End: lit()}),
		"range loop end":               stmt(&verb.RangeLoopStmt{Value: "a", Start: lit()}),
		"range loop body":              stmt(&verb.RangeLoopStmt{Value: "a", Start: lit(), End: lit(), Body: []verb.Stmt{nil}}),
		"return typed nil":             stmt(&verb.ReturnStmt{Value: typedNil}),
		"try no clause":                stmt(&verb.TryStmt{}),
		"try body":                     stmt(&verb.TryStmt{Body: []verb.Stmt{nil}, Finalizer: &verb.Finalizer{}}),
		"handler no selector":          stmt(&verb.TryStmt{Handlers: []verb.ExceptionHandler{{}}}),
		"handler both selectors":       stmt(&verb.TryStmt{Handlers: []verb.ExceptionHandler{{IsAny: true, Codes: []string{"E_TYPE"}}}}),
		"handler empty code":           stmt(&verb.TryStmt{Handlers: []verb.ExceptionHandler{{Codes: []string{""}}}}),
		"handler body":                 stmt(&verb.TryStmt{Handlers: []verb.ExceptionHandler{{IsAny: true, Body: []verb.Stmt{nil}}}}),
		"finalizer body":               stmt(&verb.TryStmt{Finalizer: &verb.Finalizer{Body: []verb.Stmt{nil}}}),
		"fork delay":                   stmt(&verb.ForkStmt{}),
		"fork body":                    stmt(&verb.ForkStmt{Delay: lit(), Body: []verb.Stmt{nil}}),
	}
	for name, program := range cases {
		if program != nil {
			program.Statements = append([]verb.Stmt{&verb.EmptyStmt{}}, program.Statements...)
		}
		t.Run(name, func(t *testing.T) {
			for consumer, check := range map[string]func(){
				"validator": func() { assertSemanticValidationError(t, verb.Validate(program)) },
				"compiler": func() {
					compiled, err := newLowerer(nil).compileProgram(program)
					assertSemanticValidationError(t, err)
					if err == nil || compiled != nil {
						t.Errorf("got program %v, error %v", compiled, err)
					}
				},
				"formatter": func() {
					lines, err := parser.FormatMOOChecked(program)
					assertSemanticValidationError(t, err)
					if err == nil || lines != nil {
						t.Errorf("got source %q, error %v", lines, err)
					}
				},
				"fully parenthesized formatter": func() {
					lines, err := parser.FormatMOOFullyParenthesizedChecked(program)
					assertSemanticValidationError(t, err)
					if lines != nil {
						t.Errorf("got source %q", lines)
					}
				},
				"convenience formatters": func() {
					if parser.FormatMOO(program) != nil || parser.FormatMOOFullyParenthesized(program) != nil {
						t.Error("invalid program produced source")
					}
				},
			} {
				t.Run(consumer, func(t *testing.T) {
					defer func() {
						if recovered := recover(); recovered != nil {
							t.Errorf("panic: %v", recovered)
						}
					}()
					check()
				})
			}
		})
	}
}
