package engine

import (
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// Finalization at released cost (issue #1083, design C2 in
// agent-notes/fix-finalization-order-released-cost.md).
//
// Toast queues a WAIF or anonymous object whose last reference a task slice
// dropped and runs its :recycle verb before the next task slice starts, so the
// releasing task resumes (or its command completes) after the verb ran. Barn
// cannot count references, so the batched sweep decides liveness by walking
// every root, at a time unrelated to the release. This file decides, at the
// releasing task's boundary and from that task's own state, the values only
// that task can hold (types.FinalizerTag): one it created and never handed to
// any other holder is garbage exactly when its own VM no longer holds it. Those
// are recycled here, on the task's goroutine, before the task is handed back.
// Everything else goes to the batched sweep as before.
//
// Other tasks running at the same time are not ordered against these recycle
// verbs (a known difference from Toast, whose main loop runs them before any
// other task slice).

// boundaryFinalization carries a completing slice's request for the boundary
// decision through settleCompletedTaskFinalizations, and its verdict back.
type boundaryFinalization struct {
	task     *Task
	held     []types.Value
	grantSeq uint64
	verdict  confinedVerdict
}

func (b *boundaryFinalization) deadValues() []types.Value {
	if b == nil {
		return nil
	}
	return b.verdict.dead
}

// confinedVerdict is one boundary's decision over the values its slice
// released.
type confinedVerdict struct {
	// dead are the WAIFs and anonymous objects to recycle now.
	dead []types.Value
	// sweepWaifs are the released WAIFs left to the batched sweep.
	sweepWaifs []types.Value
	// undecided counts the released values that neither are dead nor were
	// shown alive here; the sweep settles them, unordered. A boundary that
	// should hold its task until they are settled would test this count.
	undecided int
}

// decideConfined classifies what a slice of t released: the WAIFs its VM
// recorded (releasedWaifs), the anonymous objects in its release ledger and
// the anonymous objects it created. exec is its VM after the slice's commit;
// held are further values its task state keeps (the slice's result, its call
// stack snapshot). grantSeq is the exclusive commit-gate grant the slice held
// at that commit, or 0. Values the decision leaves are anonymous objects (the
// creation-floor request the caller queues covers them) or sweepWaifs.
func (s *Runtime) decideConfined(t *Task, exec *vm.VM, releasedWaifs []types.Value, held []types.Value, grantSeq uint64) confinedVerdict {
	verdict := confinedVerdict{}
	createdAnon := t.TakeCreatedAnonymous()
	var confined []types.Value
	for _, waif := range releasedWaifs {
		if s.store.FinalizerTag(waif).ConfinedTo(t.ID, grantSeq) {
			confined = append(confined, waif)
		} else {
			verdict.sweepWaifs = append(verdict.sweepWaifs, waif)
			verdict.undecided++
		}
	}
	seenAnon := make(map[types.ObjID]struct{})
	considerAnon := func(id types.ObjID) {
		if _, seen := seenAnon[id]; seen {
			return
		}
		seenAnon[id] = struct{}{}
		value := types.NewAnon(id)
		if view, ok := s.store.Get(id); !ok || !view.Anonymous {
			return // already recycled
		}
		if s.store.FinalizerTag(value).ConfinedTo(t.ID, grantSeq) {
			confined = append(confined, value)
		} else {
			verdict.undecided++
		}
	}
	if exec != nil {
		for _, value := range exec.PendingFinalizations {
			if value.Type() == types.TYPE_ANON {
				considerAnon(value.ID())
			}
		}
	}
	for _, id := range createdAnon {
		considerAnon(id)
	}
	if len(confined) == 0 {
		return verdict
	}
	// The task's own holds, walked once for the whole slice.
	ownAnon := make(map[types.ObjID]struct{})
	ownWaifs := types.NewWaifSet(nil)
	if exec != nil {
		vm.CollectAnonymousRefsFromVM(exec, ownAnon)
		vm.CollectWaifsFromVMInto(exec, ownWaifs)
	}
	for _, value := range held {
		types.CollectAnonymousRefs(value, ownAnon, true)
		ownWaifs.AddReachable(value, true)
	}
	for _, frame := range t.GetCallStack() {
		for _, value := range activationValues(frame) {
			types.CollectAnonymousRefs(value, ownAnon, true)
			ownWaifs.AddReachable(value, true)
		}
	}
	for _, value := range confined {
		if value.Type() == types.TYPE_WAIF {
			if ownWaifs.Has(value) {
				// Alive in its own task: a later release records it again. The
				// sweep is where every released WAIF went before; keep it so.
				verdict.sweepWaifs = append(verdict.sweepWaifs, value)
			} else {
				verdict.dead = append(verdict.dead, value)
			}
			continue
		}
		if _, alive := ownAnon[value.ID()]; !alive {
			verdict.dead = append(verdict.dead, value)
		}
	}
	return verdict
}

// activationValues lists the values one call-stack activation snapshot holds.
func activationValues(frame types.ActivationFrame) []types.Value {
	values := append([]types.Value{frame.ThisValue, frame.RuntimeVariables}, frame.Args...)
	if snapshot := frame.RuntimeVariableSnapshot; snapshot != nil {
		values = append(values, snapshot.Values...)
	}
	return values
}

// recycleConfinedDead runs the recycle of each value decideConfined found dead,
// on the releasing task's goroutine, each in its own transaction exactly as
// the batched sweep runs it: a WAIF's :recycle through the sweep-owned verb
// runner, an anonymous object through recycle(), which calls its :recycle and
// destroys it. ctx is the releasing task's context.
func (s *Runtime) recycleConfinedDead(ctx *task.Context, dead []types.Value) {
	if len(dead) == 0 {
		return
	}
	recycleFn, haveRecycle := s.session.Registry().Get("recycle")
	for _, value := range dead {
		switch value.Type() {
		case types.TYPE_WAIF:
			if !value.BeginWaifRecycle() {
				continue
			}
			result := s.callVerbWithArgstr(value, ":recycle", nil, types.ObjNothing, "", vmOwnershipSweep, 0, nil)
			if result.Flow != types.FlowSuspend {
				s.oracleNoteFastDead(value)
			}
		case types.TYPE_ANON:
			if !haveRecycle {
				continue
			}
			gcCtx := s.gcRecycleContext(ctx)
			s.acquireSweepContext(gcCtx)
			_ = recycleFn(s.session.NewExecution(gcCtx, nil), []types.Value{value})
			s.releaseSweepContext(gcCtx)
			if view, ok := s.store.Get(value.ID()); !ok || !view.Anonymous {
				s.oracleNoteFastDead(value)
			}
		}
	}
}
