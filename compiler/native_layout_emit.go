package compiler

import (
	"fmt"
	"math"

	"github.com/MongooseMoo/barn/ir"
)

type nativeRefKind uint8

const (
	nativeLiteralRef nativeRefKind = iota
	nativeForkRef
	nativeVariableRef
	nativeLabelRef
	nativeStackRef
)

type nativeRef struct {
	at   int
	kind nativeRefKind
}

// nativeReduceOp is one op Toast's BYTECODE_REDUCE_REF pass reads
// (code_gen.cc stmt_to_code): a push or store of a variable in slot < 32,
// an ending op (OP_RETURN, OP_RETURN0, OP_DONE) or OP_CALL_VERB, at its raw
// byte position, and whether it lies inside a try body.
type nativeReduceOp struct {
	at   int
	kind nativeReduceKind
	slot int
	try  bool
}

type nativeReduceKind uint8

const (
	nativeReducePush nativeReduceKind = iota
	nativeReducePut
	nativeReduceEnd
	nativeReduceCallVerb
)

// nativeVRNames are the variables a verb call passes implicitly, which Toast
// never clears before one (code_gen.cc NON_VR_VAR_MASK).
var nativeVRNames = []string{"ARGSTR", "DOBJ", "DOBJSTR", "PREPSTR", "IOBJ", "IOBJSTR", "PLAYER"}

type nativeLoop struct {
	node                               ir.Node
	value, index                       string
	top, bottom, topStack, bottomStack int
}
type nativeGenerator struct {
	names        *nativeNames
	literals     map[string]int
	forks        []NativeVectorLayout
	variableRefs int
}
type nativeVector struct {
	global                           *nativeGenerator
	rawSize                          int
	refs                             []nativeRef
	counts                           [5]int
	maxLiteral, maxFork, maxVariable int
	stack                            []NativeValueOrigin
	maxStack                         int
	labels                           []int
	labelOrigins                     map[int]NativeLabelOrigin
	controls                         []NativeControlOrigin
	loops                            []nativeLoop
	continuations                    []NativeContinuation
	savedStack                       int
	tryDepth                         int
	reduceOps                        []nativeReduceOp
	err                              error
}

// NativeLayout reconstructs the original compiler's layout without executing
// source or depending on an installed native server. Saved names are not slot
// order: Toast itself recompiles and reorders that environment on import.
func NativeLayout(program *ir.Program, savedNames []string, languageVersion int) (*NativeProgramLayout, error) {
	if err := ir.Validate(program); err != nil {
		return nil, err
	}
	names, err := makeNativeNames(program, languageVersion)
	if err != nil {
		return nil, err
	}
	_ = savedNames
	g := &nativeGenerator{names: names, literals: make(map[string]int)}
	main, err := g.vector(program.Statements, -1)
	if err != nil {
		return nil, err
	}
	return &NativeProgramLayout{VariableNames: append([]string(nil), names.names...), Vectors: append([]NativeVectorLayout{main}, g.forks...)}, nil
}

func (g *nativeGenerator) vector(block []ir.Stmt, vector int) (NativeVectorLayout, error) {
	s := &nativeVector{global: g, savedStack: -1}
	s.block(block)
	s.reduceOp(nativeReduceEnd, 0)
	s.emit(1) // OP_DONE
	if s.err != nil {
		return NativeVectorLayout{}, s.err
	}
	if len(s.stack) != 0 {
		return NativeVectorLayout{}, fmt.Errorf("native vector leaves %d operands", len(s.stack))
	}
	layout := s.finish(vector)
	layout.Body = block
	return layout, nil
}

func (s *nativeVector) emit(bytes int) { s.rawSize += bytes }
func (s *nativeVector) ref(kind nativeRefKind) {
	s.refs = append(s.refs, nativeRef{s.rawSize, kind})
	s.counts[kind]++
	s.rawSize++
}
func (s *nativeVector) push(origin NativeValueOrigin) {
	s.stack = append(s.stack, origin)
	s.maxStack = max(s.maxStack, len(s.stack))
}
func (s *nativeVector) pop(count int) {
	if count > len(s.stack) {
		if s.err == nil {
			s.err = fmt.Errorf("native layout operand underflow: %d from %d", count, len(s.stack))
		}
		return
	}
	s.stack = s.stack[:len(s.stack)-count]
}
func (s *nativeVector) result(node ir.Node, count int) {
	s.pop(count)
	s.push(NativeValueOrigin{Kind: NativeExpression, Node: node})
}
func (s *nativeVector) label() int         { s.labels = append(s.labels, -1); return len(s.labels) - 1 }
func (s *nativeVector) bind(label int)     { s.labels[label] = s.rawSize }
func (s *nativeVector) labelRef(label int) { s.ref(nativeLabelRef) }

