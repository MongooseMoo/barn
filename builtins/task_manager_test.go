package builtins

import (
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
)

func wireTestTaskManager(ctx *kernel.TaskContext) *task.Manager {
	manager := task.NewManager()
	registry, ok := ctx.Registry.(*Registry)
	if !ok {
		registry = NewRegistry()
		ctx.Registry = registry
	}
	registry.SetTaskManager(manager)
	return manager
}
