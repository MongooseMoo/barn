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
func (s *nativeVector) varOp(name string) {
	s.emit(1)
	if s.global.names.slots[canonicalIdentifier(name)] >= 32 {
		s.varRef(name)
	}
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
