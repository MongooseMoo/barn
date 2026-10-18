# Issue 269 slice B: format/server capture integration

This change integrates the owned store capture into the staged checkpoint
lifecycle. `Checkpoint.Capture(store, tasks, connections)` collects exactly the
typed task persistence roots through TransformPersistenceValues, takes ownership
of those immutable snapshots and pins one Store.CheckpointCapture. The server
captures task/connection state and this store view under one runtime barrier.
Running checkpoints release the barrier before materialization and writing.
Terminal shutdown/panic checkpoints retain the barrier across Capture,
Serialize and Publish, preserving slice A's terminal lifecycle repair.

`Checkpoint.Serialize()` explicitly handles Materialize's error result, rewrites
every saved task persistence value through the same detached graph memo, then
closes the store capture before Writer.WriteDatabase begins. It never calls the
panic convenience SnapshotWithRoots path. Close releases a pending capture and
clears saved task/connection references on every abort path, while retaining the
existing staged publication/recovery behavior. Standalone ordinary and panic
writers use the same Capture -> Serialize -> Publish pipeline. The slice A
Serialize(store, ...) API was replaced, with no compatibility facade.

Debug-only phase messages report numeric milliseconds for materialization plus
task rewriting/release, portable serialization, and file publication separately.
They contain no object IDs, values or private payload. They do not establish a
latency or CPU claim. Root owns separate runtime barrier phase instrumentation.

## Regression evidence

Before moving serialization outside admission, the new held-Serialize server
test failed for both ordinary and coarse runtime commands. Raw evidence:
`.tmp/269-b-format-red.log`.

```text
TestCheckpointSerializationAllowsRuntimeCommit/ordinary
runtime command could not commit during checkpoint serialization
TestCheckpointSerializationAllowsRuntimeCommit/coarse
runtime command could not commit during checkpoint serialization
serialize_red_exit=1
```

The final server probe holds the Serialize entry after Capture, before calling
the real format serializer. An admitted ordinary VM property write and a coarse
add_verb/property write each commit while held. Reload excludes both later
property and verb changes. This is a server stage-boundary probe. The store
capture suite separately holds the actual numbered-object walk internally.

The format test replaces the private staged file with os.Pipe and writes a
roughly 2 MB portable fixture. It reads one byte, then holds the reader. That
prefix proves Writer.WriteDatabase has started; serialization remains incomplete
until the reader drains. Both ordinary transaction commits and coarse writes
complete while held. Calling Materialize on the retained store capture returns
the closed-handle error, proving history was released before file writing.
After later WAIF/anonymous mutations, the drained portable database reloads with
the old numbered property, WAIF marker and anonymous marker. This test aborts
the pipe-backed stage after consuming its bytes; it does not publish a pipe or
make a sidecar identity claim. The normal publication graph test covers identity.

The server graph test creates two real runtime suspended tasks through the
production compiler/scheduler. Their maps hold WAIFs as keys, anonymous objects
as keys/values, shared aliases and WAIF/anonymous cycles. After capture it
mutates the live graphs, resumes one task, kills the other, removes pending
roots and runs run_gc. The live anonymous objects are collected. Reload retains
both captured task graphs, old markers, shared WAIF sidecar identity, cycles and
exactly one pending-only anonymous owner, with no duplicate task-owned roots in
pending finalization.

Foreign-domain WAIF materialization remains a visible ordinary error. Its stage
test checks capture/timestamp release and cleared owned references. Stage abort
tests now include abort after capture and verify release, alongside the existing
write/sync/sidecar/publication/recovery failures. Existing terminal pending-worker,
panic namespace, hook, request coalescing and input preservation tests pass.

## Validation

Raw full targeted suite output: `.tmp/269-b-format-suites.log`.

```text
go test ./db/format ./server ./engine -count=1 -timeout=240s
ok github.com/MongooseMoo/barn/db/format 6.213s
ok github.com/MongooseMoo/barn/server 10.036s
ok github.com/MongooseMoo/barn/engine 26.184s
format_suites_exit=0
```

Raw race output: `.tmp/269-b-format-race.log`.

```text
go test -race ./db/format ./server -run 'Checkpoint|Panic|Shutdown' -count=1 -timeout=240s
ok github.com/MongooseMoo/barn/db/format 6.274s
ok github.com/MongooseMoo/barn/server 6.760s
format_race_exit=0
```

Scoped lint output: `.tmp/269-b-format-lint.log`.

```text
golangci-lint run ./db/format/... ./server/...
0 issues.
format_lint_exit=0
```

This report establishes neither a Mongoose performance/memory verdict nor managed
MOO conformance or pinned Elle isolation verdicts. Root coordinates those gates
and exact-head publication independently. Source ownership here is format stage,
server orchestration and their tests; store/Elle/runtime measurement edits belong
to the other workers and were left out of this commit.
