package vm

import (
	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/task"
)

func newTestSession(registry *builtins.Registry) *builtins.Session {
	return builtins.NewSession(registry, builtins.NoHost())
}

func newTestSessionWithTaskManager(registry *builtins.Registry) *builtins.Session {
	return builtins.NewSession(registry, builtins.Host{TaskManager: task.NewManager()})
}
