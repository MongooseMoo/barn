package engine

import (
	"log/slog"
	"strings"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/internal/admission"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// Keep entry metadata before a completed activation releases its frame. The
// lag report and the finished_tasks() record both describe the activation on
// top of the stack when the slice started, as Toast's run_interpreter does.
type lagInvocation struct {
	definer types.ObjID
	names   []string
	verb    string

	invoked    string      // the verb name as called (Toast activation.verb)
	this       types.Value // Toast activation._this
	player     types.ObjID
	programmer types.ObjID
	receiver   types.ObjID // Toast activation.recv
	resumed    bool        // the slice continues a suspended VM
}

func (info *lagInvocation) capture(frame *vm.StackFrame, programmer types.ObjID) {
	if frame != nil {
		info.definer = frame.VerbLoc
		info.names = frame.StoredVerbNames
		info.verb = frame.StoredVerb
		if info.verb == "" {
			info.verb = frame.Verb
		}
		info.invoked = frame.Verb
		info.player = frame.Player
		info.programmer = programmer
		// Barn's frame.This is the object the verb was looked up on: the
		// waif's class or the primitive's prototype. Toast's recv is that
		// object, except #-1 for an anonymous object.
		info.receiver = frame.This
		info.this = types.NewObj(frame.This)
		if !frame.ThisValue.IsNone() {
			info.this = frame.ThisValue
			if frame.ThisValue.Type() == types.TYPE_OBJ && frame.ThisValue.IsAnonymous() {
				info.receiver = types.ObjNothing
			}
		}
	}
}

// fullVerb is the verb's whole names string (Toast activation.verbname).
func (info *lagInvocation) fullVerb() string {
	if len(info.names) > 0 {
		return strings.Join(info.names, " ")
	}
	return info.verb
}

// recordFinishedTask keeps Toast's SAVE_FINISHED_TASKS record for one slice.
// Toast marks new command and server tasks foreground and forked tasks and
// every resumption background.
func (s *Runtime) recordFinishedTask(t *Task, result types.Result, elapsed time.Duration, info lagInvocation) {
	if !s.session.RecordsFinishedTasks() {
		return
	}
	limit := builtins.DefaultFinishedTasksLimit
	tx := s.store.DirectTxn()
	if options, code := tx.FindProperty(0, "server_options"); code == types.E_NONE && options.Value.Type() == types.TYPE_OBJ {
		if option, code := tx.FindProperty(options.Value.Obj(), "finished_tasks_limit"); code == types.E_NONE && option.Value.Type() == types.TYPE_INT {
			limit = int(option.Value.Int())
		}
	}
	// finished_tasks() hands the record to any task.
	s.store.MarkFinalizersEscaped(info.this)
	s.session.RecordFinishedTask(builtins.FinishedTask{
		This:       info.this,
		Player:     info.player,
		Programmer: info.programmer,
		Receiver:   info.receiver,
		Object:     info.definer,
		Verb:       info.invoked,
		FullVerb:   info.fullVerb(),
		Foreground: t.Kind != task.TaskForked && !info.resumed,
		Suspended:  result.Flow == types.FlowSuspend,
		Seconds:    elapsed.Seconds(),
	}, limit)
}

func (s *Runtime) reportLaggingTask(t *Task, scope *admission.Scope, result types.Result, elapsed time.Duration, info lagInvocation) {
	if elapsed < 100*time.Millisecond {
		return
	}
	threshold := 5.0
	tx := s.store.DirectTxn()
	if options, code := tx.FindProperty(0, "server_options"); code == types.E_NONE && options.Value.Type() == types.TYPE_OBJ {
		if option, code := tx.FindProperty(options.Value.Obj(), "task_lag_threshold"); code == types.E_NONE && option.Value.Type() == types.TYPE_FLOAT {
			threshold = option.Value.Float()
		}
	}
	if !(threshold >= 0.1 && elapsed.Seconds() >= threshold) {
		return
	}
	name := info.fullVerb()
	slog.Warn("LAG", slog.Float64("seconds", elapsed.Seconds()), slog.Int64("object", int64(info.definer)), slog.String("verb", name))
	if _, _, err := tx.FindCallableVerb(0, "handle_lagging_task"); err != nil {
		return
	}
	var payload types.Value
	if result.Flow == types.FlowException || result.Flow == types.FlowSuspend {
		stack := result.CallStack
		if len(stack) == 0 {
			stack = types.ExpandBuiltinFrames(t.GetCallStack())
		}
		frames := make([]types.Value, 0, len(stack))
		for i := len(stack) - 1; i >= 0; i-- {
			frames = append(frames, stack[i].ToList(true, s.session.IncludeRTVars(t.Context)))
		}
		payload = types.NewList(frames)
	} else {
		nothing := types.NewObj(types.ObjNothing)
		payload = types.NewList([]types.Value{nothing, types.NewStr(name), nothing, types.NewObj(info.definer), nothing, nothing})
	}
	_, err := s.runServerVerbTask(0, "handle_lagging_task", []types.Value{payload, types.NewFloat(elapsed.Seconds())}, t.Owner, "", nil, scope, true)
	if err != nil {
		slog.Warn("lag callback failed", slog.Any("err", err))
	}
}
