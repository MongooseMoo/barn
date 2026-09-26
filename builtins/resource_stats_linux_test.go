//go:build linux

package builtins

import (
	"testing"
	"time"
)

func TestProcessUsageMeasuresCPUActivity(t *testing.T) {
	before, err := readProcessUsage()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Millisecond)
	for time.Now().Before(deadline) {
	}
	after, err := readProcessUsage()
	if err != nil {
		t.Fatal(err)
	}
	if after.userSeconds+after.systemSeconds <= before.userSeconds+before.systemSeconds {
		t.Fatalf("CPU time did not increase: before=%+v after=%+v", before, after)
	}
	if len(after.loadAverage) != 3 {
		t.Fatalf("load average has %d entries, want 3", len(after.loadAverage))
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
