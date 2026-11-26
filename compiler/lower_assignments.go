package compiler

import (
	"fmt"
	"slices"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/types"
)

type indexBoundaryContext byte

const (
	indexBoundaryIndex indexBoundaryContext = iota
	indexBoundaryRange
)

// compileAssign compiles an assignment expression
func (c *lowerer) compileAssign(n *ir.AssignExpr) error {
	if target, ok := n.Target.(*ir.PropertyTarget); ok {
		// Sequential property assignments have disjoint temporary lifetimes, so
		// reuse their slots. Nested assignments increment the depth and therefore
		// retain distinct live slots while their outer assignment is suspended.
		depth := c.propertyAssignDepth
		c.propertyAssignDepth++
		defer func() { c.propertyAssignDepth-- }()

		// Property targets are evaluated before the assigned value. Capture the
		// object and dynamic name because compiling the value may mutate either.
		if err := c.compileNode(target.Object); err != nil {
			return err
		}
		objectVar := c.declareInternalVariable(fmt.Sprintf("__propassignobj_depth_%d__", depth))
		c.emitStoreLocal(objectVar)

		nameVar := -1
		if target.Name == "" {
			if target.NameExpr == nil {
				return fmt.Errorf("property expression has neither static name nor dynamic expression")
			}
			if err := c.compileNode(target.NameExpr); err != nil {
				return err
			}
			nameVar = c.declareInternalVariable(fmt.Sprintf("__propassignname_depth_%d__", depth))
			c.emitStoreLocal(nameVar)
		}

		if err := c.compileNode(n.Value); err != nil {
			return err
		}
		c.emit(bytecode.OP_DUP)
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(objectVar))
		if target.Name != "" {
			c.emitStaticNameOperation(bytecode.OP_SET_PROP, bytecode.OP_SET_PROP_WIDE, target.Name)
		} else {
			c.emit(bytecode.OP_GET_VAR)
			c.emitByte(byte(nameVar))
			c.emit(bytecode.OP_SET_PROP_DYNAMIC)
		}
		return nil
	}
	if target, ok := n.Target.(*ir.IndexTarget); ok {
		return c.compileIndexAssign(target, n.Value)
	}
	if target, ok := n.Target.(*ir.RangeTarget); ok {
		return c.compileRangeAssign(target, n.Value)
	}

	// Compile value
	if err := c.compileNode(n.Value); err != nil {
		return err
	}

	// Duplicate value (assignment returns the value)
	c.emit(bytecode.OP_DUP)

	// Handle different target types
	switch target := n.Target.(type) {
	case *ir.VariableTarget:
		// Simple variable assignment
		idx := c.declareVariable(target.Name)
		c.emit(bytecode.OP_SET_VAR)
		c.emitByte(byte(idx))
	case *ir.DestructuringTarget:
		return c.compileDestructuringTarget(target)
	default:
		return fmt.Errorf("invalid assignment target: %T", target)
	}

	return nil
}

// compileRangeIndex compiles a range index expression against its collection,
// the value of target held in varIdx. bytecode.OP_INDEX_MARKER resolves ^ and
// $ while preserving those semantic operations in the compiled program.
func (c *lowerer) compileRangeIndex(expr ir.Expr, varIdx int, target ir.CollectionTarget) error {
	if !containsIndexBoundary(expr) {
		return c.compileNode(expr)
	}

	oldContextVar := c.indexContextVar
	oldBoundaryContext := c.indexBoundaryContext

	tempIdx := c.collectionAssignTemp("rngsetctx", 0)
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(varIdx))
	c.emitStoreLocal(tempIdx)
	c.markHeldTarget(tempIdx, target)

	c.indexContextVar = tempIdx
	c.indexBoundaryContext = indexBoundaryRange

	err := c.compileNode(expr)

	c.indexContextVar = oldContextVar
	c.indexBoundaryContext = oldBoundaryContext

	return err
}

// markHeldTarget names the compiler temporary slot as holding target's value,
// which Toast holds as an operand while it evaluates the indexes after it
// (nativeVector.target), so a task suspended there converts to and from
// Toast's frame.
func (c *lowerer) markHeldTarget(slot int, target ir.CollectionTarget) {
	if c.importLayout != nil {
		c.importLayout.flush(c)
		c.importLayout.locals[slot] = barnImportValue{origin: NativeValueOrigin{Kind: NativeExpression, Node: target}}
	}
}

