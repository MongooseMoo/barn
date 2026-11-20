package vm

import (
	"strings"
	"testing"
)

// A task restored from Toast's form runs a program compiled from one Toast
// vector, and a fork it runs afterwards extracts its body from that program.
// ExportNativeSnapshot exported such a body as the whole vector, ignoring its
// fork index, and refused it ("program is not the compile the Toast tables
// describe"), issue #965. Each case must end the same with every suspension
// checkpointed through Toast's form.
func TestCheckpointExportsForkOfRestoredProgram(t *testing.T) {
	for _, source := range []string{
		// The shape of issue #965's fuzz program, reduced.
		"suspend(0);\nfork (0)\n  suspend(0);\nendfork\nreturn 1;",
		"suspend(0);\nfork (0)\n  return 2;\nendfork\nfork (0)\n  suspend(0);\nendfork\nreturn 1;",
		// A nested fork, run after its parent body was itself restored.
		"suspend(0);\nfork (0)\n  suspend(0);\n  fork (0)\n    suspend(0);\n  endfork\nendfork\nreturn 1;",
		"x = 1;\nfork (suspend(0))\n  while (suspend(0) && 0)\n  endwhile\n  fork n (x + x)\n    suspend(0);\n  endfork\nendfork\nreturn x;",
	} {
		t.Run(strings.ReplaceAll(source, "\n", " "), func(t *testing.T) {
			difference, skip := checkpointResumeDifference(t, source, checkpointSuspendedVM, sourceFuzzTicks, false)
			if difference != "" || skip != "" {
				t.Fatalf("difference %q, skip %q; want neither\nsource:\n%s", difference, skip, source)
			}
		})
	}
}
