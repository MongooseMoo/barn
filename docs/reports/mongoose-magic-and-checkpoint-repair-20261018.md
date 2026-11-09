# Offline Mongoose: magic termination, checkpoint I/O and startup observation

The offline `weave` timeout exposed a Barn correctness defect. Mongoose probes
numeric prepositions until `set_verb_args` returns an error. Barn accepted values
beyond the valid range, so the initializer continued until its seconds budget
expired. The repaired Barn rejects 15, allowing initialization to finish.

A separate checkpoint bottleneck wrote each WAIF identity directly to the
filesystem. A bounded buffer reduces those writes while preserving the bytes,
checked flush, sync and publication sequence. The corrected offline world now
completes a productive Lumos/Nox pair and shuts down with a verified checkpoint.
This establishes those outcomes; it does not establish a statistical gameplay
speedup, lower retained memory, or the cause of live Mongoose latency.

## Why the old magic attempt ran out of time

The immutable input database has SHA-256
`fab6957acffa1db8bc1c4b911fb53a8e8d6e605f4de2a31b9f8e4fa874c3200d`.
The supported database reader exported `#59:_fix_preps` and `full_prep` from it;
the export SHA-256 is
`49ae2a87ad84648c8342c2cdd474d97a35862ce3138cba1ec4c7c3e3d27325c6`.
The initializer starts at 0, calls the native setter with decimal strings and
increments until the caught native error ends the loop. Argument restoration,
cached preposition publication and server-version assignment follow that loop.

The original task 1788854219 ended with `E_MAXREC/seconds_budget` after one
10.0005-second VM attempt and 14,623,744 ticks. Its sampled task CPU was 9.96s.
The setter's 6.33s, adoption's 4.34s and cloning's 2.54s are overlapping nested
costs. There were no recorded retry-waste, readmission or asynchronous records
for this task. It held the exclusive commit gate during execution, which could
delay other committers; concurrent process-wide waits are not its exclusive cost.

I measured and optimized the repeated publication work before checking why the
loop kept running. The bounded oracle comparison exposed the termination defect.
The earlier source-sharing cohort and its failed outcome remain unchanged.

### Native contract and repair

The documented managed conformance runner tested the canonical Debian Toast
oracle, SHA-256
`72fb1cf96cb303647a8ee72808e7c1ff62a491ecf44f547e6757e71ba2402bde`,
before Barn parser analysis. Each session included `capability_admission`,
explicit test-owned receivers, bounded probes and cleanup.

| Observation | Canonical oracle | Unrepaired Barn | Repaired Barn |
| --- | --- | --- | --- |
| Strings 0 through 14 | Success; canonical spellings | Success; raw numeric strings | Success; canonical spellings |
| Strings 15 through 32 | `E_INVARG`; argument spec unchanged | All accepted | `E_INVARG`; argument spec unchanged |
| Leading zeros, such as `01` | Accepted and normalized | Contract not enforced | Accepted and normalized |
| Probed signed, padded, overflowing, suffix and empty values | Rejected | Contract not enforced | Rejected |

