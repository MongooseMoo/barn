# Restored queued-fork checkpoint investigation, October 14, 2026

The [gameplay diagnostic cohorts](mongoose-barn-diagnostics-20261014.md) exposed
a checkpoint failure independent of completed flight: queued task 473852115
could not be written because its environment had three names and 24 values.
The historical source checkpoint itself contains 24 named values, not that
inconsistent pair. This record follows a deterministic managed witness; it
does not attribute CPU shares or claim an engine speedup or memory reduction.

## Native expectation and Barn regression

The initial stock probe observed the parent-only local before checkpoint, then
found it absent after reload while the used value remained 73. Its provisional
preserve-all expectation failed:

```text
expected [1, 0, 73, 1], actual [1, 0, 73, 0]
1 failed, 1 passed in 7.58s
```

That rejected expectation is retained in `.tmp/conformance/stock-fork-probe-*`.
Pinned stock Toast source `aecc51e9449c6e7c95272f0f044b5ba38948459e` explains
the result: `read_forked_task` recompiles the isolated fork body and
`reorder_rt_env` remaps matching saved variable names case-insensitively to the
new program, discarding names it no longer contains. `queued_tasks(1)` exposes
that remapped environment. Preserving all unused names would diverge from the
native behavior.

The corrected candidate
[restored_fork_environment.yaml](../../tests/mongoose-conformance/server/restored_fork_environment.yaml)
has SHA-256 `547e6cfe5ec9b6bf6d85e1275cd5a35e176bf49154bce152becc5daa084995c0`.
It creates an owned verb with a used local and a parent-only local, then checks
two fresh requested checkpoints and managed restarts. Each restart uses
`checkpoint_timeout_ms: 5000`; distinct persisted generation markers reject a
stale second checkpoint. The final assertion proves the same fork ID executes
once with value 73 and leaves the queue. A bounded harness wait keeps long
suspended evaluations outside the transport's per-command timeout.

An intermediate long-evaluation variant hit `SocketTransport._receive`
`TimeoutError: timed out` (1 failed, 1 passed in 12.17s); the exact timed-out
step was not identified by that stack. This is retained separately as a fixture
transport failure, not a native semantic verdict or successful roundtrip.

Through the documented managed command, with canonical capability admission in
each session and matched outbound-on TestDB profiles, the final witness gave:

```text
Stock WSL Toast: 2 passed, 1 warning in 43.55s
Unchanged Barn:  1 failed, 1 passed, 1 warning in 8.06s
Barn after first reload: expected [1, 0, 73, 0], actual [1, 0, 73, 1]
```

The warning concerns pytest record properties with JUnit xunit2. Raw XML and
admission evidence are `stock-fork-roundtrip-wait-20261014-*` and
`barn-fork-roundtrip-before-20261014-*` under `.tmp/conformance` (local retained
artifacts, not shipped files). The unchanged Barn executable SHA-256 was
`2bb5a49b6da0736fe3424653f77b301eabef37e1eb65153906c11e9a12f04d9a`.

The final expanded candidate also reads an uninitialized local inside the fork
and requires `E_VARNF`, rather than silently accepting integer zero. Its SHA-256
is `2e50515b5459ac254d3fcccc1c3848214815f7c254d78e3ef78df7645c339f07`;
stock passed admission and the complete expanded case in 43.58s (2 passed).
This separate evidence is `stock-fork-roundtrip-unbound-20261014-*` under
`.tmp/conformance`. Unchanged Barn's value codec turned an internal unbound slot
into zero; the loader and codec therefore both needed repair before the expanded
witness or missing-local persistence could pass.

## Repair and verification state

The loader now remaps saved values case-insensitively to the recompiled program's
names, drops unused parent slots, and records the same values in its installed
execution frame and checkpoint environment. Missing slots remain unbound,
including missing predefined locals. Because the reader has discarded serialized
name order, ambiguous differently-cased saved matches are explicitly rejected
before queueing rather than selected by random map iteration. Native well-formed
name tables do not contain those ambiguous matches; compatibility with forged
duplicate tables is not claimed. NUM/INT names remain distinct.

The codec now reads existing native NONE tag 6 as the internal unbound sentinel
and writes that sentinel as tag 6 without a payload. Integer zero and CLEAR tag 5
retain their encodings. Public value type behavior and strict runtime-environment
count/name validation are unchanged. No new database format was introduced.

RED regressions reproduced the real count mismatch, lost case-folded values,
overwritten mutable predefined values and missing-local persistence. Loader-only
validation exposed the remaining absent-to-zero codec defect; the combined
tests now use the production loader and cached frame, actual writer/read cycles
twice, preserved task metadata/values, missing-local E_VARNF and saved-map
independence. Raw outputs are retained under `.tmp/barn-lifecycle-worker` and
`.tmp/format-unbound-worker`.

