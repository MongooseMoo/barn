# Checkpoint capture and encoding isolation

## Goal

Freeze checkpoint state under the world/task barrier, then encode running
checkpoints after releasing it. Preserve terminal checkpoint protection,
identity, task aliases and file publication/recovery ownership.

## State

Transfer complete. All six source/test files carry the original 198-addition,
33-deletion phase split, and only those checkpoint edits were removed from the
campaign. Its unrelated startup cleanup was left for that owner. Both existing
reports moved here. No new behavior, test run, build or publication was added.

```text
gofmt_exit=0
diff_check_exit=0
campaign_checkpoint_files_clean=True
```

Campaign overview: [system layering plan](/C:/Users/Q/code/barn-system-layering/agent-notes/refactor-system-layering.md).

## Decisions and why

Keep Capture under the world/task barrier. The existing Writer owns the frozen
view so running checkpoints can serialize and publish afterward. Terminal
checkpoints retain the barrier through publication. Reuse the existing
Checkpoint owner, phase validation and cleanup; add no parallel snapshot model.

## Surprises

The campaign's server file also carries an unrelated startup-failure change.
Only its checkpoint-stage and dumpCheckpoint hunks belong here.

## Next step

Coordinator inspects the independent branch, commits its scoped source/note/
reports, opens the PR and collects required CI and pinned Elle evidence.
Existing focused regression results remain recorded in the coder report;
they were not rerun merely for this exact transfer.
