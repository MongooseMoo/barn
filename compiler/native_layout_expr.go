package compiler

import (
	"fmt"
	"github.com/MongooseMoo/barn/ir"
)

func (s *nativeVector) args(parent ir.Node, args []ir.Expr) {
	if len(args) == 0 {
		s.emit(1)
		s.push(NativeValueOrigin{Kind: NativeListPrefix, Node: parent})
		return
	}
	for i, arg := range args {
		if splice, ok := arg.(*ir.SpliceExpr); ok {
			s.expr(splice.Expr)
		} else {
			s.expr(arg)
		}
		s.emit(1)
		if i > 0 {
			s.pop(1)
		}
		s.stack[len(s.stack)-1] = NativeValueOrigin{Kind: NativeListPrefix, Node: parent, Aux: i + 1}
	}
}
func (s *nativeVector) codes(parent ir.Node, index int, codes []ir.Expr, any bool) {
	if any {
		s.emit(1)
		s.push(NativeValueOrigin{Kind: NativeExceptionCodes, Node: parent, Aux: index})
	} else {
		s.args(parent, codes)
		s.stack[len(s.stack)-1] = NativeValueOrigin{Kind: NativeExceptionCodes, Node: parent, Aux: index}
	}
}
func (s *nativeVector) expr(expr ir.Expr) {
	if s.err != nil {
		return
	}
	if s.number(expr) {
		return
	}
	switch e := expr.(type) {
	case *ir.LiteralExpr:
		switch e.Kind {
		case ir.LiteralString:
			s.literal("s:" + e.StringValue)
		case ir.LiteralObj:
			s.literal(fmt.Sprintf("o:%d", e.ObjID))
		case ir.LiteralErr:
			s.literal("e:" + canonicalIdentifier(e.ErrorName))
		default:
			s.err = fmt.Errorf("unsupported native literal kind %d", e.Kind)
			return
		}
		s.push(NativeValueOrigin{Kind: NativeExpression, Node: e})
	case *ir.IdentifierExpr:
		s.varOp(e.Name)
		s.push(NativeValueOrigin{Kind: NativeExpression, Node: e})
	case *ir.UnaryExpr:
		s.expr(e.Operand)
		if e.Operator == ir.UnaryBitwiseNot {
			s.emit(2)
		} else {
			s.emit(1)
		}
		s.result(e, 1)
	case *ir.BinaryExpr:
		s.expr(e.Left)
		if e.Operator == ir.BinaryAnd || e.Operator == ir.BinaryOr {
			end := s.label()
			s.markLabel(end, e, NativeBranchEnd, 0)
			s.emit(1)
			s.labelRef(end)
			s.pop(1)
			s.expr(e.Right)
			s.bind(end)
			s.result(e, 1)
		} else {
			s.expr(e.Right)
			switch e.Operator {
			case ir.BinaryPower, ir.BinaryBitAnd, ir.BinaryBitOr, ir.BinaryBitXor, ir.BinaryShiftLeft, ir.BinaryShiftRight:
				s.emit(2)
			default:
				s.emit(1)
			}
			s.result(e, 2)
		}
	case *ir.TernaryExpr:
		s.expr(e.Condition)
		otherwise, end := s.label(), s.label()
		s.markLabel(otherwise, e, NativeBranchElse, 0)
		s.markLabel(end, e, NativeBranchEnd, 0)
		s.emit(1)
		s.labelRef(otherwise)
		s.pop(1)
		s.expr(e.ThenExpr)
		s.emit(1)
		s.labelRef(end)
		s.pop(1)
		s.bind(otherwise)
		s.expr(e.ElseExpr)
		s.bind(end)
		s.result(e, 1)
	case *ir.PropertyExpr:
		s.expr(e.Expr)
		if e.Property != "" {
			s.stringLiteral(e.Property, e, NativePropertyName)
		} else {
			s.expr(e.PropertyExpr)
		}
		s.emit(1)
		s.result(e, 2)
	case *ir.IndexExpr:
		s.expr(e.Expr)
		old := s.savedStack
		s.savedStack = len(s.stack) - 1
		s.expr(e.Index)
		s.savedStack = old
		s.emit(1)
		s.result(e, 2)
	case *ir.RangeExpr:
		s.expr(e.Expr)
		old := s.savedStack
		s.savedStack = len(s.stack) - 1
		s.expr(e.Start)
		s.expr(e.End)
		s.savedStack = old
		s.emit(1)
		s.result(e, 3)
	case *ir.IndexBoundaryExpr:
		if s.savedStack < 0 {
			s.err = fmt.Errorf("native index boundary without base")
			return
		}
		s.emit(2)
		s.ref(nativeStackRef)
		s.push(NativeValueOrigin{Kind: NativeExpression, Node: e})
	case *ir.BuiltinCallExpr:
		s.args(e, e.Args)
		s.call(e, 1, 2)
	case *ir.VerbCallExpr:
		s.expr(e.Expr)
		if e.Verb != "" {
			s.stringLiteral(e.Verb, e, NativePropertyName)
		} else {
			s.expr(e.VerbExpr)
		}
		s.args(e, e.Args)
		s.call(e, 3, 1)
	case *ir.ListExpr:
		s.args(e, e.Elements)
		s.result(e, 1)
	case *ir.MapExpr:
		s.emit(1)
		s.push(NativeValueOrigin{Kind: NativeMapPrefix, Node: e})
		for i, pair := range e.Pairs {
			s.expr(pair.Value)
			s.expr(pair.Key)
			s.emit(1)
			s.pop(2)
			s.stack[len(s.stack)-1] = NativeValueOrigin{Kind: NativeMapPrefix, Node: e, Aux: i + 1}
		}
		s.result(e, 1)
	case *ir.AssignExpr:
		s.assign(e)
	case *ir.CatchExpr:
		s.catch(e)
	default:
		s.err = fmt.Errorf("unsupported native expression %T", expr)
	}
}

