package vm

import (
	"sync"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// VM reuse.
//
// Every Go builtin that invokes a MOO verb (waif method dispatch, move hooks,
// $sysobj hooks) goes through the runtime's registry VerbCaller, which built a
// fresh VM per call. Each one allocated a 256-entry operand stack and a 16-entry
// frame slice that were thrown away microseconds later; on a real workload that
// was the single largest source of allocated bytes in the server.
//
// Acquire/Release recycle those backing arrays. A pooled VM is only ever handed
// back by a call site that has proven nothing outlives the call — see ReleaseVM's
// refusal conditions and the audit notes on each call site. The task VMs
// (engine/task_runtime.go, task_factory.go, task_load.go) are deliberately NOT
// pooled: they are stored on the task via SetBytecodeVM and resumed after a
// suspend, and are walked for GC roots by other goroutines.
const (
	// defaultTickLimit and defaultMaxStackDepth are the values NewVM has always
	// installed; a recycled VM must come back indistinguishable from a fresh one.
	defaultTickLimit     = 30000
	defaultMaxStackDepth = 50

	// initialStackCap / initialFramesCap are the fresh-VM capacities.
	initialStackCap  = 256
	initialFramesCap = 16

	// A VM whose stack or frame slice grew past these is dropped rather than
	// pooled, so one pathological deep recursion does not pin a large array in
	// the pool for the life of the process.
	maxPooledStackCap  = 4096
	maxPooledFramesCap = 256
)

var vmPool = sync.Pool{
	New: func() any { return NewVM(nil, nil) },
}

// AcquireVM returns a VM ready for use, reusing a pooled one when available.
// The caller must pass it to ReleaseVM once it has proven nothing still
// references the VM, its frames, or its stack; skipping ReleaseVM is always
// safe and simply forgoes the reuse.
func AcquireVM(store *dbstore.Store, session *builtins.Session) *VM {
	machine := vmPool.Get().(*VM)
	machine.Store = store
	machine.Builtins = session
	return machine
}

// ReleaseVM returns a VM to the pool after scrubbing it. It silently declines to
// pool a VM that may still be referenced:
//
//   - a yielded VM (suspend/fork) can still be Resume()d by whoever holds it;
//   - an oversized VM would pin its backing arrays.
//
// Declining is not an error — the VM is simply left to the garbage collector.
func ReleaseVM(machine *VM) {
	if machine == nil || machine.yielded {
		return
	}
	if cap(machine.Stack) > maxPooledStackCap || cap(machine.Frames) > maxPooledFramesCap {
		return
	}
	machine.reset()
	vmPool.Put(machine)
}

// reset scrubs a VM back to fresh-NewVM state, keeping only the two backing
// arrays. The whole-struct assignment is deliberate: every field not named here
// gets its zero value, so a field added to VM later cannot be silently left
// dirty across a reuse.
func (vm *VM) reset() {
	// Zero the used region of the stack and every frame pointer first: a pooled
	// VM must not keep values (and through them whole object graphs) alive.
	// len(Stack) is the high-water mark, since Push appends past it and only SP
	// is rewound.
	for i := range vm.Stack {
		vm.Stack[i] = types.Value{}
	}
	for i := range vm.Frames {
		vm.Frames[i] = nil
	}

	stack := vm.Stack[:0]
	frames := vm.Frames[:0]
	*vm = VM{
		Stack:         stack,
		Frames:        frames,
		TickLimit:     defaultTickLimit,
		MaxStackDepth: defaultMaxStackDepth,
	}
}
