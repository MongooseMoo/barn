package bytecode

import (
	"encoding/binary"
	"fmt"
)

// VerifyProgram validates persisted bytecode before it reaches the VM's
// deliberately unchecked execution loop. Compilation remains the source of
// trusted programs; this verifier is for deserialization trust boundaries.
func VerifyProgram(program *Program) error {
	_, err := StackHeights(program)
	return err
}

// StackHeights verifies program as VerifyProgram does and returns the
// frame-relative operand stack height on entry to each reachable instruction,
// indexed by instruction address; every other entry is -1. Each reachable
// instruction is reached at one height and never pops more than it holds, so
// no frame can pop values belonging to its caller.
func StackHeights(program *Program) ([]int, error) {
	if program == nil || len(program.Code) == 0 {
		return nil, fmt.Errorf("empty bytecode program")
	}
	// Every named variable owns a slot, but the compiler also allocates unnamed
	// temporaries from the top of the one-byte operand range, so a program may
	// legitimately have far more slots than names (up to MaxLocals).
	if program.NumLocals < len(program.VarNames) || program.NumLocals > MaxLocals {
		return nil, fmt.Errorf("invalid local metadata: %d locals, %d names", program.NumLocals, len(program.VarNames))
	}
	slots := program.BuiltinSlots
	for _, constant := range slots.TypeConstants {
		if constant.Slot <= 0 || constant.Slot > program.NumLocals {
			return nil, fmt.Errorf("invalid type constant local slot %d", constant.Slot)
		}
	}
	for name, slot := range map[string]int{
		"this": slots.This, "player": slots.Player, "caller": slots.Caller,
		"verb": slots.Verb, "args": slots.Args, "argstr": slots.Argstr,
		"dobjstr": slots.Dobjstr, "iobjstr": slots.Iobjstr, "prepstr": slots.Prepstr,
		"dobj": slots.Dobj, "iobj": slots.Iobj,
	} {
		if slot < 0 || slot > program.NumLocals {
			return nil, fmt.Errorf("invalid %s built-in local slot %d", name, slot)
		}
	}

	boundaries, instructions, err := decodeInstructions(program.Code)
	if err != nil {
		return nil, err
	}
	last := instructions[len(instructions)-1].op
	if last != OP_RETURN && last != OP_RETURN_NONE {
		return nil, fmt.Errorf("program does not end in a terminal instruction")
	}

	for _, instruction := range instructions {
		if err := verifyInstruction(program, instruction, boundaries); err != nil {
			return nil, fmt.Errorf("bytecode at %d (%s): %w", instruction.ip, instruction.op, err)
		}
	}
	heights, err := stackHeights(len(program.Code), instructions)
	if err != nil {
		return nil, err
	}
	if err := verifyLoopsTick(program.Code, instructions, heights); err != nil {
		return nil, err
	}
	return heights, nil
}

// verifyLoopsTick rejects a reachable control-flow cycle in which no
// instruction charges a tick. Jumps, loops and FOR_RANGE_NEXT are tick-free
// (vm/vm.go fkJump, fkLoop, fkForRangeNext), so such a cycle (a LOOP that
// targets itself, say) never reaches the tick limit or a seconds checkpoint
// and spins forever. Compiled loops always pass through a ticking test
// (JUMP_IF_FALSE, FOR_RANGE_CHECK, FOR_LIST_CHECK). An OP_TICKS prefix
// executes together with the instruction it modifies and replaces its charge,
// so it stands for that instruction here.
func verifyLoopsTick(code []byte, instructions []decodedInstruction, heights []int) error {
	index := make(map[int]int, len(instructions))
	for i, instruction := range instructions {
		index[instruction.ip] = i
	}
	// freeEdges reports whether the instruction at ip charges no tick and, if
	// so, where control can go next.
	freeEdges := func(ip int) (bool, []flowEdge) {
		instruction := instructions[index[ip]]
		var checked, unchecked int64
		if instruction.op == OP_TICKS {
			checked = int64(instruction.operands[0] &^ TicksUnchecked)
			instruction = instructions[index[instruction.next]]
		} else {
			checked, unchecked = InstructionTicks(instruction.op, code, instruction.ip)
		}
		if checked|unchecked != 0 {
			return false, nil
		}
		return true, successors(instruction, 0, 0)
	}
	const (
		unvisited = iota
		active
		done
	)
	state := make([]uint8, len(code))
	type visit struct {
		edges []flowEdge
		next  int
	}
	for _, root := range instructions {
		if heights[root.ip] < 0 || state[root.ip] != unvisited {
			continue
		}
		free, edges := freeEdges(root.ip)
		if !free {
			state[root.ip] = done
			continue
		}
		state[root.ip] = active
		stack := []visit{{edges: edges}}
		ips := []int{root.ip}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.next == len(top.edges) {
				state[ips[len(ips)-1]] = done
				stack, ips = stack[:len(stack)-1], ips[:len(ips)-1]
				continue
			}
			target := top.edges[top.next].ip
			top.next++
			switch state[target] {
			case active:
				return fmt.Errorf("bytecode at %d (%s): control-flow cycle through %d charges no tick",
					ips[len(ips)-1], instructions[index[ips[len(ips)-1]]].op, target)
			case unvisited:
				free, edges := freeEdges(target)
				if !free {
					state[target] = done
					continue
				}
				state[target] = active
				stack = append(stack, visit{edges: edges})
				ips = append(ips, target)
			}
		}
	}
	return nil
}

