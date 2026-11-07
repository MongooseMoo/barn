# Activation frame list ownership

## Goal

Construct each MOO activation list in ActivationFrame.ToList, including its
optional line and runtime-variable fields. Keep one formatting owner and call
the existing literal error-name owner directly.

Campaign overview: [system layering](/C:/Users/Q/code/barn-system-layering/agent-notes/refactor-system-layering.md).

## State

The six scoped source edits and their two reports are moved into this independent
branch from remote master. The isolated source diff exactly matches the original
campaign diff: thirty additions and sixty-two deletions. The six campaign files
are clean relative to their committed versions; the reports no longer exist in
the campaign checkout. Other workers' changes remain outside this scope.

Raw move checks:

```text
gofmt_exit=0
diff_check_exit=0
removed_wrapper_search_exit=1
isolated_source_diff_identical=true
campaign_owned_diff_exit=0
```

Six production ToList calls supply both flags. The wrapper search has no Go
match. No local tests or builds ran during the source move, and no source
behavior changed during that move. The coordinator published PR #640. The
independent analyst inspected its
published diff, list ownership, all six observers and relevant existing tests;
see reports/system-layering-frame-format-analyst.md. No correctness finding was
identified. The later benchmark work below supplies bounded observer evidence;
compiler inlining and all-input performance non-regression remain unknown.
List ownership establishes no extra source-level backing-array copy.

Raw PR checks at the analyst checkpoint:

```text
Build & unit tests pass 1m58s
Serializable histories and negative control pass 4m42s
MOO conformance suite pending 0
```

## Decisions and why

Use the existing frame owner with explicit line/variable flags and an exact-size
final slice. Preserve filtering, authority redaction, frame order and lazy
variables at their current callers. Delete the one-caller error-name forwarding
wrapper. Add no parallel representation or compatibility surface.

## Surprises

NewList retains its input backing array without copying it. Both the former
literal and the new dynamic slice therefore supply returned list storage; this
source fact does not establish compiler inlining or runtime performance.

## Next step

The benchmark worker added real callers and task_stack observer measurements
in engine/task_builtins_test.go. Ten samples per side and a second ten-pair
CPU-affinity/interleaved comparison are retained under reports; the report is
reports/system-layering-frame-measurement.md. All six production files are
restored exactly to their committed PR versions (explicit diff exit zero).

The common six-field callers(1) path retains 6128 B/op and 25 allocs/op on both
sides; neither comparison detected a significant timing difference. The pinned
comparison is 3.131us to 3.223us, p=0.363, so a small regression remains unresolved.
callers(0) reduces allocations from 39 to 25; both task_stack cases reduce 44 to
28. WSL frequency/host interference and timing intervals limit CPU conclusions.
No application-wide or all-input non-regression claim follows. No speculative
performance special case was added.

The coordinator reviews these evidence limits, publishes the substantive
benchmark additions and completes final verification. The worker publishes
nothing. Broad local tests were not run; only the edited test file's explicit
benchmarks were measured. The checkpoint-requested primary-checkout temporary
measurement note was consolidated here and removed before handoff.