// enterCollectionAssign opens one index or range assignment and returns the
// function that closes it. Sequential collection assignments have disjoint
// temporary lifetimes, so they reuse slots; an assignment nested anywhere in
// another one's target or value runs one depth deeper and keeps distinct slots
// while the outer assignment's temporaries are live.
func (c *lowerer) enterCollectionAssign() func() {
	c.collectionAssignDepth++
	return func() { c.collectionAssignDepth-- }
}

// collectionAssignTemp returns the slot for the innermost active collection
// assignment's temporary of the given kind and position.
func (c *lowerer) collectionAssignTemp(kind string, position int) int {
	return c.declareInternalVariable(fmt.Sprintf("__%s_%d_depth_%d__", kind, position, c.collectionAssignDepth))
}

// propertyWriteback describes how a collection assignment stores its updated
// base. For a property it is the retained object and name; with variableSlot
// set (heldVariable), the base is a variable whose value was held in a
// temporary and is stored back with one OP_PUT.
type propertyWriteback struct {
	objectVar    int
	nameVar      int
	name         string
	heldVariable bool
	variableSlot int
}

// prepareCollectionBase evaluates a collection target before the assignment
// value. Property objects and dynamic names are retained for the eventual
// write-back, so no target expression is evaluated twice. keyedBase fetches a
// static property with OP_GET_PROP_KEYBASE, for an assignment that writes it
// back with OP_SET_PROP_KEYED. operands are the index, range bound and value
// expressions the assignment evaluates after its base.
func (c *lowerer) prepareCollectionBase(target ir.CollectionTarget, keyedBase bool, operands ...ir.Expr) (int, *propertyWriteback, error) {
	if variable, ok := target.(*ir.VariableTarget); ok {
		// Toast pushes the base variable before it generates any index, range
		// bound or value (code_gen.cc push_lvalue, EXPR_ASGN), so an unassigned
		// base raises E_VARNF before any of them runs. Both instructions are
		// tick-free, as Toast's OP_PUSH is.
		slot := c.declareVariable(variable.Name)
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(slot))
		if !slices.ContainsFunc(operands, func(operand ir.Expr) bool { return assignsVariable(operand, variable.Name) }) {
			// Nothing after the base can change the variable, so the
			// assignment updates the slot in place and this read is only
			// the check.
			c.emit(bytecode.OP_POP)
			return slot, nil, nil
		}
		// An operand assigns the variable. Toast updates the value it pushed
		// here and stores the result over whatever the operand assigned
		// (l = {1, 2}; l[l = 1] = 5 leaves l == {5, 2}), so hold that value.
		baseVar := c.collectionAssignTemp("collectionbase", 0)
		c.emitStoreLocal(baseVar)
		c.markHeldTarget(baseVar, variable)
		return baseVar, &propertyWriteback{objectVar: -1, nameVar: -1, heldVariable: true, variableSlot: slot}, nil
	}
	property, ok := target.(*ir.PropertyTarget)
	if !ok {
		return 0, nil, fmt.Errorf("collection assignment target must be a variable or property")
	}

	if err := c.compileNode(property.Object); err != nil {
		return 0, nil, err
	}
	writeback := &propertyWriteback{
		objectVar: c.collectionAssignTemp("collectionobj", 0),
		nameVar:   -1,
		name:      property.Name,
	}
	c.emitStoreLocal(writeback.objectVar)

	if property.Name == "" {
		if property.NameExpr == nil {
			return 0, nil, fmt.Errorf("property expression has neither static name nor dynamic expression")
		}
		if err := c.compileNode(property.NameExpr); err != nil {
			return 0, nil, err
		}
		writeback.nameVar = c.collectionAssignTemp("collectionname", 0)
		c.emitStoreLocal(writeback.nameVar)
	}

	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(writeback.objectVar))
	if property.Name != "" && keyedBase {
		c.emit(bytecode.OP_GET_PROP_KEYBASE)
		c.emitShort(uint16(c.addConstant(types.NewStr(property.Name))))
	} else if property.Name != "" {
		c.emitStaticNameOperation(bytecode.OP_GET_PROP, bytecode.OP_GET_PROP_WIDE, property.Name)
	} else {
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(writeback.nameVar))
		c.emit(bytecode.OP_GET_PROP_DYNAMIC)
	}
	baseVar := c.collectionAssignTemp("collectionbase", 0)
	c.emitStoreLocal(baseVar)
	c.markHeldTarget(baseVar, property)
	return baseVar, writeback, nil
}

