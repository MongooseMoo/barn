package task

import (
	"time"

	"golang.org/x/sys/unix"
)

const threadCPUTime = true

type threadCPUClock int32

func openThreadCPUClock() (threadCPUClock, error) {
	// Linux encodes a thread clock as the complemented TID and clock type.
	// CPUCLOCK_VIRT counts user time and can be read from a sibling thread.
	const perThread = 4
	const virtual = 1
	return threadCPUClock((^int32(unix.Gettid()) << 3) | perThread | virtual), nil
}

func (c threadCPUClock) read() (time.Duration, error) {
	var now unix.Timespec
	err := unix.ClockGettime(int32(c), &now)
	return time.Duration(now.Nano()), err
}

func (threadCPUClock) close() {}
