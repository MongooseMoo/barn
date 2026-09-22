package engine

import (
	"context"
	"github.com/MongooseMoo/barn/internal/admission"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func inputAdmissionKey(player types.ObjID) admission.Key {
	if player < 0 {
		return admission.Key{Group: admission.Anonymous, Class: admission.Input}
	}
	return admission.Key{Principal: int64(player), Class: admission.Input}
}
func backgroundAdmissionKey(t *task.Task) admission.Key {
	programmer := t.Programmer
	if stack := t.GetCallStack(); len(stack) > 0 {
		programmer = stack[len(stack)-1].Programmer
	}
	return admission.Key{Principal: int64(programmer), Class: admission.Background}
}
func (s *Runtime) enterInput(player types.ObjID, system bool) (*admission.Scope, error) {
	ctx, key := s.inputAdmissionContext, inputAdmissionKey(player)
	if system {
		ctx = s.ctx
		key = admission.Key{Group: admission.System, Class: admission.Background}
	}
	return s.admission.Enter(ctx, key)
}
func (s *Runtime) CloseInputAdmission()            { s.closeInputAdmission() }
func (s *Runtime) AdmissionStats() admission.Stats { return s.admission.Snapshot() }

// WithCheckpointBarrier captures tasks and store state at one cooperative
// boundary. The callback must not execute MOO hooks. Mutable WAIF graphs require
// retaining this barrier until serialization has consumed the snapshots.
func (s *Runtime) WithCheckpointBarrier(write func() error) error {
	resume, err := s.admission.Pause(s.ctx)
	if err != nil {
		return err
	}
	defer resume()
	s.lifecycle.SweepMu.Lock()
	defer s.lifecycle.SweepMu.Unlock()
	s.lifecycle.VMStartMu.Lock()
	defer s.lifecycle.VMStartMu.Unlock()
	return write()
}

// enterBackground is called on an unclaimed task. Kill publishes cancellation
// before the wait and a final state check prevents resurrection at the lease.
func (s *Runtime) enterBackground(t *task.Task) (*admission.Scope, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(s.ctx)
	t.SetCancelFunc(cancel)
	scope, err := s.admission.Enter(ctx, backgroundAdmissionKey(t))
	return scope, ctx, cancel, err
}

// Direct entry arguments have no published VM yet. Pin them before waiting so
// a quiescent sweep can proceed without losing anonymous or waif roots.
type admissionRoot struct{ runtime *Runtime }

func (s *Runtime) pinAdmissionRoots(obj types.ObjID, args []types.Value) *admissionRoot {
	r := &admissionRoot{runtime: s}
	if s.lifecycle.ExecutionStartObserver != nil {
		s.lifecycle.ExecutionStartObserver()
	}
	s.lifecycle.VMStartMu.Lock()
	defer s.lifecycle.VMStartMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admissionRoots == nil {
		s.admissionRoots = make(map[*admissionRoot][]types.Value)
	}
	values := append([]types.Value{types.NewAnon(obj)}, args...)
	s.admissionRoots[r] = values
	return r
}
func (r *admissionRoot) release() {
	r.runtime.mu.Lock()
	defer r.runtime.mu.Unlock()
	delete(r.runtime.admissionRoots, r)
}

// Called under the runtime mutex and the collector's VM start barrier.
func (s *Runtime) collectAdmissionRoots(anon map[types.ObjID]struct{}, waifs *types.WaifSet) {
	for _, values := range s.admissionRoots {
		for _, value := range values {
			vm.CollectAnonymousRefsFromValue(value, anon)
			if waifs != nil {
				var found []types.Value
				vm.CollectWaifsFromValue(value, &found)
				for _, w := range found {
					waifs.Add(w)
				}
			}
		}
	}
}
