package compiler

import (
	"encoding/binary"
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/types"
)

// barnImportValue describes an existing value; it never reevaluates MOO code.
type barnImportValue struct {
	origin     NativeValueOrigin
	literal    types.Value
	isLiteral  bool
	projection uint8
}

const (
	importIdentity uint8 = iota
	importValues
	importKeys
	importLength
	importCursor
)

type barnImportCall struct {
	IP       int
	Stack    []barnImportValue
	Locals   map[int]barnImportValue
	Handlers []barnImportHandler
}

type barnImportHandler struct {
	Node       ir.Node
	SetupIP    int
	StackDepth int
}

type barnImportNode struct {
	node     ir.Node
	stack    []barnImportValue
	handlers []barnImportHandler
}

// barnImportLayout observes the instructions the existing lowerer emits. The
// verifier's authoritative stack effects also drive this symbolic value stack.
type barnImportLayout struct {
	nodes       []barnImportNode
	stack       []barnImportValue
	locals      map[int]barnImportValue
	localOwners map[int]ir.Node
	calls       map[ir.Node]barnImportCall
	handlers    []barnImportHandler
	pending     int
	pendingNode ir.Node
	err         error
}

func newBarnImportLayout() *barnImportLayout {
	return &barnImportLayout{
		locals: make(map[int]barnImportValue), localOwners: make(map[int]ir.Node),
		calls: make(map[ir.Node]barnImportCall), pending: -1,
	}
}

func (s *barnImportLayout) currentNode() ir.Node {
	if len(s.nodes) == 0 {
		return nil
	}
	return s.nodes[len(s.nodes)-1].node
}

func (s *barnImportLayout) emit(c *lowerer, ip int) {
	s.flush(c)
	s.pending, s.pendingNode = ip, s.currentNode()
}

func (s *barnImportLayout) enter(c *lowerer, node ir.Node) func() {
	s.flush(c)
	entry := append([]barnImportValue(nil), s.stack...)
	entryHandlers := append([]barnImportHandler(nil), s.handlers...)
	s.nodes = append(s.nodes, barnImportNode{node: node, stack: entry, handlers: entryHandlers})
	return func() {
		s.flush(c)
		if _, expression := node.(ir.Expr); expression {
			s.stack = append(entry, barnImportValue{origin: NativeValueOrigin{Kind: NativeExpression, Node: node}})
		} else {
			s.stack = entry
			if statement, ok := node.(ir.Stmt); ok && isLoopStmt(statement) {
				s.stack = append(s.stack, barnImportValue{literal: types.NewInt(0), isLiteral: true})
			}
		}
		switch node.(type) {
		case *ir.VerbCallExpr, *ir.BuiltinCallExpr:
			locals := make(map[int]barnImportValue)
			for slot, value := range s.locals {
				// A call's own argument/name helpers have already been consumed.
				// Ancestor expression helpers remain live across this call.
				if s.localOwners[slot] == node {
					continue
				}
				for _, active := range s.nodes {
					if s.localOwners[slot] == active.node {
						locals[slot] = value
						break
					}
				}
			}
			s.calls[node] = barnImportCall{IP: len(c.program.Code), Stack: append([]barnImportValue(nil), s.stack...), Locals: locals, Handlers: append([]barnImportHandler(nil), s.handlers...)}
		}
		s.nodes = s.nodes[:len(s.nodes)-1]
		s.handlers = entryHandlers
	}
}

// resetBranch starts a separately compiled arm with its enclosing expression's
// incoming operand stack rather than the previously emitted arm's result.
func (s *barnImportLayout) resetBranch(c *lowerer) {
	s.flush(c)
	if len(s.nodes) != 0 {
		s.stack = append([]barnImportValue(nil), s.nodes[len(s.nodes)-1].stack...)
	}
}

