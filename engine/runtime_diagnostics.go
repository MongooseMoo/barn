package engine

import (
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
)

func taskRuntimeIdentity(t *task.State) metrics.Identity {
	return metrics.Identity{TaskID: t.ID, This: int64(t.This), Verb: t.VerbName}
}