```text
Combined focused engine: PASS; ok .../engine 0.279s
Codec focused tests:    PASS; ok .../db/format 1.062s
Full format package:    ok .../db/format 5.318s
Full Go suite:          GO_ALL_GIT_CONTEXT_EXIT=0
```

The first full-Go attempt failed only the root repository hygiene test because
WSL Git could not resolve the Windows worktree's `gitdir: C:/...` pointer.
Other packages passed. The rerun supplied verified Linux `GIT_DIR` and
`GIT_WORK_TREE` paths; hygiene passed and unchanged packages used Go's cache.
Both complete outputs remain separate under `.tmp/checkpoint-repair-go-all-*`.
No source test was weakened to bypass this environment error.

Focused race checks completed with `RACE_EXIT=0`: engine 3.151s, task 1.019s,
format 1.022s. The rebuilt Linux Barn executable SHA-256 is
`13ef3890cf48a0c53ae2d48bdb471059062765232a4c59802342e4b2ea749e16`.
Build inputs were frozen loader
`cbea4d77d43d2229601d47df28b28f11184dc0db9a3b7587a8ac57948c1e3689`, reader
`e9b34eb6ecbe7de73b298f6d2098fab1167b47f46768503a40a43dcf7c9c10ef`, and writer
`3fa968294625afc1925cd921cb6e092897ab67e729baf2acab202f05ceda35ed`, on the
`c567e372` tooling checkout plus the uncommitted repair. `go list` confirmed the
main package before build; Go 1.27.1, linux/amd64, GOAMD64=v1 and the repository's
existing default PGO profile were used.

```text
Changed Barn expanded managed witness:
2 passed, 1 warning in 59.56s
```

Its raw XML/admission evidence is `barn-fork-roundtrip-after-20261014-*` under
`.tmp/conformance`. This includes both fresh requested checkpoint publications,
the two persisted markers, dropped parent-only local, uninitialized E_VARNF and
once-only execution with the same fork ID/used value. Its elapsed test duration
includes fixed waits and startup; it is not an engine performance comparison.

The pinned Elle workflow completed with default seeds 412/395/396, 40 rounds,
four admitted workers and all five deliberate faults. The CLI checkout was clean
at `9c2a18b3719f573dab06ae05c955c9c9d5225673`, freshly rebuilt using Java 21;
standalone jar SHA-256
`e53120b925291586d00c4b99cb1f8ac31439d372b2c932da05634d2f6ded5373`.

```text
Checker exit: 0; results.errors: []
Healthy list-append/rw-register: 6 true verdicts
Fault append histories:         5 false verdicts
Nonempty anomaly witnesses:    1 for each deliberate fault
```

Actual aggregate committed-path coverage, from each healthy manifest:

| Seed | Parallel | Exclusive | Gate | Hot renewal | Verb memo | Suspend | Fork |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 412 | 590 | 730 | 846 | 1 | 458 | 321 | 14 |
| 395 | 586 | 736 | 850 | 2 | 425 | 321 | 14 |
| 396 | 572 | 745 | 847 | 3 | 444 | 321 | 14 |

Each seed also records 48 native exceptions and four read-only observations.
Raw EDN histories, manifests, Go/CLI logs and graph witnesses remain under
`/root/.cache/barn-checkpoint-repair-elle-20261014` in Debian. The portable
archive `.tmp/checkpoint-repair-elle-20261014.tar.gz` was listed successfully and
has SHA-256 `d00668b10d645a32d4c6c11ac5d7a4a16e859871dc0157cfccb54b999a4de078`.
This is bounded isolation-corpus evidence; the managed witness supplies the
specific cold-restoration/checkpoint contract. Neither establishes performance.

The first checker preflight stopped before execution because the Barn Git
override was applied to the separate CLI pin lookup. Verifying that repository
before exporting Barn's Git paths corrected it; no history/verdict was discarded.

## Actual Mongoose flight acceptance

The fresh owned flight cohort at
[barn-flight-checkpoint-fixed-01](../../.tmp/barn-flight-checkpoint-fixed-01/manifest-final.json)
uses the unchanged gameplay oracle, historical input/catalog/bootstrap hashes,
registered promotion-enabled Mongoose profile and repaired binary above.
Go/Toast/native CPU collectors were off; runtime debug diagnostics were on.

```text
cohort.status: complete; controller exit: 0
flight.domain_status: complete; completed_iterations: 1
owned shutdown: exit 0; forced: false
owned_final_checkpoint: true; all 10 final checks: true
finalization_errors: []
```