func (s *barnImportLayout) flush(c *lowerer) {
	if s.pending < 0 || s.err != nil {
		return
	}
	ip, node := s.pending, s.pendingNode
	s.pending = -1
	op, operands := bytecode.OpCode(c.program.Code[ip]), c.program.Code[ip+1:]
	pops, pushes, err := bytecode.InstructionStackEffect(op, operands)
	if err != nil {
		s.err = err
		return
	}
	if pops > len(s.stack) {
		s.err = fmt.Errorf("native import symbolic stack underflow at Barn IP %d (%s)", ip, op)
		return
	}
	consumed := append([]barnImportValue(nil), s.stack[len(s.stack)-pops:]...)
	s.stack = s.stack[:len(s.stack)-pops]
	value := barnImportValue{origin: NativeValueOrigin{Kind: NativeExpression, Node: node}}
	switch {
	case bytecode.IsImmediateInt(op):
		value = barnImportValue{literal: types.NewInt(int64(bytecode.GetImmediateValue(op))), isLiteral: true}
	case op == bytecode.OP_PUSH:
		value = barnImportValue{literal: c.program.Constants[operands[0]], isLiteral: true}
	case op == bytecode.OP_PUSH_WIDE:
		value = barnImportValue{literal: c.program.Constants[binary.BigEndian.Uint16(operands)], isLiteral: true}
	case op == bytecode.OP_PUSH_INT:
		value = barnImportValue{literal: types.NewInt(int64(binary.BigEndian.Uint64(operands))), isLiteral: true}
	case op == bytecode.OP_GET_VAR:
		if saved, ok := s.locals[int(operands[0])]; ok {
			value = saved
		}
	case op == bytecode.OP_SET_VAR || op == bytecode.OP_SET_LOCAL:
		s.locals[int(operands[0])] = consumed[0]
	case op == bytecode.OP_DUP:
		value = consumed[0]
	case op == bytecode.OP_MAKE_LIST:
		value.origin = NativeValueOrigin{Kind: NativeListPrefix, Node: node, Aux: int(operands[0])}
	case op == bytecode.OP_LIST_APPEND || op == bytecode.OP_LIST_EXTEND:
		value.origin = NativeValueOrigin{Kind: NativeListPrefix, Node: node, Aux: consumed[0].origin.Aux + 1}
	case op == bytecode.OP_MAKE_MAP:
		value.origin = NativeValueOrigin{Kind: NativeMapPrefix, Node: node}
	case op == bytecode.OP_INDEX_SET:
		slot := int(operands[0])
		prior := s.locals[slot]
		if prior.origin.Kind == NativeMapPrefix {
			prior.origin.Aux++
			s.locals[slot] = prior
		}
	case op == bytecode.OP_ITER_PREP_COLUMNS:
		container := barnImportValue{origin: NativeValueOrigin{Kind: NativeLoopContainer, Node: node}, projection: importValues}
		keys := container
		keys.projection = importKeys
		s.stack = append(s.stack, container, keys)
		return
	case op == bytecode.OP_LENGTH:
		value = consumed[0]
		value.projection = importLength
	case op == bytecode.OP_TRY_EXCEPT || op == bytecode.OP_TRY_EXCEPT_WIDE || op == bytecode.OP_TRY_EXCEPT_LOCAL_WIDE || op == bytecode.OP_TRY_FINALLY || op == bytecode.OP_TRY_FINALLY_WIDE:
		s.handlers = append(s.handlers, barnImportHandler{Node: node, SetupIP: ip, StackDepth: len(s.stack)})
	case op == bytecode.OP_END_EXCEPT:
		if len(s.handlers) != 0 && s.handlers[len(s.handlers)-1].Node == node {
			s.handlers = s.handlers[:len(s.handlers)-1]
		}
	case op == bytecode.OP_END_FINALLY || op == bytecode.OP_END_FINALLY_WIDE:
		if len(s.handlers) != 0 && s.handlers[len(s.handlers)-1].Node == node {
			s.handlers = s.handlers[:len(s.handlers)-1]
		}
	}
	for range pushes {
		s.stack = append(s.stack, value)
	}
}
