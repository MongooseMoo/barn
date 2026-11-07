package engine

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A reader keeps its first observation in one publication while a writer commits
// both the observed dependency and a hot property. The next hot read can renew
// only if everything the reader has already observed is still current.
func elleRenewalStepSlot() builtins.Descriptor {
	type pair struct {
		read, published         chan struct{}
		readOnce, publishedOnce sync.Once
	}
	pairs := [2]*pair{}
	for i := range pairs {
		pairs[i] = &pair{read: make(chan struct{}), published: make(chan struct{})}
	}
	var step builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
		index := args[0].Int()
		if index < 0 || index >= int64(len(pairs)) {
			return types.Err(types.E_INVARG)
		}
		p := pairs[index]
		wait := p.read
		switch args[1].Int() {
		case 0:
			p.readOnce.Do(func() { close(p.read) })
			wait = p.published
		case 1:
		case 2:
			ctx.PendingEffects = append(ctx.PendingEffects, task.PendingEffect{Kind: task.PendingEffectAsyncStart, Start: func() {
				p.publishedOnce.Do(func() { close(p.published) })
			}})
			return types.Ok(types.None)
		default:
			return types.Err(types.E_INVARG)
		}
		select {
		case <-wait:
			return types.Ok(types.None)
		case <-time.After(10 * time.Second):
			return types.Err(types.E_QUOTA)
		}
	}
	return testBuiltinSlot("elle_renew_step", 2, 2, []int64{int64(types.TYPE_INT), int64(types.TYPE_INT)}, &step)
}

func elleRenewalPairs(t *testing.T, rt *Runtime, store *dbstore.Store, id int64) int64 {
	t.Helper()
	renewedBefore, refusedBefore, _ := store.HotReadStats()
	elleDrain(t, rt, []*Task{
		elleTask(t, rt, id, `v=#0.elle_map["a"]; elle_observe("r","map-a",v); elle_renew_step(0,0); elle_observe("r","property",#11.data);`),
		elleTask(t, rt, id+1, `elle_renew_step(0,1);`+elleAppend(9, 210)+elleAppend(0, 211)+`elle_renew_step(0,2);`),
	})
	renewed, refused, _ := store.HotReadStats()
	if isolationprobe.Fault("hot-map-carry") {
		if renewed <= renewedBefore {
			t.Fatal("hot-map-carry pair did not renew")
		}
	} else if isolationprobe.ActiveFault() == "" && refused <= refusedBefore {
		t.Fatal("stale carried map dependency did not refuse renewal")
	}
	isolationprobe.Note("hot_map_pair")
	id += 2

	// Warm the negative dispatch memo without mutating anything. The reader's
	// missing call then depends on the memo's shape guard, not an existing verb's
	// value version. Creating the verb leaves every previously resolved verb alone.
	// #18's property participates in the append model, but its own verb list
	// is unused. The moving invariant objects already have scratchV/scratch;
	// assert the actual fixture is separate before adding our dispatch verb.
	const receiver = 18
	missing := fmt.Sprintf(`try v=#%d:renew_missing(); except (E_VERBNF) v={}; endtry `, receiver)
	isolated := fmt.Sprintf(`if (length(verbs(#%[1]d)) != 0 || "scratch" in properties(#%[1]d)) raise(E_INVARG); endif `, receiver)
	elleDrain(t, rt, []*Task{elleTask(t, rt, id, isolated+missing)})
	id++
	memoBefore := isolationprobe.Snapshot()["verb_memo"]
	renewedBefore, refusedBefore, _ = store.HotReadStats()
	elleDrain(t, rt, []*Task{
		elleTask(t, rt, id, missing+`elle_observe("r","hot-memo",v); elle_renew_step(1,0); elle_observe("r","property",#11.data);`),
		elleTask(t, rt, id+1, fmt.Sprintf(`elle_renew_step(1,1); n=add_verb(#%[1]d,{#0,"rxd","renew_missing"},{"this","none","none"}); set_verb_code(#%[1]d,n,{"return {212};"}); elle_observe("append","hot-memo",212);`, receiver)+elleAppend(0, 213)+`elle_renew_step(1,2);`),
	})
	renewed, refused, _ = store.HotReadStats()
	if isolationprobe.Fault("hot-memo-guard") {
		if renewed <= renewedBefore {
			t.Fatal("hot-memo-guard pair did not renew")
		}
	} else if isolationprobe.ActiveFault() == "" && refused <= refusedBefore {
		t.Fatal("stale memo dependency did not refuse renewal")
	}
	if isolationprobe.Snapshot()["verb_memo"] <= memoBefore {
		t.Fatal("hot memo pair did not use dispatch memo")
	}
	isolationprobe.Note("hot_memo_pair")
	elleDrain(t, rt, []*Task{elleTask(t, rt, id+2, fmt.Sprintf(`elle_observe("r","hot-memo",#%d:renew_missing()); elle_observe("r","property",#11.data);`, receiver))})
	return id + 3
}
