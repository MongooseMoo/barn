# Owner cleanup analyst review

## Scope and state

Reviewing actual open PR #638, Release eval resources and standalone hook
snapshots. Published Go patches and the eval/snapshot coder and builtin/store
scout reports inspected. Local file-grep work is excluded and preserved.
No source edits, local tests/builds or publication performed.

## Findings so far

No concrete finding yet. The eval cleanup delegates to existing owners in the
order Runtime.Stop then builtin Session.Close. Existing expression/file callers
join cleanup errors into their return and CLI cleanup can set its named failure
exit status. The standalone snapshot change replaces the sweep-only captured
receiver defer with one unconditional closure reading the current StoreTxn.

## Completed owner and caller inspection

`Session.Close` drains and removes file/SQLite handles, cancels and waits for
active SQLite operations, and joins close errors. Sequential repeated closure
sees empty handle maps. Runtime.Stop is the existing cancellation/join owner.
All production NewEvalSession callers were inspected: EvalExpression, EvalFile
and cmd/moo.run now consume closure errors. Named returns allow their defers
to preserve earlier failures; CLI cleanup sets exitFailed after either the
batch result or repl result. The handle regression opens real file and SQLite
resources through eval, checks validity before Close and invalidity afterward,
and exercises repeated Close. Cleanup-error fault injection is not covered by
that regression and was not executed by this reviewer.

Real standalone hook producers include normal login
(`server/input_login.go:66`) and phantom login
(`server/input_processor.go:530`). The new defer executes after result/commit,
effect publication and saved sweep VM handoff, but before execution ownership
is released. `CommitAndRenew` releases the old transaction and returns a new
snapshot; the closure therefore releases the current registration, including
GC's replacement. StoreTxn.Release is nil-safe, direct-safe and idempotent
(`db/store/store_history_gc.go:342-369`). It deregisters the read timestamp;
it does not erase saved VM frames or transaction payloads.

Sweep suspension saves VM/context before returning. The normal task runner
releases that previous transaction and installs a fresh snapshot before the
next execution (`engine/task_runtime.go:300-309`), so the deferred release does
not make a continuation execute against a deregistered read view. Existing
physical ownership/root handling is unchanged. Runtime GC, task runtime and
GC handshake files have no published diff. Inspected the sweep's reentrant
run_gc path, saved-root handling and existing root/suspension regressions.

The new snapshot regression disables Go finalization, retains observed txn
pointers and asserts zero active readers after read, write, native exception
and actual GC-renewed ordinary hooks, plus corresponding real sweep-owned
calls. It checks result/published writes as well as reader counts. It does not
add a suspended sweep case; that path's correctness here is a source trace,
not a new focused runtime observation.

## Bounded verdict and remaining unknowns

No actionable correctness, concurrency or ownership defect found in the
published change. This is an analyst result, not merge approval. The isolation
runbook was read; ordinary Go success does not establish isolation coverage.

Latest raw PR check snapshot:

```text
Build & unit tests: pass (1m15s)
MOO conformance suite: pending
Serializable histories and negative control: pending
Check for an earlier pass: pass (7s)
Check for an earlier pass: pass (7s)
```

Full CI, pinned checker verdicts/raw histories/nonzero path evidence and any
cleanup-error fault injection remain outside this review's completed evidence.
No performance measurement is claimed. Unrelated file-grep and other agents'
reports were preserved. A required checkpoint also updated the shared living
task note with this analyst's current scope and next step.
