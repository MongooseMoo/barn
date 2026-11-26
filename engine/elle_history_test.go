package engine

import (
	"fmt"
	"os"
	"slices"
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
	task       *taskpkg.State
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
				if tx.task != nil && tx.task.IsForked {
					isolationprobe.Note("fork")
				}
				if tx.task != nil && tx.task.Result.Flow == types.FlowException {
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

// checkTerminalAppends compares a designated MOO read with published appends.
// It is called after all tasks drain, separately from Elle's serializability check.
func (r *elleRecorder) checkTerminalAppends(reader *taskpkg.State, keys []string) error {
	expected := make(map[string]map[int64]bool, len(keys))
	for _, key := range keys {
		expected[key] = make(map[int64]bool)
	}
	observed := make(map[string][]any, len(keys))
	r.Lock()
	defer r.Unlock()
	for _, tx := range r.txns {
		if !tx.committed {
			continue
		}
		for _, op := range tx.ops {
			key := op[1].(string)
			tokens, check := expected[key]
			if !check {
				continue
			}
			switch {
			case op[0] == "append":
				token := op[2].(int64)
				if tokens[token] {
					return fmt.Errorf("%s: token %d was published twice", key, token)
				}
				tokens[token] = true
			case tx.task == reader && op[0] == "r":
				observed[key] = op[2].([]any)
			}
		}
	}
	for _, key := range keys {
		items, ok := observed[key]
		if !ok {
			return fmt.Errorf("%s: missing committed terminal read", key)
		}
		for _, item := range items {
			token := item.(int64)
			if !expected[key][token] {
				return fmt.Errorf("%s: terminal read repeated or invented token %d", key, token)
			}
			delete(expected[key], token)
		}
		if len(expected[key]) != 0 {
			return fmt.Errorf("%s: terminal read omitted committed tokens %v", key, expected[key])
		}
	}
	return nil
}

// write is called only after the worker pool has drained. Unpublished retry
// attempts are discarded. A terminal failed commit is fail; a still-live or
// otherwise unconfirmed attempt is info. Native exceptions whose writes were
// published are ok, independently of the task's language return value.
func (r *elleRecorder) write(path string, keys ...string) (map[string]int, error) {
	r.Lock()
	defer r.Unlock()
	type event struct {
		index int
		text  string
	}
	events := []event{}
	counts := map[string]int{}
	clock := r.clock
	for _, tx := range r.txns {
		end := tx.end
		outcome := "ok"
		if !tx.committed {
			if tx.task != nil && tx.task.ContextValue() != tx.context {
				counts["discarded_attempts"]++
				continue
			}
			outcome = "info"
			if isolationprobe.Failed(tx.context) {
				outcome = "fail"
			}
			clock++
			end = clock
		}
		observed, invoked := []string{}, []string{}
		for _, op := range tx.ops {
			if len(keys) != 0 && !slices.Contains(keys, op[1].(string)) {
				continue
			}
			prefix := fmt.Sprintf("[:%s %s ", op[0], elleEDN(op[1]))
			observed = append(observed, prefix+elleEDN(op[2])+"]")
			value := op[2]
			if op[0] == "r" {
				value = nil
			}
			invoked = append(invoked, prefix+elleEDN(value)+"]")
		}
		if len(observed) == 0 {
			continue
		}
		// Every committed publication has its own process: serializable checks do
		// not impose a stronger session/realtime order on old read-only snapshots.
		for _, row := range []struct {
			index int
			kind  string
			ops   []string
		}{
			{tx.start, "invoke", invoked}, {end, outcome, observed},
		} {
			events = append(events, event{row.index, fmt.Sprintf("{:index %d :process %d :type :%s :f :txn :value [%s]}", row.index, tx.start, row.kind, strings.Join(row.ops, " "))})
		}
		counts[outcome]++
		if outcome == "ok" {
			for _, op := range tx.ops {
				if len(keys) == 0 || slices.Contains(keys, op[1].(string)) {
					counts[fmt.Sprintf("%s/%s", op[0], op[1])]++
				}
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
