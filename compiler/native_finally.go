package compiler

import (
	"encoding/binary"
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Toast's reasons for running a finally body (execute.cc Finally_Reason). It
// holds the reason and a value as the body's two operands, where Barn holds a
// pending reason on the frame: the value is 0 for a fall-through or an
// uncaught error, the returned value, the raised {code, message, value,
// stack}, or a loop exit's {stack depth, pc}.
const (
	nativeFinallyFallThrough = 0
	nativeFinallyRaise       = 1
	nativeFinallyUncaught    = 2
	nativeFinallyReturn      = 3
	nativeFinallyExit        = 5
)

// finallyIPAt is the handler address of the OP_TRY_FINALLY_WIDE at ip, which
// identifies its finally body in a pending reason.
func finallyIPAt(program *bytecode.Program, ip int) (int, bool) {
	if ip < 0 || ip+5 > len(program.Code) || bytecode.OpCode(program.Code[ip]) != bytecode.OP_TRY_FINALLY_WIDE {
		return 0, false
	}
	return int(binary.BigEndian.Uint32(program.Code[ip+1:])), true
}

// unwindJumpAt is the target and retained handler depth of the
// OP_UNWIND_JUMP at ip, a break or continue statement.
func unwindJumpAt(program *bytecode.Program, ip int) (target, depth int, ok bool) {
	if ip < 0 || ip+7 > len(program.Code) || bytecode.OpCode(program.Code[ip]) != bytecode.OP_UNWIND_JUMP {
		return 0, 0, false
	}
	return int(binary.BigEndian.Uint32(program.Code[ip+1:])), int(binary.BigEndian.Uint16(program.Code[ip+5:])), true
}

// importFinallyReasons converts the reason and value Toast holds for each
// finally body the call site is inside into Barn's pending reasons, outermost
// first. Values are the frame's saved operands by origin, and stack is the
// frame's converted operand stack.
func importFinallyReasons(compiled *bytecode.Program, importing *barnImportLayout, vector *NativeVectorLayout, site *NativeContinuation, values map[NativeValueOrigin]types.Value, stack []types.Value) ([]task.PendingReasonSnapshot, error) {
	var pending []task.PendingReasonSnapshot
	for _, control := range site.Controls {
		if control.Kind != NativeFinallyHandler {
			continue
		}
		reason, held := values[NativeValueOrigin{Kind: NativeFinallyReason, Node: control.Node}]
		value := values[NativeValueOrigin{Kind: NativeFinallyValue, Node: control.Node}]
		setup, recorded := importing.finallys[control.Node]
		finallyIP, found := finallyIPAt(compiled, setup)
		if !held || !recorded || !found || reason.Type() != types.TYPE_INT {
			return nil, fmt.Errorf("native finally reason is not held")
		}
		entry := task.PendingReasonSnapshot{FinallyIP: finallyIP}
		switch reason.Int() {
		case nativeFinallyFallThrough:
			continue
		case nativeFinallyReturn:
			entry.Kind, entry.Value = task.PendingReturn, value
		case nativeFinallyRaise:
			if value.Type() != types.TYPE_LIST || value.Len() != 4 {
				return nil, fmt.Errorf("native finally holds a raised error that is not {code, message, value, stack}")
			}
			entry.Kind, entry.Error = task.PendingError, task.VMErrorSnapshot{Present: true, Value: value}
			// raise() of a value that is not an error carries no error code.
			if code := value.Get(1); code.Type() == types.TYPE_ERR {
				entry.Error.Code = code.ErrCode()
			}
		case nativeFinallyUncaught:
			entry.Kind = task.PendingAbort
		case nativeFinallyExit:
			if value.Type() != types.TYPE_LIST || value.Len() != 2 || value.Get(1).Type() != types.TYPE_INT || value.Get(2).Type() != types.TYPE_INT {
				return nil, fmt.Errorf("native finally holds a loop exit that is not {stack depth, pc}")
			}
			// Every break or continue with these operands leaves the same
			// loop the same way, so any one of them gives Barn's jump.
			for _, exit := range vector.Exits {
				if int64(exit.Stack) != value.Get(1).Int() || int64(exit.PC) != value.Get(2).Int() {
					continue
				}
				if ip, recorded := importing.exits[exit.Node]; recorded {
					if target, depth, ok := unwindJumpAt(compiled, ip); ok {
						entry.Kind, entry.TargetIP, entry.HandlerDepth = task.PendingJump, target, depth
						break
					}
				}
			}
			if entry.Kind != task.PendingJump {
				return nil, fmt.Errorf("native finally holds loop exit %s, which no break or continue of its program makes", value)
			}
			// The jump resumes with the operands held where the loop's
			// statements run, which the finally body still holds beneath its own.
			heights, err := bytecode.StackHeights(compiled)
			if err != nil {
				return nil, err
			}
			if height := heights[entry.TargetIP]; height < 0 || height > len(stack) {
				return nil, fmt.Errorf("native loop exit target holds %d operands, the frame %d", height, len(stack))
			} else {
				entry.Stack = append([]types.Value(nil), stack[:height]...)
			}
		default:
			return nil, fmt.Errorf("unknown native finally reason %d", reason.Int())
		}
		pending = append(pending, entry)
	}
	return pending, nil
}

// exportFinallyReasons is importFinallyReasons's inverse: for each finally
// body the call site is inside it adds to values the reason and value Toast
// holds, from the frame's pending reasons. Described is the frame's program,
// which starts offset bytes into the compile importing observed. A pending
// reason with no such Toast operands is an error.
func exportFinallyReasons(described *bytecode.Program, offset int, importing *barnImportLayout, vector *NativeVectorLayout, site *NativeContinuation, pending []task.PendingReasonSnapshot, values nativeHeld) error {
	matched := 0
	for _, control := range site.Controls {
		if control.Kind != NativeFinallyHandler {
			continue
		}
		setup, recorded := importing.finallys[control.Node]
		finallyIP, found := finallyIPAt(described, setup-offset)
		if !recorded || !found {
			return fmt.Errorf("finally body has no Barn handler")
		}
		reason, value := nativeFinallyFallThrough, types.NewInt(0)
		for _, entry := range pending {
			if entry.FinallyIP != finallyIP {
				continue
			}
			matched++
			switch entry.Kind {
			case task.PendingReturn:
				reason, value = nativeFinallyReturn, entry.Value
			case task.PendingError:
				if !entry.Error.Present || entry.Error.Value.Type() != types.TYPE_LIST || entry.Error.Value.Len() != 4 {
					return fmt.Errorf("pending error holds no {code, message, value, stack}")
				}
				reason, value = nativeFinallyRaise, entry.Error.Value
			case task.PendingAbort:
				reason = nativeFinallyUncaught
			case task.PendingJump:
				for _, exit := range vector.Exits {
					ip, recorded := importing.exits[exit.Node]
					if target, depth, ok := unwindJumpAt(described, ip-offset); recorded && ok && target == entry.TargetIP && depth == entry.HandlerDepth {
						reason, value = nativeFinallyExit, types.NewList([]types.Value{types.NewInt(int64(exit.Stack)), types.NewInt(int64(exit.PC))})
						break
					}
				}
				if reason != nativeFinallyExit {
					return fmt.Errorf("pending loop exit to %d is not a break or continue Toast compiles", entry.TargetIP)
				}
			default:
				return fmt.Errorf("pending finally reason kind %d has no Toast form", entry.Kind)
			}
		}
		values[nativeHeldKey{NativeValueOrigin{Kind: NativeFinallyReason, Node: control.Node}, importIdentity}] = types.NewInt(int64(reason))
		values[nativeHeldKey{NativeValueOrigin{Kind: NativeFinallyValue, Node: control.Node}, importIdentity}] = value
	}
	if matched != len(pending) {
		return fmt.Errorf("pending finally reason has no Toast form")
	}
	return nil
}
