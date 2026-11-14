package compiler

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strings"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

type NativeResumeCall uint8

const (
	NativeResumeVerb NativeResumeCall = iota
	NativeResumeBuiltin
	NativeResumeEval
	NativeResumePass
)

// Rebuild compiles source the way the compile named by origin did: the whole
// source, or one vector of a Toast checkpoint, then the numbered fork body.
// A saved task's frame runs the result instead of stored bytecode.
func (c *Compiler) Rebuild(source []string, origin bytecode.Origin) (*bytecode.Program, error) {
	var program *bytecode.Program
	if origin.Native {
		compiled, _, _, _, err := c.compileNativeVector(source, origin.LanguageVersion, origin.NativeVector, nil)
		if err != nil {
			return nil, err
		}
		program = compiled
	} else {
		compiled, diagnostics := c.CompileMOO(source)
		if len(diagnostics) != 0 {
			return nil, fmt.Errorf("source does not compile: %w", diagnostics[0])
		}
		program = compiled
	}
	if origin.Fork == 0 {
		return program, nil
	}
	fork := program.ExtractFork(origin.Fork)
	if fork == nil {
		return nil, fmt.Errorf("source has no fork body %d", origin.Fork)
	}
	return fork, nil
}

// compileNativeVector lowers one vector of Toast-compiled source. Importing,
// when not nil, observes the lowering to map Toast continuation state; it
// never changes the code emitted. It also returns the whole source's layout.
func (c *Compiler) compileNativeVector(source []string, languageVersion, vector int, importing *barnImportLayout) (*bytecode.Program, *NativeProgramLayout, *NativeVectorLayout, *lowerer, error) {
	program, err := parser.NewParser(strings.Join(source, "\n")).ParseProgram()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parse native saved source: %w", err)
	}
	layout, err := NativeLayout(program, nil, languageVersion)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var selected *NativeVectorLayout
	for index := range layout.Vectors {
		if layout.Vectors[index].Vector == vector {
			selected = &layout.Vectors[index]
			break
		}
	}
	if selected == nil {
		return nil, nil, nil, nil, fmt.Errorf("native root vector %d does not exist", vector)
	}
	lower := newLowerer(c.builtinIDs)
	lower.importLayout = importing
	compiled, err := lower.compileProgram(&ir.Program{Statements: selected.Body})
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("compile native continuation: %w", err)
	}
	if importing != nil {
		importing.flush(lower)
		if importing.err != nil {
			return nil, nil, nil, nil, importing.err
		}
	}
	compiled.Source = append([]string(nil), source...)
	compiled.BuiltinLayout = c.layout
	compiled.Origin = bytecode.Origin{Native: true, NativeVector: vector, LanguageVersion: languageVersion}
	return compiled, layout, selected, lower, nil
}