// stackHeights propagates operand stack heights from the entry point (height
// 0) over every control edge the VM can take. Instructions must already be
// individually verified, so every target it follows is an instruction boundary.
func stackHeights(codeLength int, instructions []decodedInstruction) ([]int, error) {
	heights := make([]int, codeLength)
	index := make([]int, codeLength)
	for ip := range heights {
		heights[ip], index[ip] = -1, -1
	}
	for i, instruction := range instructions {
		index[instruction.ip] = i
	}
	var pending []int
	reach := func(from decodedInstruction, ip, height int) error {
		switch {
		case ip >= codeLength:
			return fmt.Errorf("bytecode at %d (%s): control reaches the end of the program", from.ip, from.op)
		case heights[ip] == -1:
			heights[ip] = height
			pending = append(pending, ip)
		case heights[ip] != height:
			return fmt.Errorf("bytecode at %d (%s): stack height %d at %d, which is already reached with height %d",
				from.ip, from.op, height, ip, heights[ip])
		}
		return nil
	}
	heights[0], pending = 0, []int{0}
	for len(pending) > 0 {
		ip := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		instruction := instructions[index[ip]]
		height := heights[ip]
		pops, pushes, err := InstructionStackEffect(instruction.op, instruction.operands)
		if err != nil {
			return nil, fmt.Errorf("bytecode at %d (%s): %w", ip, instruction.op, err)
		}
		if pops > height {
			return nil, fmt.Errorf("bytecode at %d (%s): stack underflow: pops %d values from a stack of %d", ip, instruction.op, pops, height)
		}
		after := height - pops + pushes
		for _, edge := range successors(instruction, height, after) {
			if err := reach(instruction, edge.ip, edge.height); err != nil {
				return nil, err
			}
		}
	}
	return heights, nil
}

type flowEdge struct{ ip, height int }

// successors lists where control goes after instruction, with the stack
// height on arrival. height is the height before the instruction and after the
// height its stack effect leaves on the ordinary path.
func successors(instruction decodedInstruction, height, after int) []flowEdge {
	next := flowEdge{instruction.next, after}
	switch op := instruction.op; op {
	case OP_RETURN, OP_RETURN_NONE:
		return nil
	case OP_JUMP, OP_JUMP_WIDE, OP_LOOP, OP_LOOP_WIDE, OP_FOR_RANGE_NEXT, OP_FOR_RANGE_NEXT_WIDE, OP_UNWIND_JUMP:
		return []flowEdge{{branchTarget(instruction), after}}
	case OP_JUMP_IF_FALSE, OP_JUMP_IF_FALSE_WIDE, OP_JUMP_IF_TRUE, OP_JUMP_IF_TRUE_WIDE,
		OP_FOR_RANGE_CHECK, OP_FOR_RANGE_CHECK_WIDE, OP_FOR_LIST_CHECK_WIDE:
		return []flowEdge{next, {branchTarget(instruction), after}}
	case OP_AND, OP_AND_WIDE, OP_OR, OP_OR_WIDE:
		// The jump keeps the deciding operand (vm/op_logic.go executeAnd/Or).
		return []flowEdge{next, {branchTarget(instruction), height}}
	case OP_TRY_FINALLY, OP_TRY_FINALLY_WIDE:
		// Unwinding resets SP to the depth recorded here (vm/vm.go HandleError).
		return []flowEdge{next, {branchTarget(instruction), height}}
	case OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE:
		edges := []flowEdge{next}
		// The visitor never fails, and forEachExceptClause fails only through it.
		_ = forEachExceptClause(instruction, func(_, handler int) error {
			edges = append(edges, flowEdge{handler, height})
			return nil
		})
		return edges
	case OP_FORK, OP_FORK_WIDE, OP_FORK_LOCAL_WIDE:
		// The parent skips the body, which runs as its own extracted program.
		_, bodyLength := forkOperands(instruction)
		return []flowEdge{{instruction.next + bodyLength, after}}
	}
	return []flowEdge{next}
}

