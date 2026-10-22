# Magic error evidence and collector follow-up, October 14, 2026

The [checkpoint repair](mongoose-fork-checkpoint-20261014.md) made it possible to
retain a verified postfailure magic dump. Offline public-reader inspection
recovered `E_MAXREC`, while the original gameplay collector had retained only
frames. This slice closes that collector gap and tests the inherited builtin
wrapper contract. It does not establish a magic speedup or the cause of the
failed command.

## Saved runtime evidence

The actor-matched saved issue has no task ID. A bounded scan of the existing
repaired magic cohort's runtime and text logs found no explicit frame, tick or
seconds guard reason. One nearby task matches the root label `weave`, receiver
277800 and the issue's time window:

```text
candidate_task_id: 2081519309
vm_attempt.count: 1
vm_attempt.total_ns: 10000595836
vm_attempt.ticks: 9505792
matched_issue_task_id: unavailable
known E_MAXREC/tick/seconds literals in retained text logs: 0
```

The documented task-filtered `barn_logs -runtime -task 2081519309 -json` output
returned exit 1 because an error elsewhere in the complete log remains counted.
That later record lacks a task ID or budget reason. It is not evidence that this
task's filter or the log is error-free. The safe evidence is retained locally as
`.tmp/barn-lifecycle-worker/magic-runtime-labels05.json` and
`magic-runtime-task2081519309-05.json`.

The roughly ten-second interval is VM wall time; ticks are budget charges.
Neither is CPU time or proof that the seconds guard fired. The earlier private
exact-literal comparisons also left the limit reason unknown. An exact event to
task join and an explicit guard discriminator are still missing.

## Typed error collection

`mongoose_error_capture.py` now emits projection schema 2. It reads only the
typed ERR slot of a structurally recognized history row and maps its bounded
ordinal to the fixed 19-name native enum table. Integer or string spellings of
errors cannot supply the enum. It never reads message/value slots for export
and avoids `tostr()` when extracting the code.

Four-field rows carry formatted frames in slot 4. Five-field rows carry structured
frames in slot 4 and formatted frames in slot 5. Both can now join to the actor's
issue through exact equality of the complete, nonempty bounded formatted list.
Each history event retains its own enum. Only one uniquely matching row can
supply the primary `error_code`; ambiguous, missing, malformed and unknown cases
remain `unavailable`. The issue's frame-only record itself does not acquire an
inferred enum. This join still does not establish exclusive command causality.

The Python validator rejects schema drift, non-whitelisted enums, extra payload
fields, inconsistent indices and primary codes unsupported by the unique join.
Existing files from schema 1 remain historical evidence; they are not rewritten.
The collector remains best effort and cannot replace the primary gameplay error
or turn a failed domain into success.

Targeted privacy, bounds, file ownership, retry and correlation fixtures gave:

```text
Ran 22 tests in 0.103s
OK
```

The broader selected gameplay/process/profile/error/runner and benchmark-tooling
suite separately ran `158 tests in 0.542s / OK`; its exact ten-module selection
and raw output are retained in
`.tmp/error-enum-worker-20261014/combined-green-01.txt`.

An exact-body managed probe used the generated production projection with only
synthetic local input bindings and the final FileIO boundary replaced by a safe
return. It exercised all 19 typed enums, both history forms, integer/string
spoofs, ambiguous/unmatched joins, a malformed fifth field and a JSON roundtrip.
Capability admission was included in each session:

```text
Stock WSL Toast: 26 passed, 1 warning in 14.40s; ORACLE_EXIT=0
Repaired Barn:  26 passed, 1 warning in 39.84s; CANDIDATE_EXIT=0
```

The warning concerns pytest record properties with JUnit xunit2. This is not a
performance comparison. These probes do not exercise the production FileIO
boundary; ownership and payload rejection there have separate Python fixtures.
Local retained evidence:

- `.tmp/conformance/projection-schema2/source-proof.json` and generated YAML.
- `.tmp/conformance/{stock,barn}-error-projection-schema2-01-*` raw output,
  admission evidence and XML.
- `.tmp/error-enum-worker-20261014/enum-{red,green}-01.txt`.

The tested projection request SHA-256 is
`43e8a9cbd6384f1a9d7a31aacf77d81a5e2590f3a79ca10a4d2da88baeceab37`;
collector source SHA-256 is
`c278780dc12f59f4009a84cedeceaed3c4132768fa6a22596c664a6e6cc06d65`.

## Inherited protected builtin witness

