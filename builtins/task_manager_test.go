package builtins

import (
	"github.com/MongooseMoo/barn/task"
)

func wireTestTaskManager(ctx *Execution) *task.Manager {
	manager := task.NewManager()
	registry := ctx.Registry
	if registry == nil {
		registry = NewRegistry()
		ctx.Registry = registry
	}
	registry.SetTaskManager(manager)
	return manager
}