func (s *nativeVector) markLabel(label int, node ir.Node, kind NativeLabelKind, aux int) {
	if s.labelOrigins == nil {
		s.labelOrigins = make(map[int]NativeLabelOrigin)
	}
	s.labelOrigins[label] = NativeLabelOrigin{Node: node, Kind: kind, Aux: aux}
}
func (s *nativeVector) varRef(name string) {
	slot, ok := s.global.names.slots[canonicalIdentifier(name)]
	if !ok {
		if s.err == nil {
			s.err = fmt.Errorf("native variable %q not declared", name)
		}
		return
	}
	s.ref(nativeVariableRef)
	s.maxVariable = max(s.maxVariable, slot)
	s.global.variableRefs++
}
func (s *nativeVector) varOp(name string, kind nativeReduceKind) {
	slot, ok := s.global.names.slots[canonicalIdentifier(name)]
	if ok && slot < 32 {
		s.reduceOp(kind, slot)
	}
	s.emit(1)
	if slot >= 32 {
		s.varRef(name)
	}
}

// reduceOp records the op about to be emitted at the current raw position.
func (s *nativeVector) reduceOp(kind nativeReduceKind, slot int) {
	s.reduceOps = append(s.reduceOps, nativeReduceOp{at: s.rawSize, kind: kind, slot: slot, try: s.tryDepth > 0})
}

