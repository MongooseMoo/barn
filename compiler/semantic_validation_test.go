package compiler

import (
	"errors"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
)

type unknownExpression struct{ ir.Expr }
type unknownStatement struct{ ir.Stmt }
type unknownTarget struct{ ir.CollectionTarget }
type unknownBinding struct{ ir.Binding }

type nonSemanticCycle struct{ Next *nonSemanticCycle }
type cyclicUnknownExpression struct {
	ir.Expr
	Extra *nonSemanticCycle
}

func TestUnknownSemanticFamilyDoesNotTraverseForeignCycles(t *testing.T) {
	cycle := &nonSemanticCycle{}
	cycle.Next = cycle
	finished := make(chan error, 1)
	go func() { finished <- ir.ValidateNode(&cyclicUnknownExpression{Extra: cycle}) }()
	select {
	case err := <-finished:
		assertSemanticValidationError(t, err)
	case <-time.After(time.Second):
		t.Fatal("validation followed a foreign non-semantic cycle")
	}
}

func assertSemanticValidationError(t *testing.T, err error) {
	t.Helper()
	var shape *ir.ValidationError
	if !errors.Is(err, ir.ErrInvalidProgram) || !errors.As(err, &shape) {
		t.Errorf("error %v is not a typed semantic validation error", err)
	}
}

