# Uncaught-error task identity and startup evidence, October 14, 2026

The [fresh magic capture](mongoose-magic-diagnostics-20261014.md) showed why an
actor's latest issue cannot identify the command that failed: ten matching
`E_INVARG` rows belonged to a flush context, while the earlier cohort had a
uniquely matched `E_MAXREC` event. We tested whether an uncaught-error handler's
`task_id()` could identify the originating task.

## Native contract and Barn result

The [diagnostic witness](../../tests/mongoose-diagnostics/server/uncaught_error_task_identity.yaml)
uses an explicit owned driver and zero-delay fork. The fork records its actual
task ID before an uncaught `raise(E_INVARG)`. The server handler matches the
driver's structured frame and records only identity/count/code predicates.
No exception messages, values, arguments or source are exported. On the stock
success path it also restores and asserts the original handler code and attrs;
fallback cleanup remains available on failure in the disposable managed server.

With capability admission in each session and the exact unchanged YAML:

```text
Stock WSL Toast: 2 passed, 1 warning in 7.11s; ORACLE_EXIT=0
Barn: 1 failed, 1 passed, 1 warning in 7.79s; CANDIDATE_EXIT=1
Expected: [1,1,1,1,1,1,1,1,1]
Actual:   [1,1,0,1,1,1,1,1,1]
```

Only the third predicate differs: **the handler's task ID equals the originating
fork's task ID**. Both servers recorded one fork execution and one matching
handler call; fork-returned ID matched the recorded origin; handler receiver #0,
player, expected error code and driver receiver all matched.

Thus stock retains the originating ID in this verified uncaught-fork path, while
Barn's handler reports a different ID. Source tracing agrees: native dispatch
calls the handler directly, while Barn's `reportUncaughtException` dispatches
through a new server task. The observed predicate is the evidence; it is not a
general proof about every callback path or the cause of the magic failure.
No task-identity semantic fix has been implemented.

Witness SHA-256:
`4e17856aa1f2f6ac6e69e009c4f05d6fa8fb0b11bf64796039acd6c98c415fc5`.
It was moved byte-for-byte from its temporary candidate location into the
explicit diagnostics suite after the runs. It currently exposes a known Barn
failure. For reproduction, follow the [managed candidate command](../mongoose-conformance.md)
with these suite selectors and include canonical capability admission:

```bash
--moo-suite-root="$repo/tests/mongoose-diagnostics" \
--moo-suite-path=server/uncaught_error_task_identity.yaml \
-k 'capability_admission or uncaught_error_task_identity'
```

Stock used canonical `/root/src/toaststunt/build-release/moo`, pinned source
`aecc51e9449c6e7c95272f0f044b5ba38948459e`. Barn used the unchanged repaired
binary `13ef3890cf48a0c53ae2d48bdb471059062765232a4c59802342e4b2ea749e16`.
The disposable TestDB SHA was
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
Exact commands, source copies, XML, admission and exit records remain under
`.tmp/uncaught-task-identity-stock-02` and
`.tmp/uncaught-task-identity-barn-03`. The initial stock attempt was a collection
failure (`cleanup steps cannot have expectations`, exit 2), retained separately;
it supplied no semantic result.

## Startup evidence retention repair

Two initial Barn attempts failed before admission: connection readiness exceeded
the fixed 30-second deadline, yielding one setup error in 36.09s and 36.40s.
No identity assertion ran. Verified binary, YAML, fixture and config hashes
showed no drift. Both temporary directories were removed by final cleanup,
leaving only a three-line startup tail. The exact stalled phase remains unknown.

The harness already latches terminal lifecycle failure. `start()` attempted to
preserve its failed directory, but the fixture's final `stop()` deleted it.
The isolated harness repair now permits deletion only when there is no latched
lifecycle failure. Repeated cleanup therefore preserves failure artifacts;
ordinary healthy restart and final cleanup still work. Process stopping,
startup deadlines, checkpoint selection and rollback behavior are unchanged.

The repair is local commit `59f1c64227e2b647ad2937538880fef05548516f` in
`C:/Users/Q/code/moo-conformance-diagnostics`, based on `6e55e6c9`. The original
checkout's tracked sources matched that base and were left untouched. Validation:

```text
RED: 2 failed, 1 passed
Lifecycle GREEN: 71 passed in 1.90s
Full harness units: 581 passed, 1 skipped in 165.69s; exit 0
ruff: All checks passed!
mypy: Success: no issues found in 26 source files
duplicate check: exact recorded baseline; exit 0
```

The regressions exercise actual fixture finalization with fake processes,
including timeout and early exit, complete logs larger than the error tail,
checkpoint bytes, repeated cleanup and healthy restart. Raw commands/results
are in that worktree's `.tmp/startup-evidence-worker/final-checks-01.json`.

The third managed run proved its imported harness module path and patched
source SHA `c1db25c3fd0cee646ec7bb68782118f8ba0490ffd020b52d5734ff0ed65b9682`
in a fresh Linux-only environment. It reached admission and the identity test.
It had no startup failure, so real failed-start retention was not exercised by
that run; the fixture unit regressions supply that evidence. Two wrapper-only
preflight failures (Python quoting, then missing `uv` in a non-login PATH) are
also retained separately; neither launched the managed server. The faster
third session does not explain the earlier startup failures or establish a
performance improvement.

## Next implementation

Preserve originating task identity deliberately across error handling and
expose the originating ID in terminal diagnostics. An ID alias alone needs
careful review against queue/kill/suspension/continuation behavior. Carry a
typed frame/tick/seconds guard reason through uncaught results; do not infer it
from the shared `E_MAXREC` enum or a later actor issue. Retain caught/replaced
errors and discarded-attempt distinctions. Task-lifecycle changes require the
pinned Elle workflow as well as the unchanged managed witness and focused tests.
No speed, memory reduction, production fix or deployment is claimed here.
