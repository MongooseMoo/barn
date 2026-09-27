package builtins

import (
	"errors"

	"github.com/MongooseMoo/barn/types"
)

// processUsage mirrors Toast's usage(): load averages are sysinfo()'s
// fixed-point integers (load * 65536), and any unavailable measurement is 0.
type processUsage struct {
	loadAverage                                         [3]int64
	userSeconds, systemSeconds                          float64
	minorFaults, majorFaults, inputBlocks, outputBlocks int64
	voluntarySwitches, involuntarySwitches, signals     int64
}

type processMemory struct {
	total, resident, shared, text, data int64
}

var (
	// errProcessMemoryUnreadable: /proc/self/statm could not be opened (Toast: E_FILE).
	errProcessMemoryUnreadable = errors.New("process memory statistics are unreadable")
	// errProcessMemoryMalformed: /proc/self/statm could not be parsed (Toast: E_NACC).
	errProcessMemoryMalformed = errors.New("process memory statistics are malformed")
)

// processMemoryErrorCode maps a readProcessMemory failure to the error
// memory_usage() raises in Toast (server.cc bf_memory_usage).
func processMemoryErrorCode(err error) types.ErrorCode {
	if errors.Is(err, errProcessMemoryMalformed) {
		return types.E_NACC
	}
	return types.E_FILE
}
