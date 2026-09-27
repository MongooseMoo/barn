//go:build linux

package builtins

import (
	"testing"
	"time"
)

func TestProcessUsageMeasuresCPUActivity(t *testing.T) {
	before := readProcessUsage()
	deadline := time.Now().Add(20 * time.Millisecond)
	for time.Now().Before(deadline) {
	}
	after := readProcessUsage()
	if after.userSeconds+after.systemSeconds <= before.userSeconds+before.systemSeconds {
		t.Fatalf("CPU time did not increase: before=%+v after=%+v", before, after)
	}
}

func TestProcessMemoryReportsKernelPageCounts(t *testing.T) {
	stats, err := readProcessMemory()
	if err != nil {
		t.Fatal(err)
	}
	if stats.total == 0 || stats.resident == 0 || stats.data == 0 {
		t.Fatalf("expected measured nonzero process memory fields, got %+v", stats)
	}
}