// ConvertNativeFrame compiles a saved native program and translates existing
// continuation values. It executes no MOO source and never replays effects.
func (c *Compiler) ConvertNativeFrame(saved task.VMFrameSnapshot, vector int, top bool) (task.VMFrameSnapshot, NativeResumeCall, error) {
	if saved.Native == nil {
		return saved, NativeResumeBuiltin, nil
	}
	if saved.BuiltinPC != 0 || saved.Native.Builtin != nil {
		return saved, 0, fmt.Errorf("native builtin continuation requires conversion")
	}
	compiled, _, selected, lower, err := c.compileNativeVector(saved.Program.Source, saved.Native.LanguageVersion, vector, newBarnImportLayout())
	if err != nil {
		return saved, 0, err
	}
	importing := lower.importLayout
	var site *NativeContinuation
	for index := range selected.Continuations {
		if selected.Continuations[index].PC == saved.IP {
			site = &selected.Continuations[index]
			break
		}
	}
	if site == nil || len(site.Stack) != len(saved.Stack)+1 {
		return saved, 0, fmt.Errorf("native continuation PC %d does not match its saved stack", saved.IP)
	}
	barn, found := importing.calls[site.Node]
	if !found {
		return saved, 0, fmt.Errorf("native continuation has no Barn callsite")
	}
	values := make(map[NativeValueOrigin]types.Value)
	for index, value := range saved.Stack {
		origin := site.Stack[index]
		if index >= len(saved.Native.OperandTypes) {
			return saved, 0, fmt.Errorf("native operand type metadata is incomplete")
		}
		kind := saved.Native.OperandTypes[index]
		switch origin.Kind {
		case NativeCatchMarker:
			if kind != 7 || value.Type() != types.TYPE_INT || value.Int() != int64(origin.Aux) {
				return saved, 0, fmt.Errorf("native catch marker does not match its compiled handler")
			}
		case NativeFinallyMarker:
			if kind != 8 || value.Type() != types.TYPE_INT || value.Int() != int64(origin.LabelPC) {
				return saved, 0, fmt.Errorf("native finally marker does not match its compiled handler")
			}
		case NativeHandlerLabel:
			if kind != 0 || value.Type() != types.TYPE_INT || value.Int() != int64(origin.LabelPC) {
				return saved, 0, fmt.Errorf("native handler label does not match its compiled handler")
			}
		default:
			if kind == 7 || kind == 8 {
				return saved, 0, fmt.Errorf("native control marker appears in an ordinary operand")
			}
		}
		values[origin] = value
		if origin.Kind == NativeListPrefix {
			addNativePrefixValues(values, origin, value)
		}
	}
	resolve := func(value barnImportValue) (types.Value, error) {
		if value.isLiteral {
			return value.literal, nil
		}
		resolved, ok := values[value.origin]
		if !ok {
			return types.None, fmt.Errorf("native continuation lacks %d value from %T", value.origin.Kind, value.origin.Node)
		}
		if value.projection == importIdentity {
			return resolved, nil
		}
		container := resolved
		if value.projection == importCursor {
			container = values[NativeValueOrigin{Kind: NativeLoopContainer, Node: value.origin.Node}]
		}
		column, keys, code := container.IterationColumns()
		if code != types.E_NONE {
			return types.None, fmt.Errorf("native iteration container is invalid")
		}
		if container.Type() == types.TYPE_STR {
			bytes := container.Str()
			if column.Len() != len(bytes) {
				return types.None, fmt.Errorf("unsupported native byte-string iteration: normalized columns are not byte-equivalent")
			}
			for index := range len(bytes) {
				element := column.Get(index + 1)
				if element.Type() != types.TYPE_STR || element.Str() != bytes[index:index+1] {
					return types.None, fmt.Errorf("unsupported native byte-string iteration: normalized columns are not byte-equivalent")
				}
			}
		}
		switch value.projection {
		case importValues:
			return column, nil
		case importKeys:
			return keys, nil
		case importLength:
			return types.NewInt(int64(column.Len())), nil
		case importCursor:
			if container.Type() != types.TYPE_MAP {
				if resolved.Type() != types.TYPE_INT || resolved.Int() < 2 || resolved.Int() > int64(column.Len()+1) {
					return types.None, fmt.Errorf("native iteration cursor is invalid")
				}
				return types.NewInt(resolved.Int() - 1), nil
			}
			// Native map iterators serialize the next key, or CLEAR at the end.
			// Barn's cursor still names the current element during the body.
			if resolved.IsNone() {
				return types.NewInt(int64(column.Len())), nil
			}
			for index, key := range keys.Elements() {
				if index > 0 && types.MapKeysEqual(key, resolved) {
					return types.NewInt(int64(index)), nil
				}
			}
			return types.None, fmt.Errorf("native map iteration cursor is absent from its snapshot")
		}
		return types.None, fmt.Errorf("unknown native iteration projection %d", value.projection)
	}
	converted := saved
	converted.Native = nil
	converted.Program = *compiled
	converted.IP = barn.IP
	converted.Locals = make([]types.Value, compiled.NumLocals)
	for index := range converted.Locals {
		converted.Locals[index] = types.Unbound
	}
	for index, name := range compiled.VarNames {
		for nativeIndex, nativeName := range saved.Program.VarNames {
			if strings.EqualFold(name, nativeName) {
				converted.Locals[index] = saved.Locals[nativeIndex]
				break
			}
		}
	}
	// Command context is inherited even through a verb which never references
	// these names, so its values cannot depend on the compiled named slots.
	for commandIndex, commandName := range task.CommandVariableNames {
		converted.CommandEnvironment[commandIndex] = types.Unbound
		for nativeIndex, nativeName := range saved.Program.VarNames {
			if strings.EqualFold(nativeName, commandName) {
				converted.CommandEnvironment[commandIndex] = saved.Locals[nativeIndex]
				break
			}
		}
	}
	gap := lower.importSlotGap(compiled)
	for slot, value := range barn.Locals {
		resolved, err := resolve(value)
		if err != nil {
			return saved, 0, fmt.Errorf("native local %d: %w", slot, err)
		}
		converted.Locals[slot-gap] = resolved
	}
	converted.Stack = nil
	for index, value := range barn.Stack {
		if index == len(barn.Stack)-1 {
			if top {
				// Barn's yielded builtin already pushed the resume-value slot.
				converted.Stack = append(converted.Stack, types.NewInt(0))
			}
			break
		}
		resolved, err := resolve(value)
		if err != nil {
			return saved, 0, fmt.Errorf("native operand %d: %w", index, err)
		}
		converted.Stack = append(converted.Stack, resolved)
	}
	for _, control := range site.Controls {
		if control.Kind == NativeFinallyHandler {
			return saved, 0, fmt.Errorf("native pending finally reason requires conversion")
		}
	}
	converted.ExceptStack, err = importedHandlers(compiled, barn.Handlers, site.Controls)
	if err != nil {
		return saved, 0, err
	}
	for index, name := range saved.Program.VarNames {
		if strings.EqualFold(name, "args") && saved.Locals[index].Type() == types.TYPE_LIST {
			converted.Args = append([]types.Value(nil), saved.Locals[index].Elements()...)
		}
		if strings.EqualFold(name, "caller") && saved.Locals[index].Type() == types.TYPE_OBJ {
			converted.Caller = saved.Locals[index].Obj()
		}
	}
	kind := NativeResumeVerb
	if builtin, ok := site.Node.(*ir.BuiltinCallExpr); ok {
		kind = NativeResumeBuiltin
		switch strings.ToLower(builtin.Name) {
		case "eval":
			kind = NativeResumeEval
		case "pass":
			kind = NativeResumePass
		}
	}
	return converted, kind, nil
}