// InstructionStackEffect is how many values an instruction pops and pushes on its ordinary
// path, as implemented by vm.Execute and the dispatch fast paths in vm/vm.go.
// A call's push is its result: the callee frame's return pushes it onto the
// caller's stack (vm/stack.go Return).
func InstructionStackEffect(op OpCode, operand []byte) (pops, pushes int, err error) {
	if IsImmediateInt(op) {
		return 0, 1, nil
	}
	callArgs := func(argc byte) int {
		if argc == 0xff { // spliced: one list holds the arguments
			return 1
		}
		return int(argc)
	}
	switch op {
	case OP_PUSH, OP_PUSH_WIDE, OP_PUSH_INT, OP_GET_VAR:
		return 0, 1, nil
	case OP_POP, OP_SET_VAR, OP_SET_LOCAL, OP_JUMP_IF_FALSE, OP_JUMP_IF_FALSE_WIDE,
		OP_JUMP_IF_TRUE, OP_JUMP_IF_TRUE_WIDE, OP_SCATTER, OP_RETURN,
		OP_FORK, OP_FORK_WIDE, OP_FORK_LOCAL_WIDE:
		return 1, 0, nil
	case OP_AND, OP_AND_WIDE, OP_OR, OP_OR_WIDE:
		// The fallthrough pops the operand; successors keeps it on the jump.
		return 1, 0, nil
	case OP_DUP:
		return 1, 2, nil
	case OP_RETURN_NONE, OP_JUMP, OP_JUMP_WIDE, OP_LOOP, OP_LOOP_WIDE, OP_UNWIND_JUMP,
		OP_FOR_RANGE_CHECK, OP_FOR_RANGE_CHECK_WIDE, OP_FOR_LIST_CHECK_WIDE,
		OP_FOR_RANGE_NEXT, OP_FOR_RANGE_NEXT_WIDE, OP_FOR_LIST_LOAD, OP_FOR_LIST_LOAD_KV,
		OP_FOR_LIST_LOAD_COLUMNS, OP_FOR_LIST_LOAD_VALUE, OP_SCATTER_TAKE,
		OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE, OP_END_EXCEPT,
		OP_TRY_FINALLY, OP_TRY_FINALLY_WIDE, OP_END_FINALLY, OP_END_FINALLY_WIDE, OP_TICKS:
		return 0, 0, nil
	case OP_NEG, OP_NOT, OP_BITNOT, OP_LENGTH, OP_SPLICE, OP_INDEX_MARKER,
		OP_CHECK_MAP_LIMIT, OP_GET_PROP_WIDE, OP_GET_PROP_KEYBASE:
		return 1, 1, nil
	case OP_ADD, OP_STRING_APPEND, OP_SUB, OP_MUL, OP_DIV, OP_MOD, OP_POW,
		OP_EQ, OP_NE, OP_LT, OP_LE, OP_GT, OP_GE, OP_IN,
		OP_BITOR, OP_BITAND, OP_BITXOR, OP_SHL, OP_SHR,
		OP_INDEX, OP_LIST_RANGE, OP_LIST_APPEND, OP_LIST_EXTEND, OP_GET_PROP_DYNAMIC:
		return 2, 1, nil
	case OP_GET_PROP_KEYED, OP_ITER_PREP, OP_ITER_PREP_COLUMNS:
		return 1, 2, nil
	case OP_INDEX_SET, OP_SET_PROP_WIDE:
		return 2, 0, nil
	case OP_RANGE, OP_INDEX_KEYED:
		return 3, 1, nil
	case OP_MAPHASKEY_KEYED:
		return 3, 2, nil
	case OP_RANGE_SET, OP_SET_PROP_DYNAMIC, OP_SET_PROP_KEYED:
		return 3, 0, nil
	case OP_GET_PROP: // operand 0xff is the legacy dynamic form: name above the object
		if operand[0] == 0xff {
			return 2, 1, nil
		}
		return 1, 1, nil
	case OP_SET_PROP:
		if operand[0] == 0xff {
			return 3, 0, nil
		}
		return 2, 0, nil
	case OP_MAKE_LIST:
		return int(operand[0]), 1, nil
	case OP_MAKE_MAP:
		return 2 * int(operand[0]), 1, nil
	case OP_CALL_BUILTIN:
		return callArgs(operand[1]), 1, nil
	case OP_CALL_BUILTIN_WIDE:
		return callArgs(operand[2]), 1, nil
	case OP_CALL_VERB_WIDE: // arguments, then the object
		return callArgs(operand[2]) + 1, 1, nil
	case OP_CALL_VERB:
		pops = callArgs(operand[1]) + 1
		if operand[0] == 0xff { // legacy dynamic form: name above the arguments
			pops++
		}
		return pops, 1, nil
	case OP_CALL_VERB_DYNAMIC:
		return callArgs(operand[0]) + 2, 1, nil
	case OP_PASS:
		if operand[0] == 0 { // inherits the frame's own arguments
			return 0, 1, nil
		}
		return callArgs(operand[0]), 1, nil
	}
	return 0, 0, fmt.Errorf("no stack effect for opcode %d", op)
}

