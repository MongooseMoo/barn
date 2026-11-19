package builtins

import "github.com/MongooseMoo/barn/types"

// DefaultFinishedTasksLimit is ToastStunt's SAVE_FINISHED_TASKS value from
// options.h: how many finished_tasks() records are kept when
// $server_options.finished_tasks_limit is not an integer.
const DefaultFinishedTasksLimit = 15

// FinishedTask is the record ToastStunt's run_interpreter (execute.cc) keeps
// for each interpreter run when SAVE_FINISHED_TASKS is defined: one per new
// task and one per resumption. The identity fields describe the activation on
// top of the stack when the run started.
type FinishedTask struct {
	This       types.Value
	Player     types.ObjID
	Programmer types.ObjID
	Receiver   types.ObjID
	Object     types.ObjID
	Verb       string
	FullVerb   string
	Foreground bool
	Suspended  bool
	Seconds    float64
}

// RecordsFinishedTasks reports whether finished_tasks() is registered, so
// servers built without it skip recording.
func (s *Session) RecordsFinishedTasks() bool {
	return s != nil && s.registry.Has("finished_tasks")
}

// RecordFinishedTask appends rec and keeps the newest limit records. Like
// Toast, at least one record is always kept.
func (s *Session) RecordFinishedTask(rec FinishedTask, limit int) {
	if s == nil {
		return
	}
	limit = max(limit, 1)
	ft := &s.runtime.finishedTasks
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.entries = append(ft.entries, rec)
	if over := len(ft.entries) - limit; over > 0 {
		n := copy(ft.entries, ft.entries[over:])
		clear(ft.entries[n:])
		ft.entries = ft.entries[:n]
	}
}

func (s *Session) finishedTaskValues() []types.Value {
	ft := &s.runtime.finishedTasks
	ft.mu.Lock()
	defer ft.mu.Unlock()
	values := make([]types.Value, 0, len(ft.entries))
	for _, rec := range ft.entries {
		values = append(values, rec.value())
	}
	return values
}

func finishedTaskName(name string) types.Value {
	if name == "" {
		return types.NewStr("n/a")
	}
	return types.NewStr(name)
}

func boolInt(b bool) types.Value {
	if b {
		return types.NewInt(1)
	}
	return types.NewInt(0)
}

func (rec FinishedTask) value() types.Value {
	this := rec.This
	if this.IsNone() {
		this = types.NewObj(types.ObjNothing)
	}
	return types.NewMap([][2]types.Value{
		{types.NewStr("this"), this},
		{types.NewStr("player"), types.NewObj(rec.Player)},
		{types.NewStr("programmer"), types.NewObj(rec.Programmer)},
		{types.NewStr("receiver"), types.NewObj(rec.Receiver)},
		{types.NewStr("object"), types.NewObj(rec.Object)},
		{types.NewStr("verb"), finishedTaskName(rec.Verb)},
		{types.NewStr("fullverb"), finishedTaskName(rec.FullVerb)},
		{types.NewStr("foreground"), boolInt(rec.Foreground)},
		{types.NewStr("suspended"), boolInt(rec.Suspended)},
		{types.NewStr("time"), types.NewFloat(rec.Seconds)},
	})
}
