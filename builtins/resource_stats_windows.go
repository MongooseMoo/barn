//go:build windows

package builtins

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows has no sysinfo() load averages or getrusage(), so usage() reports
// zeros, as Toast does when those calls fail.
func readProcessUsage() processUsage { return processUsage{} }

var procGetProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// processMemoryCounters is PROCESS_MEMORY_COUNTERS from psapi.h.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// readProcessMemory follows the branch Toast's bf_memory_usage takes on macOS,
// its one platform without /proc: the resident set size in bytes is the only
// value reported, the rest stay 0, and a failed query is E_FILE.
func readProcessMemory() (processMemory, error) {
	var counters processMemoryCounters
	counters.cb = uint32(unsafe.Sizeof(counters))
	ok, _, err := procGetProcessMemoryInfo.Call(
		uintptr(windows.CurrentProcess()),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.cb),
	)
	if ok == 0 {
		return processMemory{}, fmt.Errorf("%w: %v", errProcessMemoryUnreadable, err)
	}
	return processMemory{resident: int64(counters.workingSetSize)}, nil
}
