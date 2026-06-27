package scheduler

import (
	"strings"

	"barn/parser"
	"barn/task"
	"barn/types"
)

type propertyAccess struct {
	obj  types.ObjID
	name string
}

type accessFootprint struct {
	propertyReads  map[propertyAccess]struct{}
	propertyWrites map[propertyAccess]struct{}
	unknown        bool
}

func analyzeAccessFootprint(stmts []parser.Stmt, knownObjects map[string]types.ObjID) accessFootprint {
	analyzer := footprintAnalyzer{
		knownObjects: knownObjects,
		footprint: accessFootprint{
			propertyReads:  make(map[propertyAccess]struct{}),
			propertyWrites: make(map[propertyAccess]struct{}),
		},
	}
	for _, stmt := range stmts {
		analyzer.stmt(stmt)
	}
	return analyzer.footprint
}

func analyzeTaskAccessFootprint(t *task.Task) accessFootprint {
	if t == nil || t.BytecodeVMValue() != nil {
		return unknownAccessFootprint()
	}
	stmts, ok := t.Code.([]parser.Stmt)
	if !ok {
		return unknownAccessFootprint()
	}
	return analyzeAccessFootprint(stmts, knownTaskObjects(t))
}

func unknownAccessFootprint() accessFootprint {
	return accessFootprint{
		propertyReads:  make(map[propertyAccess]struct{}),
		propertyWrites: make(map[propertyAccess]struct{}),
		unknown:        true,
	}
}

func knownTaskObjects(t *task.Task) map[string]types.ObjID {
	known := map[string]types.ObjID{
		"player":     t.Owner,
		"programmer": t.Programmer,
		"this":       t.This,
		"caller":     t.Caller,
		"dobj":       t.Dobj,
		"iobj":       t.Iobj,
	}
	for name, objID := range known {
		if objID == types.ObjNothing {
			delete(known, name)
		}
	}
	return known
}

func accessFootprintsCommute(left, right accessFootprint) bool {
	if left.unknown || right.unknown {
		return false
	}
	for write := range left.propertyWrites {
		if _, ok := right.propertyWrites[write]; ok {
			return false
		}
		if _, ok := right.propertyReads[write]; ok {
			return false
		}
	}
	for write := range right.propertyWrites {
		if _, ok := left.propertyReads[write]; ok {
			return false
		}
	}
	return true
}

type footprintAnalyzer struct {
	knownObjects map[string]types.ObjID
	footprint    accessFootprint
}

func (a *footprintAnalyzer) markUnknown() {
	a.footprint.unknown = true
}

func (a *footprintAnalyzer) read(access propertyAccess) {
	a.footprint.propertyReads[access] = struct{}{}
}

func (a *footprintAnalyzer) write(access propertyAccess) {
	a.footprint.propertyWrites[access] = struct{}{}
}

func (a *footprintAnalyzer) stmt(stmt parser.Stmt) {
	switch n := stmt.(type) {
	case *parser.ExprStmt:
		a.expr(n.Expr)
	case *parser.IfStmt:
		a.expr(n.Condition)
		for _, stmt := range n.Body {
			a.stmt(stmt)
		}
		for _, clause := range n.ElseIfs {
			a.expr(clause.Condition)
			for _, stmt := range clause.Body {
				a.stmt(stmt)
			}
		}
		for _, stmt := range n.Else {
			a.stmt(stmt)
		}
	case *parser.WhileStmt:
		a.expr(n.Condition)
		for _, stmt := range n.Body {
			a.stmt(stmt)
		}
	case *parser.ForStmt:
		a.expr(n.Container)
		a.expr(n.RangeStart)
		a.expr(n.RangeEnd)
		for _, stmt := range n.Body {
			a.stmt(stmt)
		}
	case *parser.BreakStmt:
		// break carries only an optional loop label (no expression) — nothing to analyze.
	case *parser.ReturnStmt:
		a.expr(n.Value)
	case *parser.TryExceptStmt:
		for _, stmt := range n.Body {
			a.stmt(stmt)
		}
		for _, clause := range n.Excepts {
			for _, stmt := range clause.Body {
				a.stmt(stmt)
			}
		}
	case *parser.TryFinallyStmt:
		for _, stmt := range n.Body {
			a.stmt(stmt)
		}
		for _, stmt := range n.Finally {
			a.stmt(stmt)
		}
	case *parser.TryExceptFinallyStmt:
		for _, stmt := range n.Body {
			a.stmt(stmt)
		}
		for _, clause := range n.Excepts {
			for _, stmt := range clause.Body {
				a.stmt(stmt)
			}
		}
		for _, stmt := range n.Finally {
			a.stmt(stmt)
		}
	case *parser.ScatterStmt:
		a.expr(n.Value)
		for _, target := range n.Targets {
			a.expr(target.Default)
		}
	case *parser.ForkStmt:
		a.expr(n.Delay)
		a.markUnknown()
	}
}

