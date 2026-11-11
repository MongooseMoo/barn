//go:build elle && linux

package isolationprobe

import (
	"runtime"
	"syscall"
)

const canStopSelf = true

// stopSelf directs SIGSTOP at the calling thread. A process-directed
// kill(getpid(), SIGSTOP) may be delivered to another thread (signal(7)),
// leaving this one running past its stop point until the group stop reaches
// it. The calling thread dequeues a signal sent to itself on its return from
// tgkill, so the whole process stops before this function returns.
func stopSelf() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return syscall.Tgkill(syscall.Getpid(), syscall.Gettid(), syscall.SIGSTOP)
}
