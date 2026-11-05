package vm

import (
	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func newTestSession(registry *builtins.Registry) *builtins.Session {
	return builtins.NewSession(registry, builtins.NoHost())
}

func newTestSessionWithTaskManager(registry *builtins.Registry) *builtins.Session {
	return builtins.NewSession(registry, builtins.Host{TaskManager: emptyTaskHost{}})
}

// Standalone interpreter tests have no registered tasks. Scheduling and catalog
// behavior are exercised by engine tests with executable task ownership.
type emptyTaskHost struct{}

func (emptyTaskHost) GetAllTasks() []*task.State                   { return nil }
func (emptyTaskHost) GetQueuedTasks() []*task.State                { return nil }
func (emptyTaskHost) FindTaskState(int64) *task.State              { return nil }
func (emptyTaskHost) FindReadingTaskState(types.ObjID) *task.State { return nil }