func (a *footprintAnalyzer) expr(expr parser.Expr) {
	switch n := expr.(type) {
	case nil:
		return
	case *parser.LiteralExpr, *parser.IdentifierExpr, *parser.IndexMarkerExpr:
		return
	case *parser.UnaryExpr:
		a.expr(n.Operand)
	case *parser.BinaryExpr:
		a.expr(n.Left)
		a.expr(n.Right)
	case *parser.TernaryExpr:
		a.expr(n.Condition)
		a.expr(n.ThenExpr)
		a.expr(n.ElseExpr)
	case *parser.ParenExpr:
		a.expr(n.Expr)
	case *parser.AssignExpr:
		a.expr(n.Value)
		a.assignmentTarget(n.Target)
	case *parser.IndexExpr:
		a.expr(n.Expr)
		a.expr(n.Index)
	case *parser.RangeExpr:
		a.expr(n.Expr)
		a.expr(n.Start)
		a.expr(n.End)
	case *parser.PropertyExpr:
		a.expr(n.Expr)
		a.expr(n.PropertyExpr)
		if access, ok := a.staticPropertyAccess(n); ok {
			a.read(access)
		} else {
			a.markUnknown()
		}
	case *parser.VerbCallExpr:
		a.expr(n.Expr)
		a.expr(n.VerbExpr)
		for _, arg := range n.Args {
			a.expr(arg)
		}
		a.markUnknown()
	case *parser.BuiltinCallExpr:
		a.builtinCall(n)
	case *parser.SpliceExpr:
		a.expr(n.Expr)
	case *parser.CatchExpr:
		a.expr(n.Expr)
		a.expr(n.Default)
	case *parser.ListExpr:
		for _, elem := range n.Elements {
			a.expr(elem)
		}
	case *parser.ListRangeExpr:
		a.expr(n.Start)
		a.expr(n.End)
	case *parser.MapExpr:
		for _, pair := range n.Pairs {
			a.expr(pair.Key)
			a.expr(pair.Value)
		}
	}
}

func (a *footprintAnalyzer) assignmentTarget(target parser.Expr) {
	switch n := target.(type) {
	case nil:
		return
	case *parser.IdentifierExpr:
		return
	case *parser.PropertyExpr:
		a.expr(n.Expr)
		a.expr(n.PropertyExpr)
		if access, ok := a.staticPropertyAccess(n); ok {
			a.write(access)
		} else {
			a.markUnknown()
		}
	case *parser.IndexExpr:
		a.expr(n.Index)
		a.collectionMutationTarget(n.Expr)
	case *parser.RangeExpr:
		a.expr(n.Start)
		a.expr(n.End)
		a.collectionMutationTarget(n.Expr)
	default:
		a.expr(n)
		a.markUnknown()
	}
}

func (a *footprintAnalyzer) collectionMutationTarget(target parser.Expr) {
	switch n := target.(type) {
	case nil:
		return
	case *parser.PropertyExpr:
		a.expr(n)
		if access, ok := a.staticPropertyAccess(n); ok {
			a.write(access)
		} else {
			a.markUnknown()
		}
	case *parser.IndexExpr:
		a.expr(n.Index)
		a.collectionMutationTarget(n.Expr)
	case *parser.RangeExpr:
		a.expr(n.Start)
		a.expr(n.End)
		a.collectionMutationTarget(n.Expr)
	case *parser.IdentifierExpr:
		return
	default:
		a.expr(n)
	}
}

func (a *footprintAnalyzer) builtinCall(call *parser.BuiltinCallExpr) {
	for _, arg := range call.Args {
		a.expr(arg)
	}

	switch strings.ToLower(call.Name) {
	case "add_property", "delete_property", "set_property_info", "clear_property":
		if access, ok := a.staticBuiltinPropertyAccess(call); ok {
			a.write(access)
		} else {
			a.markUnknown()
		}
	case "property_info", "is_clear_property":
		if access, ok := a.staticBuiltinPropertyAccess(call); ok {
			a.read(access)
		} else {
			a.markUnknown()
		}
	case "properties":
		a.markUnknown()
	case "eval", "call_function", "pass":
		a.markUnknown()
	default:
		a.markUnknown()
	}
}

func (a *footprintAnalyzer) staticBuiltinPropertyAccess(call *parser.BuiltinCallExpr) (propertyAccess, bool) {
	if len(call.Args) < 2 {
		return propertyAccess{}, false
	}
	objID, ok := a.staticObjectID(call.Args[0])
	if !ok {
		return propertyAccess{}, false
	}
	name, ok := staticString(call.Args[1])
	if !ok {
		return propertyAccess{}, false
	}
	return propertyAccess{obj: objID, name: canonicalPropertyName(name)}, true
}

func (a *footprintAnalyzer) staticPropertyAccess(expr *parser.PropertyExpr) (propertyAccess, bool) {
	objID, ok := a.staticObjectID(expr.Expr)
	if !ok {
		return propertyAccess{}, false
	}
	name, ok := staticPropertyName(expr)
	if !ok {
		return propertyAccess{}, false
	}
	return propertyAccess{obj: objID, name: canonicalPropertyName(name)}, true
}

func (a *footprintAnalyzer) staticObjectID(expr parser.Expr) (types.ObjID, bool) {
	switch n := expr.(type) {
	case *parser.LiteralExpr:
		if n.Kind == parser.LiteralObj {
			return types.ObjID(n.ObjID), true
		}
	case *parser.IdentifierExpr:
		if a.knownObjects != nil {
			objID, ok := a.knownObjects[strings.ToLower(n.Name)]
			return objID, ok
		}
	case *parser.ParenExpr:
		return a.staticObjectID(n.Expr)
	}
	return types.ObjNothing, false
}

func staticPropertyName(expr *parser.PropertyExpr) (string, bool) {
	if expr.Property != "" {
		return expr.Property, true
	}
	return staticString(expr.PropertyExpr)
}

func staticString(expr parser.Expr) (string, bool) {
	switch n := expr.(type) {
	case *parser.LiteralExpr:
		if n.Kind == parser.LiteralString {
			return n.StringValue, true
		}
	case *parser.ParenExpr:
		return staticString(n.Expr)
	}
	return "", false
}

func canonicalPropertyName(name string) string {
	return strings.ToLower(name)
}
