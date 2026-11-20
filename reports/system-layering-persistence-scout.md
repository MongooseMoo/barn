# Persistence and finalization lifetime survey

## State

The P8 read-only survey is complete; this report now follows the independent
checkpoint-capture branch.
The checkpoint encoding barrier is a source-proven retained resource, without
a measured workload ranking.

## Detached capture proof

Store.SnapshotWithRoots freezes mutable WAIF payloads with same-identity cycle
memoization, eagerly captures external task roots, rewrites pending finalization
values and all object/anonymous properties and LastMove values. Relations/maps
are copied; verb source/name arrays are COW immutable. Task snapshots copy
metadata/maps/slices; VM persistence clones programs/frame arrays. The value
transform covers exactly the queued/suspended/interrupted surfaces the writer
emits. The VM activation writer uses the Programmer scalar and rewritten frame
ThisValue. No remaining live mutable alias was found in emitted captured values.
Existing snapshot tests exercise WAIF detachment and shared external identity.

## Split capture from encoding at existing Checkpoint owner

Checkpoint.Serialize currently captures/freezes and encodes while server holds
admission Pause, SweepMu and VMStartMu. Separate Capture with the current inputs,
retaining the existing Writer as owner of the detached view. Serialize then
consumes only that writer and clears it; Close clears unfinished captured state.
Do not add a snapshot DTO or facade. Running checkpoints hold the barrier only
for task/connection/store capture; Serialize and Publish follow outside it.
Terminal checkpoints retain Capture/Serialize/Publish under the barrier, because
already-dispatched background work remains live. Direct WriteCheckpoint follows
the same phases. Preserve path locks, recovery and cleanup-error ownership.

## Required regression

Use the existing checkpointProbe phase interception to block real serialization
after capture, prove an admitted Eval can commit while blocked, then reload the
dump to prove pre-capture property and WAIF values/aliases were retained.
Preserve task/store capture consistency, publication progress, resumed failure
hooks and terminal background/failure barrier regressions. Format writer tests
can block an actual io.Writer and mutate originals to prove detachment.
No new production observer is needed. Coordinator inspected the current stage
interface, direct WriteCheckpoint callers and abort/state tests. The selected
direct phase split is Capture(current inputs) then Serialize() on the existing
Checkpoint, with the existing Writer retaining the frozen view and clearing on
Serialize/Close. Terminal barrier behavior stays intact.
