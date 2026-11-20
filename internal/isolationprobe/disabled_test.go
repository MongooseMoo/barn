//go:build !elle

package isolationprobe

import (
	"bytes"
	"testing"
)

func TestNormalBuildHasNoCheckpointStop(t *testing.T) {
	t.Setenv("BARN_ELLE_CHECKPOINT_STOP", CheckpointCapture+":1")
	var record bytes.Buffer
	if err := InstallCheckpointStop(&record); err != nil {
		t.Fatal(err)
	}
	called := false
	SetPause(func(string, []string) { called = true })
	for _, point := range CheckpointPoints {
		Pause(point, "artifact")
	}
	if called || record.Len() != 0 {
		t.Fatal("normal build ran a checkpoint pause")
	}
}
