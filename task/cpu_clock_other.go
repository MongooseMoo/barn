//go:build !linux && !windows

package task

import "time"

// As in Toast on platforms without virtual timers, use elapsed time.
const threadCPUTime = false

type threadCPUClock struct{ start time.Time }

func openThreadCPUClock() (threadCPUClock, error) {
	return threadCPUClock{start: time.Now()}, nil
}

func (c threadCPUClock) read() (time.Duration, error) { return time.Since(c.start), nil }

func (threadCPUClock) close() {}
