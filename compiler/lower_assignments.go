package compiler

import (
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/verb"
)

type indexBoundaryContext byte

const (
	indexBoundaryIndex indexBoundaryContext = iota
	indexBoundaryRange
)

// compileAssign compiles an assignment expression
func (c *lowerer) compileAssign(n *verb.AssignExpr) error {
	if target, ok := n.Target.(*verb.PropertyTarget); ok {
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
	if target, ok := n.Target.(*verb.IndexTarget); ok {
		return c.compileIndexAssign(target, n.Value)
	}
	if target, ok := n.Target.(*verb.RangeTarget); ok {
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
	case *verb.VariableTarget:
		// Simple variable assignment
		idx := c.declareVariable(target.Name)
		c.emit(bytecode.OP_SET_VAR)
		c.emitByte(byte(idx))
	case *verb.DestructuringTarget:
		return c.compileDestructuringTarget(target)
	default:
		return fmt.Errorf("invalid assignment target: %T", target)
	}

	return nil
}

// compileRangeIndex compiles a range index expression against its collection.
// bytecode.OP_INDEX_MARKER resolves ^ and $ while preserving those semantic operations
// in the compiled program.
func (c *lowerer) compileRangeIndex(expr verb.Expr, varIdx int) error {
	if !containsIndexBoundary(expr) {
		return c.compileNode(expr)
	}

	oldContextVar := c.indexContextVar
	oldBoundaryContext := c.indexBoundaryContext

	tempIdx := c.declareInternalVariable(c.tempVar("rngsetctx"))
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(varIdx))
	c.emitStoreLocal(tempIdx)

	c.indexContextVar = tempIdx
	c.indexBoundaryContext = indexBoundaryRange

	err := c.compileNode(expr)

	c.indexContextVar = oldContextVar
	c.indexBoundaryContext = oldBoundaryContext

	return err
}

type propertyWriteback struct {
	objectVar int
	nameVar   int
	name      string
}

// prepareCollectionBase evaluates a collection target before the assignment
// value. Property objects and dynamic names are retained for the eventual
// write-back, so no target expression is evaluated twice.
func (c *lowerer) prepareCollectionBase(target verb.CollectionTarget) (int, *propertyWriteback, error) {
	if variable, ok := target.(*verb.VariableTarget); ok {
		return c.declareVariable(variable.Name), nil, nil
	}
	property, ok := target.(*verb.PropertyTarget)
	if !ok {
		return 0, nil, fmt.Errorf("collection assignment target must be a variable or property")
	}

	if err := c.compileNode(property.Object); err != nil {
		return 0, nil, err
	}
	writeback := &propertyWriteback{
		objectVar: c.declareInternalVariable(c.tempVar("collectionobj")),
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
		writeback.nameVar = c.declareInternalVariable(c.tempVar("collectionname"))
		c.emitStoreLocal(writeback.nameVar)
	}

	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(writeback.objectVar))
	if property.Name != "" {
		c.emitStaticNameOperation(bytecode.OP_GET_PROP, bytecode.OP_GET_PROP_WIDE, property.Name)
	} else {
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(writeback.nameVar))
		c.emit(bytecode.OP_GET_PROP_DYNAMIC)
	}
	baseVar := c.declareInternalVariable(c.tempVar("collectionbase"))
	c.emitStoreLocal(baseVar)
	return baseVar, writeback, nil
}

func (c *lowerer) emitCollectionWriteback(baseVar int, writeback *propertyWriteback) {
	if writeback == nil {
		return
	}
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(baseVar))
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

