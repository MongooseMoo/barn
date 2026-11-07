# Checkpoint capture final verification

## Goal

Gate PR 641 at its assigned published head. Default verdict is NO-MERGE until
source review, analyst findings, terminal CI and retained pinned Elle evidence
have been checked. No implementation changes, local test runs or publication.

## State

Published head matches the assigned head. Read docs/elle-isolation.md and the
actual PR file inventory. The instructed legacy gauntlet file is absent;
read the available gauntlet protocol entrypoint instead. Existing untracked
analyst report is preserved.

## Evidence and decisions

NO-MERGE. The required MOO conformance job failed. Source review found no
actionable defect in the checkpoint change, but that does not override red CI.

```text
MOO conformance suite fail 10m43s
Build & unit tests pass 1m21s
Serializable histories and negative control pass 4m42s
Record pass (conformance) skipping
```

The terminal failure is
audit/gap_followups_toast_oracle.yaml::audit_binary_mode_dispatches_raw_chunk_without_newline.
Its final read of #0.audit_binary_seen expected [''] and returned []. The raw
job output is retained in system-layering-checkpoint-conformance-failure.txt.
The downloaded original XML, admission evidence and server logs remain under
.tmp/conformance-failure. No retry was requested and no flake claim is made.

## Source gate

Read the actual published diff, analyst and coder reports, checkpoint stage,
server dump callers, store SnapshotWithRoots and task persistence-value walk.
Capture retains the existing Writer with frozen task/store values; Serialize
consumes it and Close drops any unused capture. Running serialization follows
barrier release, while terminal serialization/publication remain inside it.
Phase errors, deferred Close error joining, path ownership and recovery behavior
remain explicit. The new regression demonstrates runtime commit while encoding
is held and verifies reloaded pre-capture scalar/WAIF data and queued aliases.
Its test does not force GC or recycling during encoding. No source defect was
identified by this verifier, and the analyst established no major/blocker issue.

## Raw isolation evidence

Downloaded the pinned workflow artifact from run 42366637396. The original
.tmp/elle-evidence/elle-evidence.tar.gz remains intact, including raw histories,
manifests, Go/Elle logs and witnesses. The workflow is attached to the assigned
head and ran the PR merge tree. results.json contains no errors:

```text
results_read_exit=0 healthy=6 faults=20 errors=0
archive_exit=0 histories=46 logs=49 witnesses=105
```

Each healthy result is literal true/exit 0; every negative-control result is
literal false/exit 1. All three seed manifests have nonzero committed coverage.
Aggregate manifest counts (append and register) are:

| Seed | Parallel | Exclusive | Exclusive gate | Hot renewal | Fork | Suspend | Native exception | Verb memo |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 412 | 805 | 677 | 860 | 3 | 14 | 481 | 48 | 486 |
| 395 | 800 | 681 | 864 | 4 | 14 | 481 | 48 | 478 |
| 396 | 806 | 676 | 866 | 4 | 14 | 481 | 48 | 472 |

Read the commit-exclusion G2-item witness: one task misses the other's append
to inherited-property, while the other misses the first task's append to
property, establishing opposite dependency edges. Other fault witnesses are
preserved in the archive. This bounded isolation result is not a performance or
whole-system liveness result.

## Failed producer investigation

Confirmed CI used the main conformance checkout containing the actual test.
It creates a player/auditbin verb, opens a secondary connection and sends login,
waits 500 ms, enables binary mode from the primary connection, sends the seven
ASCII bytes auditbin without CR/LF, waits 500 ms and reads the result property.
runner.py invokes TestConnection.send_bytes, which calls socket.sendall(data)
without adding framing. The test's command handler appends its empty argstr.

Barn network.go does wake the input reader after enabling binary mode:
Connection.WakeInputReader uses an immediate read deadline for TCP. The logged-in
HandleConnection timeout path loops, resets the deadline and selects ReadChunk
when binary is enabled. Therefore the simple explanation that enabling binary
never wakes an old text read is contradicted by source. No exact failing
interleaving or root cause has yet been proved. Retained server logs rotate and
do not label this input, so they do not establish its delivery or dispatch.
PR 639's earlier full conformance run passed with the same conformance main;
that comparison alone does not establish that this failure is intermittent.

## Next step

Coordinator owns further diagnosis/repair and green required CI. No broad local
tests or implementation edits were run by this verifier. Re-gate the affected
change after the failure is resolved.

## Managed producer comparison

At coordinator request, ran the documented managed workflow in verified Debian
with admission included. First confirmed the exact unchanged binary case on the
canonical /root/src/toaststunt/build-release/moo, then built the assigned Barn
head after verifying cmd/barn is main and ran that case. Then compared the whole
unchanged gap_followups_toast_oracle suite to check suite-local ordering.

```text
Toast binary case: 2 passed, 16 deselected, 1 warning in 11.87s
Barn binary case: 2 passed, 16 deselected, 1 warning in 11.27s
Toast complete gap suite: 18 passed, 1 warning in 36.45s
Barn complete gap suite: 18 passed, 1 warning in 35.89s
```

The warning is record_property/xunit2 compatibility, not a behavioral failure.
No tests were skipped in these comparisons. Artifacts are .tmp/toast-binary.xml,
.tmp/barn-binary.xml, .tmp/toast-gap-suite.xml and .tmp/barn-gap-suite.xml, each
with its separately named -admission.json evidence. The Barn binary is retained
at .tmp/barn-checkpoint-verifier (SHA256
e9613e163297700c64c81a4878f0e320d8ea0a0be8c20617dea24b84144a8130).

Commands use uv run --project the companion --frozen moo-conformance, the
explicit suite path, -m "admission or conformance", bundled Test.db/startup,
127.0.0.1, strict markers and fail-on-unexpected-skip. The binary-only selection
adds -k "capability_admission or audit_binary_mode_dispatches_raw_chunk_without_newline".
Toast uses the stock profile manifest for oracle and target; Barn uses the
outbound-on config and generates its target manifest through {manifest}.
Linux uv environment is /root/.cache/barn-system-layering-conformance-linux.

These results do not reproduce CI's failure and do not establish its cause or
clear the failed required check. Broader preceding-suite state, scheduling,
transport chunking and failure-environment differences remain unproved
possibilities. Source also refreshes player identity immediately before command
dispatch, so a stale input-event player is not established as the cause.
