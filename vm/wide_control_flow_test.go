package vm

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

const oversizedControlFlowStatements = 25000

func oversizedAssignments(statement string) string {
	return strings.Repeat(statement, oversizedControlFlowStatements)
}

func TestOversizedCatchExpressionReachesHandler(t *testing.T) {
	expression := "{" + strings.Repeat("value,", oversizedControlFlowStatements) + "missing}"
	result := runBytecodeProgram(t, "value = 1; return `"+expression+" ! E_VARNF => 69';", nil, nil)
	requireInt(t, result, 69)
}

func TestOversizedTryExceptReachesHandler(t *testing.T) {
	code := "try " + oversizedAssignments("padding = 1;") +
		"failure = 1 / 0; except (E_DIV) return 69; endtry return 0;"
	result := runBytecodeProgram(t, code, nil, nil)
	requireInt(t, result, 69)
}

func TestOversizedTryFinallyReachesFinalizerWhileUnwinding(t *testing.T) {
	code := "value = 0; try try " + oversizedAssignments("padding = 1;") +
		"failure = 1 / 0; finally value = 69; endtry " +
		"except (E_DIV) return value; endtry return 0;"
	result := runBytecodeProgram(t, code, nil, nil)
	requireInt(t, result, 69)
}

func TestOversizedWhileLoopExecutesBackwardEdge(t *testing.T) {
	code := "iterations = 0; while (iterations < 1) " +
		oversizedAssignments("padding = 1;") +
		"iterations = iterations + 1; endwhile return iterations;"
	result := runBytecodeProgram(t, code, nil, nil)
	requireInt(t, result, 1)
}

func TestOversizedRangeLoopExecutesBackwardEdge(t *testing.T) {
	code := "iterations = 0; for value in [1..1] " +
		oversizedAssignments("padding = 1;") +
		"iterations = iterations + 1; endfor return iterations;"
	result := runBytecodeProgram(t, code, nil, nil)
	requireInt(t, result, 1)
}

func TestOversizedCollectionLoopExecutesBackwardEdge(t *testing.T) {
	code := "iterations = 0; for value in ({1}) " +
		oversizedAssignments("padding = 1;") +
		"iterations = iterations + 1; endfor return iterations;"
	result := runBytecodeProgram(t, code, nil, nil)
	requireInt(t, result, 1)
}

func TestOversizedForkParentSkipsCompleteBody(t *testing.T) {
	code := "padding = 0; fork (0) " + oversizedAssignments("padding = 1;") +
		"endfork return padding;"
	result := runBytecodeProgram(t, code, nil, nil)
	requireInt(t, result, 0)
}

func TestOversizedForkChildExecutesCompleteBody(t *testing.T) {
	code := "padding = 0; fork (0) " + oversizedAssignments("padding = 1;") +
		"return 70; endfork return 0;"
	registry := BuildVMRegistry()
	registry.SetTaskManager(task.NewManager())
	program, diagnostics := registry.Compiler().CompileMOO([]string{code})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO failed: %v", diagnostics)
	}

	store := dbstore.NewStore()
	parent := NewVM(store, registry)
	parent.Context, parent.Task = bytecodeTestContext(store)
	result := parent.Run(program)
	if result.Flow != types.FlowFork || result.ForkInfo == nil {
		t.Fatalf("parent result = flow %v, error %v, want fork", result.Flow, result.Error)
	}
	body, ok := result.ForkInfo.Body.([3]interface{})
	if !ok {
		t.Fatalf("fork body = %T, want bytecode tuple", result.ForkInfo.Body)
	}
	parentProgram, okProgram := body[0].(*bytecode.Program)
	bodyIP, okIP := body[1].(int)
	bodyLen, okLen := body[2].(int)
	if !okProgram || !okIP || !okLen {
		t.Fatalf("fork body tuple = %#v, want program, IP, length", body)
	}
	childProgram := parentProgram.ExtractForkBody(bodyIP, bodyLen)
	if childProgram == nil {
		t.Fatal("ExtractForkBody rejected compiler-produced fork range")
	}

	child := NewVM(store, registry)
	child.Context, child.Task = bytecodeTestContext(store)
	childResult := child.Run(childProgram)
	requireInt(t, childResult, 70)
}

func bytecodeTestContext(store *dbstore.Store) (*kernel.TaskContext, *task.Task) {
	ctx := kernel.NewTaskContext()
	ctx.Store = store
	return ctx, task.NewTask(1, types.ObjID(0), ctx.TicksRemaining, 1)
}
