package verb

import (
	"errors"
	"testing"
)

func TestValidateEverySemanticFamily(t *testing.T) {
	lit := &LiteralExpr{Kind: LiteralInt}
	variable := &VariableTarget{Name: "a"}
	nodes := []Node{
		lit, &LiteralExpr{Kind: LiteralFloat}, &LiteralExpr{Kind: LiteralString},
		&LiteralExpr{Kind: LiteralBool}, &LiteralExpr{Kind: LiteralObj}, &LiteralExpr{Kind: LiteralErr, ErrorName: "E_TYPE"},
		&IdentifierExpr{Name: "a"}, &UnaryExpr{Operand: lit}, &BinaryExpr{Left: lit, Right: lit},
		&TernaryExpr{Condition: lit, ThenExpr: lit, ElseExpr: lit}, &IndexBoundaryExpr{Boundary: IndexFirst},
		&IndexBoundaryExpr{Boundary: IndexLast}, &IndexExpr{Expr: lit, Index: lit},
		&RangeExpr{Expr: lit, Start: lit, End: lit}, &PropertyExpr{Expr: lit, Property: "p"},
		&PropertyExpr{Expr: lit, PropertyExpr: lit}, &VerbCallExpr{Expr: lit, Verb: "v", Args: []Expr{lit}},
		&VerbCallExpr{Expr: lit, VerbExpr: lit}, &BuiltinCallExpr{Name: "f", Args: []Expr{lit}},
		&SpliceExpr{Expr: lit}, &CatchExpr{Expr: lit, IsAny: true}, &CatchExpr{Expr: lit, Codes: []string{"E_TYPE"}, Default: lit},
		&AssignExpr{Target: variable, Value: lit}, &ListExpr{}, &ListExpr{Elements: []Expr{lit}},
		&ListRangeExpr{Start: lit, End: lit}, &MapExpr{}, &MapExpr{Pairs: []MapPair{{Key: lit, Value: lit}}},
		variable, &PropertyTarget{Object: lit, Name: "p"}, &PropertyTarget{Object: lit, NameExpr: lit},
		&IndexTarget{Collection: variable, Index: lit}, &RangeTarget{Collection: variable, Start: lit, End: lit},
		&DestructuringTarget{}, &DestructuringTarget{Bindings: []Binding{&RequiredBinding{Name: "a"}, &OptionalBinding{Name: "b"}, &RestBinding{Name: "c"}}},
		&RequiredBinding{Name: "a"}, &OptionalBinding{Name: "a"}, &OptionalBinding{Name: "a", Default: lit}, &RestBinding{Name: "a"},
		&EmptyStmt{}, &ExprStmt{Expr: lit}, &IfStmt{Condition: lit}, &WhileStmt{Condition: lit},
		&CollectionLoopStmt{Value: "a", Collection: lit}, &RangeLoopStmt{Value: "a", Start: lit, End: lit},
		&BreakStmt{}, &ContinueStmt{}, &ReturnStmt{}, &ReturnStmt{Value: lit},
		&TryStmt{Handlers: []ExceptionHandler{{IsAny: true}}}, &TryStmt{Finalizer: &Finalizer{}},
		&TryStmt{Handlers: []ExceptionHandler{{Codes: []string{"E_TYPE"}}}, Finalizer: &Finalizer{}}, &ForkStmt{Delay: lit},
	}
	for _, node := range nodes {
		if err := ValidateNode(node); err != nil {
			t.Errorf("%T: %v", node, err)
		}
	}
	for op := BinaryAdd; op <= BinaryShiftRight; op++ {
		if err := ValidateNode(&BinaryExpr{Left: lit, Right: lit, Operator: op}); err != nil {
			t.Error(err)
		}
	}
	for op := UnaryNegate; op <= UnaryBitwiseNot; op++ {
		if err := ValidateNode(&UnaryExpr{Operand: lit, Operator: op}); err != nil {
			t.Error(err)
		}
	}
}

func TestValidateBoundsCyclesAndPreservesPosition(t *testing.T) {
	cycle := &UnaryExpr{}
	cycle.Operand = cycle
	if err := ValidateNode(cycle); !errors.Is(err, ErrMaxNestingDepth) {
		t.Fatalf("cycle: %v", err)
	}
	program := &Program{Statements: make([]Stmt, 4096)}
	for i := range program.Statements {
		program.Statements[i] = &EmptyStmt{}
	}
	if err := Validate(program); err != nil {
		t.Fatal(err)
	}
	pos := Position{Line: 7, Column: 9, Offset: 30}
	program.Statements[4095] = &ReturnStmt{Value: &UnaryExpr{Pos: pos}}
	var shape *ValidationError
	if err := Validate(program); !errors.As(err, &shape) || shape.Position != pos || shape.Field != "Operand" {
		t.Fatalf("position error: %#v (%v)", shape, err)
	}
	for _, node := range []Node{nil, (*ReturnStmt)(nil)} {
		if err := ValidateNode(node); !errors.Is(err, ErrInvalidProgram) {
			t.Fatalf("nil node: %v", err)
		}
	}
}
