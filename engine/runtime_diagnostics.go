package engine

import (
	"github.com/MongooseMoo/barn/metrics"
)

func taskRuntimeIdentity(t *Task) metrics.Identity {
	return metrics.Identity{TaskID: t.ID, This: int64(t.This), Verb: t.VerbName}
}