// IsInstructionBoundary reports whether ip is the start of a fully decoded
// instruction. The end of the program is not executable and is not a boundary.
func (program *Program) IsInstructionBoundary(ip int) bool {
	if program == nil || ip < 0 || ip >= len(program.Code) {
		return false
	}
	boundaries, _, err := decodeInstructions(program.Code)
	return err == nil && boundaries[ip]
}

type decodedInstruction struct {
	ip, next int
	op       OpCode
	operands []byte
}

func decodeInstructions(code []byte) (map[int]bool, []decodedInstruction, error) {
	boundaries := make(map[int]bool)
	var instructions []decodedInstruction
	for ip := 0; ip < len(code); {
		start := ip
		op := OpCode(code[ip])
		if op.String() == "UNKNOWN" {
			return nil, nil, fmt.Errorf("unknown opcode %d at byte %d", op, ip)
		}
		if op == OP_CONTINUE || op == OP_CATCH {
			return nil, nil, fmt.Errorf("dead opcode %s at byte %d", op, ip)
		}
		ip++
		count := instructionOperandCount(op, code[ip:])
		if count > len(code)-ip || variableOperandsTruncated(op, code[ip:], count) {
			return nil, nil, fmt.Errorf("truncated operands for %s at byte %d", op, start)
		}
		ip += count
		boundaries[start] = true
		instructions = append(instructions, decodedInstruction{start, ip, op, code[start+1 : ip]})
	}
	return boundaries, instructions, nil
}

func variableOperandsTruncated(op OpCode, remaining []byte, count int) bool {
	if op != OP_TRY_EXCEPT && op != OP_TRY_EXCEPT_WIDE && op != OP_TRY_EXCEPT_LOCAL_WIDE {
		return false
	}
	if len(remaining) == 0 {
		return true
	}
	pos := 1
	ipBytes, varBytes := 2, 1
	if op != OP_TRY_EXCEPT {
		ipBytes = 4
	}
	if op == OP_TRY_EXCEPT_LOCAL_WIDE {
		varBytes = 2
	}
	for clause := 0; clause < int(remaining[0]); clause++ {
		if pos >= len(remaining) {
			return true
		}
		pos += 1 + int(remaining[pos]) + varBytes + ipBytes
		if pos > len(remaining) {
			return true
		}
	}
	return pos != count
}

