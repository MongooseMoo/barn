package vm

import (
	"strconv"
	"time"

	"github.com/MongooseMoo/barn/internal/mooprof"
	"github.com/MongooseMoo/barn/types"
)

// MOO-level profiling (internal/mooprof).
//
// A sample is taken at the 1,024-tick checkpoint and when an execution loop
// returns, and only while a session is active. It bills the ticks and wall
// time since this VM's previous sample to the frames present now, followed by
// the frames of every VM beneath it in the same task (profParent).
//
// A nested VM's ticks are later added to its caller's Ticks
// (BuiltinTicksConsumed), so each sample advances every ancestor's tick
// baseline by what it billed; the ancestor's next sample then counts only its
// own work. Wall time is one clock for the whole chain: each sample resets
// the baseline on the VM and its ancestors.

// profileState is the per-VM sampling baseline. It is zero on a fresh or
// pooled VM; reset clears it with the rest of the struct.
type profileState struct {
	parent  *VM // VM whose builtin started this one, in the same task
	session *mooprof.Session
	ticks   int64     // Ticks at the previous sample
	time    time.Time // when the previous sample (or slice start) was taken
	entry   string    // bottom frame's verb at loop entry, billed once frames are gone
}

// SetProfileParent links a nested VM to the VM whose builtin started it, so
// its samples carry the caller's frames. The caller must absorb this VM's
// Ticks into its own once the nested call returns.
func (vm *VM) SetProfileParent(parent *VM) {
	vm.prof.parent = parent
}

// profileEnter opens a sampling interval when an execution loop starts or
// resumes. Time before it (suspension, admission wait, the caller's own
// work for a nested VM) is not billed here.
func (vm *VM) profileEnter() {
	s := mooprof.Active()
	if s == nil {
		return
	}
	vm.profileBind(s)
	if p := vm.prof.parent; p != nil && p.prof.session == s {
		vm.prof.time = p.prof.time
	} else {
		vm.prof.time = time.Now()
	}
}

// profileExit bills what the loop did since its last checkpoint. After the
// loop completes no frames remain and the remainder goes to its entry verb.
func (vm *VM) profileExit() {
	if s := mooprof.Active(); s != nil {
		vm.profileSample(s)
	}
}

// profileCheckpoint samples at a tick checkpoint and returns true when a
// session is active, so the caller can restart the wall clock after any wait.
func (vm *VM) profileCheckpoint() bool {
	s := mooprof.Active()
	if s == nil {
		return false
	}
	vm.profileSample(s)
	return true
}

// profileBind starts this VM's baseline for session s.
func (vm *VM) profileBind(s *mooprof.Session) {
	vm.prof.session = s
	vm.prof.ticks = vm.Ticks
	if len(vm.Frames) > 0 {
		vm.prof.entry = profileFuncName(vm.Frames[0].VerbLoc, vm.Frames[0].Verb)
	}
}

func (vm *VM) profileSample(s *mooprof.Session) {
	now := time.Now()
	if vm.prof.session != s {
		// The session started while this loop was running: there is no
		// baseline for it, so this checkpoint only opens one.
		vm.profileBind(s)
		for p := vm; p != nil; p = p.prof.parent {
			p.prof.time = now
		}
		return
	}
	ticks := vm.Ticks - vm.prof.ticks
	wall := now.Sub(vm.prof.time).Nanoseconds()
	vm.prof.ticks = vm.Ticks
	vm.prof.time = now
	for p := vm.prof.parent; p != nil; p = p.prof.parent {
		if p.prof.session != s {
			p.profileBind(s)
		}
		p.prof.ticks += ticks
		p.prof.time = now
	}
	if ticks <= 0 && wall <= 0 {
		return
	}
	s.Add(vm.profileStack(), vm.profileLabel(), ticks, wall)
}

// profileStack lists the frames of this VM and every VM beneath it,
// innermost first. A VM whose loop has completed, leaving no frames, reports
// the verb it was entered with.
func (vm *VM) profileStack() []mooprof.Frame {
	var stack []mooprof.Frame
	for m := vm; m != nil; m = m.prof.parent {
		if len(m.Frames) == 0 && m.prof.entry != "" {
			stack = append(stack, mooprof.Frame{Func: m.prof.entry})
		}
		for i := len(m.Frames) - 1; i >= 0; i-- {
			f := m.Frames[i]
			line := 0
			if f.Program != nil {
				line = f.Program.LineForIP(f.IP)
			}
			stack = append(stack, mooprof.Frame{Func: profileFuncName(f.VerbLoc, f.Verb), Line: int64(line)})
		}
	}
	return stack
}

// profileLabel is the task's entry verb, formatted as the Go profile's
// moo.verb pprof label (engine/task_runtime.go).
func (vm *VM) profileLabel() string {
	if vm.Task == nil {
		return ""
	}
	return profileFuncName(vm.Task.This, vm.Task.VerbName)
}

func profileFuncName(obj types.ObjID, verb string) string {
	return "#" + strconv.FormatInt(int64(obj), 10) + ":" + verb
}
