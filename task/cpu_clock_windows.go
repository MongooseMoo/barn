package task

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const threadCPUTime = true

var getThreadTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetThreadTimes")

type threadCPUClock windows.Handle

func openThreadCPUClock() (threadCPUClock, error) {
	thread, err := windows.OpenThread(windows.THREAD_QUERY_LIMITED_INFORMATION, false, windows.GetCurrentThreadId())
	return threadCPUClock(thread), err
}

func (c threadCPUClock) read() (time.Duration, error) {
	var created, exited, kernel, user windows.Filetime
	ok, _, err := getThreadTimes.Call(uintptr(c), uintptr(unsafe.Pointer(&created)),
		uintptr(unsafe.Pointer(&exited)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if ok == 0 {
		return 0, err
	}
	interval := uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime)
	return time.Duration(interval) * (100 * time.Nanosecond), nil
}

func (c threadCPUClock) close() { _ = windows.CloseHandle(windows.Handle(c)) }
