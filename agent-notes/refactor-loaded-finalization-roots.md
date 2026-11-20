# Loaded finalization roots

## Goal
Reuse the direct-reference visitor introduced by PR 553 for database-loaded
pending finalizations. Q authorized this implementation with "Make it so!".

## Plan and decisions
Replace the sole collector call in engine/waif_lifecycle.go with a callback and
delete collectLoadedFinalizationRoots. Approximately 7 added / 21 removed lines.
Keep anonymous-ID deduplication and the existing WAIF set at the caller. The
visitor walks list elements and map keys/values, stopping at anonymous and WAIF
leaves. No property expansion or lifecycle scheduling changes are needed.

## State
Isolated branch refactor/loaded-finalization-roots starts from refreshed master.
Implementation is complete: 7 production lines added, 21 removed in one file.
Source inspection confirms IsAnonymous tests TYPE_ANON only, so the old TYPE_OBJ
case contributes no roots. Existing shutdown/startup adoption tests remain in CI.

## Surprises
None. No performance claim or new traversal framework is part of this change.

## Next step
Diff inspected. Commit and publish the scoped PR, then follow its CI result.
Use existing PR CI; do not duplicate its checks locally.
