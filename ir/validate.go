package ir

import (
	"errors"
	"fmt"
	"reflect"
)

var ErrInvalidProgram = errors.New("invalid semantic program")

// ValidationError identifies the malformed semantic node and field. It carries
// no frontend token spelling or runtime value semantics.
type ValidationError struct {
	Position Position
	Field    string
	Message  string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }
func (e *ValidationError) Unwrap() error { return ErrInvalidProgram }

// Validate checks the complete shape and depth of a program before consumers
// traverse it. Nil interfaces and typed nil nodes are both invalid children.
// Optional children must be absent interfaces, rather than typed nil pointers.
func Validate(program *Program) error {
	if program == nil {
		return invalid(Position{}, "Program", "must not be nil")
	}
	for _, stmt := range program.Statements {
		if nilNode(stmt) {
			return invalid(Position{}, "Statements", "requires a non-nil statement")
		}
	}
	return validateGraph(reflect.ValueOf(program), -2, true)
}

func invalid(pos Position, field, message string) error {
	return &ValidationError{Position: pos, Field: field, Message: message}
}

func nilNode(node Node) bool {
	if node == nil {
		return true
	}
	v := reflect.ValueOf(node)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func validateNodeShape(node Node) error {
	// Reject embedded-interface impostors before invoking their Position method.
	switch node.(type) {
	case *LiteralExpr, *IdentifierExpr, *UnaryExpr, *BinaryExpr, *TernaryExpr,
		*IndexBoundaryExpr, *IndexExpr, *RangeExpr, *PropertyExpr, *VerbCallExpr,
		*BuiltinCallExpr, *SpliceExpr, *CatchExpr, *AssignExpr, *ListExpr,
		*ListRangeExpr, *MapExpr, *VariableTarget, *PropertyTarget, *IndexTarget,
		*RangeTarget, *DestructuringTarget, *RequiredBinding, *OptionalBinding,
		*RestBinding, *EmptyStmt, *ExprStmt, *IfStmt, *WhileStmt,
		*CollectionLoopStmt, *RangeLoopStmt, *BreakStmt, *ContinueStmt,
		*ReturnStmt, *TryStmt, *ForkStmt:
	default:
		return invalid(Position{}, "Node", fmt.Sprintf("unknown semantic node %T", node))
	}
	pos := node.Position()
	require := func(field string, children ...Node) error {
		for _, child := range children {
			if nilNode(child) {
				return invalid(pos, field, "requires a non-nil child")
			}
		}
		return nil
	}
	name := func(field, value string) error {
		if value == "" {
			return invalid(pos, field, "requires a name")
		}
		return nil
	}
	choice := func(field, static string, dynamic Expr) error {
		if (static != "") == (dynamic != nil) {
			return invalid(pos, field, "requires exactly one static or dynamic name")
		}
		return nil
	}
	stmts := func(field string, list []Stmt) error {
		for _, child := range list {
			if err := require(field, child); err != nil {
				return err
			}
		}
		return nil
	}
	exprs := func(field string, list []Expr) error {
		for _, child := range list {
			if err := require(field, child); err != nil {
				return err
			}
		}
		return nil
	}
	switch n := node.(type) {
	case *LiteralExpr:
		if n.Kind < LiteralInt || n.Kind > LiteralErr {
			return invalid(pos, "Kind", "unknown literal kind")
		}
		if n.Kind != LiteralInt && n.IntValue != 0 || n.Kind != LiteralFloat && n.FloatValue != 0 ||
			n.Kind != LiteralString && n.StringValue != "" || n.Kind != LiteralBool && n.BoolValue ||
			n.Kind != LiteralObj && n.ObjID != 0 || n.Kind != LiteralErr && n.ErrorName != "" {
			return invalid(pos, "LiteralExpr", "inactive literal payload must be zero")
		}
		if n.Kind == LiteralErr {
			return name("ErrorName", n.ErrorName)
		}
	case *IdentifierExpr:
		return name("Name", n.Name)
	case *UnaryExpr:
		if n.Operator < UnaryNegate || n.Operator > UnaryBitwiseNot {
			return invalid(pos, "Operator", "unknown unary operator")
		}
		return require("Operand", n.Operand)
	case *BinaryExpr:
		if n.Operator < BinaryAdd || n.Operator > BinaryShiftRight {
			return invalid(pos, "Operator", "unknown binary operator")
		}
		return require("Left/Right", n.Left, n.Right)
	case *TernaryExpr:
		return require("Condition/ThenExpr/ElseExpr", n.Condition, n.ThenExpr, n.ElseExpr)
	case *IndexBoundaryExpr:
		if n.Boundary != IndexFirst && n.Boundary != IndexLast {
			return invalid(pos, "Boundary", "unknown index boundary")
		}
	case *IndexExpr:
		return require("Expr/Index", n.Expr, n.Index)
	case *RangeExpr:
		return require("Expr/Start/End", n.Expr, n.Start, n.End)
	case *PropertyExpr:
		if err := require("Expr", n.Expr); err != nil {
			return err
		}
		return choice("Property/PropertyExpr", n.Property, n.PropertyExpr)
	case *VerbCallExpr:
		if err := require("Expr", n.Expr); err != nil {
			return err
		}
		if err := choice("Verb/VerbExpr", n.Verb, n.VerbExpr); err != nil {
			return err
		}
		return exprs("Args", n.Args)
	case *BuiltinCallExpr:
		if err := name("Name", n.Name); err != nil {
			return err
		}
		return exprs("Args", n.Args)
	case *SpliceExpr:
		return require("Expr", n.Expr)
	case *CatchExpr:
		if err := require("Expr", n.Expr); err != nil {
			return err
		}
		return validateSelector(pos, n.IsAny, n.Codes)
	case *AssignExpr:
		return require("Target/Value", n.Target, n.Value)
	case *ListExpr:
		return exprs("Elements", n.Elements)
	case *ListRangeExpr:
		return require("Start/End", n.Start, n.End)
	case *MapExpr:
		for _, pair := range n.Pairs {
			if err := require("Pairs.Key/Value", pair.Key, pair.Value); err != nil {
				return err
			}
		}
	case *VariableTarget:
		return name("Name", n.Name)
	case *PropertyTarget:
		if err := require("Object", n.Object); err != nil {
			return err
		}
		return choice("Name/NameExpr", n.Name, n.NameExpr)
	case *IndexTarget:
		return require("Collection/Index", n.Collection, n.Index)
	case *RangeTarget:
		return require("Collection/Start/End", n.Collection, n.Start, n.End)
	case *DestructuringTarget:
		rests := 0
		for _, binding := range n.Bindings {
			if err := require("Bindings", binding); err != nil {
				return err
			}
			if _, ok := binding.(*RestBinding); ok {
				rests++
			}
		}
		if rests > 1 {
			return invalid(pos, "Bindings", "at most one rest binding is allowed")
		}
	case *RequiredBinding:
		return name("Name", n.Name)
	case *OptionalBinding:
		return name("Name", n.Name)
	case *RestBinding:
		return name("Name", n.Name)
	case *EmptyStmt, *BreakStmt, *ContinueStmt, *ReturnStmt:
		// No mandatory children; optional return values are visited below.
	case *ExprStmt:
		return require("Expr", n.Expr)
	case *IfStmt:
		if err := require("Condition", n.Condition); err != nil {
			return err
		}
		if err := stmts("Body", n.Body); err != nil {
			return err
		}
		return stmts("Else", n.Else)
	case *WhileStmt:
		if err := require("Condition", n.Condition); err != nil {
			return err
		}
		return stmts("Body", n.Body)
	case *CollectionLoopStmt:
		if err := name("Value", n.Value); err != nil {
			return err
		}
		if err := require("Collection", n.Collection); err != nil {
			return err
		}
		return stmts("Body", n.Body)
	case *RangeLoopStmt:
		if err := name("Value", n.Value); err != nil {
			return err
		}
		if err := require("Start/End", n.Start, n.End); err != nil {
			return err
		}
		return stmts("Body", n.Body)
	case *TryStmt:
		if len(n.Handlers) == 0 && n.Finalizer == nil {
			return invalid(pos, "TryStmt", "requires a handler or finalizer")
		}
		if err := stmts("Body", n.Body); err != nil {
			return err
		}
		for _, handler := range n.Handlers {
			if err := validateSelector(handler.Pos, handler.IsAny, handler.Codes); err != nil {
				return err
			}
			if err := stmts("Handlers.Body", handler.Body); err != nil {
				return err
			}
		}
		if n.Finalizer != nil {
			return stmts("Finalizer.Body", n.Finalizer.Body)
		}
	case *ForkStmt:
		if err := require("Delay", n.Delay); err != nil {
			return err
		}
		return stmts("Body", n.Body)
	default:
		return invalid(pos, "Node", fmt.Sprintf("unknown semantic node %T", node))
	}
	return nil
}

func validateSelector(pos Position, any bool, codes []string) error {
	if any == (len(codes) != 0) {
		return invalid(pos, "Selector", "requires either all errors or nonempty codes")
	}
	for _, code := range codes {
		if code == "" {
			return invalid(pos, "Codes", "code must not be empty")
		}
	}
	return nil
}

// MaxNestingDepth is the maximum number of nested semantic constructs accepted
// by every frontend and backend that recursively walks a verb program.
const MaxNestingDepth = 256

var ErrMaxNestingDepth = errors.New("maximum nesting depth exceeded (max 256)")

// ValidateNesting checks semantic IR without recursively walking attacker
// controlled data. Siblings retain the same depth, so wide programs are not
// penalized. Leaf nodes are allowed beneath the deepest construct.
func ValidateNesting(program *Program) error {
	return validateGraph(reflect.ValueOf(program), -2, false)
}

// ValidateNode checks shape and depth for semantic inputs outside a Program.
func ValidateNode(node Node) error {
	if nilNode(node) {
		return invalid(Position{}, "Node", "must not be nil")
	}
	return validateGraph(reflect.ValueOf(node), -1, true)
}

// validateGraph combines shape and depth checks before following node fields.
// Unknown families can embed non-semantic cycles, so full validation must not
// run a separate depth-only traversal first. Reverse pushes preserve order.
func validateGraph(root reflect.Value, initialDepth int, full bool) error {
	type item struct {
		value reflect.Value
		depth int
	}
	// The program container and its root statement are structural anchors, not
	// nested source constructs.
	stack := []item{{root, initialDepth}}
	nodeType := reflect.TypeOf((*Node)(nil)).Elem()
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		v := current.value
		if v.IsValid() && v.Type().Implements(nodeType) {
			if full && v.CanInterface() {
				if node, ok := v.Interface().(Node); ok {
					if nilNode(node) {
						return invalid(Position{}, "Node", "must not be nil")
					}
					if err := validateNodeShape(node); err != nil {
						return err
					}
				}
			}
			current.depth++
			if current.depth > MaxNestingDepth {
				return ErrMaxNestingDepth
			}
		}
		for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
			if v.IsNil() {
				v = reflect.Value{}
				break
			}
			v = v.Elem()
		}
		if !v.IsValid() {
			continue
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := v.NumField() - 1; i >= 0; i-- {
				f := v.Field(i)
				stack = append(stack, item{f, current.depth})
			}
		case reflect.Slice:
			for i := v.Len() - 1; i >= 0; i-- {
				stack = append(stack, item{v.Index(i), current.depth})
			}
		}
	}
	return nil
}