After the owned signal boundary, Barn published a fresh regular database/WAIF
identity pair, with matching sidecar database hash and no publication journal
or checkpoint-failure record. Published database: 217321702 bytes,
SHA-256 `f68ec91f7a4215ccbf269cb4f5fee48e3487cdf68531f5646fdbb929aa16c7c3`.
Sidecar: 1574123 bytes,
SHA-256 `fd0be70c730bdf91c5a4a7de21fcbdd2e9425eee2934a11f587d920776dde2d9`.
The manifest retains inode/time evidence and the owned shutdown log hash.
This proves publication integrity, not a full semantic Mongoose reload.

Its action window was 10.103310384s wall / 13.71s process CPU, including scheduled
background and observer work. One debug-instrumented cohort cannot establish a
speedup or memory improvement against the earlier failed cohorts.

The separate actual magic rerun remains **failed**: its ordinary zero-focus
refusal still produces a coding error rather than the expected refusal. Its
controller returned 1 and no complete magic sequence was proved. All ten
final integrity checks passed, owned shutdown returned 0 without forcing, and
the post-failure database/WAIF pair was verified. Thus checkpoint repair does
not conceal or resolve the independent gameplay failure.

The [magic manifest](../../.tmp/barn-magic-checkpoint-fixed-01/manifest-final.json)
retains that distinction. Published database: 217575316 bytes,
SHA-256 `0afb2bfccec98614d5cafb19ad02cda6d38fb1a4082534247ef41b8a420cbcf0`;
sidecar: 1570163 bytes,
SHA-256 `1b834086e8aed331f05cd402a61fab864e6de012e133343bae83d6b56eeb4182`.
It can now support bounded offline error inspection, unlike the earlier failed
checkpoint cohorts.

Before cleanup the metadata projection uniquely matched this actor's issue to
one bounded history row. Its latest chain starts at `#1584:bf_tostr` (receiver
`#0`), then `#59:_fix_preps`, `#59:full_prep`, the actor `my_huh` frames,
`#56:do_huh` and the command-facing `#2700:weave` frame. This differs from the
earlier `#55:make` innermost frame. Most positions remain line zero and the
frame collector still lacks the error enum, programmer and per-frame player.

A separate read-only public `moodb` projection of the SHA-256-verified published
dump recovered the actor-matched **E_MAXREC** error. It required agreement with
the captured actor 279303, issue timestamp 1792014699, complete bounded traceback
list and sanitized frame signature before selecting one typed `MooError` enum:

```text
status: matched_published_primary_issue
error_code: E_MAXREC; matching_rows: 1; history_total: 15
malformed: 0; unsupported_headers: 0; unknown_lines: 6
PROJECTION_EXIT=0
```

The public reader parsed the checkpoint in memory; it exported only whitelisted
enum/identity/shape data, not messages, source, arguments, error values or raw
traceback rows. The original frame-only capture is unchanged and still reports
its own enum as unavailable. Proof DTO/script remain local at
`.tmp/barn-lifecycle-worker/postfailure-primary-error-01.json` and
`postfailure_primary_error.py`.

The same public-reader projection confirms the problematic future task was not
discarded to make checkpoint publication pass: task 473852115 remains exactly
once, with three named environment values, one source line, scheduled time
1797319629, player 38280, programmer 2778, receiver 4266 and definer 1007. Its
body fingerprint is `b4adcc0a9af6445680cf508b7c950cba87d902e9373318f18d6b9d1eeabc0bd2`
(UTF-8 with LF after each source line). No variable values/names or body text were
exported. This proof is retained as
`.tmp/barn-lifecycle-worker/postfailure-primary-error-and-task-02.json`.

This identifies an E_MAXREC event in the builtin/preposition call chain. Barn
currently reuses that enum for frame-depth, tick-budget and seconds-budget aborts
(`vm.go` frame guard and execution-loop budget guards). The enum and seven
displayed frames therefore do not establish which limit fired, total physical
depth, a specific expression or exclusive command causality. A fresh offline
projection compared the matched event's message/value privately against the exact
controlled tick/seconds literals in that source. All six comparisons were false:

```text
PROJECTION_EXIT=0; matching_rows=1; reason=unknown
known_literal_matches: all false
```

This leaves the actual limit unclassified; absence of those literals does not
prove frame exhaustion. The safe proof is retained at
`.tmp/barn-lifecycle-worker/postfailure-primary-reason03.json`; earlier proofs
are unchanged. The wrapper's
receiver #0 agrees with the native protected-builtin bypass contract; its inherited
definer and stored aliases alone do not prove recursive redirection. Inspect
task budget diagnostics, then verify the next dispatch/budget witness against
stock Toast before another engine repair. No production deployment or performance
benefit is claimed.