Commit `05f3680ccfcddf6a94e0d73ebc57a725d6ac17d9` validates string
prepositions against the existing named table or strict unsigned decimal syntax,
overflow and range checks. Refusals occur before the targeted `Store.SetVerbArgs`
publication and live-verb adoption. Object, verb and owner admission order and
the existing `beforeCoarse` prelude remain: that prelude may still flush earlier
staged work. Existing non-string coercion remains unchanged.
Source: [normalization at builtins/verbs.go:36](../../builtins/verbs.go#L36)
and [setter admission/publication at line 633](../../builtins/verbs.go#L633).

The baseline witness failed with admission passing. Both strengthened canonical
and repaired Barn sessions passed all three tests with zero failures, errors or
skips; their native range and lexical messages matched byte for byte. Companion
fixture commit: `441c0bfe535a067f66e5a58aa1638fd551b4894d`.
These are MOO conformance receipts, not a canonical-stock whole-world gameplay run.

## Checkpoint I/O evidence

The original controller sent an owned SIGTERM, waited its unchanged 90 seconds,
then sent SIGKILL. Exit -9 records that controller action; it is not OOM evidence.
The partial sidecar had space for 43,035 records after its header, but those
unpublished bytes were not declared valid. Source ordering places its creation
after database serialization, sync, close and hashing. The process's exact stack
at termination is unavailable.

The production-function diagnostic used 48,000 valid identities. Each row is
**one observation per side**, not a statistical estimate or gameplay comparison.
Validation of exact bytes, ordering and readback occurred outside timing.

| Filesystem / observation | Direct writes, seconds | Buffered, seconds |
| --- | ---: | ---: |
| Native Linux, untraced | 0.103544854 | 0.019125783 |
| WSL Windows mount, untraced | 21.697321437 | 0.193185733 |
| Native Linux, strace | 6.161767789 | 0.084372919 |
| WSL Windows mount, strace | 28.429576655 | 0.249630538 |

Write calls fell from 48,001 to 387; `fsync` remained one. All outputs were
1,584,089 bytes with SHA-256
`939d307d52d582c67979ac71485957a605a09795f9d7d543bb62d6c0248f913d`.
Traced timings are separate because tracing changes the cost substantially.
Commit `82c042132037282dcb90307a1b2f4a5a03e4bc47` adds the checked buffered
flush before unchanged sync/close. Tests cover large exact output, roundtrip and
deferred ENOSPC. This proves a relevant I/O bottleneck, not the exact entire cause
of the old forced termination.
Source: [db/format/waif_identity_sidecar.go:112](../../db/format/waif_identity_sidecar.go#L112).

## Startup observer failure and repair

The first combined repair cohort stopped before any gameplay because the
readiness observer's `server.log.read_text()` raised `OSError(61, ENODATA)`.
Its exact traceback and failed invocation remain preserved. I let a single
log-read failure abort readiness; the fix retries that specific error inside
the existing startup deadline, after checking process ownership.

Commit `f08fd946d2442c2d6cbad9c20032ab48d1599d29` changes the observer and
its mocked tests. It retains a bounded count, errno and first/last timestamps in
the server identity. Unrelated OS errors, ownership failures and deadline
exhaustion remain failures; successful log observation and readiness validation
are still required. The underlying filesystem/kernel cause is unknown.
Source: [scripts/mongoose_barn_process.py:220](../../scripts/mongoose_barn_process.py#L220).

The regression reproduced three failures before repair. Afterward, all 19
focused tests and 67 combined lifecycle/runner/collector tests passed.
Independent review found no blockers. The new actual cohort did not encounter
ENODATA, so it is not evidence of real-system recovery from that error.

The startup-failed cohort nevertheless shut down unforced with exit 0 and a
verified database/identity pair in 33.658s. It has no gameplay/profile result.
It and the earlier source-sharing cohort remain failed historical evidence.

## Corrected actual-world outcome

One separately declared corrected-observer cohort used the original database,
catalog, compatible bootstrap, scenario and seed, with 30s warmup, 120s action
deadline, 15s CPU sampling, 180s startup and 90s shutdown limits. There was no
hidden rerun or concurrent test, build or profile-analysis workload.

```text
Domain status: complete; completed magic iterations: 1
Attempts: 8 = 6 fizzles + productive Lumos + productive Nox
Room light: 0 -> 0.238732414637843 -> 0
Negative controls: both independently verify no effect
Actor: player true; wizard false; 8 unique receipts and matching resource debits
Cleanup: actor, room, tutor and wand removed; private account retained explicitly
Shutdown: exit 0; forced false; 32.006s from shutdown entry to completion
Preservation/ownership checks: 10/10 true
Owned PID absent; exact executable process matches []; unreadable executables 0
```

The fresh published database is 217,230,884 bytes, SHA-256
`73a026ca627a311d4656de5c24717a3e31fa98e1772d492409e15627c4425d5d`.
The identity sidecar is 1,573,166 bytes, SHA-256
`ce2e44d033eb8834e7fc092bcb68a32cfbcdfa045701221bb1d2f1d6bbad28b0`.
Both rehashes match the original manifest. This verifies published-pair integrity;
it is not a separate whole-database semantic reload test.

The immutable controller result is `diagnostic-unvalidated`, exit 1; Go
diagnostic transport succeeded, and its validation field remains false.
Independent offline parsing subsequently passed all 11 base Go views and two
task-focused CPU views, with expected sample types and nonzero totals. The
offline profile receipt exits 0 and confirms original manifests unchanged.
This derived verdict does not rewrite or promote the controller's original gate.
The broader runtime command exits 1 because it found one full-log error; a
separate typed census found 78 terminal exceptions: 77 `E_INVARG`, one
`E_INVIND`, zero `E_MAXREC`, and zero invalid census records. Native root metadata
places 76 `E_INVARG` records at `#2478:schedule_flush`, one at `#592:exitfunc`,
and the `E_INVIND` record at `#0:user_client_disconnected`. Their typed
termination reasons are `unknown`. Root identities are not exclusive joins to
player actions or nested frames. No terminal `weave` record was found.
The full-log error is `traceback undeliverable (no connection)`, `E_INVIND`;
its row lacks task/receiver/verb fields, so its exact origin cannot be joined.
The same fixed error/undeliverable metadata class occurs in both earlier
retained cohorts. That is evidence of prior occurrence, not proof of harmlessness
or identical causation. The independent domain/census audit exits 0: controls,
receipts, debits, cleanup, pair hashes/header and preservation gates verify.
Ten recorded `weave` root tasks each have one VM attempt and no retry-waste
record; their maximum attempt wall duration is 12.581173ms. Wall durations are
not CPU measurements, and this is not a paired performance comparison.

### Measured next targets

The 15-second CPU capture contains 2,064 samples totaling 20.64 CPU-seconds
across process threads. Root labels attribute 15.76s (76.36% of samples) to
`#1007:s_run`, 1.43s to `#9655:start_rearm`, 2.34s to unlabeled work, and 0.05s
to four sampled `weave` task IDs. Sampling omission is not proof of zero work.
These root shares concern this process window; they do not measure exclusive
command latency. Cumulative VM execution accounts for 16.70s, overlapping its
callees, and must not be added to those root shares.

The process observer recorded 76 samples and a peak RSS of 3,627,148 KiB
(about 3.46 GiB). The after heap profile estimates 717,180,503 in-use bytes;
its largest flat allocation site is property `setSlot` slice cloning, 114.36 MiB
(16.72%). Heap sampling, garbage-collection boundaries, allocator reservations
and capture times differ from RSS and runtime counters. These numbers are
descriptive; they do not establish growth or reduced retained memory.

The next bounded investigations should inspect the work rooted at `#1007:s_run`
and the lifetime of cloned property slices, then form separate controlled CPU
and memory hypotheses. The former repeatedly timed-out initializer is no longer
the dominant sampled CPU root in this successful magic run.

## Build and verification provenance

The Go executable was built from `05f3680ccfcddf6a94e0d73ebc57a725d6ac17d9`
using the standard main package and default PGO input. Its SHA-256 is
`d47d6ace45f9115198141a49a83eb8ccdc8140c15e15f0083422a1044cfe5a72`.
The Python controller source is separately committed at `f08fd946...`.
All 697 Go/PGO/module inputs equal the tested build; seven runtime controllers
are unchanged and the readiness observer is the sole intended controller delta.
All 2,379 inventoried source inputs stayed byte-identical throughout capture.
The original manifest's null `source_commit` is preserved; external bindings
supply the explicit source identities.

Full Go tests passed. Race tests passed for builtins and database format. The
pinned Elle workflow and strict audit passed: 12 expected verdicts, 20 positive
coverage keys per seed, 48 native-exception paths per seed, zero healthy-history
fail/info outcomes, six nonempty fault witnesses and 67 matching archived files.
Raw histories, manifests and anomaly witnesses remain retained. Archive SHA-256:
`eb6fc1f452d2eef0cede39d9e1d9ae6685c2e81bd9a9966f80dadd3e10c10ccb`.
The later Python-only change did not require rebuilding or rerunning Go checks.

## Evidence locations and limits

The committed machine-readable summary is
[`mongoose-magic-checkpoint-repair-20261018-results.json`](../../experiments/mongoose-magic-checkpoint-repair-20261018-results.json).
Raw local evidence is retained under these isolated worktrees:

- `barn-shutdown-diagnostics/.tmp/sidecar-investigation`: all eight I/O observations and source/binary seals.
- `barn-preposition-bounds/.tmp/preposition-verification`: full Go/race/Elle, build and failed startup evidence.
- `barn-gameplay-baseline/.tmp/mongoose-control-evidence-20261018`: canonical receipts, baseline RED and repaired GREEN.
- `barn-startup-log-retry/.tmp/startup-observer-study-20261018`: declared plan, exact command, source bindings and preservation proof.
- `barn-startup-log-retry/.tmp/barn-magic-startup-observer-repair-20261018-01`: immutable actual cohort.
- `barn-startup-log-retry/.tmp/final-repair-review-20261018`: independent derived diagnostics.

One successful magic iteration cannot support throughput or retained-memory
claims. Old failed cohorts are unsuitable paired baselines. A future performance
experiment needs comparable productive work, unchanged observer semantics and
finalized paired cohorts, with CPU and memory reported in their measured scopes.
No live server, deployment, push or remote merge was involved.
