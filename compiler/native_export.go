package compiler

import (
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Toast's checkpoint type codes for its control-stack markers (db_io.cc
// dbio_write_var): a catch marker holds its handler count, a finally marker
// its handler pc.
const (
	nativeCatchType   = 7
	nativeFinallyType = 8
)

// ExportNativeFrame is ConvertNativeFrame's inverse over the same tables: it
// returns frame as Toast holds the same activation paused at the same call,
// in the form the checkpoint reader produces from a Toast file, and the Toast
// vector it runs in. Top is true for the task's running frame, paused in a
// builtin; every other frame is paused in a verb call. A frame Toast cannot
// hold, or whose Toast operands Barn no longer has, is an error.
func (c *Compiler) ExportNativeFrame(frame task.VMFrameSnapshot, top bool) (task.VMFrameSnapshot, int, error) {
	switch {
	case frame.Native != nil:
		return frame, 0, fmt.Errorf("frame is already in Toast's form")
	case frame.BuiltinPC != 0 || frame.MoveContinuation != nil || frame.RecycleContinuation != nil:
		return frame, 0, fmt.Errorf("builtin continuation has no Toast form")
	case len(frame.PendingReasons) != 0:
		return frame, 0, fmt.Errorf("pending finally reason has no Toast form")
	case frame.ReturnLocal != 0:
		return frame, 0, fmt.Errorf("WAIF setter result local has no Toast form")
	}
	origin := frame.Program.Origin
	languageVersion, vector, fork := 17, -1, 0
	switch {
	case origin.Native:
		languageVersion, vector = origin.LanguageVersion, origin.NativeVector
	case origin.Fork > 0:
		fork = origin.Fork
	case origin.Fork < 0:
		return frame, 0, fmt.Errorf("program is not a fork body")
	}
	compiled, layout, _, lower, err := c.compileNativeVector(frame.Program.Source, languageVersion, vector, newBarnImportLayout())
	if err != nil {
		return frame, 0, err
	}
	importing := lower.importLayout
	// The tables describe the frame only when they describe its program.
	described, offset, outer := compiled, 0, 0
	if fork != 0 {
		start, _, ok := compiled.ForkBody(fork)
		if !ok || fork > len(importing.forkDepths) {
			return frame, 0, fmt.Errorf("source has no fork body %d", fork)
		}
		described, offset, outer = compiled.ExtractFork(fork), start, importing.forkDepths[fork-1]
	}
	same := *described
	same.Origin = origin
	if same.Fingerprint() != frame.Program.Fingerprint() {
		return frame, 0, fmt.Errorf("program is not the compile the Toast tables describe")
	}

	var node ir.Node
	for candidate, call := range importing.calls {
		if call.IP != offset+frame.IP {
			continue
		}
		if node != nil {
			return frame, 0, fmt.Errorf("instruction position %d ends two calls", frame.IP)
		}
		node = candidate
	}
	if node == nil {
		return frame, 0, fmt.Errorf("instruction position %d is not paused at a call", frame.IP)
	}
	var site *NativeContinuation
	var siteVector *NativeVectorLayout
	for index := range layout.Vectors {
		for continuation := range layout.Vectors[index].Continuations {
			if layout.Vectors[index].Continuations[continuation].Node == node {
				site, siteVector = &layout.Vectors[index].Continuations[continuation], &layout.Vectors[index]
			}
		}
	}
	if site == nil || (origin.Native && site.Vector != vector) || (fork != 0 && site.Vector < 0) {
		return frame, 0, fmt.Errorf("call has no Toast continuation in the frame's vector")
	}
	for _, control := range site.Controls {
		switch control.Kind {
		case NativeFinallyHandler:
			return frame, 0, fmt.Errorf("pending finally reason has no Toast form")
		}
	}

	// What Barn holds, by the Toast origin it was computed from.
	barn := importing.calls[node]
	held := len(frame.Stack)
	if top {
		held-- // the yielded builtin's resume-value slot
	}
	if held < 0 || outer+held != len(barn.Stack)-1 {
		return frame, 0, fmt.Errorf("operand stack of %d does not match its call site", len(frame.Stack))
	}
	values := make(nativeHeld)
	record := func(value barnImportValue, actual types.Value) {
		if !value.isLiteral {
			values[nativeHeldKey{value.origin, value.projection}] = actual
		}
	}
	for index := range held {
		record(barn.Stack[outer+index], frame.Stack[index])
	}
	gap := lower.importSlotGap(compiled)
	for slot, value := range barn.Locals {
		if local := slot - gap; local >= 0 && local < len(frame.Locals) {
			record(value, frame.Locals[local])
		}
	}

	stack := make([]types.Value, 0, len(site.Stack)-1)
	operandTypes := make([]int, 0, len(site.Stack)-1)
	for _, wanted := range site.Stack[:len(site.Stack)-1] {
		kind := 0
		var value types.Value
		switch wanted.Kind {
		case NativeCatchMarker:
			kind, value = nativeCatchType, types.NewInt(int64(wanted.Aux))
		case NativeFinallyMarker:
			kind, value = nativeFinallyType, types.NewInt(int64(wanted.LabelPC))
		case NativeHandlerLabel:
			value = types.NewInt(int64(wanted.LabelPC))
		default:
			value, err = nativeOperand(wanted, values)
			if err != nil {
				return frame, 0, err
			}
		}
		stack = append(stack, value)
		operandTypes = append(operandTypes, kind)
	}
	if len(stack) > siteVector.MaxStack {
		return frame, 0, fmt.Errorf("native stack of %d exceeds its max_stack %d", len(stack), siteVector.MaxStack)
	}

	exported := frame
	exported.Program = bytecode.Program{Source: frame.Program.Source}
	exported.Locals = nil
	for _, name := range layout.VariableNames {
		if value, ok := nativeVariable(frame, name); ok {
			exported.Program.VarNames = append(exported.Program.VarNames, name)
			exported.Locals = append(exported.Locals, value)
		}
	}
	exported.Program.NumLocals = len(exported.Locals)
	exported.Stack = stack
	exported.IP = site.PC
	exported.ExceptStack = nil
	// Toast's error pc is where the instruction it is executing starts
	// (execute.cc next_opcode: error_bv = bv): OP_BI_FUNC_CALL and its one-byte
	// function number, or the one-byte OP_CALL_VERB.
	errorPC := site.PC - 1
	if _, builtin := node.(*ir.BuiltinCallExpr); builtin {
		errorPC = site.PC - 2
	}
	exported.Native = &task.NativeFrameSnapshot{
		LanguageVersion: languageVersion,
		ErrorPC:         errorPC,
		OperandTypes:    operandTypes,
		Sentinel:        types.NewInt(-111),
		Temporary:       types.Unbound,
	}
	return exported, site.Vector, nil
}

// nativeHeldKey names a value Barn holds: the Toast origin it was computed
// from and the import's projection of it (identity, or a loop column/cursor).
type nativeHeldKey struct {
	origin     NativeValueOrigin
	projection uint8
}

type nativeHeld map[nativeHeldKey]types.Value

// nativeOperand finds the value Toast holds for wanted among the values Barn
// holds, or computes a compile-time one.
func nativeOperand(wanted NativeValueOrigin, values nativeHeld) (types.Value, error) {
	if value, ok := values[nativeHeldKey{wanted, importIdentity}]; ok {
		return value, nil
	}
	switch wanted.Kind {
	case NativeLoopContainer, NativeLoopCursor:
		return nativeLoopOperand(wanted, values)
	case NativeExpression:
		if literal, ok := wanted.Node.(*ir.LiteralExpr); ok {
			return valueFromLiteral(literal)
		}
		// Barn may already have appended an argument Toast still holds alone.
		for key, value := range values {
			origin := key.origin
			if key.projection != importIdentity || origin.Kind != NativeListPrefix || value.Type() != types.TYPE_LIST {
				continue
			}
			for index, element := range nativePrefixExpressions(origin) {
				if element == wanted.Node && index < value.Len() {
					return value.Get(index + 1), nil
				}
			}
		}
	case NativeListPrefix:
		expressions := nativePrefixExpressions(wanted)
		if wanted.Aux <= len(expressions) {
			elements := make([]types.Value, 0, wanted.Aux)
			for _, expression := range expressions[:wanted.Aux] {
				if _, splice := expression.(*ir.SpliceExpr); splice {
					return types.None, fmt.Errorf("native list prefix holds a splice")
				}
				element, err := nativeOperand(NativeValueOrigin{Kind: NativeExpression, Node: expression}, values)
				if err != nil {
					return types.None, err
				}
				elements = append(elements, element)
			}
			return types.NewList(elements), nil
		}
	case NativePropertyName:
		switch node := wanted.Node.(type) {
		case *ir.PropertyExpr:
			return types.NewStr(node.Property), nil
		case *ir.VerbCallExpr:
			return types.NewStr(node.Verb), nil
		case *ir.PropertyTarget:
			return types.NewStr(node.Name), nil
		}
	case NativeExceptionCodes:
		return nativeExceptionCodes(wanted)
	}
	return types.None, fmt.Errorf("no held value for native operand kind %d from %T", wanted.Kind, wanted.Node)
}

// nativeLoopOperand rebuilds what Toast holds for a collection loop from the
// columns and cursor Barn holds, inverting ConvertNativeFrame's projections:
// the container, and the 1-based next index, or for a map the next key
// (CLEAR after its last element). Barn's cursor names the current element,
// 1-based. A map comes back from its key and value columns. A list's value
// column is the list itself. A string's column is its characters and keeps no
// mark that it was a string, so its container is written as the list of those
// characters: Toast then binds the same elements Barn would have.
func nativeLoopOperand(wanted NativeValueOrigin, values nativeHeld) (types.Value, error) {
	container := NativeValueOrigin{Kind: NativeLoopContainer, Node: wanted.Node}
	column, heldValues := values[nativeHeldKey{container, importValues}]
	keys, heldKeys := values[nativeHeldKey{container, importKeys}]
	if !heldValues || !heldKeys || column.Type() != types.TYPE_LIST {
		return types.None, fmt.Errorf("loop iteration columns are not held")
	}
	isMap := keys.Type() == types.TYPE_LIST
	if isMap && keys.Len() != column.Len() {
		return types.None, fmt.Errorf("loop key and value columns differ in length")
	}
	if wanted.Kind == NativeLoopContainer {
		if !isMap {
			return column, nil
		}
		pairs := make([][2]types.Value, column.Len())
		for index := range pairs {
			pairs[index] = [2]types.Value{keys.Get(index + 1), column.Get(index + 1)}
		}
		return types.NewMap(pairs), nil
	}
	cursor, held := values[nativeHeldKey{NativeValueOrigin{Kind: NativeLoopCursor, Node: wanted.Node}, importCursor}]
	if !held || cursor.Type() != types.TYPE_INT || cursor.Int() < 1 || cursor.Int() > int64(column.Len()) {
		return types.None, fmt.Errorf("loop cursor is not held or out of range")
	}
	if !isMap {
		return types.NewInt(cursor.Int() + 1), nil
	}
	if cursor.Int() == int64(column.Len()) {
		return types.None, nil
	}
	return keys.Get(int(cursor.Int()) + 1), nil
}

// nativePrefixExpressions lists the expressions a list prefix origin builds.
func nativePrefixExpressions(origin NativeValueOrigin) []ir.Expr {
	switch node := origin.Node.(type) {
	case *ir.BuiltinCallExpr:
		return node.Args
	case *ir.VerbCallExpr:
		return node.Args
	case *ir.ListExpr:
		return node.Elements
	}
	return nil
}

// nativeExceptionCodes is the codes operand Toast pushes for one handler: 0
// for ANY, else the list of its codes, here only when every code is literal.
func nativeExceptionCodes(origin NativeValueOrigin) (types.Value, error) {
	var codes []ir.Expr
	var any bool
	switch node := origin.Node.(type) {
	case *ir.TryStmt:
		if origin.Aux >= len(node.Handlers) {
			return types.None, fmt.Errorf("exception handler %d does not exist", origin.Aux)
		}
		codes, any = node.Handlers[origin.Aux].Codes, node.Handlers[origin.Aux].IsAny
	case *ir.CatchExpr:
		codes, any = node.Codes, node.IsAny
	default:
		return types.None, fmt.Errorf("exception codes of %T", origin.Node)
	}
	if any {
		return types.NewInt(0), nil
	}
	values := make([]types.Value, len(codes))
	for index, code := range codes {
		literal, ok := code.(*ir.LiteralExpr)
		if !ok {
			return types.None, fmt.Errorf("computed exception code has no Toast operand Barn holds")
		}
		value, err := valueFromLiteral(literal)
		if err != nil {
			return types.None, err
		}
		values[index] = value
	}
	return types.NewList(values), nil
}

// nativeVariable is the value Toast's environment holds for name, or false
// when Toast holds it unbound: the frame's own variable, then the call
// context and command environment, then Toast's constants.
func nativeVariable(frame task.VMFrameSnapshot, name string) (types.Value, bool) {
	for index, declared := range frame.Program.VarNames {
		if strings.EqualFold(declared, name) && index < len(frame.Locals) {
			value := frame.Locals[index]
			return value, !value.IsUnbound()
		}
	}
	switch strings.ToLower(name) {
	case "this":
		if frame.ThisValue.IsNone() {
			return types.NewObj(frame.This), true
		}
		return frame.ThisValue, true
	case "player":
		return types.NewObj(frame.Player), true
	case "caller":
		return types.NewObj(frame.Caller), true
	case "verb":
		return types.NewStr(frame.Verb), true
	case "args":
		return types.NewList(append([]types.Value(nil), frame.Args...)), true
	case "true":
		return types.NewBool(true), true
	case "false":
		return types.NewBool(false), true
	}
	for index, command := range task.CommandVariableNames {
		if strings.EqualFold(command, name) {
			value := frame.CommandEnvironment[index]
			return value, !value.IsUnbound()
		}
	}
	if value, ok := builtinConstants[strings.ToUpper(name)]; ok {
		return value, true
	}
	return types.None, false
}
