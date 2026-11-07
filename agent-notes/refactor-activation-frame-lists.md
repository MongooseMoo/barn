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
match. No local tests or builds ran, and no source behavior changed during this
move. Commits, PR CI, review and publication remain pending with the coordinator.

## Decisions and why

Use the existing frame owner with explicit line/variable flags and an exact-size
final slice. Preserve filtering, authority redaction, frame order and lazy
variables at their current callers. Delete the one-caller error-name forwarding
wrapper. Add no parallel representation or compatibility surface.

## Surprises

None observed during the initial source/base inspection.

## Next step

The coordinator reviews the isolated six-file diff, commits this note and the
two reports with the source, then handles publication, CI and merge. No commit,
push, PR or merge was performed by this move task.
