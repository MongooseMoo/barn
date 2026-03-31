package vm

import (
	"barn/task"
	"barn/types"
	"fmt"
)

// makeEvalErrorValue formats eval() runtime failures as a LIST of lines.
// This matches the contract expected by ToastCore eval verbs:
// {0, {"line 1", "line 2", ...}}
func makeEvalErrorValue(errCode types.ErrorCode, errMsg string, callStack interface{}) types.Value {
	if errMsg == "" {
		errMsg = errCode.Message()
	}
	if stack, ok := callStack.([]task.ActivationFrame); ok && len(stack) > 0 {
		return types.NewList(formatEvalTracebackLines(stack, errCode, errMsg))
	}
	return types.NewList([]types.Value{types.NewStr(errMsg)})
}

// makeEvalErrorValueFromFrames builds eval() runtime error lines from the live VM
// frame stack, including the synthetic built-in eval() call boundary.
func (vm *VM) makeEvalErrorValueFromFrames(errCode types.ErrorCode, errMsg string) types.Value {
	if errMsg == "" {
		errMsg = errCode.Message()
	}
	if len(vm.Frames) == 0 {
		return types.NewList([]types.Value{types.NewStr(errMsg)})
	}

	lines := make([]types.Value, 0, len(vm.Frames)+3)
	top := len(vm.Frames) - 1
	topLine := vm.CurrentLine()
	if topLine < 1 {
		topLine = 1
	}

	for i := top; i >= 0; i-- {
		frame := vm.Frames[i]
		line := 1
		if i == top {
			line = topLine
		} else if frame.Program != nil {
			ip := frame.IP - 1
			if ip < 0 {
				ip = 0
			}
			line = frame.Program.LineForIP(ip)
			if line < 1 {
				line = 1
			}
		}

		verbName := frame.Verb
		if frame.IsEvalFrame && verbName == "" {
			verbName = "Input to EVAL"
		}

		if i == top {
			lines = append(lines, types.NewStr(fmt.Sprintf(
				"#%d:%s (this == #%d), line %d:  %s",
				frame.VerbLoc, verbName, frame.This, line, errMsg,
			)))
		} else {
			lines = append(lines, types.NewStr(fmt.Sprintf(
				"... called from #%d:%s (this == #%d), line %d",
				frame.VerbLoc, verbName, frame.This, line,
			)))
		}

		if frame.IsEvalFrame && i > 0 {
			lines = append(lines, types.NewStr("... called from built-in function eval()"))
		}
	}

	lines = append(lines, types.NewStr("(End of traceback)"))
	return types.NewList(lines)
}

func formatEvalTracebackLines(stack []task.ActivationFrame, errCode types.ErrorCode, errMsg string) []types.Value {
	if errMsg == "" {
		errMsg = errCode.Message()
	}
	lines := make([]types.Value, 0, len(stack)+3)
	for i := len(stack) - 1; i >= 0; i-- {
		frame := stack[i]
		line := frame.LineNumber
		if line < 1 {
			line = 1
		}

		verbName := frame.Verb
		if frame.IsEvalFrame && verbName == "" {
			verbName = "Input to EVAL"
		}

		if i == len(stack)-1 {
			lines = append(lines, types.NewStr(fmt.Sprintf(
				"#%d:%s (this == #%d), line %d:  %s",
				frame.VerbLoc, verbName, frame.This, line, errMsg,
			)))
		} else {
			lines = append(lines, types.NewStr(fmt.Sprintf(
				"... called from #%d:%s (this == #%d), line %d",
				frame.VerbLoc, verbName, frame.This, line,
			)))
		}

		if frame.IsEvalFrame && i > 0 {
			lines = append(lines, types.NewStr("... called from built-in function eval()"))
		}
	}
	lines = append(lines, types.NewStr("(End of traceback)"))
	return lines
}
