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

// Issue #943: a queued fork's saved body starts with an empty statement,
// which emits no code, so the line of the body's first instruction is the
// line after it. The checkpoint numbered the saved body from that line, and
// the restored fork's traceback named a line one past the uninterrupted
// run's for each leading `;`, one more again for each later checkpoint taken
// while it stayed queued. A try with computed except codes, whose first
// instruction is on the except line, was misnumbered the same way.
func TestCheckpointedForkBodyKeepsFirstStatementLine(t *testing.T) {
	for name, source := range map[string]string{
		"empty statement":        "n = 0;\nfork (0)\n  ;\n  n[n];\nendfork\nsuspend(0);\nreturn 1;",
		"two empty statements":   "n = 0;\nfork (0)\n  ;\n  ;\n  n[n];\nendfork\nsuspend(0);\nreturn 1;",
		"checkpointed twice":     "n = 0;\nfork (0)\n  ;\n  n[n];\nendfork\nsuspend(0);\nsuspend(0);\nreturn 1;",
		"empty statement in try": "n = 0;\nfork (0)\n  ;\n  try\n    n[n];\n  finally\n  endtry\nendfork\nsuspend(0);\nreturn 1;",
		"computed except codes":  "n = {};\nfork (0)\n  try\n    n;\n    n[1.5];\n  except (E_PERM, @n)\n  endtry\nendfork\nsuspend(0);\nreturn 1;",
	} {
		t.Run(name, func(t *testing.T) { checkCheckpointResumeEquivalence(t, source) })
	}
}