func (c *lowerer) compileIndexForCollection(expr verb.Expr, collectionVar int) error {
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

func (c *lowerer) compileIndexAssign(target *verb.IndexTarget, value verb.Expr) error {
	var indices []verb.Expr
	var base verb.CollectionTarget = target
	for {
		index, ok := base.(*verb.IndexTarget)
		if !ok {
			break
		}
		indices = append(indices, index.Index)
		base = index.Collection
	}
	for left, right := 0, len(indices)-1; left < right; left, right = left+1, right-1 {
		indices[left], indices[right] = indices[right], indices[left]
	}

	baseVar, writeback, err := c.prepareCollectionBase(base)
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
		indexVars[i] = c.declareInternalVariable(c.tempVar("assignindex"))
		c.emitStoreLocal(indexVars[i])
		if i < len(indices)-1 {
			c.emit(bytecode.OP_GET_VAR)
			c.emitByte(byte(collectionVar))
			c.emit(bytecode.OP_GET_VAR)
			c.emitByte(byte(indexVars[i]))
			c.emitTicks(0) // Toast's OP_PUSH_REF is tick-free
			c.emit(bytecode.OP_INDEX)
			intermediateVars[i] = c.declareInternalVariable(c.tempVar("assignintermediate"))
			c.emitStoreLocal(intermediateVars[i])
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

func (c *lowerer) compileRangeAssign(target *verb.RangeTarget, value verb.Expr) error {
	if nested, ok := target.Collection.(*verb.IndexTarget); ok {
		return c.compileNestedRangeAssign(nested, target.Start, target.End, value)
	}
	baseVar, writeback, err := c.prepareCollectionBase(target.Collection)
	if err != nil {
		return err
	}
	if err := c.compileRangeIndex(target.Start, baseVar); err != nil {
		return err
	}
	startVar := c.declareInternalVariable(c.tempVar("rangestart"))
	c.emitStoreLocal(startVar)
	if err := c.compileRangeIndex(target.End, baseVar); err != nil {
		return err
	}
	endVar := c.declareInternalVariable(c.tempVar("rangeend"))
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
func (c *lowerer) compileNestedRangeAssign(indexTarget *verb.IndexTarget, start, end, value verb.Expr) error {
	var indices []verb.Expr
	var base verb.CollectionTarget = indexTarget
	for {
		index, ok := base.(*verb.IndexTarget)
		if !ok {
			break
		}
		indices = append(indices, index.Index)
		base = index.Collection
	}
	for left, right := 0, len(indices)-1; left < right; left, right = left+1, right-1 {
		indices[left], indices[right] = indices[right], indices[left]
	}

	baseVar, writeback, err := c.prepareCollectionBase(base)
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
		indexVars[i] = c.declareInternalVariable(c.tempVar("nestedrangeindex"))
		c.emitStoreLocal(indexVars[i])
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(collectionVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(indexVars[i]))
		c.emitTicks(0) // Toast's OP_PUSH_REF is tick-free
		c.emit(bytecode.OP_INDEX)
		collectionVars[i] = c.declareInternalVariable(c.tempVar("nestedrangecollection"))
		c.emitStoreLocal(collectionVars[i])
		collectionVar = collectionVars[i]
	}
	innerVar := collectionVars[len(collectionVars)-1]

	if err := c.compileRangeIndex(start, innerVar); err != nil {
		return err
	}
	startVar := c.declareInternalVariable(c.tempVar("nestedrangestart"))
	c.emitStoreLocal(startVar)
	if err := c.compileRangeIndex(end, innerVar); err != nil {
		return err
	}
	endVar := c.declareInternalVariable(c.tempVar("nestedrangeend"))
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

func (c *lowerer) compileIndexBoundary(n *verb.IndexBoundaryExpr) error {
	if c.indexContextVar >= 0 {
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(c.indexContextVar))
		c.emit(bytecode.OP_INDEX_MARKER)
		if c.indexBoundaryContext == indexBoundaryRange {
			if n.Boundary == verb.IndexFirst {
				c.emitByte(bytecode.RangeMarkerFirst)
			} else {
				c.emitByte(bytecode.RangeMarkerLast)
			}
		} else if n.Boundary == verb.IndexFirst {
			c.emitByte(bytecode.IndexMarkerFirst)
		} else {
			c.emitByte(bytecode.IndexMarkerLast)
		}
		return nil
	}

	if n.Boundary == verb.IndexFirst {
		c.emitConstant(types.NewInt(1))
		return nil
	}

	// No index context (shouldn't happen for well-formed index/range
	// expressions). Fall back to -1, which produces E_RANGE at runtime.
	c.emitConstant(types.NewInt(-1))

	return nil
}

func (c *lowerer) compileExprStmt(n *verb.ExprStmt) error {
	// Guard against nil expression (e.g. bare semicolons)
	if n.Expr == nil {
		return nil
	}

	// Effect-context fast path: a simple variable assignment used as a statement
	// does not need its value. Emit the store directly, skipping the assignment's
	// value-preserving bytecode.OP_DUP and the trailing bytecode.OP_POP (dead value shuffling that
	// otherwise dominates assignment-heavy loops). Complex targets (scatter /
	// index / property) keep the general value-producing path below.
	if assign, ok := n.Expr.(*verb.AssignExpr); ok {
		if ident, ok := assign.Target.(*verb.VariableTarget); ok {
			// Self-add idiom: x = x + expr. This statement does not need the
			// assignment's result value, so emit the compact compatibility opcode.
			// Its VM handler shares the normal `+` implementation for every type.
			if bin, ok := assign.Value.(*verb.BinaryExpr); ok && bin.Operator == verb.BinaryAdd {
				if leftIdent, ok := bin.Left.(*verb.IdentifierExpr); ok && canonicalIdentifier(leftIdent.Name) == canonicalIdentifier(ident.Name) {
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
			if list, ok := assign.Value.(*verb.ListExpr); ok && len(list.Elements) > 0 {
				if sp, ok := list.Elements[0].(*verb.SpliceExpr); ok {
					if spIdent, ok := sp.Expr.(*verb.IdentifierExpr); ok && canonicalIdentifier(spIdent.Name) == canonicalIdentifier(ident.Name) {
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
						for i, elem := range list.Elements[1:] {
							if splice, ok := elem.(*verb.SpliceExpr); ok {
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
func containsIndexBoundary(expr verb.Expr) bool {
	switch n := expr.(type) {
	case nil:
		return false
	case *verb.IndexBoundaryExpr:
		return n.Boundary == verb.IndexLast || n.Boundary == verb.IndexFirst
	case *verb.BinaryExpr:
		return containsIndexBoundary(n.Left) || containsIndexBoundary(n.Right)
	case *verb.UnaryExpr:
		return containsIndexBoundary(n.Operand)
	case *verb.TernaryExpr:
		return containsIndexBoundary(n.Condition) || containsIndexBoundary(n.ThenExpr) || containsIndexBoundary(n.ElseExpr)
	case *verb.IndexExpr:
		// n.Index is scoped to this IndexExpr's own brackets; only n.Expr
		// (the collection being indexed) is in the enclosing context.
		return containsIndexBoundary(n.Expr)
	case *verb.RangeExpr:
		// n.Start/n.End are scoped to this RangeExpr's own brackets.
		return containsIndexBoundary(n.Expr)
	case *verb.PropertyExpr:
		return containsIndexBoundary(n.Expr) || containsIndexBoundary(n.PropertyExpr)
	case *verb.VerbCallExpr:
		if containsIndexBoundary(n.Expr) || containsIndexBoundary(n.VerbExpr) {
			return true
		}
		for _, arg := range n.Args {
			if containsIndexBoundary(arg) {
				return true
			}
		}
		return false
	case *verb.BuiltinCallExpr:
		for _, arg := range n.Args {
			if containsIndexBoundary(arg) {
				return true
			}
		}
		return false
	case *verb.SpliceExpr:
		return containsIndexBoundary(n.Expr)
	case *verb.CatchExpr:
		return containsIndexBoundary(n.Expr) || containsIndexBoundary(n.Default)
	case *verb.AssignExpr:
		return containsTargetIndexBoundary(n.Target) || containsIndexBoundary(n.Value)
	case *verb.ListExpr:
		for _, el := range n.Elements {
			if containsIndexBoundary(el) {
				return true
			}
		}
		return false
	case *verb.ListRangeExpr:
		return containsIndexBoundary(n.Start) || containsIndexBoundary(n.End)
	case *verb.MapExpr:
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

func containsTargetIndexBoundary(target verb.Target) bool {
	switch target := target.(type) {
	case *verb.VariableTarget:
		return false
	case *verb.PropertyTarget:
		return containsIndexBoundary(target.Object) || containsIndexBoundary(target.NameExpr)
	case *verb.IndexTarget:
		return containsTargetIndexBoundary(target.Collection) || containsIndexBoundary(target.Index)
	case *verb.RangeTarget:
		return containsTargetIndexBoundary(target.Collection) || containsIndexBoundary(target.Start) || containsIndexBoundary(target.End)
	case *verb.DestructuringTarget:
		for _, binding := range target.Bindings {
			if optional, ok := binding.(*verb.OptionalBinding); ok && containsIndexBoundary(optional.Default) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (c *lowerer) compileDestructuringTarget(target *verb.DestructuringTarget) error {
	// Scatter assignment: {a, ?b, @rest} = list
	//
	// Runtime strategy:
	// 1. Validate list shape via bytecode.OP_SCATTER.
	// 2. Track two cursors (left/right) into the list.
	// 3. Bind suffix targets (after @rest) from the right.
	// 4. Bind prefix targets from the left.
	// 5. Bind @rest to the remaining slice between left..right.
	numRequired := 0
	numOptional := 0
	restIndex := -1
	type compiledBinding struct {
		name     string
		optional bool
		rest     bool
		default_ verb.Expr
	}
	bindings := make([]compiledBinding, len(target.Bindings))
	for i, binding := range target.Bindings {
		switch binding := binding.(type) {
		case *verb.RequiredBinding:
			bindings[i].name = binding.Name
		case *verb.OptionalBinding:
			bindings[i] = compiledBinding{name: binding.Name, optional: true, default_: binding.Default}
		case *verb.RestBinding:
			bindings[i] = compiledBinding{name: binding.Name, rest: true}
		}
		if bindings[i].rest {
			restIndex = i
			continue
		}
		if bindings[i].optional {
			numOptional++
		} else {
			numRequired++
		}
	}
	hasRest := restIndex >= 0

	listVar := c.declareInternalVariable(c.tempVar("scatter_list"))
	lenVar := c.declareInternalVariable(c.tempVar("scatter_len"))
	leftVar := c.declareInternalVariable(c.tempVar("scatter_left"))
	rightVar := c.declareInternalVariable(c.tempVar("scatter_right"))

	c.emitStoreLocal(listVar)

	// Preserve the original assignment value while validating the stored copy.
	// OP_SCATTER is Toast's EOP_SCATTER, which binds every target for its one
	// tick; the bookkeeping below is therefore tick-free, and only an optional
	// target's default expression and its OP_PUT are charged.
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(listVar))
	c.emit(bytecode.OP_SCATTER)
	c.emitByte(byte(numRequired))
	c.emitByte(byte(numOptional))
	if hasRest {
		c.emitByte(1)
	} else {
		c.emitByte(0)
	}

	// len = length(list)
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(listVar))
	c.emit(bytecode.OP_LENGTH)
	c.emitStoreLocal(lenVar)

	// left = 1
	if op, ok := bytecode.MakeImmediateOpcode(1); ok {
		c.emit(op)
	}
	c.emitStoreLocal(leftVar)

	// right = len
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(lenVar))
	c.emitStoreLocal(rightVar)

	// countRequired returns number of required non-rest targets in [start, end].
	countRequired := func(start, end int) int {
		count := 0
		for i := start; i <= end && i < len(bindings); i++ {
			if i < 0 {
				continue
			}
			binding := bindings[i]
			if !binding.rest && !binding.optional {
				count++
			}
		}
		return count
	}

	// emitTake binds target = list[cursor] and steps the cursor toward the
	// middle: forward from the left (step 0) or backward from the right (1).
	emitTake := func(targetVar, cursorVar int, step byte) {
		c.emit(bytecode.OP_SCATTER_TAKE)
		c.emitByte(byte(listVar))
		c.emitByte(byte(cursorVar))
		c.emitByte(byte(targetVar))
		c.emitByte(step)
	}

	emitOptionalCondition := func(requiredReserve int) {
		// (right - left + 1) > requiredReserve
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(rightVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(leftVar))
		c.emitTicks(0)
		c.emit(bytecode.OP_SUB)
		if op, ok := bytecode.MakeImmediateOpcode(1); ok {
			c.emit(op)
		}
		c.emitTicks(0)
		c.emit(bytecode.OP_ADD)
		c.emitIntLiteral(int64(requiredReserve))
		c.emitTicks(0)
		c.emit(bytecode.OP_GT)
		c.emitTicks(0)
	}

	emitOptionalMissingValue := func(binding compiledBinding, targetVar int) error {
		if binding.default_ != nil {
			if err := c.compileNode(binding.default_); err != nil {
				return err
			}
			c.emit(bytecode.OP_SET_VAR)
			c.emitByte(byte(targetVar))
		}
		// When no default is specified, leave the variable as-is.
		return nil
	}

	// Bind suffix targets from the right when @rest is present.
	if hasRest {
		for i := len(bindings) - 1; i > restIndex; i-- {
			binding := bindings[i]
			if binding.rest {
				continue
			}
			targetVar := c.declareVariable(binding.name)
			if binding.optional {
				requiredBefore := countRequired(0, i-1)
				emitOptionalCondition(requiredBefore)
				elseJump := c.emitJump(bytecode.OP_JUMP_IF_FALSE)

				emitTake(targetVar, rightVar, 1)
				endJump := c.emitJump(bytecode.OP_JUMP)

				c.patchJump(elseJump)
				if err := emitOptionalMissingValue(binding, targetVar); err != nil {
					return err
				}
				c.patchJump(endJump)
			} else {
				emitTake(targetVar, rightVar, 1)
			}
		}
	}

	// Bind prefix targets from the left.
	prefixEnd := len(bindings) - 1
	if hasRest {
		prefixEnd = restIndex - 1
	}
	for i := 0; i <= prefixEnd; i++ {
		binding := bindings[i]
		if binding.rest {
			continue
		}

		targetVar := c.declareVariable(binding.name)
		if binding.optional {
			requiredAfter := countRequired(i+1, prefixEnd)
			emitOptionalCondition(requiredAfter)
			elseJump := c.emitJump(bytecode.OP_JUMP_IF_FALSE)

			emitTake(targetVar, leftVar, 0)
			endJump := c.emitJump(bytecode.OP_JUMP)

			c.patchJump(elseJump)
			if err := emitOptionalMissingValue(binding, targetVar); err != nil {
				return err
			}
			c.patchJump(endJump)
		} else {
			emitTake(targetVar, leftVar, 0)
		}
	}

	// Bind @rest to the remaining middle slice.
	if hasRest {
		restBinding := bindings[restIndex]
		restVar := c.declareVariable(restBinding.name)

		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(leftVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(rightVar))
		c.emitTicks(0)
		c.emit(bytecode.OP_LE)
		c.emitTicks(0)
		elseJump := c.emitJump(bytecode.OP_JUMP_IF_FALSE)

		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(listVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(leftVar))
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(rightVar))
		c.emitTicks(0)
		c.emit(bytecode.OP_RANGE)
		c.emitStoreLocal(restVar)
		endJump := c.emitJump(bytecode.OP_JUMP)

		c.patchJump(elseJump)
		c.emit(bytecode.OP_MAKE_LIST)
		c.emitByte(0)
		c.emitStoreLocal(restVar)
		c.patchJump(endJump)
	}

	return nil
}