The new candidate `builtins/protected_builtin_inherited_alias.yaml` installs an
inherited wrapper with aliases `bf_tostr tostr` and calls it from an explicit
ordinary-programmer driver. It asserts receiver #0, caller identity, unchanged
task ID, caller permissions, invoked alias and a nested native `tostr(7)` result.
It restores root ancestry and the prior protection property in `finally`.

The exact unchanged candidate passed stock first, then Barn, with admission:

```text
Stock WSL Toast: 2 passed, 1 warning in 6.61s; exit 0
Repaired Barn:   2 passed, 1 warning in 33.15s; exit 0
Assertion: [[1,1,1,1,"bf_tostr","7"],[1,1,1,1,"tostr","7"]]
```

Quiet output records assertion passes, not a separate dump of the actual tuple.
The initial stock preflight failed with Git exit 128 before the harness ran;
its Windows worktree pointer was not readable by WSL Git. The fresh attempt used
verified explicit `GIT_DIR`/`GIT_WORK_TREE` and preserved that failed preflight.
Evidence remains under `.tmp/protected-inherited-{stock-01,stock-02,barn-01}`.
Candidate SHA-256:
`33aac30e24859634540943337d919d88c1574a5859ee05bd95346c9923171e17`.
Barn used the unchanged repaired binary
`13ef3890cf48a0c53ae2d48bdb471059062765232a4c59802342e4b2ea749e16`.
Stock used canonical `/root/src/toaststunt/build-release/moo`, source pin
`aecc51e9449c6e7c95272f0f044b5ba38948459e`.

This standalone inherited-alias dispatch contract agrees between the servers.
It does not establish what exhausted a limit in the actual magic workload.

## Fresh actual-world capture

The updated collector was then exercised in `.tmp/barn-magic-error-enum-01`,
using the same immutable Mongoose checkpoint/catalog, compatible Toast bootstrap,
repaired Barn binary and ordinary-actor scenario settings: one magic iteration,
seed 1, warmup 30 seconds, deadline 120 seconds, debug runtime diagnostics, and
all profilers disabled. Source/controller hashes were checked at both boundaries.
The zero-focus Lumos negative control failed because its expected ordinary
refusal was absent; this is the assertion classification, not an error cause.

```text
cohort.status: failed; domain.status: failed
metadata.status: collected; projection.schema: 2
primary error_code: unavailable; correlation: ambiguous
matching retained rows: 10; candidate enum counts: E_INVARG=10
new_event_count: unknown; command_exclusive_causality: unknown
all ten final checks: true
owned shutdown: exit 0, forced false
final checkpoint: verified; finalization_errors: []
```

The actor's latest issue in this run contains `#2478:flush` line 44 and
`#2478:schedule_flush` line 9, receiver 2478. Its actor is 279303 and timestamp
1792016482. Ten rows have the same complete formatted traceback. All ten typed
candidate enums are `E_INVARG`, but the collector correctly refuses to choose
one as the primary error. The latest flush context cannot be labeled the magic
command's failure. It is a different cohort/context from the earlier uniquely
matched `E_MAXREC` event. This demonstrates why actor identity and traceback
equality alone are insufficient when errors repeat or later tasks overwrite the
latest issue.

The published database is 218225096 bytes, SHA-256
`b45498465b382f6991e0b07991cf8ab2460e50b6777d6e29724ba2e54dd16914`;
the WAIF identity sidecar is 1570955 bytes, SHA-256
`0dacaf55bd64718706005f94f44a1db680c93400c9bde06004bf437006bfedfc`.
These gates verify publication integrity, not complete semantic reload or domain
success. The failed cohort and original manifests remain unchanged. No speed,
exclusive CPU or memory improvement follows from this capture.

## Next diagnostic boundary

A source survey identifies `engine/task_runtime.go` immediately before intrinsic
eval/uncaught-exception dispatch as the place to emit a terminal task outcome:
retry, commit, cancellation and suspension decisions have already been resolved.
An earlier slice-return record can describe an attempt that is later discarded.

A trustworthy limit discriminator needs typed metadata originating at the actual
frame, tick and seconds guards, preserved through exception/finally/builtin
conversions. Caught or replaced errors and discarded retries must not leak an old
reason. Manually raised `E_MAXREC` and restored legacy pending errors must remain
unknown. A sticky VM field or message parsing cannot provide that contract.
This is a proposal only; this tooling slice changes no Go runtime code or limits.

The subsequent [uncaught-error identity witness and harness repair](mongoose-error-task-identity-20261014.md)
confirmed an originating/handler task-ID difference on Barn's tested fork path.
It also preserves failed-start evidence through repeated harness cleanup.