func importedHandlers(program *bytecode.Program, recorded []barnImportHandler, controls []NativeControlOrigin) ([]bytecode.Handler, error) {
	var handlers []bytecode.Handler
	for _, entry := range recorded {
		op := bytecode.OpCode(program.Code[entry.SetupIP])
		active := false
		for _, control := range controls {
			if control.Node == entry.Node && (control.Kind == NativeCatch || control.Kind == NativeExcept || control.Kind == NativeFinallyBody) {
				active = true
				break
			}
		}
		if !active {
			return nil, fmt.Errorf("imported handler has no corresponding native control scope")
		}
		operands := program.Code[entry.SetupIP+1:]
		if op == bytecode.OP_TRY_FINALLY_WIDE {
			handlers = append(handlers, bytecode.Handler{Type: bytecode.HandlerFinally, HandlerIP: int(binary.BigEndian.Uint32(operands)), VarIndex: -1, StackDepth: entry.StackDepth})
			continue
		}
		if op != bytecode.OP_TRY_EXCEPT_LOCAL_WIDE {
			return nil, fmt.Errorf("unsupported Barn import handler opcode %s", op)
		}
		base, position := len(handlers), 1
		for range int(operands[0]) {
			count := int(operands[position])
			position++
			codes := make([]types.ErrorCode, count)
			for index := range codes {
				codes[index] = types.ErrorCode(operands[position+index])
			}
			position += count
			variable := int(binary.BigEndian.Uint16(operands[position:])) - 1
			position += 2
			target := int(binary.BigEndian.Uint32(operands[position:]))
			position += 4
			handlers = append(handlers, bytecode.Handler{Type: bytecode.HandlerExcept, HandlerIP: target, Codes: codes, VarIndex: variable, StackDepth: entry.StackDepth, ExceptBase: base + 1})
		}
		slices.Reverse(handlers[base:])
	}
	return handlers, nil
}

func addNativePrefixValues(values map[NativeValueOrigin]types.Value, origin NativeValueOrigin, value types.Value) {
	if value.Type() != types.TYPE_LIST {
		return
	}
	var expressions []ir.Expr
	switch node := origin.Node.(type) {
	case *ir.BuiltinCallExpr:
		expressions = node.Args
	case *ir.VerbCallExpr:
		expressions = node.Args
	case *ir.ListExpr:
		expressions = node.Elements
	}
	if origin.Aux > len(expressions) || value.Len() != origin.Aux {
		return
	}
	for index, expression := range expressions[:origin.Aux] {
		if _, splice := expression.(*ir.SpliceExpr); splice {
			return
		}
		values[NativeValueOrigin{Kind: NativeExpression, Node: expression}] = value.Get(index + 1)
	}
}
