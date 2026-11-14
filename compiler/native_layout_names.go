package compiler

import (
	"fmt"

	"github.com/MongooseMoo/barn/ir"
)

// Toast assigns slots when yacc reduces identifiers, rather than in codegen
// order. Loop/fork/except binding names are reduced after their bodies.
type nativeNames struct {
	names []string
	slots map[string]int
}

func makeNativeNames(program *ir.Program, version int) (*nativeNames, error) {
	if version < 0 || version > 17 {
		return nil, fmt.Errorf("unsupported native language version %d", version)
	}
	n := &nativeNames{slots: make(map[string]int)}
	for _, name := range []string{"NUM", "OBJ", "STR", "LIST", "ERR", "player", "this", "caller", "verb", "args", "argstr", "dobj", "dobjstr", "prepstr", "iobj", "iobjstr"} {
		n.add(name)
	}
	if version >= 3 {
		n.add("INT")
		n.add("FLOAT")
	}
	if version >= 7 {
		n.add("MAP")
	}
	if version >= 13 {
		n.add("ANON")
	}
	if version >= 14 {
		n.add("WAIF")
	}
	if version >= 17 {
		n.add("BOOL")
		n.add("true")
		n.add("false")
	}
	if err := n.block(program.Statements); err != nil {
		return nil, err
	}
	return n, nil
}

// NativeVariableNames is the variable-name table Toast's compiler would give the
// program: its builtin variables first, then each source variable in reduction
// order, spelled the first way the parser met it.
func NativeVariableNames(program *ir.Program, version int) ([]string, error) {
	n, err := makeNativeNames(program, version)
	if err != nil {
		return nil, err
	}
	return n.names, nil
}

func (n *nativeNames) add(name string) {
	if name == "" {
		return
	}
	key := canonicalIdentifier(name)
	if _, exists := n.slots[key]; !exists {
		n.slots[key] = len(n.names)
		n.names = append(n.names, name)
	}
}

func (n *nativeNames) expr(e ir.Expr) error {
	if e == nil {
		return nil
	}
	var operands []ir.Expr
	switch e := e.(type) {
	case *ir.LiteralExpr, *ir.IndexBoundaryExpr:
	case *ir.IdentifierExpr:
		n.add(e.Name)
	case *ir.UnaryExpr:
		operands = []ir.Expr{e.Operand}
	case *ir.BinaryExpr:
		operands = []ir.Expr{e.Left, e.Right}
	case *ir.TernaryExpr:
		operands = []ir.Expr{e.Condition, e.ThenExpr, e.ElseExpr}
	case *ir.IndexExpr:
		operands = []ir.Expr{e.Expr, e.Index}
	case *ir.RangeExpr:
		operands = []ir.Expr{e.Expr, e.Start, e.End}
	case *ir.PropertyExpr:
		operands = []ir.Expr{e.Expr, e.PropertyExpr}
	case *ir.VerbCallExpr:
		operands = append([]ir.Expr{e.Expr, e.VerbExpr}, e.Args...)
	case *ir.BuiltinCallExpr:
		operands = e.Args
	case *ir.SpliceExpr:
		operands = []ir.Expr{e.Expr}
	case *ir.CatchExpr:
		operands = append([]ir.Expr{e.Expr}, e.Codes...)
		operands = append(operands, e.Default)
	case *ir.AssignExpr:
		if err := n.target(e.Target); err != nil {
			return err
		}
		operands = []ir.Expr{e.Value}
	case *ir.ListExpr:
		operands = e.Elements
	case *ir.MapExpr:
		for _, pair := range e.Pairs {
			operands = append(operands, pair.Key, pair.Value)
		}
	default:
		return fmt.Errorf("unsupported native expression %T", e)
	}
	for _, operand := range operands {
		if err := n.expr(operand); err != nil {
			return err
		}
	}
	return nil
}

func (n *nativeNames) target(t ir.Target) error {
	switch t := t.(type) {
	case *ir.VariableTarget:
		n.add(t.Name)
	case *ir.PropertyTarget:
		if err := n.expr(t.Object); err != nil {
			return err
		}
		return n.expr(t.NameExpr)
	case *ir.IndexTarget:
		if err := n.target(t.Collection); err != nil {
			return err
		}
		return n.expr(t.Index)
	case *ir.RangeTarget:
		if err := n.target(t.Collection); err != nil {
			return err
		}
		if err := n.expr(t.Start); err != nil {
			return err
		}
		return n.expr(t.End)
	case *ir.DestructuringTarget:
		for _, binding := range t.Bindings {
			switch b := binding.(type) {
			case *ir.RequiredBinding:
				n.add(b.Name)
			case *ir.RestBinding:
				n.add(b.Name)
			case *ir.OptionalBinding:
				if err := n.expr(b.Default); err != nil {
					return err
				}
				n.add(b.Name)
			}
		}
	default:
		return fmt.Errorf("unsupported native target %T", t)
	}
	return nil
}

func (n *nativeNames) block(stmts []ir.Stmt) error {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ir.EmptyStmt:
		case *ir.ExprStmt:
			if err := n.expr(s.Expr); err != nil {
				return err
			}
		case *ir.ReturnStmt:
			if err := n.expr(s.Value); err != nil {
				return err
			}
		case *ir.IfStmt:
			if err := n.expr(s.Condition); err != nil {
				return err
			}
			if err := n.block(s.Body); err != nil {
				return err
			}
			if err := n.block(s.Else); err != nil {
				return err
			}
		case *ir.WhileStmt:
			if err := n.expr(s.Condition); err != nil {
				return err
			}
			if err := n.block(s.Body); err != nil {
				return err
			}
			n.add(s.Label)
		case *ir.CollectionLoopStmt:
			if err := n.expr(s.Collection); err != nil {
				return err
			}
			if err := n.block(s.Body); err != nil {
				return err
			}
			n.add(s.Value)
			n.add(s.Index)
		case *ir.RangeLoopStmt:
			if err := n.expr(s.Start); err != nil {
				return err
			}
			if err := n.expr(s.End); err != nil {
				return err
			}
			if err := n.block(s.Body); err != nil {
				return err
			}
			n.add(s.Value)
		case *ir.ForkStmt:
			if err := n.expr(s.Delay); err != nil {
				return err
			}
			if err := n.block(s.Body); err != nil {
				return err
			}
			n.add(s.VarName)
		case *ir.BreakStmt:
			n.add(s.Label)
		case *ir.ContinueStmt:
			n.add(s.Label)
		case *ir.TryStmt:
			if err := n.block(s.Body); err != nil {
				return err
			}
			for _, h := range s.Handlers {
				for _, code := range h.Codes {
					if err := n.expr(code); err != nil {
						return err
					}
				}
				if err := n.block(h.Body); err != nil {
					return err
				}
				n.add(h.Variable)
			}
			if s.Finalizer != nil {
				if err := n.block(s.Finalizer.Body); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported native statement %T", stmt)
		}
	}
	return nil
}
