# Checkpoint capture and encoding isolation

## Goal

Freeze checkpoint state under the world/task barrier, then encode running
checkpoints after releasing it. Preserve terminal checkpoint protection,
identity, task aliases and file publication/recovery ownership.

## State

Transfer complete. All six source/test files carry the original 198-addition,
33-deletion phase split, and only those checkpoint edits were removed from the
campaign. Its unrelated startup cleanup was left for that owner. Both existing
reports moved here. That transfer itself added no behavior or verification.

PR 641 is open. Analyst and verifier source reviews found no actionable defect
in this slice. Final verifier verdict is NO-MERGE because required conformance
failed in audit_binary_mode_dispatches_raw_chunk_without_newline (expected an
empty argstr entry, observed no entries). Failed job log and original workflow
artifacts are preserved. The cause remains unknown. Focused admission-plus-case
and the complete gap suite both pass on canonical managed Toast and the exact
Barn candidate; those narrower results do not clear the failed required check.

```text
MOO conformance suite fail 10m43s
Build & unit tests pass 1m21s
Serializable histories and negative control pass 4m42s
Elle results: healthy=6 true/exit0; faults=20 false/exit1; errors=0
Toast/Barn focused: 2 passed each
Toast/Barn complete gap suite: 18 passed each
```

Raw histories, all three nonzero-path manifests and fault witnesses remain in
.tmp/elle-evidence/elle-evidence.tar.gz. Original failed conformance artifacts
remain under .tmp/conformance-failure. See reports/system-layering-checkpoint-verifier.md
for source review, comparison limits and exact evidence locations.

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

The investigation proved the binary fixture assumes one TCP write is one input
chunk: verified setup/login/mode plus controlled audi and tbin sends produces
the same empty result on canonical Toast (one failed, one admission passed).
A test-player-filtered do_command observer concatenates raw chunks and passes
on canonical Toast. See reports/system-layering-binary-failure-investigation.md
and the raw .tmp/toast-binary-split.xml and toast-binary-chunks.xml artifacts.
The original failed run's actual packetization remains unknown; this is a proven
test defect under a valid producer, not a claim to reconstruct that run.

The existing conformance case is repaired independently in companion PR 179,
using its existing command hook, original unsplit eight-byte producer and exact
raw-byte assertion. Focused and complete surrounding suite comparisons pass on
canonical Toast and this Barn candidate. Full companion profile CI remains
required before merging that repair; the failed run's packetization stays unknown.

The checkpoint branch now integrates merged default-branch fixes, including
the cursor/capacity behavior required by the new conformance main. The merge
is conflict-free. Its six checkpoint files have exactly the same added/removed
source lines against the updated default branch as the original reviewed slice:

```text
checkpoint_changed_lines_identical=true
diff_check_exit=0
```

Preserve the original failed CI and its raw archives. Push this integrated branch
after companion 179 reaches main, then collect new exact-head full CI and final
verification. No checkpoint behavior repair or speculative CI retry is selected.
