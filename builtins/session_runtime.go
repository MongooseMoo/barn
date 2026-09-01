package builtins

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/MongooseMoo/barn/types"
)

// sessionRuntime owns the mutable resources created by builtin calls. Keeping
// it private prevents callers from bypassing Session's lifecycle.
type sessionRuntime struct {
	files struct {
		mu      sync.Mutex
		nextID  int64
		handles map[int64]*mooFileHandle
	}
	sqlite struct {
		mu      sync.Mutex
		nextID  int64
		handles map[int64]*sqliteHandle
	}
	recycle struct {
		mu  sync.Mutex
		ids map[types.ObjID]int
	}
	protected         atomic.Pointer[protectedSet]
	serverOptions     serverOptionsState
	connectionOptions struct {
		mu       sync.RWMutex
		byPlayer map[types.ObjID]map[string]types.Value
	}
	heldCommands struct {
		mu       sync.Mutex
		byPlayer map[types.ObjID][]string
	}
	heldHTTPInput struct {
		mu       sync.Mutex
		byPlayer map[types.ObjID]*httpHeldInput
	}
}

type serverOptionsState struct {
	mu                 sync.RWMutex
	maxStringConcat    int
	maxListValueBytes  int
	maxMapValueBytes   int
	fgTicks            int64
	bgTicks            int64
	fgSeconds          float64
	bgSeconds          float64
	maxStackDepth      int
	maxCryptBcryptCost int
	maxCryptSHARounds  int
}

func newSessionRuntime() *sessionRuntime {
	s := &sessionRuntime{}
	s.files.nextID, s.files.handles = 1, make(map[int64]*mooFileHandle)
	s.sqlite.nextID, s.sqlite.handles = 1, make(map[int64]*sqliteHandle)
	s.recycle.ids = make(map[types.ObjID]int)
	s.connectionOptions.byPlayer = make(map[types.ObjID]map[string]types.Value)
	s.heldCommands.byPlayer = make(map[types.ObjID][]string)
	s.heldHTTPInput.byPlayer = make(map[types.ObjID]*httpHeldInput)
	s.protected.Store(&protectedSet{byName: map[string]bool{}})
	defaults := defaultServerOptionsSnapshot()
	s.serverOptions.maxStringConcat = defaults.MaxStringConcat
	s.serverOptions.maxListValueBytes = defaults.MaxListValueBytes
	s.serverOptions.maxMapValueBytes = defaults.MaxMapValueBytes
	s.serverOptions.fgTicks = defaults.FgTicks
	s.serverOptions.bgTicks = defaults.BgTicks
	s.serverOptions.fgSeconds = defaults.FgSeconds
	s.serverOptions.bgSeconds = defaults.BgSeconds
	s.serverOptions.maxStackDepth = defaults.MaxStackDepth
	s.serverOptions.maxCryptBcryptCost = defaults.MaxCryptBcryptCost
	s.serverOptions.maxCryptSHARounds = defaults.MaxCryptSHARounds
	return s
}

// Close deterministically releases every external resource owned by s. It is
// idempotent; sessions remain independent even when their handle IDs overlap.
func (s *Session) Close() error {
	if s == nil || s.runtime == nil {
		return nil
	}
	r := s
	var closeErrors []error
	r.runtime.files.mu.Lock()
	files := r.runtime.files.handles
	r.runtime.files.handles = make(map[int64]*mooFileHandle)
	r.runtime.files.mu.Unlock()
	for _, h := range files {
		if err := h.file.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}

	r.runtime.sqlite.mu.Lock()
	handles := r.runtime.sqlite.handles
	r.runtime.sqlite.handles = make(map[int64]*sqliteHandle)
	r.runtime.sqlite.mu.Unlock()
	for _, h := range handles {
		h.mu.Lock()
		h.closed = true
		if h.currentCancel != nil {
			h.currentCancel()
		}
		for h.activeOps > 0 {
			h.cond.Wait()
		}
		h.mu.Unlock()
		if h.conn != nil {
			if err := h.conn.Close(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if h.db != nil {
			if err := h.db.Close(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
	}

	r.runtime.recycle.mu.Lock()
	r.runtime.recycle.ids = make(map[types.ObjID]int)
	r.runtime.recycle.mu.Unlock()

	r.runtime.connectionOptions.mu.Lock()
	r.runtime.connectionOptions.byPlayer = make(map[types.ObjID]map[string]types.Value)
	r.runtime.connectionOptions.mu.Unlock()

	r.runtime.heldCommands.mu.Lock()
	r.runtime.heldCommands.byPlayer = make(map[types.ObjID][]string)
	r.runtime.heldCommands.mu.Unlock()

	r.runtime.heldHTTPInput.mu.Lock()
	var waiters []httpReadWaiter
	for _, state := range r.runtime.heldHTTPInput.byPlayer {
		waiters = append(waiters, state.waiters...)
	}
	r.runtime.heldHTTPInput.byPlayer = make(map[types.ObjID]*httpHeldInput)
	r.runtime.heldHTTPInput.mu.Unlock()
	for _, waiter := range waiters {
		if waiter.task != nil {
			waiter.task.Kill()
		}
	}

	r.applyProtectedBuiltins(nil)
	r.applyServerOptionsSnapshot(nil)
	return errors.Join(closeErrors...)
}