func (s *nativeVector) catch(e *ir.CatchExpr) {
	base := len(s.stack)
	handler, end := s.label(), s.label()
	s.markLabel(handler, e, NativeHandler, 0)
	s.markLabel(end, e, NativeHandlerEnd, 0)
	s.codes(e, 0, e.Codes, e.IsAny)
	s.emit(2)
	s.labelRef(handler)
	s.push(NativeValueOrigin{Kind: NativeHandlerLabel, Node: e, LabelPC: handler})
	s.emit(2)
	s.push(NativeValueOrigin{Kind: NativeCatchMarker, Node: e, Aux: 1})
	s.controls = append(s.controls, NativeControlOrigin{Kind: NativeCatch, Node: e, StackBase: base, HandlerPCs: []int{handler}})
	s.expr(e.Expr)
	s.controls = s.controls[:len(s.controls)-1]
	s.emit(2)
	s.labelRef(end)
	s.pop(4)
	s.push(NativeValueOrigin{Kind: NativeExpression, Node: e})
	s.bind(handler)
	if e.Default != nil {
		s.emit(1)
		s.pop(1)
		s.expr(e.Default)
	} else {
		s.emit(2)
	}
	s.bind(end)
	s.result(e, 1)
}

func (s *nativeVector) target(t ir.Target, indexed bool) {
	switch t := t.(type) {
	case *ir.VariableTarget:
		if indexed {
			s.varOp(t.Name)
			s.push(NativeValueOrigin{Kind: NativeExpression, Node: t})
		}
	case *ir.PropertyTarget:
		s.expr(t.Object)
		if t.Name != "" {
			s.stringLiteral(t.Name, t, NativePropertyName)
		} else {
			s.expr(t.NameExpr)
		}
		if indexed {
			s.emit(1)
			s.push(NativeValueOrigin{Kind: NativeExpression, Node: t})
		}
	case *ir.IndexTarget:
		s.target(t.Collection, true)
		old := s.savedStack
		s.savedStack = len(s.stack) - 1
		s.expr(t.Index)
		s.savedStack = old
		if indexed {
			s.emit(1)
			s.push(NativeValueOrigin{Kind: NativeExpression, Node: t})
		}
	case *ir.RangeTarget:
		s.target(t.Collection, true)
		old := s.savedStack
		s.savedStack = len(s.stack) - 1
		s.expr(t.Start)
		s.expr(t.End)
		s.savedStack = old
	default:
		s.err = fmt.Errorf("unsupported native lvalue %T", t)
	}
}

func (s *nativeVector) assign(e *ir.AssignExpr) {
	if target, ok := e.Target.(*ir.DestructuringTarget); ok {
		s.scatter(e, target)
		return
	}
	s.target(e.Target, false)
	s.expr(e.Value)
	target := e.Target
	indexed := false
	if _, ok := target.(*ir.IndexTarget); ok {
		s.emit(1)
	}
	if _, ok := target.(*ir.RangeTarget); ok {
		s.emit(1)
	}
	for {
		switch t := target.(type) {
		case *ir.IndexTarget:
			s.emit(1)
			s.result(t, 3)
			target = t.Collection
			indexed = true
			continue
		case *ir.RangeTarget:
			s.emit(2)
			s.result(t, 4)
			target = t.Collection
			indexed = true
			continue
		case *ir.VariableTarget:
			s.varOp(t.Name)
		case *ir.PropertyTarget:
			s.emit(1)
			s.result(t, 3)
		default:
			s.err = fmt.Errorf("unsupported native store %T", target)
		}
		break
	}
	if indexed {
		s.emit(2)
	}
	s.result(e, 1)
}

func (s *nativeVector) scatter(e *ir.AssignExpr, target *ir.DestructuringTarget) {
	s.expr(e.Value)
	s.emit(5) // EXTENDED SCATTER nargs nreq rest
	labels := make([]int, len(target.Bindings))
	for i, binding := range target.Bindings {
		switch b := binding.(type) {
		case *ir.RequiredBinding:
			s.varRef(b.Name)
		case *ir.RestBinding:
			s.varRef(b.Name)
		case *ir.OptionalBinding:
			s.varRef(b.Name)
		}
		labels[i] = s.label()
		if b, ok := binding.(*ir.OptionalBinding); ok && b.Default != nil {
			s.markLabel(labels[i], target, NativeScatterDefault, i)
		}
		s.labelRef(labels[i])
	}
	done := s.label()
	s.markLabel(done, target, NativeScatterEnd, 0)
	s.labelRef(done)
	for i, binding := range target.Bindings {
		if b, ok := binding.(*ir.OptionalBinding); ok && b.Default != nil {
			s.bind(labels[i])
			s.expr(b.Default)
			s.varOp(b.Name)
			s.emit(1)
			s.pop(1)
		}
	}
	s.bind(done)
	s.result(e, 1)
}
