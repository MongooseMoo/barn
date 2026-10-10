//go:build windows

package builtins

import "testing"

func TestProcessMemoryReportsOnlyTheResidentSetOnWindows(t *testing.T) {
	stats, err := readProcessMemory()
	if err != nil {
		t.Fatalf("readProcessMemory() failed: %v", err)
	}
	if stats.resident <= 0 {
		t.Fatalf("resident = %d, want the working set size in bytes", stats.resident)
	}
	if stats.total != 0 || stats.shared != 0 || stats.text != 0 || stats.data != 0 {
		t.Fatalf("readProcessMemory() = %+v, want every field but resident 0", stats)
	}
}
