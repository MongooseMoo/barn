package engine

import (
	"github.com/MongooseMoo/barn/metrics"
)

// BindCommandDiagnostic runs from task factories' onStart callbacks, before
// execution. Root command tasks and their error handlers share the identity;
// unrelated forks receive fresh contexts and do not inherit it.
func (s *Runtime) BindCommandDiagnostic(id int64, diagnostic *metrics.Command) {
	if diagnostic == nil {
		return
	}
	if t := s.GetTask(id); t != nil {
		t.Context.CommandDiagnostic = diagnostic
		diagnostic.TaskStarted(id)
	}
}

func taskRuntimeIdentity(t *Task) metrics.Identity {
	id := metrics.Identity{TaskID: t.ID, This: int64(t.This), Verb: t.VerbName}
	if t.Context != nil && t.Context.CommandDiagnostic != nil {
		id.CommandID = t.Context.CommandDiagnostic.ID
	}
	return id
}
