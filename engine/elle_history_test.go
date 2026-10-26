package engine

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	taskpkg "github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// The correctness witness contains only values observed by MOO. Context identity
// groups observations until publication, but no store timestamp/version is used
// by Elle. Pending effects already implement commit/discard and carry through hot
// renewal; an irreversible boundary flushes the current group inside a slice.
type elleTxn struct {
	task       *taskpkg.Task
	context    *taskpkg.Context
	start, end int
	ops        [][]any
	committed  bool
}

type elleRecorder struct {
	sync.Mutex
	clock  int
	active map[*taskpkg.Context]*elleTxn
	txns   []*elleTxn
}

func newElleRecorder() *elleRecorder {
	return &elleRecorder{active: make(map[*taskpkg.Context]*elleTxn)}
}

func elleObserved(v types.Value) (any, error) {
	switch v.Type() {
	case types.TYPE_INT:
		return v.Int(), nil
	case types.TYPE_STR:
		return v.Str(), nil
	case types.TYPE_LIST:
		list := make([]any, v.Len())
		for i, item := range v.Elements() {
			value, err := elleObserved(item)
			if err != nil {
				return nil, err
			}
			list[i] = value
		}
		return list, nil
	default:
		return nil, fmt.Errorf("unsupported Elle witness type %v", v.Type())
	}
}

func (r *elleRecorder) observe(ctx *builtins.Execution, args []types.Value) types.Result {
	value, err := elleObserved(args[2])
	if err != nil {
		return types.Err(types.E_TYPE)
	}
	op := args[0].Str()
	if op != "r" && op != "w" && op != "append" {
		return types.Err(types.E_INVARG)
	}
	r.Lock()
	defer r.Unlock()
	tx := r.active[ctx.Context]
	if tx == nil {
		r.clock++
		tx = &elleTxn{task: ctx.Task, context: ctx.Context, start: r.clock}
		r.active[ctx.Context] = tx
		r.txns = append(r.txns, tx)
		taskCtx := ctx.Context
		ctx.PendingEffects = append(ctx.PendingEffects, taskpkg.PendingEffect{
			Kind: taskpkg.PendingEffectAsyncStart,
			Start: func() {
				r.Lock()
				defer r.Unlock()
				r.clock++
				tx.end, tx.committed = r.clock, true
				if tx.task != nil && tx.taskpkg.IsForked {
					isolationprobe.Note("fork")
				}
				if tx.task != nil && tx.taskpkg.Result.Flow == types.FlowException {
					isolationprobe.Note("native_exception")
				}
				readOnly := true
				for _, op := range tx.ops {
					if op[0] != "r" {
						readOnly = false
					}
				}
				if readOnly {
					isolationprobe.Note("read_only")
				}
				if r.active[taskCtx] == tx {
					delete(r.active, taskCtx)
				}
			},
		})
	}
	tx.ops = append(tx.ops, []any{op, args[1].Str(), value})
	return types.Ok(types.NewInt(0))
}

func elleEDN(value any) string {
	switch v := value.(type) {
	case nil:
		return "nil"
	case string:
		return strconv.Quote(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = elleEDN(item)
		}
		return "[" + strings.Join(parts, " ") + "]"
	default:
		panic(fmt.Sprintf("unsupported EDN value %T", value))
	}
}

// write is called only after the worker pool has drained. Unpublished retry
// attempts are discarded. A terminal failed commit is fail; a still-live or
// otherwise unconfirmed attempt is info. Native exceptions whose writes were
// published are ok, independently of the task's language return value.
func (r *elleRecorder) write(path string) (map[string]int, error) {
	r.Lock()
	defer r.Unlock()
	type event struct {
		index int
		text  string
	}
	events := []event{}
	counts := map[string]int{}
	for _, tx := range r.txns {
		outcome := "ok"
		if !tx.committed {
			if tx.task != nil && tx.taskpkg.ContextValue() != tx.context {
				counts["discarded_attempts"]++
				continue
			}
			outcome = "info"
			if isolationprobe.Failed(tx.context) {
				outcome = "fail"
			}
			r.clock++
			tx.end = r.clock
		}
		observed, invoked := []string{}, []string{}
		for _, op := range tx.ops {
			prefix := fmt.Sprintf("[:%s %s ", op[0], elleEDN(op[1]))
			observed = append(observed, prefix+elleEDN(op[2])+"]")
			value := op[2]
			if op[0] == "r" {
				value = nil
			}
			invoked = append(invoked, prefix+elleEDN(value)+"]")
		}
		// Every committed publication has its own process: serializable checks do
		// not impose a stronger session/realtime order on old read-only snapshots.
		for _, row := range []struct {
			index int
			kind  string
			ops   []string
		}{
			{tx.start, "invoke", invoked}, {tx.end, outcome, observed},
		} {
			events = append(events, event{row.index, fmt.Sprintf("{:index %d :process %d :type :%s :f :txn :value [%s]}", row.index, tx.start, row.kind, strings.Join(row.ops, " "))})
		}
		counts[outcome]++
		if outcome == "ok" {
			for _, op := range tx.ops {
				counts[fmt.Sprintf("%s/%s", op[0], op[1])]++
			}
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].index < events[j].index })
	var text strings.Builder
	for _, event := range events {
		text.WriteString(event.text)
		text.WriteByte('\n')
	}
	return counts, os.WriteFile(path, []byte(text.String()), 0600)
}
