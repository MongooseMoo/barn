package compiler

import (
	"fmt"
	"github.com/MongooseMoo/barn/ir"
)

func (s *nativeVector) block(stmts []ir.Stmt) {
	for _, stmt := range stmts {
		s.stmt(stmt)
		if s.err != nil {
			return
		}
	}
}
func (s *nativeVector) stmt(stmt ir.Stmt) {
	switch n := stmt.(type) {
	case *ir.EmptyStmt:
	case *ir.ExprStmt:
		s.expr(n.Expr)
		s.emit(1)
		s.pop(1)
	case *ir.ReturnStmt:
		if n.Value != nil {
			s.expr(n.Value)
			s.emit(1)
			s.pop(1)
		} else {
			s.emit(1)
		}
	case *ir.IfStmt:
		otherwise, end := s.label(), s.label()
		s.markLabel(otherwise, n, NativeBranchElse, 0)
		s.markLabel(end, n, NativeBranchEnd, 0)
		s.expr(n.Condition)
		s.emit(1)
		s.labelRef(otherwise)
		s.pop(1)
		s.block(n.Body)
		s.emit(1)
		s.labelRef(end)
		s.bind(otherwise)
		s.block(n.Else)
		s.bind(end)
	case *ir.WhileStmt:
		top, end := s.label(), s.label()
		s.markLabel(top, n, NativeLoopTop, 0)
		s.markLabel(end, n, NativeLoopEnd, 0)
		s.bind(top)
		base := len(s.stack)
		s.controls = append(s.controls, NativeControlOrigin{Kind: NativeWhileLoop, Node: n, StackBase: base})
		s.expr(n.Condition)
		if n.Label == "" {
			s.emit(1)
		} else {
			s.emit(2)
			s.varRef(n.Label)
		}
		s.labelRef(end)
		s.pop(1)
		s.loops = append(s.loops, nativeLoop{node: n, value: n.Label, top: top, bottom: end, topStack: base, bottomStack: base})
		s.block(n.Body)
		s.loops = s.loops[:len(s.loops)-1]
		s.controls = s.controls[:len(s.controls)-1]
		s.emit(1)
		s.labelRef(top)
		s.bind(end)
	case *ir.CollectionLoopStmt:
		base := len(s.stack)
		s.expr(n.Collection)
		s.result(n, 1)
		s.stack[len(s.stack)-1].Kind = NativeLoopContainer
		s.literal("none")
		s.push(NativeValueOrigin{Kind: NativeLoopCursor, Node: n})
		top, end := s.label(), s.label()
		s.markLabel(top, n, NativeLoopTop, 0)
		s.markLabel(end, n, NativeLoopEnd, 0)
		s.bind(top)
		s.emit(2)
		s.varRef(n.Value)
		if n.Index != "" {
			s.varRef(n.Index)
		}
		s.labelRef(end)
		s.controls = append(s.controls, NativeControlOrigin{Kind: NativeCollectionLoop, Node: n, StackBase: base})
		s.loops = append(s.loops, nativeLoop{node: n, value: n.Value, index: n.Index, top: top, bottom: end, topStack: base + 2, bottomStack: base})
		s.block(n.Body)
		s.loops = s.loops[:len(s.loops)-1]
		s.controls = s.controls[:len(s.controls)-1]
		s.emit(1)
		s.labelRef(top)
		s.bind(end)
		s.pop(2)
	case *ir.RangeLoopStmt:
		base := len(s.stack)
		s.expr(n.Start)
		s.stack[len(s.stack)-1] = NativeValueOrigin{Kind: NativeRangeNext, Node: n}
		s.expr(n.End)
		s.stack[len(s.stack)-1] = NativeValueOrigin{Kind: NativeRangeLimit, Node: n}
		top, end := s.label(), s.label()
		s.markLabel(top, n, NativeLoopTop, 0)
		s.markLabel(end, n, NativeLoopEnd, 0)
		s.bind(top)
		s.emit(1)
		s.varRef(n.Value)
		s.labelRef(end)
		s.controls = append(s.controls, NativeControlOrigin{Kind: NativeRangeLoop, Node: n, StackBase: base})
		s.loops = append(s.loops, nativeLoop{node: n, value: n.Value, top: top, bottom: end, topStack: base + 2, bottomStack: base})
		s.block(n.Body)
		s.loops = s.loops[:len(s.loops)-1]
		s.controls = s.controls[:len(s.controls)-1]
		s.emit(1)
		s.labelRef(top)
		s.bind(end)
		s.pop(2)
	case *ir.BreakStmt:
		s.exit(n.Label, false)
	case *ir.ContinueStmt:
		s.exit(n.Label, true)
	case *ir.ForkStmt:
		s.expr(n.Delay)
		s.emit(1)
		fork, err := s.global.vector(n.Body, -2)
		if err != nil {
			s.err = err
			return
		}
		index := len(s.global.forks)
		fork.Vector = index
		for i := range fork.Continuations {
			fork.Continuations[i].Vector = index
		}
		s.global.forks = append(s.global.forks, fork)
		s.ref(nativeForkRef)
		s.maxFork = max(s.maxFork, index)
		if n.VarName != "" {
			s.varRef(n.VarName)
		}
		s.pop(1)
	case *ir.TryStmt:
		if n.Finalizer != nil {
			if len(n.Handlers) > 0 {
				s.err = fmt.Errorf("native source cannot combine except and finally in one try")
				return
			}
			s.finally(n)
		} else {
			s.except(n)
		}
	default:
		s.err = fmt.Errorf("unsupported native statement %T", stmt)
	}
}

