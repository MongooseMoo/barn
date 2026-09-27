//go:build linux

package builtins

import (
	"fmt"
	"os"
	"syscall"
)

// readProcessUsage follows Toast's bf_usage: load averages come from
// sysinfo() and rusage from getrusage(); a failed call leaves its fields 0.
func readProcessUsage() processUsage {
	var stats processUsage
	var info syscall.Sysinfo_t
	if syscall.Sysinfo(&info) == nil {
		for i := range stats.loadAverage {
			stats.loadAverage[i] = int64(info.Loads[i])
		}
	}
	var r syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &r) == nil {
		seconds := func(v syscall.Timeval) float64 { return float64(v.Sec) + float64(v.Usec)/1e6 }
		stats.userSeconds, stats.systemSeconds = seconds(r.Utime), seconds(r.Stime)
		stats.minorFaults, stats.majorFaults = r.Minflt, r.Majflt
		stats.inputBlocks, stats.outputBlocks = r.Inblock, r.Oublock
		stats.voluntarySwitches, stats.involuntarySwitches = r.Nvcsw, r.Nivcsw
		stats.signals = r.Nsignals
	}
	return stats
}

func readProcessMemory() (processMemory, error) {
	f, err := os.Open("/proc/self/statm")
	if err != nil {
		return processMemory{}, fmt.Errorf("%w: %v", errProcessMemoryUnreadable, err)
	}
	defer f.Close()
	var m processMemory
	var ignored int64
	if _, err := fmt.Fscan(f, &m.total, &m.resident, &m.shared, &m.text, &ignored, &m.data); err != nil {
		return processMemory{}, fmt.Errorf("%w: %v", errProcessMemoryMalformed, err)
	}
	return m, nil
}
