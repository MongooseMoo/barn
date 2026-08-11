package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestBuildTracebackInvalidatesAnonymousThis(t *testing.T) {
	taskValue := task.NewTask(1, 2, 1000, 1)
	taskValue.PushFrame(task.ActivationFrame{
		This:       12,
		ThisValue:  types.NewAnon(12),
		Programmer: 2,
		Verb:       "boom",
		VerbLoc:    0,
		Player:     2,
	})
	ctx := kernel.NewTaskContext()

	machine := NewVM(nil, nil)
	machine.Context = ctx
	machine.Task = taskValue
	traceback := machine.buildTraceback(false)
	thisValue := traceback.Get(1).Get(1)
	if thisValue.Type() != types.TYPE_ANON {
		t.Fatalf("traceback this type = %v, want ANON", thisValue.Type())
	}
	if thisValue.ID() != types.ObjNothing {
		t.Fatalf("traceback this = %s, want invalid anonymous value", thisValue.String())
	}
}