func (s *nativeVector) exit(name string, continuing bool) {
	s.emit(2)
	if name != "" {
		s.varRef(name)
	}
	for i := len(s.loops) - 1; i >= 0; i-- {
		loop := s.loops[i]
		if name != "" && canonicalIdentifier(name) != canonicalIdentifier(loop.value) && canonicalIdentifier(name) != canonicalIdentifier(loop.index) {
			continue
		}
		s.ref(nativeStackRef)
		if continuing {
			s.labelRef(loop.top)
		} else {
			s.labelRef(loop.bottom)
		}
		return
	}
	s.err = fmt.Errorf("native loop exit %q has no target", name)
}

func (s *nativeVector) except(n *ir.TryStmt) {
	base := len(s.stack)
	labels := make([]int, len(n.Handlers))
	end := s.label()
	s.markLabel(end, n, NativeHandlerEnd, 0)
	for i, h := range n.Handlers {
		s.codes(n, i, h.Codes, h.IsAny)
		labels[i] = s.label()
		s.markLabel(labels[i], n, NativeHandler, i)
		s.emit(2)
		s.labelRef(labels[i])
		s.push(NativeValueOrigin{Kind: NativeHandlerLabel, Node: n, Aux: i, LabelPC: labels[i]})
	}
	s.emit(3)
	s.push(NativeValueOrigin{Kind: NativeCatchMarker, Node: n, Aux: len(n.Handlers)})
	s.controls = append(s.controls, NativeControlOrigin{Kind: NativeExcept, Node: n, StackBase: base, HandlerPCs: append([]int(nil), labels...)})
	s.block(n.Body)
	s.controls = s.controls[:len(s.controls)-1]
	s.emit(2)
	s.labelRef(end)
	s.pop(2*len(n.Handlers) + 1)
	for i, h := range n.Handlers {
		s.bind(labels[i])
		s.push(NativeValueOrigin{Kind: NativeExpression, Node: n, Aux: i})
		if h.Variable != "" {
			s.varOp(h.Variable)
		}
		s.emit(1)
		s.pop(1)
		s.block(h.Body)
		if i+1 < len(n.Handlers) {
			s.emit(1)
			s.labelRef(end)
		}
	}
	s.bind(end)
}

func (s *nativeVector) finally(n *ir.TryStmt) {
	base := len(s.stack)
	handler := s.label()
	s.markLabel(handler, n, NativeHandler, 0)
	s.emit(2)
	s.labelRef(handler)
	s.push(NativeValueOrigin{Kind: NativeFinallyMarker, Node: n, LabelPC: handler})
	s.controls = append(s.controls, NativeControlOrigin{Kind: NativeFinallyBody, Node: n, StackBase: base, HandlerPCs: []int{handler}})
	s.block(n.Body)
	s.controls = s.controls[:len(s.controls)-1]
	s.emit(2)
	s.pop(1)
	s.bind(handler)
	s.push(NativeValueOrigin{Kind: NativeFinallyReason, Node: n})
	s.push(NativeValueOrigin{Kind: NativeFinallyValue, Node: n})
	s.controls = append(s.controls, NativeControlOrigin{Kind: NativeFinallyHandler, Node: n, StackBase: base, HandlerPCs: []int{handler}})
	s.block(n.Finalizer.Body)
	s.controls = s.controls[:len(s.controls)-1]
	s.emit(2)
	s.pop(2)
}