func (c *lowerer) emitCollectionWriteback(baseVar int, writeback *propertyWriteback) {
	if writeback == nil {
		return
	}
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(baseVar))
	if writeback.heldVariable {
		c.emit(bytecode.OP_SET_VAR) // Toast's closing OP_PUT
		c.emitByte(byte(writeback.variableSlot))
		return
	}
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(writeback.objectVar))
	if writeback.name != "" {
		c.emitStaticNameOperation(bytecode.OP_SET_PROP, bytecode.OP_SET_PROP_WIDE, writeback.name)
	} else {
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(writeback.nameVar))
		c.emit(bytecode.OP_SET_PROP_DYNAMIC)
	}
}

func (c *lowerer) compileIndexForCollection(expr ir.Expr, collectionVar int) error {
	oldContextVar := c.indexContextVar
	oldBoundaryContext := c.indexBoundaryContext
	defer func() {
		c.indexContextVar = oldContextVar
		c.indexBoundaryContext = oldBoundaryContext
	}()
	if containsIndexBoundary(expr) {
		c.indexContextVar = collectionVar
		c.indexBoundaryContext = indexBoundaryIndex
	}
	return c.compileNode(expr)
}

// collectionIndices returns target's index expressions from the outermost
// collection inward, the index target each one completes, and the base
// collection they index.
func collectionIndices(target *ir.IndexTarget) ([]ir.Expr, []*ir.IndexTarget, ir.CollectionTarget) {
	var indices []ir.Expr
	var targets []*ir.IndexTarget
	var base ir.CollectionTarget = target
	for {
		index, ok := base.(*ir.IndexTarget)
		if !ok {
			break
		}
		indices = append(indices, index.Index)
		targets = append(targets, index)
		base = index.Collection
	}
	slices.Reverse(indices)
	slices.Reverse(targets)
	return indices, targets, base
}

func (c *lowerer) compileIndexAssign(target *ir.IndexTarget, value ir.Expr) error {
	defer c.enterCollectionAssign()()
	indices, targets, base := collectionIndices(target)

	// obj.prop[key] = v with a static name and no ^ or $ in key changes one
	// entry of the property's value; the store can track it per key.
	property, isProperty := base.(*ir.PropertyTarget)
	keyed := isProperty && property.Name != "" && len(indices) == 1 && !containsIndexBoundary(indices[0])
	baseVar, writeback, err := c.prepareCollectionBase(base, keyed, append(slices.Clone(indices), value)...)
	if err != nil {
		return err
	}
	indexVars := make([]int, len(indices))
	intermediateVars := make([]int, len(indices)-1)
	collectionVar := baseVar
	for i, index := range indices {
		if err := c.compileIndexForCollection(index, collectionVar); err != nil {
			return err
		}
		indexVars[i] = c.collectionAssignTemp("assignindex", i)
		c.emitStoreLocal(indexVars[i])
		if i < len(indices)-1 {
			c.emit(bytecode.OP_GET_VAR)
			c.emitByte(byte(collectionVar))
			c.emit(bytecode.OP_GET_VAR)
			c.emitByte(byte(indexVars[i]))
			c.emit(bytecode.OP_PUSH_REF) // tick-free, like Toast's
			intermediateVars[i] = c.collectionAssignTemp("assignintermediate", i)
			c.emitStoreLocal(intermediateVars[i])
			c.markHeldTarget(intermediateVars[i], targets[i])
			collectionVar = intermediateVars[i]
		}
	}

	if err := c.compileNode(value); err != nil {
		return err
	}
	c.emit(bytecode.OP_DUP)
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(indexVars[len(indexVars)-1]))
	c.emitCollectionIndexSet(collectionVar, baseVar, writeback)

	for i := len(intermediateVars) - 1; i >= 0; i-- {
		childVar := intermediateVars[i]
		parentVar := baseVar
		if i > 0 {
			parentVar = intermediateVars[i-1]
		}
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(childVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(indexVars[i]))
		c.emitCollectionIndexSet(parentVar, baseVar, writeback)
	}
	if keyed {
		// OP_SET_PROP_KEYED pops obj, key, value: the key rides between them.
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(baseVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(indexVars[0]))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(writeback.objectVar))
		c.emit(bytecode.OP_SET_PROP_KEYED)
		c.emitShort(uint16(c.addConstant(types.NewStr(writeback.name))))
		return nil
	}
	c.emitCollectionWriteback(baseVar, writeback)
	return nil
}