func verifyInstruction(program *Program, instruction decodedInstruction, boundaries map[int]bool) error {
	op, operand := instruction.op, instruction.operands
	local := func(index int) error {
		if index < 0 || index >= program.NumLocals {
			return fmt.Errorf("local index %d outside %d locals", index, program.NumLocals)
		}
		return nil
	}
	constant := func(index int) error {
		if index < 0 || index >= len(program.Constants) {
			return fmt.Errorf("constant index %d outside pool of %d", index, len(program.Constants))
		}
		return nil
	}
	target := func(ip int) error {
		if !boundaries[ip] {
			return fmt.Errorf("target %d is not an instruction boundary", ip)
		}
		return nil
	}
	// A handler follows the instruction that installs it, as the compiler
	// emits it. Unwinding to a handler at or before its own TRY would re-run
	// the TRY, re-arm the handler and unwind to it again: a cycle with no
	// checked tick, which no tick budget ends.
	handler := func(ip int) error {
		if ip <= instruction.ip {
			return fmt.Errorf("handler target %d does not follow its instruction", ip)
		}
		return target(ip)
	}

	switch op {
	case OP_PUSH:
		return constant(int(operand[0]))
	case OP_GET_VAR, OP_SET_VAR, OP_SET_LOCAL, OP_INDEX_SET, OP_RANGE_SET:
		return local(int(operand[0]))
	case OP_TICKS:
		// The prefix modifies exactly the instruction that follows it.
		if instruction.next >= len(program.Code) || OpCode(program.Code[instruction.next]) == OP_TICKS {
			return fmt.Errorf("tick prefix does not precede an instruction")
		}
	case OP_SCATTER_TAKE:
		for _, index := range operand[:3] {
			if err := local(int(index)); err != nil {
				return err
			}
		}
		if operand[3] > 1 {
			return fmt.Errorf("invalid scatter step %d", operand[3])
		}
	case OP_GET_PROP, OP_SET_PROP:
		if operand[0] != 0xff {
			return constant(int(operand[0]))
		}
	case OP_PUSH_WIDE, OP_GET_PROP_WIDE, OP_SET_PROP_WIDE,
		OP_GET_PROP_KEYED, OP_INDEX_KEYED, OP_GET_PROP_KEYBASE, OP_SET_PROP_KEYED:
		return constant(int(binary.BigEndian.Uint16(operand)))
	case OP_MAPHASKEY_KEYED:
		return constant(int(binary.BigEndian.Uint16(operand[:2])))
	case OP_CALL_VERB:
		if operand[0] != 0xff {
			return constant(int(operand[0]))
		}
	case OP_CALL_VERB_WIDE:
		return constant(int(binary.BigEndian.Uint16(operand[:2])))
	case OP_FOR_RANGE_CHECK, OP_FOR_RANGE_NEXT,
		OP_FOR_RANGE_CHECK_WIDE, OP_FOR_RANGE_NEXT_WIDE, OP_FOR_LIST_CHECK_WIDE:
		if err := local(int(operand[0])); err != nil {
			return err
		}
		if err := local(int(operand[1])); err != nil {
			return err
		}
		return target(branchTarget(instruction))
	case OP_FOR_LIST_LOAD, OP_FOR_LIST_LOAD_KV, OP_FOR_LIST_LOAD_COLUMNS, OP_FOR_LIST_LOAD_VALUE:
		for _, index := range operand {
			if err := local(int(index)); err != nil {
				return err
			}
		}
	case OP_AND, OP_OR, OP_JUMP, OP_JUMP_IF_FALSE, OP_JUMP_IF_TRUE, OP_LOOP,
		OP_AND_WIDE, OP_OR_WIDE, OP_JUMP_WIDE, OP_JUMP_IF_FALSE_WIDE, OP_JUMP_IF_TRUE_WIDE, OP_LOOP_WIDE,
		OP_END_FINALLY, OP_END_FINALLY_WIDE:
		return target(branchTarget(instruction))
	case OP_TRY_FINALLY, OP_TRY_FINALLY_WIDE:
		return handler(branchTarget(instruction))
	case OP_UNWIND_JUMP:
		return target(branchTarget(instruction))
	case OP_FORK, OP_FORK_WIDE, OP_FORK_LOCAL_WIDE:
		return verifyFork(program, instruction, boundaries)
	case OP_TRY_EXCEPT, OP_TRY_EXCEPT_WIDE, OP_TRY_EXCEPT_LOCAL_WIDE:
		return forEachExceptClause(instruction, func(varOperand, handlerIP int) error {
			if varOperand > 0 && varOperand-1 >= program.NumLocals {
				return fmt.Errorf("local index %d outside %d locals", varOperand-1, program.NumLocals)
			}
			return handler(handlerIP)
		})
	case OP_INDEX_MARKER:
		if operand[0] > RangeMarkerLast {
			return fmt.Errorf("invalid index marker %d", operand[0])
		}
	case OP_ITER_PREP:
		if operand[0] > 1 {
			return fmt.Errorf("invalid iteration flag %d", operand[0])
		}
	}
	return nil
}