// clearedAt returns the variables Toast holds unbound at the call that ends
// at raw position pc. Toast splits a vector into blocks at every label and
// fork operand and, scanning each block backward, compiles the last push of a
// slot before a store of it, or before an ending op, as OP_PUSH_CLEAR; ops in
// a try body set no bits and a verb call keeps the implicit variables
// (code_gen.cc stmt_to_code). A store binds the slot again, so a slot is
// unbound at pc when, walking the call's block forward to pc, its last access
// was a cleared push. A label inside the block before pc is a jump target
// that skips the block's start; when walking from it gives another answer,
// unknown is true. Earlier blocks leave no slot unbound: a cleared push is
// followed in its own block by a store of the slot or an ending op.
func (s *nativeVector) clearedAt(pc int) (cleared []string, unknown bool) {
	last := pc - 1
	lo, hi := 0, s.rawSize
	for _, r := range s.refs {
		if r.kind != nativeLabelRef && r.kind != nativeForkRef {
			continue
		}
		if r.at <= last {
			lo = max(lo, r.at)
		} else {
			hi = min(hi, r.at)
		}
	}
	vr := ^uint32(0)
	for _, name := range nativeVRNames {
		if slot, ok := s.global.names.slots[name]; ok && slot < 32 {
			vr &^= 1 << slot
		}
	}
	clears := make(map[int]bool)
	var bits uint32
	for i := len(s.reduceOps) - 1; i >= 0; i-- {
		op := s.reduceOps[i]
		if op.at < lo || op.at >= hi {
			continue
		}
		switch {
		case op.kind == nativeReducePush:
			if bits&(1<<op.slot) != 0 {
				bits &^= 1 << op.slot
				clears[op.at] = true
			}
		case op.try:
		case op.kind == nativeReducePut:
			bits |= 1 << op.slot
		case op.kind == nativeReduceEnd:
			bits = ^uint32(0)
		case op.kind == nativeReduceCallVerb:
			bits &= vr
		}
	}
	walk := func(from int) uint32 {
		var unbound uint32
		for _, op := range s.reduceOps {
			if op.at < from || op.at >= pc {
				continue
			}
			if clears[op.at] {
				unbound |= 1 << op.slot
			} else if op.kind == nativeReducePut {
				unbound &^= 1 << op.slot
			}
		}
		return unbound
	}
	unbound := walk(lo)
	for _, label := range s.labels {
		if label > lo && label < pc && walk(label) != unbound {
			return nil, true
		}
	}
	for slot := range 32 {
		if unbound&(1<<slot) != 0 {
			cleared = append(cleared, s.global.names.names[slot])
		}
	}
	return cleared, false
}
func (s *nativeVector) literal(key string) {
	index, exists := s.global.literals[key]
	if !exists {
		index = len(s.global.literals)
		s.global.literals[key] = index
	}
	s.emit(1)
	s.ref(nativeLiteralRef)
	s.maxLiteral = max(s.maxLiteral, index)
}
func (s *nativeVector) stringLiteral(value string, node ir.Node, kind NativeValueKind) {
	s.literal("s:" + value)
	s.push(NativeValueOrigin{Kind: kind, Node: node})
}
func (s *nativeVector) call(node ir.Node, consumed, bytes int) {
	s.emit(bytes)
	s.result(node, consumed)
	controls := make([]NativeControlOrigin, len(s.controls))
	for i, c := range s.controls {
		controls[i] = c
		controls[i].HandlerPCs = append([]int(nil), c.HandlerPCs...)
	}
	s.continuations = append(s.continuations, NativeContinuation{Node: node, PC: s.rawSize, Stack: append([]NativeValueOrigin(nil), s.stack...), Controls: controls})
}
func nativeRefWidth(value int) int {
	if value <= 256 {
		return 1
	}
	if value <= 65536 {
		return 2
	}
	return 4
}
func (s *nativeVector) finish(vector int) NativeVectorLayout {
	l := NativeVectorLayout{Vector: vector, MaxStack: s.maxStack, LiteralWidth: nativeRefWidth(max(s.maxLiteral, len(s.global.literals))), ForkWidth: nativeRefWidth(max(s.maxFork, len(s.global.forks))), VariableWidth: nativeRefWidth(max(s.maxVariable, s.global.variableRefs)), StackWidth: nativeRefWidth(s.maxStack)}
	base := s.rawSize + (l.LiteralWidth-1)*s.counts[nativeLiteralRef] + (l.ForkWidth-1)*s.counts[nativeForkRef] + (l.VariableWidth-1)*s.counts[nativeVariableRef]
	l.LabelWidth = 1
	if base > 256 {
		l.LabelWidth = 2
		if base+s.counts[nativeLabelRef] > 65536 {
			l.LabelWidth = 4
		}
	}
	l.Size = base + (l.LabelWidth-1)*s.counts[nativeLabelRef]
	widths := [5]int{l.LiteralWidth, l.ForkWidth, l.VariableWidth, l.LabelWidth, l.StackWidth}
	expand := func(raw int, stackRefs bool) int {
		pc := raw
		for _, r := range s.refs {
			if r.at >= raw {
				break
			}
			if r.kind != nativeStackRef || stackRefs {
				pc += widths[r.kind] - 1
			}
		}
		return pc
	}
	labelPC := func(label int) int {
		if label < 0 || label >= len(s.labels) || s.labels[label] < 0 {
			return -1
		}
		return expand(s.labels[label], false)
	}
	for _, site := range s.continuations {
		site.Cleared, site.ClearUnknown = s.clearedAt(site.PC)
		site.Vector = vector
		site.PC = expand(site.PC, true)
		for i := range site.Stack {
			if site.Stack[i].Kind == NativeHandlerLabel || site.Stack[i].Kind == NativeFinallyMarker {
				site.Stack[i].LabelPC = labelPC(site.Stack[i].LabelPC)
			}
		}
		for i := range site.Controls {
			for j, label := range site.Controls[i].HandlerPCs {
				site.Controls[i].HandlerPCs[j] = labelPC(label)
			}
		}
		l.Continuations = append(l.Continuations, site)
	}
	for label := range s.labels {
		if origin, ok := s.labelOrigins[label]; ok {
			origin.PC = labelPC(label)
			l.Labels = append(l.Labels, origin)
		}
	}
	return l
}

func nativeNumeric(e ir.Expr) (kind ir.LiteralKind, integer int64, floating float64, ok bool) {
	switch e := e.(type) {
	case *ir.LiteralExpr:
		if e.Kind == ir.LiteralInt {
			return e.Kind, e.IntValue, 0, true
		}
		if e.Kind == ir.LiteralFloat {
			return e.Kind, 0, e.FloatValue, true
		}
	case *ir.UnaryExpr:
		if e.Operator == ir.UnaryNegate {
			kind, integer, floating, ok = nativeNumeric(e.Operand)
			return kind, -integer, -floating, ok
		}
	}
	return 0, 0, 0, false
}
func (s *nativeVector) number(e ir.Expr) bool {
	kind, integer, floating, ok := nativeNumeric(e)
	if !ok {
		return false
	}
	if kind == ir.LiteralInt {
		if integer >= -10 && integer <= 97 {
			s.emit(1)
		} else {
			s.literal(fmt.Sprintf("i:%d", integer))
		}
	} else {
		if floating == 0 {
			floating = 0
		}
		s.literal(fmt.Sprintf("f:%x", math.Float64bits(floating)))
	}
	s.push(NativeValueOrigin{Kind: NativeExpression, Node: e})
	return true
}
