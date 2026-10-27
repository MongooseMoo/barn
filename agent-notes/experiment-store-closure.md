# Store closure experiment

## Goal

Determine through a bounded implementation experiment whether the researched
closure model removes production code and maintenance obligations without
changing behavior or sacrificing performance. Count replacement code across
types, VM and store, not only the db/store directory.

## State

- Worktree: C:/Users/Q/code/barn-store-closure.
- Branch: experiment/store-closure. Initial HEAD: 2b653308.
- Freshly fetched origin/master is b91e36f5 and is an ancestor of HEAD.
  Additional inherited commits are research notes only.
- Q explicitly said "Make it so!" after preparation; bounded implementation is
  now authorized. closure_implementation owns the implementation/caller changes.
- Implementation is active. Parent owns this note and publication/CI evidence.
- Existing research: agent-notes/store-immutable-roots-research.md, preferred
  section "Smaller construction: closure, sets, ordered effects".

## Smallest experiment and decisions

Stage 1: replace the two duplicate WAIF closure bodies with one directly owned
implementation in types, updating actual callers and removing the old bodies.
Expected scale: about 35 shared implementation lines replacing about 70 lines,
plus caller changes. These are estimates, not an established net saving.

Stage 2: try anonymous/direct collectors against that same implementation only
if the full diff becomes smaller and easier to follow. Count adapter functions,
options, allocations and changed ownership boundaries. No generic graph heap,
registry, mode propagation or speculative framework. Keep each graph view and
existing logical identity key separate. Preserve traversal order and filters.

Stage 3: examine shared covering-root selection separately; reject the extraction
if typed-set and ordering adapters consume the saved loops. No obligation to
retain every attempted stage. Record abandoned candidates and their evidence.

Captures, cached persistent roots, scheduling, image publication, snapshot
visibility and callback timing are outside the implementation scope. The closure
model does not establish that their work is removable. Do not add tests, guards,
benchmark harnesses or workflow steps speculatively.

## Subagents

- immutable_world_roots: read-only finished Stage 1/2 implementation plan, exact
  owners/callers and deletion accounting; no file writes.
- object_owned_changes: read-only existing correctness/benchmark coverage and
  matched measurement commands; no file writes or execution.
- Parent owns this note and integrates their source-checked results. Agents do
  not publish, commit or modify unrelated files.

## Validation and evidence

Use existing PR CI for correctness; do not rerun CI locally to gate a push.
Benchmark measurements are experiment evidence, not replacement correctness
checks. Select existing relevant benchmarks after inspecting their actual bodies.
Compare identical commands/settings on the unchanged base and candidate;
record raw results and toolchain. Report any unmeasured workload dimensions.
No tests or performance claims follow merely from reduced line count.

## Finished preparation

Both subagent preparation reports are complete. Parent inspected WaifSet,
the VM traversal, engine/admission.go:99-108, iteration_columns_test.go:34-43,
existing CI and the benchmark bodies. Stage 1's direct owner/API candidate is
`(*types.WaifSet).AddReachable(value Value, retained bool)`. Remove both
collectWaifsInto bodies and value-only VM forwarding functions. Callers in
store_pending.go, store_reachability.go, vm/waif_gc.go and vm/anonymous_gc.go
use their existing sets. Admission must still use a FRESH set per input value
and merge results; traversing directly into its destination could suppress
expansion. Preserve the wrapper's existing finalizable-value short circuit.
Update the existing hidden-iteration-state test's direct helper call only.

Do not change Add-before-expansion behavior, overlay membership suppression,
logical WAIF identity keys, map key-before-value recursion or current/retained
property enumeration. No new locking or captured-root behavior.

Stage 2 may separately consolidate the duplicate anonymous-value collectors
into types. Keep visited-map scope per current call, lazy allocation, anonymous
ID filtering and existing container behavior. Keep store object resolution at
its owner. Avoid callbacks/modes joining WAIF and anonymous collectors unless
the concrete diff gets smaller after counting adapters. Direct collectors and
cover reducers are optional further experiments, not mandatory abstractions.

Existing correctness evidence to obtain from PR CI includes historical WAIF
roots (store_txn_waif_test.go:218/262), committed graph selection and canonical
roots (vm/shutdown_roots_test.go), root cache invalidation, and hidden iteration
roots. Existing CI runs all Go tests and types/store races; it does not provide
a VM race run. No test result has been obtained for a candidate.

Existing benchmark command for LATER direct-collector changes, identical in
base and candidate worktrees with interleaved sequential samples:

    go test ./vm '-run=^$' '-bench=^(BenchmarkPendingWaifLocals|BenchmarkPendingFinalizationDiagnostic|BenchmarkPlainFrameFinalization)$' -benchmem -benchtime=200ms -count=1 -cpu=4 -timeout=60s

Preserve raw output/exit codes and toolchain/settings. Keep all diagnostic rows
and compare ns/op, B/op and allocs/op. These benchmarks exercise direct pending
collection, not current/retained WAIF closure expansion. No existing direct
closure benchmark was found. Do not claim first-stage performance parity from
these unrelated results; do not substitute history-pruning/routing benchmarks.
This is a recorded measurement limitation, not permission to add a new harness.

Accept/reject each stage from its full production diff: net code after adapters,
number of independent traversal bodies, actual reader complexity, preserved
ownership/capture/identity behavior, existing CI evidence, and relevant measured
costs where available. No arbitrary size quota or new gate. If wider sharing
adds machinery or loses the behavior, retain only the narrower useful stage.

## Implementation result

Stages 1/2 implemented; parent inspected the complete production diff and new
types/anonymous_refs.go directly. Stage 1 production +46/-99 = -53 physical
lines; Stage 2 adds +55/-91 = -36; combined +101/-190 = -89, including the new
44-line anonymous collector. Existing test caller adaptation +4/-4. No new tests,
flags, graph engine, guards or workflow changes. Four duplicated traversal bodies
become two types-owned implementations. Direct value-only VM facades disappear.

Cover reducer left unchanged: anonymous maps versus ordered WaifSet closures,
different tie-breaks and differing resolution producers would require adapters
or representation changes. A complete smaller replacement was not established.

Diagnostic baseline used the actual local DB and sound catalog through the
existing opt-in workload. It exited 0/PASS but reported home E_NACC failures;
therefore no clean throughput baseline or performance-parity claim. Raw profile
and structured logs remain local under experiments/store-closure; README records
fixture hashes, settings and limitations. No candidate benchmark run is justified
against this invalid cohort. No ordinary local CI steps were run.

## Next step

Commit the inspected implementation and notes, publish draft experiment PR and
collect its existing CI results. Implementation analyst is checking real callers
and behavioral preservation without local tests. No merge/deployment authorized.
