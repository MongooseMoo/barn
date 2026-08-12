package builtins

import (
	"sync"
	"sync/atomic"

	"github.com/MongooseMoo/barn/types"
)

// registryRuntime owns the mutable resources created by builtin calls. Keeping
// it private prevents callers from bypassing Registry's lifecycle.
type registryRuntime struct {
	files struct {
		sync.Mutex
		nextID  int64
		handles map[int64]*mooFileHandle
	}
	sqlite struct {
		sync.Mutex
		nextID  int64
		handles map[int64]*sqliteHandle
	}
	recycle struct {
		sync.Mutex
		ids map[types.ObjID]int
	}
	protected atomic.Pointer[map[string]bool]
}

func newRegistryRuntime() *registryRuntime {
	s := &registryRuntime{}
	s.files.nextID, s.files.handles = 1, make(map[int64]*mooFileHandle)
	s.sqlite.nextID, s.sqlite.handles = 1, make(map[int64]*sqliteHandle)
	s.recycle.ids = make(map[types.ObjID]int)
	empty := map[string]bool{}
	s.protected.Store(&empty)
	return s
}

// Close deterministically releases every external resource owned by r. It is
// idempotent; registries remain independent even when their handle IDs overlap.
func (r *Registry) Close() error {
	if r == nil || r.runtime == nil {
		return nil
	}
	r.runtime.files.Lock()
	files := r.runtime.files.handles
	r.runtime.files.handles = make(map[int64]*mooFileHandle)
	r.runtime.files.Unlock()
	for _, h := range files {
		_ = h.file.Close()
	}

	r.runtime.sqlite.Lock()
	handles := r.runtime.sqlite.handles
	r.runtime.sqlite.handles = make(map[int64]*sqliteHandle)
	r.runtime.sqlite.Unlock()
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
			_ = h.conn.Close()
		}
		if h.db != nil {
			_ = h.db.Close()
		}
	}
	return nil
}
