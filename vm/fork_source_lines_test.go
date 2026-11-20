package vm

import "testing"

// A fork body ending in a compound statement must keep that statement's
// closing keyword in its saved source, though the keyword emits no code and
// so has no line table entry, or the queued task does not compile on reload.
func TestCheckpointedForkBodyKeepsTrailingBlockClose(t *testing.T) {
	for name, source := range map[string]string{
		"empty fork":   "fork (0)\n  fork x (1)\n  endfork\nendfork\nsuspend(0);\nreturn 1;",
		"fork":         "n = 1;\nfork (0)\n  fork x (1)\n    n;\n  endfork\nendfork\nsuspend(0);\nreturn 1;",
		"if":           "n = 1;\nfork (0)\n  if (n)\n    n;\n  endif\nendfork\nsuspend(0);\nreturn 1;",
		"empty while":  "fork (0)\n  while (0)\n  endwhile\nendfork\nsuspend(0);\nreturn 1;",
		"empty try":    "fork (0)\n  try\n  finally\n  endtry\nendfork\nsuspend(0);\nreturn 1;",
		"nested close": "n = 1;\nfork (0)\n  while (0)\n    if (n)\n      n;\n    endif\n  endwhile\nendfork\nsuspend(0);\nreturn 1;",
	} {
		t.Run(name, func(t *testing.T) { checkCheckpointResumeEquivalence(t, source) })
	}
}

// A try whose except codes are computed evaluates them first, under line
// entries for both the try line and the except line, so the fork body's first
// IP maps to the except line. The saved source must still start at the try.
func TestCheckpointedForkBodyKeepsTryBeforeComputedCodes(t *testing.T) {
	source := "n = {};\nfork (0)\n  try\n    n;\n  except (E_PERM, @n)\n  endtry\nendfork\nsuspend(0);\nreturn 1;"
	checkCheckpointResumeEquivalence(t, source)
}
