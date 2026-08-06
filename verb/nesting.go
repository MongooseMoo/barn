package verb

import (
	"fmt"
	"reflect"
)

// MaxNestingDepth is the maximum semantic and recursive-descent nesting Barn
// accepts. It is shared by parsing, formatting, and bytecode compilation.
const MaxNestingDepth = 256

// NestingDepthError reports the first semantic or syntactic node beyond
// MaxNestingDepth.
type NestingDepthError struct {
	Position Position
}

func (e *NestingDepthError) Error() string {
	return fmt.Sprintf("maximum nesting depth exceeded (max %d)", MaxNestingDepth)
}

type depthWork struct {
	node  Node
	depth int
}

// ValidateProgramNestingDepth verifies a complete semantic program without
// recursively walking attacker-controlled IR.
func ValidateProgramNestingDepth(program *Program) error {
	if program == nil {
		return nil
	}
	work := make([]depthWork, 0, len(program.Statements))
	for i := len(program.Statements) - 1; i >= 0; i-- {
		work = append(work, depthWork{node: program.Statements[i], depth: 1})
	}
	return validateNestingDepth(work)
}

// ValidateNodeNestingDepth verifies one semantic node without recursion.
func ValidateNodeNestingDepth(node Node) error {
	return validateNestingDepth([]depthWork{{node: node, depth: 1}})
}

func validateNestingDepth(work []depthWork) error {
	push := func(node Node, depth int) {
		if node != nil {
			work = append(work, depthWork{node: node, depth: depth})
		}
	}
	pushStatements := func(statements []Stmt, depth int) {
		for i := len(statements) - 1; i >= 0; i-- {
			push(statements[i], depth)
		}
	}

	for len(work) > 0 {
		last := len(work) - 1
		item := work[last]
		work = work[:last]
		if item.node == nil {
			continue
		}
		value := reflect.ValueOf(item.node)
		if value.Kind() == reflect.Pointer && value.IsNil() {
			continue
		}
		if item.depth > MaxNestingDepth {
			return &NestingDepthError{Position: item.node.Position()}
		}
		childDepth := item.depth + 1

		switch node := item.node.(type) {
		case *LiteralExpr, *IdentifierExpr, *IndexBoundaryExpr,
			*VariableTarget, *RequiredBinding, *RestBinding,
			*BreakStmt, *ContinueStmt:
			// Leaves.
		case *UnaryExpr:
			push(node.Operand, childDepth)
		case *BinaryExpr:
			push(node.Right, childDepth)
			push(node.Left, childDepth)
		case *TernaryExpr:
			push(node.ElseExpr, childDepth)
			push(node.ThenExpr, childDepth)
			push(node.Condition, childDepth)
		case *IndexExpr:
			push(node.Index, childDepth)
			push(node.Expr, childDepth)
		case *RangeExpr:
			push(node.End, childDepth)
			push(node.Start, childDepth)
			push(node.Expr, childDepth)
		case *PropertyExpr:
			push(node.PropertyExpr, childDepth)
			push(node.Expr, childDepth)
		case *VerbCallExpr:
			for i := len(node.Args) - 1; i >= 0; i-- {
				push(node.Args[i], childDepth)
			}
			push(node.VerbExpr, childDepth)
			push(node.Expr, childDepth)
		case *BuiltinCallExpr:
			for i := len(node.Args) - 1; i >= 0; i-- {
				push(node.Args[i], childDepth)
			}
		case *SpliceExpr:
			push(node.Expr, childDepth)
		case *CatchExpr:
			push(node.Default, childDepth)
			push(node.Expr, childDepth)
		case *AssignExpr:
			push(node.Value, childDepth)
			push(node.Target, childDepth)
		case *PropertyTarget:
			push(node.NameExpr, childDepth)
			push(node.Object, childDepth)
		case *IndexTarget:
			push(node.Index, childDepth)
			push(node.Collection, childDepth)
		case *RangeTarget:
			push(node.End, childDepth)
			push(node.Start, childDepth)
			push(node.Collection, childDepth)
		case *DestructuringTarget:
			for i := len(node.Bindings) - 1; i >= 0; i-- {
				push(node.Bindings[i], childDepth)
			}
		case *OptionalBinding:
			push(node.Default, childDepth)
		case *ListExpr:
			for i := len(node.Elements) - 1; i >= 0; i-- {
				push(node.Elements[i], childDepth)
			}
		case *ListRangeExpr:
			push(node.End, childDepth)
			push(node.Start, childDepth)
		case *MapExpr:
			for i := len(node.Pairs) - 1; i >= 0; i-- {
				push(node.Pairs[i].Value, childDepth)
				push(node.Pairs[i].Key, childDepth)
			}
		case *ExprStmt:
			push(node.Expr, childDepth)
		case *IfStmt:
			pushStatements(node.Else, childDepth)
			pushStatements(node.Body, childDepth)
			push(node.Condition, childDepth)
		case *WhileStmt:
			pushStatements(node.Body, childDepth)
			push(node.Condition, childDepth)
		case *CollectionLoopStmt:
			pushStatements(node.Body, childDepth)
			push(node.Collection, childDepth)
		case *RangeLoopStmt:
			pushStatements(node.Body, childDepth)
			push(node.End, childDepth)
			push(node.Start, childDepth)
		case *ReturnStmt:
			push(node.Value, childDepth)
		case *TryStmt:
			if node.Finalizer != nil {
				pushStatements(node.Finalizer.Body, childDepth)
			}
			for i := len(node.Handlers) - 1; i >= 0; i-- {
				pushStatements(node.Handlers[i].Body, childDepth)
			}
			pushStatements(node.Body, childDepth)
		case *ForkStmt:
			pushStatements(node.Body, childDepth)
			push(node.Delay, childDepth)
		default:
			return fmt.Errorf("unsupported semantic node %T", item.node)
		}
	}
	return nil
}