// emitCollectionIndexSet emits OP_INDEX_SET (Toast's OP_INDEXSET, one tick)
// into target. When target is the assignment's own variable, the same
// instruction also performs Toast's closing OP_PUT, so it carries both ticks.
func (c *lowerer) emitCollectionIndexSet(target, baseVar int, writeback *propertyWriteback) {
	if target == baseVar && writeback == nil {
		c.emitTicks(2)
	}
	c.emit(bytecode.OP_INDEX_SET)
	c.emitByte(byte(target))
}

func (c *lowerer) compileRangeAssign(target *ir.RangeTarget, value ir.Expr) error {
	defer c.enterCollectionAssign()()
	if nested, ok := target.Collection.(*ir.IndexTarget); ok {
		return c.compileNestedRangeAssign(nested, target.Start, target.End, value)
	}
	baseVar, writeback, err := c.prepareCollectionBase(target.Collection, false, target.Start, target.End, value)
	if err != nil {
		return err
	}
	if err := c.compileRangeIndex(target.Start, baseVar, target.Collection); err != nil {
		return err
	}
	startVar := c.collectionAssignTemp("rangestart", 0)
	c.emitStoreLocal(startVar)
	if err := c.compileRangeIndex(target.End, baseVar, target.Collection); err != nil {
		return err
	}
	endVar := c.collectionAssignTemp("rangeend", 0)
	c.emitStoreLocal(endVar)

	if err := c.compileNode(value); err != nil {
		return err
	}
	c.emit(bytecode.OP_DUP)
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(startVar))
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(endVar))
	if writeback == nil {
		c.emitTicks(1) // EOP_RANGESET is free; this carries the variable's OP_PUT
	}
	c.emit(bytecode.OP_RANGE_SET)
	c.emitByte(byte(baseVar))
	c.emitCollectionWriteback(baseVar, writeback)
	return nil
}

// compileNestedRangeAssign lowers nested range assignments while retaining all
// target components before evaluating value.
func (c *lowerer) compileNestedRangeAssign(indexTarget *ir.IndexTarget, start, end, value ir.Expr) error {
	defer c.enterCollectionAssign()()
	indices, targets, base := collectionIndices(indexTarget)

	baseVar, writeback, err := c.prepareCollectionBase(base, false, append(slices.Clone(indices), start, end, value)...)
	if err != nil {
		return err
	}
	indexVars := make([]int, len(indices))
	collectionVars := make([]int, len(indices))
	collectionVar := baseVar
	for i, index := range indices {
		if err := c.compileIndexForCollection(index, collectionVar); err != nil {
			return err
		}
		indexVars[i] = c.collectionAssignTemp("nestedrangeindex", i)
		c.emitStoreLocal(indexVars[i])
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(collectionVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(indexVars[i]))
		c.emit(bytecode.OP_PUSH_REF) // tick-free, like Toast's
		collectionVars[i] = c.collectionAssignTemp("nestedrangecollection", i)
		c.emitStoreLocal(collectionVars[i])
		c.markHeldTarget(collectionVars[i], targets[i])
		collectionVar = collectionVars[i]
	}
	innerVar := collectionVars[len(collectionVars)-1]

	if err := c.compileRangeIndex(start, innerVar, indexTarget); err != nil {
		return err
	}
	startVar := c.collectionAssignTemp("nestedrangestart", 0)
	c.emitStoreLocal(startVar)
	if err := c.compileRangeIndex(end, innerVar, indexTarget); err != nil {
		return err
	}
	endVar := c.collectionAssignTemp("nestedrangeend", 0)
	c.emitStoreLocal(endVar)

	if err := c.compileNode(value); err != nil {
		return err
	}
	c.emit(bytecode.OP_DUP)
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(startVar))
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(endVar))
	c.emit(bytecode.OP_RANGE_SET)
	c.emitByte(byte(innerVar))

	for i := len(collectionVars) - 1; i >= 0; i-- {
		childVar := collectionVars[i]
		parentVar := baseVar
		if i > 0 {
			parentVar = collectionVars[i-1]
		}
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(childVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(indexVars[i]))
		c.emitCollectionIndexSet(parentVar, baseVar, writeback)
	}
	c.emitCollectionWriteback(baseVar, writeback)
	return nil
}

