# Checkpoint capture analyst review

## Scope and state

Reviewing the actual open PR 641, Release the running checkpoint barrier after
frozen capture. GitHub reports nine changed files and the requested head.
Bounded source review is complete. No implementation changes, local builds or
test runs were performed. No publication or merge is performed.

## Findings

No actionable defect found in the changed code. This report is not merge
approval and does not replace the verifier or outstanding CI/evidence work.

## Evidence inspected

- The actual PR diff separates the existing Checkpoint owner; no alternate
  snapshot representation or background writer goroutine is introduced.
- Capture eagerly freezes task roots and object/pending-finalization values.
  Store snapshots copy object metadata, property maps, relationships and
  anonymous records. WAIF payloads are separate allocations memoized by durable
  identity. Writer's class lookup uses the captured object map.
- VM persistence clones program bytecode, constants arrays, source/name arrays,
  handler arrays and codes, locals, stack, args, move state and recycle state.
  TransformPersistenceValues covers the typed values actually emitted by the
  queued, suspended and interrupted writers. Activation programmer is scalar;
  emitted suspended receiver comes from the rewritten VM frame.
- Connection writer retains a copied slice of player/listener scalar pairs.
  It does not retain connection pointers or socket state.
- Serialize clears the retained writer before encoding; Close clears an unused
  writer and retains existing path-lock, cleanup-error and recovery ownership.
- Live check output at inspection time:

  ```text
  Build & unit tests pass 1m21s
  Record pass pass 8s
  Serializable histories and negative control pass 4m42s
  MOO conformance suite pending 0
  ```

  These are GitHub job statuses, not independent inspection of Elle verdicts or
  coverage manifests. Parent owns the raw pinned evidence collection.

## Next step

Parent/verifier must collect terminal conformance results and inspect the raw
pinned Elle verdicts, histories, coverage manifests and negative-control witnesses.

## Lifetime and alias audit

- `db/store/store_snapshot.go:68` holds the exclusive publication gate and
  store read lock across capture. External task roots are eagerly rewritten at
  line 102, before the rewriter is returned. Pending roots, regular properties,
  LastMove and anonymous properties use the same plan. WAIF cycles/aliases share
  the memoized clone (`:309`); its class/owner and properties are captured.
- `types/waif.go:71` allocates each restored/frozen WAIF as a distinct Go payload.
  It does not register that payload with the store. Registry cleanup is attached
  only by `db/store/store_lifecycle.go:516`; class-property migrations walk that
  registry (`db/store/store_txn_waif.go:179`). Therefore same identity does not
  expose frozen payloads to subsequent live class migration or recycling.
- Semantic GC can proceed after capture. Its live roots are gathered from the
  runtime and store (`engine/runtime.go:680`, `engine/waif_lifecycle.go:335`),
  while the writer owns its detached records and payloads. Captured anonymous
  references already carry serialization IDs and their records are held in the
  snapshot. Later removal of a live anonymous record does not remove that copy.
  Go references in Writer keep the frozen heap graphs and immutable arrays alive
  through WriteDatabase; no live store lookup occurs during encoding.
- Lists preserve existing elements under Set and append: `types/list.go:168`
  copies on replacement; append's frontier writes only beyond the old header's
  length. Maps are persistent values. Store verb names/code are immutable backing
  arrays; mutation replaces them (`db/store/store_verbs.go:335`,
  `db/store/verb.go:241`, `:255`). Writer reads captured VerbView values rather
  than live verb nodes.
- `engine/task.go:60` takes metadata and VM snapshots under the task lock.
  `vm/persistence.go:16`, `:282`, `:290`, `:318`, `:338` copy the mutable VM,
  program, native-continuation and handler arrays. `task/snapshot.go:129`
  rewrites the emitted typed values. The writer does not retain a native context
  or executable frame pointer. Borrowed builtin-property metadata is not emitted.
- `server/connection_manager.go:470` collects scalar pairs;
  `db/format/writer_connection.go:6` copies the result. Connection mutation or
  closure after capture cannot alter the copied emitted metadata.

## State, callers and regression audit

`db/format/checkpoint_stage.go:62` rejects repeated/closed capture and makes a
failed capture non-retryable on that handle. Serialize rejects missing capture,
repetition and closed state, clears the retained writer before WriteDatabase,
and marks serialized only after the final flush succeeds. Publish cannot follow
failed encoding. Close clears the writer/identities, closes the staged file,
releases the path lock on every return and preserves recovery files only after
publication has been attempted. Existing native error wrapping/joining remains.

The only production phase callers are `server/server.go:493` and
`db/format/checkpoint.go:35`. Both defer Close with errors.Join. Running server
serialization occurs after WithCheckpointBarrier returns; terminal serialization
and publication remain inside it. Direct ordinary/panic helpers run all phases
sequentially as before. No added goroutine or concurrent Close contract exists.

The existing capture-consistency regression now intercepts before Capture
(`server/input_admission_test.go:130`), preserving its task/store gap assertion.
The new regression (`:177`) blocks at Serialize entry after actual capture,
executes a real Eval changing the live scalar and WAIF, then reloads and checks
the captured scalar, durable WAIF identity/payload and both queued-task aliases.
This is a meaningful deterministic release assertion. It does not pause midway
through a blocked OS file write or force semantic/Go GC, class deletion, anonymous
recycling, pending-finalizer execution, or suspended native continuations during
encoding. Those lifetime conclusions above are source-derived, not separately
exercised by the new regression.

The stage tests cover abort before capture, nil/repeated capture, unused captured
state, failed task encoding, sync/sidecar failure, repeated/out-of-order calls,
writer release and recovery preservation. Existing publication progress/failure,
failure-hook-after-resume and terminal background activation tests remain.
The terminal test still covers shutdown, signal shutdown and panic; its dispatch
probe now runs before capture while admission is paused.

No local tests were run by this analyst. Coder RED/GREEN evidence was read but
was not independently rerun. No workload performance measurement was performed.