// branchTarget is the address a jump, loop, range-loop, short-circuit or
// finally instruction names. Narrow forms carry 16-bit operands and wide forms
// 32-bit ones; jumps are relative to the next instruction (backward for the
// loop forms) and finally targets are absolute (vm/control.go).
func branchTarget(instruction decodedInstruction) int {
	operand := instruction.operands
	if instruction.op == OP_UNWIND_JUMP {
		return int(binary.BigEndian.Uint32(operand[:4]))
	}
	switch instruction.op {
	case OP_TRY_FINALLY, OP_END_FINALLY:
		return int(binary.BigEndian.Uint16(operand))
	case OP_TRY_FINALLY_WIDE, OP_END_FINALLY_WIDE:
		return int(binary.BigEndian.Uint32(operand))
	case OP_FOR_RANGE_CHECK, OP_FOR_RANGE_NEXT,
		OP_FOR_RANGE_CHECK_WIDE, OP_FOR_RANGE_NEXT_WIDE, OP_FOR_LIST_CHECK_WIDE:
		operand = operand[2:] // after the two local operands
	}
	offset := int(binary.BigEndian.Uint16(operand))
	if len(operand) == 4 {
		offset = int(binary.BigEndian.Uint32(operand))
	}
	switch instruction.op {
	case OP_LOOP, OP_LOOP_WIDE, OP_FOR_RANGE_NEXT, OP_FOR_RANGE_NEXT_WIDE:
		return instruction.next - offset
	}
	return instruction.next + offset
}

func forkOperands(instruction decodedInstruction) (varIndex, bodyLength int) {
	operand := instruction.operands
	switch instruction.op {
	case OP_FORK:
		return int(operand[0]), int(binary.BigEndian.Uint16(operand[1:]))
	case OP_FORK_WIDE:
		return int(operand[0]), int(binary.BigEndian.Uint32(operand[1:]))
	}
	return int(binary.BigEndian.Uint16(operand[:2])), int(binary.BigEndian.Uint32(operand[2:]))
}

func verifyFork(program *Program, instruction decodedInstruction, boundaries map[int]bool) error {
	varIndex, bodyLength := forkOperands(instruction)
	if varIndex > 0 && varIndex-1 >= program.NumLocals {
		return fmt.Errorf("local index %d outside %d locals", varIndex-1, program.NumLocals)
	}
	end := instruction.next + bodyLength
	// An empty body (`fork (0) endfork`) is length 0; ExtractForkBody runs it as
	// a lone OP_RETURN_NONE, so only a negative length is malformed.
	if bodyLength < 0 || !boundaries[instruction.next] || (end != len(program.Code) && !boundaries[end]) {
		return fmt.Errorf("fork body [%d,%d) is not on instruction boundaries", instruction.next, end)
	}
	return nil
}

// forEachExceptClause decodes each clause of a TRY_EXCEPT instruction and
// passes its one-based variable operand (0 = none) and absolute handler address.
func forEachExceptClause(instruction decodedInstruction, visit func(varOperand, handler int) error) error {
	operand, pos := instruction.operands, 1
	wide := instruction.op != OP_TRY_EXCEPT
	wideLocal := instruction.op == OP_TRY_EXCEPT_LOCAL_WIDE
	for clause := 0; clause < int(operand[0]); clause++ {
		codes := int(operand[pos])
		pos += 1 + codes
		varOperand := int(operand[pos])
		pos++
		if wideLocal {
			varOperand = int(binary.BigEndian.Uint16(operand[pos-1:]))
			pos++
		}
		handler := int(binary.BigEndian.Uint16(operand[pos:]))
		size := 2
		if wide {
			handler, size = int(binary.BigEndian.Uint32(operand[pos:])), 4
		}
		pos += size
		if err := visit(varOperand, handler); err != nil {
			return err
		}
	}
	return nil
}
