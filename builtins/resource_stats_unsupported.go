//go:build !linux

package builtins

// Toast reads load averages with sysinfo() and memory from /proc/self/statm.
// Without them, usage() reports zeros and memory_usage() cannot open statm.
func readProcessUsage() processUsage { return processUsage{} }

func readProcessMemory() (processMemory, error) {
	return processMemory{}, errProcessMemoryUnreadable
}
