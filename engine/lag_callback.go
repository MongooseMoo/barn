package engine

import (
	"log/slog"
	"strings"
	"time"

	"github.com/MongooseMoo/barn/internal/admission"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// Keep entry metadata before a completed activation releases its frame.
type lagInvocation struct {
	definer types.ObjID
	names   []string
	verb    string
}

func (info *lagInvocation) capture(frame *vm.StackFrame) {
	if frame != nil {
		info.definer = frame.VerbLoc
		info.names = frame.StoredVerbNames
		info.verb = frame.StoredVerb
		if info.verb == "" {
			info.verb = frame.Verb
		}
	}
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
	name := info.verb
	if len(info.names) > 0 {
		name = strings.Join(info.names, " ")
	}
	slog.Warn("LAG", slog.Float64("seconds", elapsed.Seconds()), slog.Int64("object", int64(info.definer)), slog.String("verb", name))
	if _, _, err := tx.FindCallableVerb(0, "handle_lagging_task"); err != nil {
		return
	}
	var payload types.Value
	if result.Flow == types.FlowException || result.Flow == types.FlowSuspend {
		stack := result.CallStack
		if len(stack) == 0 {
			stack = t.GetCallStack()
		}
		frames := make([]types.Value, 0, len(stack))
		for i := len(stack) - 1; i >= 0; i-- {
			frame := stack[i].ToList()
			if s.session.IncludeRTVars(t.Context) {
				frame = types.NewList(append(frame.Elements(), stack[i].RuntimeVariableMap()))
			}
			frames = append(frames, frame)
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