func (c *lowerer) compileIndexBoundary(n *ir.IndexBoundaryExpr) error {
	if c.indexContextVar >= 0 {
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(c.indexContextVar))
		c.emit(bytecode.OP_INDEX_MARKER)
		if c.indexBoundaryContext == indexBoundaryRange {
			if n.Boundary == ir.IndexFirst {
				c.emitByte(bytecode.RangeMarkerFirst)
			} else {
				c.emitByte(bytecode.RangeMarkerLast)
			}
		} else if n.Boundary == ir.IndexFirst {
			c.emitByte(bytecode.IndexMarkerFirst)
		} else {
			c.emitByte(bytecode.IndexMarkerLast)
		}
		return nil
	}

	if n.Boundary == ir.IndexFirst {
		c.emitConstant(types.NewInt(1))
		return nil
	}

	// No index context (shouldn't happen for well-formed index/range
	// expressions). Fall back to -1, which produces E_RANGE at runtime.
	c.emitConstant(types.NewInt(-1))

	return nil
}

func (c *lowerer) compileExprStmt(n *ir.ExprStmt) error {
	// Effect-context fast path: a simple variable assignment used as a statement
	// does not need its value. Emit the store directly, skipping the assignment's
	// value-preserving bytecode.OP_DUP and the trailing bytecode.OP_POP (dead value shuffling that
	// otherwise dominates assignment-heavy loops). Complex targets (scatter /
	// index / property) keep the general value-producing path below.
	if assign, ok := n.Expr.(*ir.AssignExpr); ok {
		if ident, ok := assign.Target.(*ir.VariableTarget); ok {
			// Self-add idiom: x = x + expr. This statement does not need the
			// assignment's result value, so emit the compact compatibility opcode.
			// Its VM handler shares the normal `+` implementation for every type.
			if bin, ok := assign.Value.(*ir.BinaryExpr); ok && bin.Operator == ir.BinaryAdd {
				if leftIdent, ok := bin.Left.(*ir.IdentifierExpr); ok && canonicalIdentifier(leftIdent.Name) == canonicalIdentifier(ident.Name) {
					idx := c.declareVariable(ident.Name)
					c.emit(bytecode.OP_GET_VAR)
					c.emitByte(byte(idx))
					if err := c.compileNode(bin.Right); err != nil {
						return err
					}
					c.emit(bytecode.OP_STRING_APPEND)
					c.emit(bytecode.OP_SET_VAR)
					c.emitByte(byte(idx))
					return nil
				}
			}

			// Self-append idiom: v = {@v, e1, ...}. Instead of building a fresh
			// list (copy all of v via LIST_EXTEND) and reassigning, append the
			// trailing elements directly onto v. With the in-place Append path
			// this turns an O(n^2) build loop into amortized O(n); the result is
			// identical (v's elements followed by the trailing items).
			if list, ok := assign.Value.(*ir.ListExpr); ok && len(list.Elements) > 0 {
				if sp, ok := list.Elements[0].(*ir.SpliceExpr); ok {
					if spIdent, ok := sp.Expr.(*ir.IdentifierExpr); ok && canonicalIdentifier(spIdent.Name) == canonicalIdentifier(ident.Name) {
						idx := c.declareVariable(ident.Name)
						c.emit(bytecode.OP_GET_VAR)
						c.emitByte(byte(idx))
						// Toast checks the leading @v with one
						// CHECK_LIST_FOR_SPLICE tick. The first append
						// carries it (Toast's own append opcodes are
						// free); a lone {@v} uses OP_SPLICE itself.
						if len(list.Elements) == 1 {
							c.emit(bytecode.OP_SPLICE)
						}
						if c.importLayout != nil {
							// v's value is the list Toast holds after splicing @v.
							c.importLayout.markListPrefix(c, list)
						}
						for i, elem := range list.Elements[1:] {
							if splice, ok := elem.(*ir.SpliceExpr); ok {
								if err := c.compileNode(splice.Expr); err != nil {
									return err
								}
								if i == 0 {
									c.emitTicks(1)
								}
								c.emit(bytecode.OP_LIST_EXTEND)
							} else {
								if err := c.compileNode(elem); err != nil {
									return err
								}
								if i == 0 {
									c.emitTicks(1)
								}
								c.emit(bytecode.OP_LIST_APPEND)
							}
						}
						c.emit(bytecode.OP_SET_VAR)
						c.emitByte(byte(idx))
						return nil
					}
				}
			}

			if err := c.compileNode(assign.Value); err != nil {
				return err
			}
			idx := c.declareVariable(ident.Name)
			c.emit(bytecode.OP_SET_VAR)
			c.emitByte(byte(idx))
			return nil
		}
	}

	// Compile expression
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Pop result (expression statement doesn't use result)
	c.emit(bytecode.OP_POP)
	return nil
}

// containsIndexBoundary reports whether expr contains a ^/$ index boundary bound
// to the *current* indexing context — i.e. one not already shadowed by a
// nested index/range expression's own brackets (which establish their own
// context for any ^/$ inside them). It must recurse into every expression
// kind that can hold a child expression so a boundary nested arbitrarily deep
// (e.g. inside a function call argument, list/map literal element, or
// assignment value) is still detected; under-detection silently falls back
// to an unbound boundary (compiles to a literal -1), not a compile error.
// Over-detection is harmless: nested index/range expressions always push
// and restore their own context around their own Index/Start/End fields.
func containsIndexBoundary(expr ir.Expr) bool {
	switch n := expr.(type) {
	case nil:
		return false
	case *ir.IndexBoundaryExpr:
		return n.Boundary == ir.IndexLast || n.Boundary == ir.IndexFirst
	case *ir.BinaryExpr:
		return containsIndexBoundary(n.Left) || containsIndexBoundary(n.Right)
	case *ir.UnaryExpr:
		return containsIndexBoundary(n.Operand)
	case *ir.TernaryExpr:
		return containsIndexBoundary(n.Condition) || containsIndexBoundary(n.ThenExpr) || containsIndexBoundary(n.ElseExpr)
	case *ir.IndexExpr:
		// n.Index is scoped to this IndexExpr's own brackets; only n.Expr
		// (the collection being indexed) is in the enclosing context.
		return containsIndexBoundary(n.Expr)
	case *ir.RangeExpr:
		// n.Start/n.End are scoped to this RangeExpr's own brackets.
		return containsIndexBoundary(n.Expr)
	case *ir.PropertyExpr:
		return containsIndexBoundary(n.Expr) || containsIndexBoundary(n.PropertyExpr)
	case *ir.VerbCallExpr:
		if containsIndexBoundary(n.Expr) || containsIndexBoundary(n.VerbExpr) {
			return true
		}
		for _, arg := range n.Args {
			if containsIndexBoundary(arg) {
				return true
			}
		}
		return false
	case *ir.BuiltinCallExpr:
		for _, arg := range n.Args {
			if containsIndexBoundary(arg) {
				return true
			}
		}
		return false
	case *ir.SpliceExpr:
		return containsIndexBoundary(n.Expr)
	case *ir.CatchExpr:
		for _, code := range n.Codes {
			if containsIndexBoundary(code) {
				return true
			}
		}
		return containsIndexBoundary(n.Expr) || containsIndexBoundary(n.Default)
	case *ir.AssignExpr:
		return containsTargetIndexBoundary(n.Target) || containsIndexBoundary(n.Value)
	case *ir.ListExpr:
		for _, el := range n.Elements {
			if containsIndexBoundary(el) {
				return true
			}
		}
		return false
	case *ir.ListRangeExpr:
		return containsIndexBoundary(n.Start) || containsIndexBoundary(n.End)
	case *ir.MapExpr:
		for _, pair := range n.Pairs {
			if containsIndexBoundary(pair.Key) || containsIndexBoundary(pair.Value) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func containsTargetIndexBoundary(target ir.Target) bool {
	switch target := target.(type) {
	case *ir.VariableTarget:
		return false
	case *ir.PropertyTarget:
		return containsIndexBoundary(target.Object) || containsIndexBoundary(target.NameExpr)
	case *ir.IndexTarget:
		return containsTargetIndexBoundary(target.Collection) || containsIndexBoundary(target.Index)
	case *ir.RangeTarget:
		return containsTargetIndexBoundary(target.Collection) || containsIndexBoundary(target.Start) || containsIndexBoundary(target.End)
	case *ir.DestructuringTarget:
		for _, binding := range target.Bindings {
			if optional, ok := binding.(*ir.OptionalBinding); ok && containsIndexBoundary(optional.Default) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// assignsVariable reports whether evaluating expr can assign the variable
// name, directly or by an index, range or scatter assignment. An expression
// kind it does not know is reported as assigning, so a caller relying on a
// false answer stays correct.
func assignsVariable(expr ir.Expr, name string) bool {
	assigns := func(exprs ...ir.Expr) bool {
		return slices.ContainsFunc(exprs, func(e ir.Expr) bool { return assignsVariable(e, name) })
	}
	switch n := expr.(type) {
	case nil:
		return false
	case *ir.LiteralExpr, *ir.IdentifierExpr, *ir.IndexBoundaryExpr:
		return false
	case *ir.UnaryExpr:
		return assigns(n.Operand)
	case *ir.BinaryExpr:
		return assigns(n.Left, n.Right)
	case *ir.TernaryExpr:
		return assigns(n.Condition, n.ThenExpr, n.ElseExpr)
	case *ir.IndexExpr:
		return assigns(n.Expr, n.Index)
	case *ir.RangeExpr:
		return assigns(n.Expr, n.Start, n.End)
	case *ir.PropertyExpr:
		return assigns(n.Expr, n.PropertyExpr)
	case *ir.VerbCallExpr:
		return assigns(n.Expr, n.VerbExpr) || assigns(n.Args...)
	case *ir.BuiltinCallExpr:
		return assigns(n.Args...)
	case *ir.SpliceExpr:
		return assigns(n.Expr)
	case *ir.CatchExpr:
		return assigns(n.Expr, n.Default) || assigns(n.Codes...)
	case *ir.AssignExpr:
		return targetAssignsVariable(n.Target, name) || assigns(n.Value)
	case *ir.ListExpr:
		return assigns(n.Elements...)
	case *ir.ListRangeExpr:
		return assigns(n.Start, n.End)
	case *ir.MapExpr:
		for _, pair := range n.Pairs {
			if assigns(pair.Key, pair.Value) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// targetAssignsVariable reports whether assigning to target can assign the
// variable name, including through the expressions inside target.
func targetAssignsVariable(target ir.Target, name string) bool {
	switch target := target.(type) {
	case *ir.VariableTarget:
		return canonicalIdentifier(target.Name) == canonicalIdentifier(name)
	case *ir.PropertyTarget:
		return assignsVariable(target.Object, name) || assignsVariable(target.NameExpr, name)
	case *ir.IndexTarget:
		return targetAssignsVariable(target.Collection, name) || assignsVariable(target.Index, name)
	case *ir.RangeTarget:
		return targetAssignsVariable(target.Collection, name) || assignsVariable(target.Start, name) || assignsVariable(target.End, name)
	case *ir.DestructuringTarget:
		for _, binding := range target.Bindings {
			switch binding := binding.(type) {
			case *ir.RequiredBinding:
				if canonicalIdentifier(binding.Name) == canonicalIdentifier(name) {
					return true
				}
			case *ir.OptionalBinding:
				if canonicalIdentifier(binding.Name) == canonicalIdentifier(name) || assignsVariable(binding.Default, name) {
					return true
				}
			case *ir.RestBinding:
				if canonicalIdentifier(binding.Name) == canonicalIdentifier(name) {
					return true
				}
			default:
				return true
			}
		}
		return false
	default:
		return true
	}
}

func (c *lowerer) compileDestructuringTarget(target *ir.DestructuringTarget) error {
	// Scatter assignment: {a, ?b = default, @rest} = list
	//
	// OP_SCATTER is Toast's EOP_SCATTER (execute.cc): one instruction and one
	// tick that checks the list and binds every target an element reaches,
	// before any default runs. The defaults follow in target order, as in
	// Toast's code (code_gen.cc EXPR_SCATTER), each run for an optional target
	// the list was too short to reach. Toast jumps to the first of those and
	// falls through the rest; here each tests the list's length, which says the
	// same: optional targets get the elements beyond the required ones from the
	// left, so the k-th is unreached when the list has fewer than required+k.
	numRequired := 0
	restIndex := -1
	type compiledBinding struct {
		name     string
		optional bool
		rest     bool
		default_ ir.Expr
	}
	bindings := make([]compiledBinding, len(target.Bindings))
	for i, binding := range target.Bindings {
		switch binding := binding.(type) {
		case *ir.RequiredBinding:
			bindings[i].name = binding.Name
		case *ir.OptionalBinding:
			bindings[i] = compiledBinding{name: binding.Name, optional: true, default_: binding.Default}
		case *ir.RestBinding:
			bindings[i] = compiledBinding{name: binding.Name, rest: true}
		}
		if bindings[i].rest {
			restIndex = i
			continue
		}
		if !bindings[i].optional {
			numRequired++
		}
	}
	hasDefault := false
	slots := make([]int, len(bindings))
	for i, binding := range bindings {
		slots[i] = c.declareVariable(binding.name)
		hasDefault = hasDefault || binding.default_ != nil
	}

	// OP_SCATTER pops the copy compileAssign made; the assignment's value stays
	// on the stack as its result, as Toast keeps the list.
	c.emit(bytecode.OP_SCATTER)
	c.emitByte(byte(len(bindings)))
	c.emitByte(byte(numRequired))
	if restIndex >= 0 {
		c.emitByte(byte(restIndex + 1))
	} else {
		c.emitByte(byte(len(bindings) + 1))
	}
	for i, binding := range bindings {
		c.emitByte(byte(slots[i]))
		if binding.optional {
			c.emitByte(1)
		} else {
			c.emitByte(0)
		}
	}
	if !hasDefault {
		return nil
	}

	// Sequential scatters have disjoint temporary lifetimes, so reuse the
	// slot. A scatter nested in a default expression increments the depth and
	// keeps a distinct slot while the outer scatter's length is live.
	depth := c.scatterDepth
	c.scatterDepth++
	defer func() { c.scatterDepth-- }()
	lenVar := c.declareInternalVariable(fmt.Sprintf("__scatter_len_depth_%d__", depth))
	c.emit(bytecode.OP_DUP)
	c.emit(bytecode.OP_LENGTH)
	c.emitStoreLocal(lenVar)

	// The length tests are bookkeeping inside EOP_SCATTER's tick; only a
	// default expression and its OP_PUT are charged.
	optionals := 0
	for i, binding := range bindings {
		if !binding.optional {
			continue
		}
		optionals++
		if binding.default_ == nil {
			continue // an unreached target without a default keeps its value
		}
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(lenVar))
		c.emitIntLiteral(int64(numRequired + optionals))
		c.emitTicks(0)
		c.emit(bytecode.OP_LT)
		c.emitTicks(0)
		reached := c.emitJump(bytecode.OP_JUMP_IF_FALSE)
		if err := c.compileNode(binding.default_); err != nil {
			return err
		}
		c.emit(bytecode.OP_SET_VAR)
		c.emitByte(byte(slots[i]))
		c.patchJump(reached)
	}
	return nil
}
