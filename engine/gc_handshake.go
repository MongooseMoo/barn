package engine

import (
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// The deferred sweep runs beside executing slices instead of pausing
// admission until they end. It asks every running VM to park at its tick
// checkpoint (the preempt hook), where the owner goroutine blocks keeping its
// VM, transaction and lease, so the VM is not being mutated and can be walked.

// parkForGC blocks a slice's VM at its tick checkpoint while a sweep holds a
// handshake. The VM is recorded under s.mu, after its last mutation, so the
// sweep reading it under s.mu sees a complete VM.
func (s *Runtime) parkForGC(taskID int64, machine *vm.VM) {
	if !s.gcHandshake.Load() {
		return
	}
	s.mu.Lock()
	release := s.gcRelease
	if release == nil {
		s.mu.Unlock()
		return
	}
	s.gcParked[taskID] = machine
	s.mu.Unlock()
	<-release
}

// requestGCHandshake asks running VMs to park and returns the function that
// releases them. Release forgets the parked set before waking anyone, so a
// later handshake never counts a VM that has resumed.
func (s *Runtime) requestGCHandshake() (release func()) {
	ch := make(chan struct{})
	s.mu.Lock()
	s.gcRelease = ch
	s.gcParked = make(map[int64]*vm.VM)
	s.mu.Unlock()
	s.gcHandshake.Store(true)
	return func() {
		s.gcHandshake.Store(false)
		s.mu.Lock()
		s.gcRelease = nil
		s.gcParked = nil
		s.mu.Unlock()
		close(ch)
	}
}

// awaitGCParked waits up to patience for every execution lease to be parked.
func (s *Runtime) awaitGCParked(patience time.Duration) bool {
	for deadline := time.Now().Add(patience); ; time.Sleep(50 * time.Microsecond) {
		s.mu.Lock()
		parked := s.leasesParkedLocked()
		s.mu.Unlock()
		if parked || !time.Now().Before(deadline) {
			return parked
		}
	}
}

// leasesParkedLocked reports whether every physical execution lease belongs to
// a VM parked at its tick checkpoint. A task holding more than one lease has
// an outer VM inside a builtin, which never parks. Caller holds s.mu.
func (s *Runtime) leasesParkedLocked() bool {
	for id, count := range s.lifecycle.ExecutingTasks {
		if count != 1 || s.gcParked[id] == nil {
			return false
		}
	}
	return true
}

// collectParkedRoots adds a parked VM's roots: its own values and the property
// values only its transaction holds (staged writes, anonymous read copies).
func collectParkedRoots(exec *vm.VM, anon map[types.ObjID]struct{}, waifs *types.WaifSet) {
	vm.CollectAnonymousRefsFromVM(exec, anon)
	vm.CollectWaifsFromVMInto(exec, waifs)
	if exec.Context == nil {
		return
	}
	exec.Context.StoreTxn.VisitPrivatePropertyValues(func(value types.Value) {
		types.CollectAnonymousRefs(value, anon, true)
		waifs.AddReachable(value, true)
	})
}

// addParkedHistoryRoots adds what each parked snapshot is served from history
// instead of the live image. History retained for idle readers (a suspended
// task's unused snapshot) is not a root: an entry judged live is dropped from
// the batch for good, so that would lose its :recycle. Parked VMs stay parked
// until the sweep releases them, so their transactions are read after s.mu.
func (s *Runtime) addParkedHistoryRoots(anon map[types.ObjID]struct{}, waifs []types.Value) []types.Value {
	s.mu.Lock()
	txns := make([]*dbstore.StoreTxn, 0, len(s.gcParked))
	for _, exec := range s.gcParked {
		if exec.Context != nil {
			txns = append(txns, exec.Context.StoreTxn)
		}
	}
	s.mu.Unlock()
	set := types.NewWaifSet(waifs)
	for _, txn := range txns {
		txn.VisitSnapshotHistoryValues(func(value types.Value) {
			types.CollectAnonymousRefs(value, anon, true)
			set.AddReachable(value, true)
		})
	}
	return set.Values
}
