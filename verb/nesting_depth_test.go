package verb_test

import (
	"errors"
	"testing"

	"barn/verb"
)

const compileTimeMaxNestingDepth = verb.MaxNestingDepth

func deepExpr(depth int) verb.Expr {
	var expr verb.Expr = &verb.LiteralExpr{Pos: verb.Position{Line: 13}, Kind: verb.LiteralInt, IntValue: 1}
	for i := 1; i < depth; i++ {
		expr = &verb.UnaryExpr{Pos: verb.Position{Line: 13}, Operator: verb.UnaryNegate, Operand: expr}
	}
	return expr
}

func literal() verb.Expr {
	return &verb.LiteralExpr{Kind: verb.LiteralInt, IntValue: 1}
}

func emptyStmt() verb.Stmt {
	return &verb.ExprStmt{}
}

func TestMaxNestingDepthIsSharedImmutableConstant(t *testing.T) {
	if compileTimeMaxNestingDepth != 256 {
		t.Fatalf("MaxNestingDepth = %d, want 256", compileTimeMaxNestingDepth)
	}
}

func TestValidateNodeNestingDepthTraversesSemanticIR(t *testing.T) {
	deep := deepExpr(256)
	variable := &verb.VariableTarget{Name: "value"}
	tests := []struct {
		name string
		node verb.Node
	}{
		{name: "unary operand", node: &verb.UnaryExpr{Operand: deep}},
		{name: "binary left", node: &verb.BinaryExpr{Left: deep, Right: literal()}},
		{name: "binary right", node: &verb.BinaryExpr{Left: literal(), Right: deep}},
		{name: "ternary condition", node: &verb.TernaryExpr{Condition: deep, ThenExpr: literal(), ElseExpr: literal()}},
		{name: "ternary then", node: &verb.TernaryExpr{Condition: literal(), ThenExpr: deep, ElseExpr: literal()}},
		{name: "ternary else", node: &verb.TernaryExpr{Condition: literal(), ThenExpr: literal(), ElseExpr: deep}},
		{name: "index base", node: &verb.IndexExpr{Expr: deep, Index: literal()}},
		{name: "index value", node: &verb.IndexExpr{Expr: literal(), Index: deep}},
		{name: "range base", node: &verb.RangeExpr{Expr: deep, Start: literal(), End: literal()}},
		{name: "range start", node: &verb.RangeExpr{Expr: literal(), Start: deep, End: literal()}},
		{name: "range end", node: &verb.RangeExpr{Expr: literal(), Start: literal(), End: deep}},
		{name: "property base", node: &verb.PropertyExpr{Expr: deep, Property: "name"}},
		{name: "property name", node: &verb.PropertyExpr{Expr: literal(), PropertyExpr: deep}},
		{name: "verb receiver", node: &verb.VerbCallExpr{Expr: deep, Verb: "call"}},
		{name: "verb name", node: &verb.VerbCallExpr{Expr: literal(), VerbExpr: deep}},
		{name: "verb argument", node: &verb.VerbCallExpr{Expr: literal(), Verb: "call", Args: []verb.Expr{deep}}},
		{name: "builtin argument", node: &verb.BuiltinCallExpr{Name: "call", Args: []verb.Expr{deep}}},
		{name: "splice", node: &verb.SpliceExpr{Expr: deep}},
		{name: "catch expression", node: &verb.CatchExpr{Expr: deep}},
		{name: "catch default", node: &verb.CatchExpr{Expr: literal(), Default: deep}},
		{name: "assign target", node: &verb.AssignExpr{Target: &verb.PropertyTarget{Object: deep, Name: "name"}, Value: literal()}},
		{name: "assign value", node: &verb.AssignExpr{Target: variable, Value: deep}},
		{name: "list element", node: &verb.ListExpr{Elements: []verb.Expr{deep}}},
		{name: "list range start", node: &verb.ListRangeExpr{Start: deep, End: literal()}},
		{name: "list range end", node: &verb.ListRangeExpr{Start: literal(), End: deep}},
		{name: "map key", node: &verb.MapExpr{Pairs: []verb.MapPair{{Key: deep, Value: literal()}}}},
		{name: "map value", node: &verb.MapExpr{Pairs: []verb.MapPair{{Key: literal(), Value: deep}}}},
		{name: "property target object", node: &verb.PropertyTarget{Object: deep, Name: "name"}},
		{name: "property target name", node: &verb.PropertyTarget{Object: literal(), NameExpr: deep}},
		{name: "index target index", node: &verb.IndexTarget{Collection: variable, Index: deep}},
		{name: "range target start", node: &verb.RangeTarget{Collection: variable, Start: deep, End: literal()}},
		{name: "range target end", node: &verb.RangeTarget{Collection: variable, Start: literal(), End: deep}},
		{name: "optional binding default", node: &verb.OptionalBinding{Name: "value", Default: deep}},
		{name: "destructuring binding", node: &verb.DestructuringTarget{Bindings: []verb.Binding{&verb.OptionalBinding{Name: "value", Default: deep}}}},
		{name: "expression statement", node: &verb.ExprStmt{Expr: deep}},
		{name: "return", node: &verb.ReturnStmt{Value: deep}},
		{name: "if condition", node: &verb.IfStmt{Condition: deep}},
		{name: "if body", node: &verb.IfStmt{Condition: literal(), Body: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}},
		{name: "if else", node: &verb.IfStmt{Condition: literal(), Else: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}},
		{name: "while condition", node: &verb.WhileStmt{Condition: deep}},
		{name: "while body", node: &verb.WhileStmt{Condition: literal(), Body: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}},
		{name: "collection loop collection", node: &verb.CollectionLoopStmt{Collection: deep}},
		{name: "collection loop body", node: &verb.CollectionLoopStmt{Collection: literal(), Body: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}},
		{name: "range loop start", node: &verb.RangeLoopStmt{Start: deep, End: literal()}},
		{name: "range loop end", node: &verb.RangeLoopStmt{Start: literal(), End: deep}},
		{name: "range loop body", node: &verb.RangeLoopStmt{Start: literal(), End: literal(), Body: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}},
		{name: "try body", node: &verb.TryStmt{Body: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}},
		{name: "handler body", node: &verb.TryStmt{Handlers: []verb.ExceptionHandler{{Body: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}}}},
		{name: "finalizer body", node: &verb.TryStmt{Finalizer: &verb.Finalizer{Body: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}}},
		{name: "fork delay", node: &verb.ForkStmt{Delay: deep}},
		{name: "fork body", node: &verb.ForkStmt{Delay: literal(), Body: []verb.Stmt{&verb.ReturnStmt{Value: deep}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verb.ValidateNodeNestingDepth(test.node)
			var depthErr *verb.NestingDepthError
			if !errors.As(err, &depthErr) {
				t.Fatalf("ValidateNodeNestingDepth() error = %T %v, want *verb.NestingDepthError", err, err)
			}
		})
	}
}

func TestValidateProgramNestingDepthCountsDepthNotNodes(t *testing.T) {
	wide := &verb.Program{Statements: make([]verb.Stmt, 2560)}
	for i := range wide.Statements {
		wide.Statements[i] = emptyStmt()
	}
	if err := verb.ValidateProgramNestingDepth(wide); err != nil {
		t.Fatalf("ValidateProgramNestingDepth(2560 siblings) error = %v", err)
	}

	tooDeep := &verb.Program{Statements: []verb.Stmt{&verb.ReturnStmt{Value: deepExpr(256)}}}
	err := verb.ValidateProgramNestingDepth(tooDeep)
	var depthErr *verb.NestingDepthError
	if !errors.As(err, &depthErr) {
		t.Fatalf("ValidateProgramNestingDepth(depth 257) error = %T %v, want *verb.NestingDepthError", err, err)
	}
}