func TestSemanticValidationDiagnosticPreservesPosition(t *testing.T) {
	pos := ir.Position{Line: 3, Column: 4, Offset: 17}
	err := ir.ValidateNode(&ir.UnaryExpr{Pos: pos})
	diagnostic := compileDiagnostic(err)
	if diagnostic.Position != pos || diagnostic.Stage != BytecodeStage || !errors.Is(diagnostic.Detail, ir.ErrInvalidProgram) {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestMalformedSemanticProgramsFailClosed(t *testing.T) {
	lit := func() ir.Expr { return &ir.LiteralExpr{Kind: ir.LiteralInt, IntValue: 1} }
	expr := func(e ir.Expr) *ir.Program {
		return &ir.Program{Statements: []ir.Stmt{&ir.ExprStmt{Expr: e}}}
	}
	stmt := func(s ir.Stmt) *ir.Program { return &ir.Program{Statements: []ir.Stmt{s}} }
	target := func(v ir.Target) *ir.Program { return expr(&ir.AssignExpr{Target: v, Value: lit()}) }
	var typedNil *ir.LiteralExpr
	cases := map[string]*ir.Program{
		"literal high kind":            expr(&ir.LiteralExpr{Kind: 99}),
		"unary negative operator":      expr(&ir.UnaryExpr{Operator: -1, Operand: lit()}),
		"binary negative operator":     expr(&ir.BinaryExpr{Left: lit(), Right: lit(), Operator: -1}),
		"boundary negative enum":       expr(&ir.IndexBoundaryExpr{Boundary: -1}),
		"typed nil binding":            target(&ir.DestructuringTarget{Bindings: []ir.Binding{(*ir.RequiredBinding)(nil)}}),
		"typed nil collection target":  target(&ir.IndexTarget{Collection: (*ir.VariableTarget)(nil), Index: lit()}),
		"unknown expression":           expr(&unknownExpression{}),
		"unknown statement":            stmt(&unknownStatement{}),
		"unknown target":               target(&unknownTarget{}),
		"unknown binding":              target(&ir.DestructuringTarget{Bindings: []ir.Binding{&unknownBinding{}}}),
		"nil program":                  nil,
		"nil statement":                stmt(nil),
		"typed nil statement":          stmt((*ir.ReturnStmt)(nil)),
		"empty expression sentinel":    stmt(&ir.ExprStmt{}),
		"typed nil expression":         expr(typedNil),
		"literal kind":                 expr(&ir.LiteralExpr{Kind: -1}),
		"literal inactive integer":     expr(&ir.LiteralExpr{Kind: ir.LiteralString, IntValue: 1}),
		"literal inactive float":       expr(&ir.LiteralExpr{Kind: ir.LiteralInt, FloatValue: 1}),
		"literal inactive string":      expr(&ir.LiteralExpr{Kind: ir.LiteralInt, StringValue: "x"}),
		"literal inactive bool":        expr(&ir.LiteralExpr{Kind: ir.LiteralInt, BoolValue: true}),
		"literal inactive object":      expr(&ir.LiteralExpr{Kind: ir.LiteralInt, ObjID: 1}),
		"literal inactive error":       expr(&ir.LiteralExpr{Kind: ir.LiteralInt, ErrorName: "E_TYPE"}),
		"literal missing error":        expr(&ir.LiteralExpr{Kind: ir.LiteralErr}),
		"identifier name":              expr(&ir.IdentifierExpr{}),
		"unary operator":               expr(&ir.UnaryExpr{Operator: 99, Operand: lit()}),
		"unary operand":                expr(&ir.UnaryExpr{}),
		"binary operator":              expr(&ir.BinaryExpr{Left: lit(), Right: lit(), Operator: 99}),
		"binary left":                  expr(&ir.BinaryExpr{Right: lit()}),
		"binary right":                 expr(&ir.BinaryExpr{Left: lit()}),
		"ternary condition":            expr(&ir.TernaryExpr{ThenExpr: lit(), ElseExpr: lit()}),
		"ternary then":                 expr(&ir.TernaryExpr{Condition: lit(), ElseExpr: lit()}),
		"ternary else":                 expr(&ir.TernaryExpr{Condition: lit(), ThenExpr: lit()}),
		"boundary enum":                expr(&ir.IndexBoundaryExpr{Boundary: 99}),
		"index base":                   expr(&ir.IndexExpr{Index: lit()}),
		"index value":                  expr(&ir.IndexExpr{Expr: lit()}),
		"range base":                   expr(&ir.RangeExpr{Start: lit(), End: lit()}),
		"range start":                  expr(&ir.RangeExpr{Expr: lit(), End: lit()}),
		"range end":                    expr(&ir.RangeExpr{Expr: lit(), Start: lit()}),
		"property base":                expr(&ir.PropertyExpr{Property: "p"}),
		"property neither name":        expr(&ir.PropertyExpr{Expr: lit()}),
		"property both names":          expr(&ir.PropertyExpr{Expr: lit(), Property: "p", PropertyExpr: lit()}),
		"verb base":                    expr(&ir.VerbCallExpr{Verb: "v"}),
		"verb neither name":            expr(&ir.VerbCallExpr{Expr: lit()}),
		"verb both names":              expr(&ir.VerbCallExpr{Expr: lit(), Verb: "v", VerbExpr: lit()}),
		"verb nil argument":            expr(&ir.VerbCallExpr{Expr: lit(), Verb: "v", Args: []ir.Expr{nil}}),
		"builtin name":                 expr(&ir.BuiltinCallExpr{}),
		"builtin nil argument":         expr(&ir.BuiltinCallExpr{Name: "f", Args: []ir.Expr{nil}}),
		"splice value":                 expr(&ir.SpliceExpr{}),
		"catch expression":             expr(&ir.CatchExpr{IsAny: true}),
		"catch no selector":            expr(&ir.CatchExpr{Expr: lit()}),
		"catch both selectors":         expr(&ir.CatchExpr{Expr: lit(), IsAny: true, Codes: []string{"E_TYPE"}}),
		"catch empty code":             expr(&ir.CatchExpr{Expr: lit(), Codes: []string{""}}),
		"catch typed nil default":      expr(&ir.CatchExpr{Expr: lit(), IsAny: true, Default: typedNil}),
		"assign target":                expr(&ir.AssignExpr{Value: lit()}),
		"assign value":                 expr(&ir.AssignExpr{Target: &ir.VariableTarget{Name: "a"}}),
		"list element":                 expr(&ir.ListExpr{Elements: []ir.Expr{nil}}),
		"list range start":             expr(&ir.ListRangeExpr{End: lit()}),
		"list range end":               expr(&ir.ListRangeExpr{Start: lit()}),
		"map key":                      expr(&ir.MapExpr{Pairs: []ir.MapPair{{Value: lit()}}}),
		"map value":                    expr(&ir.MapExpr{Pairs: []ir.MapPair{{Key: lit()}}}),
		"variable target name":         target(&ir.VariableTarget{}),
		"property target object":       target(&ir.PropertyTarget{Name: "p"}),
		"property target neither name": target(&ir.PropertyTarget{Object: lit()}),
		"property target both names":   target(&ir.PropertyTarget{Object: lit(), Name: "p", NameExpr: lit()}),
		"index target collection":      target(&ir.IndexTarget{Index: lit()}),
		"index target index":           target(&ir.IndexTarget{Collection: &ir.VariableTarget{Name: "a"}}),
		"range target collection":      target(&ir.RangeTarget{Start: lit(), End: lit()}),
		"range target start":           target(&ir.RangeTarget{Collection: &ir.VariableTarget{Name: "a"}, End: lit()}),
		"range target end":             target(&ir.RangeTarget{Collection: &ir.VariableTarget{Name: "a"}, Start: lit()}),
		"nil binding":                  target(&ir.DestructuringTarget{Bindings: []ir.Binding{nil}}),
		"required binding name":        target(&ir.DestructuringTarget{Bindings: []ir.Binding{&ir.RequiredBinding{}}}),
		"optional binding name":        target(&ir.DestructuringTarget{Bindings: []ir.Binding{&ir.OptionalBinding{}}}),
		"optional typed nil default":   target(&ir.DestructuringTarget{Bindings: []ir.Binding{&ir.OptionalBinding{Name: "a", Default: typedNil}}}),
		"rest binding name":            target(&ir.DestructuringTarget{Bindings: []ir.Binding{&ir.RestBinding{}}}),
		"multiple rest bindings":       target(&ir.DestructuringTarget{Bindings: []ir.Binding{&ir.RestBinding{Name: "a"}, &ir.RestBinding{Name: "b"}}}),
		"if condition":                 stmt(&ir.IfStmt{}),
		"if body":                      stmt(&ir.IfStmt{Condition: lit(), Body: []ir.Stmt{nil}}),
		"if else":                      stmt(&ir.IfStmt{Condition: lit(), Else: []ir.Stmt{nil}}),
		"while condition":              stmt(&ir.WhileStmt{}),
		"while body":                   stmt(&ir.WhileStmt{Condition: lit(), Body: []ir.Stmt{nil}}),
		"collection binding":           stmt(&ir.CollectionLoopStmt{Collection: lit()}),
		"collection value":             stmt(&ir.CollectionLoopStmt{Value: "a"}),
		"collection body":              stmt(&ir.CollectionLoopStmt{Value: "a", Collection: lit(), Body: []ir.Stmt{nil}}),
		"range binding":                stmt(&ir.RangeLoopStmt{Start: lit(), End: lit()}),
		"range loop start":             stmt(&ir.RangeLoopStmt{Value: "a", End: lit()}),
		"range loop end":               stmt(&ir.RangeLoopStmt{Value: "a", Start: lit()}),
		"range loop body":              stmt(&ir.RangeLoopStmt{Value: "a", Start: lit(), End: lit(), Body: []ir.Stmt{nil}}),
		"return typed nil":             stmt(&ir.ReturnStmt{Value: typedNil}),
		"try no clause":                stmt(&ir.TryStmt{}),
		"try body":                     stmt(&ir.TryStmt{Body: []ir.Stmt{nil}, Finalizer: &ir.Finalizer{}}),
		"handler no selector":          stmt(&ir.TryStmt{Handlers: []ir.ExceptionHandler{{}}}),
		"handler both selectors":       stmt(&ir.TryStmt{Handlers: []ir.ExceptionHandler{{IsAny: true, Codes: []string{"E_TYPE"}}}}),
		"handler empty code":           stmt(&ir.TryStmt{Handlers: []ir.ExceptionHandler{{Codes: []string{""}}}}),
		"handler body":                 stmt(&ir.TryStmt{Handlers: []ir.ExceptionHandler{{IsAny: true, Body: []ir.Stmt{nil}}}}),
		"finalizer body":               stmt(&ir.TryStmt{Finalizer: &ir.Finalizer{Body: []ir.Stmt{nil}}}),
		"fork delay":                   stmt(&ir.ForkStmt{}),
		"fork body":                    stmt(&ir.ForkStmt{Delay: lit(), Body: []ir.Stmt{nil}}),
	}
	for name, program := range cases {
		if program != nil {
			program.Statements = append([]ir.Stmt{&ir.EmptyStmt{}}, program.Statements...)
		}
		t.Run(name, func(t *testing.T) {
			for consumer, check := range map[string]func(){
				"validator": func() { assertSemanticValidationError(t, ir.Validate(program)) },
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
