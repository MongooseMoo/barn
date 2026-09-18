package builtins

import (
	"github.com/MongooseMoo/barn/task"
)

func wireTestTaskManager(ctx *Execution) *task.Manager {
	manager := task.NewManager()
	session := ctx.Session
	if session == nil {
		registry := ctx.Registry
		if registry == nil {
			registry = NewRegistry()
			ctx.Registry = registry
		}
		session = NewSession(registry, NoHost())
		ctx.Session = session
	}
	configureTestHost(session, func(host *Host) { host.TaskManager = manager })
	return manager
}
