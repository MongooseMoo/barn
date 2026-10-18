package engine

import (
	"context"
	"github.com/MongooseMoo/barn/internal/admission"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
	"log/slog"
	"time"
)

func inputAdmissionKey(player types.ObjID) admission.Key {
	if player < 0 {
		return admission.Key{Group: admission.Anonymous, Class: admission.Input}
	}
	return admission.Key{Principal: int64(player), Class: admission.Input}
}
func backgroundAdmissionKey(t *task.Task) admission.Key {
	programmer, frames := t.CallStackProgrammer(0)
	if frames == 0 {
		programmer = t.Programmer
	}
	// A task the command parser dispatched, running again after a suspension,
	// is still what its player is waiting for.
	return admission.Key{Principal: int64(programmer), Class: admission.Background, Command: t.FromCommand}
}
func (s *Runtime) enterInput(player types.ObjID, system bool) (*admission.Scope, error) {
	ctx, key := s.inputAdmissionContext, inputAdmissionKey(player)
	if system {
		ctx = s.ctx
		key = admission.Key{Group: admission.System, Class: admission.Background, Command: true}
	}
	started := time.Now()
	scope, err := s.admission.Enter(ctx, key)
	metrics.ObserveRuntime(metrics.InputAdmission, metrics.Identity{This: int64(player)}, time.Since(started), 0)
	return scope, err
}
func (s *Runtime) CloseInputAdmission()            { s.closeInputAdmission() }
func (s *Runtime) AdmissionStats() admission.Stats { return s.admission.Snapshot() }

// WithCheckpointBarrier captures tasks and store state at one cooperative
// boundary. The callback must not execute MOO hooks. Running checkpoints capture
// owned state here and materialize/serialize after returning; terminal dumps keep
// their complete publication inside the callback to exclude later task effects.
func (s *Runtime) WithCheckpointBarrier(write func() error) error {
	started := time.Now()
	resume, err := s.admission.Pause(s.ctx)
	if err != nil {
		return err
	}
	defer resume()
	drained := time.Now()
	s.lifecycle.SweepMu.Lock()
	defer s.lifecycle.SweepMu.Unlock()
	s.lifecycle.VMStartMu.Lock()
	defer s.lifecycle.VMStartMu.Unlock()
	entered := time.Now()
	defer func() {
		slog.Debug("checkpoint barrier timings",
			slog.Float64("admission_wait_ms", float64(drained.Sub(started))/float64(time.Millisecond)),
			slog.Float64("lifecycle_wait_ms", float64(entered.Sub(drained))/float64(time.Millisecond)),
			slog.Float64("callback_ms", float64(time.Since(entered))/float64(time.Millisecond)))
	}()
	return write()
}

// enterBackground is called on an unclaimed task. Kill publishes cancellation
// before the wait and a final state check prevents resurrection at the lease.
func (s *Runtime) enterBackground(t *task.Task) (*admission.Scope, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(s.ctx)
	t.SetCancelFunc(cancel)
	started := time.Now()
	scope, err := s.admission.Enter(ctx, backgroundAdmissionKey(t))
	metrics.ObserveRuntime(metrics.BackgroundAdmission, taskRuntimeIdentity(t), time.Since(started), 0)
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
