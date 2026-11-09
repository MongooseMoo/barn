# Binary dispatch CI failure investigation

## Goal and state

Diagnose the required PR 641 conformance failure without retrying CI or changing
production code speculatively. The failed assertion expected one empty argument
string in audit_binary_seen and observed an empty list. Earlier focused and
suite-only managed comparisons passed against canonical Toast and the PR binary;
they do not establish the failed run's cause.

The advertised legacy investigation role file is absent. Locate and read the
installed investigation protocol before choosing experiments. Existing verifier,
analyst and failure artifacts are preserved; this investigator owns only this
report and a bounded checkpoint in the phase note.

## Competing hypotheses

1. The secondary connection was not logged in when the primary enabled binary
   mode; the setter's result is not asserted by the fixture.
2. A mode-transition/wakeup interleaving leaves an old text read active or loses
   partial input.
3. The seven-byte TCP send is split into multiple binary command chunks.
4. The binary command is eligible but has not completed within the fixed wait.
5. Earlier suites leave global state affecting login, dispatch or scheduling.

These are hypotheses, not findings. Source and raw evidence will eliminate or
support each before any repair or recovery decision.

## Evidence read

Read the retained final-verifier report and failed CI log. The failure was one
case among 13457 passing and 316 skipped tests; CI used the whole managed suite
against the PR binary. Existing narrow comparisons are recorded by their owner.

## Source and observation findings

The actual fixture's payload is eight bytes, auditbin (617564697462696e), not
seven. Its setup and mode-setting run steps have no expect. runner.py:803-806
checks the result only when step.expect exists, so an error in either step is
not reported there. The secondary transport sends CRLF for login, uses a quiet
read timeout, and the fixture adds a fixed 500 ms sleep. It never asserts login
completion or the binary flag before sending the payload.

The failed case took 1.952 seconds. Existing passing focused/suite captures take
2.025 and 1.980 seconds on Barn, and 2.017 and 1.971 seconds on Toast. Those
similar durations do not establish either scheduler delay or its absence.
The failure was test index 251, after the seven preceding telnet cases in the
same suite. Earlier chronological server logs are not retained by the rotated
failure artifact, so no auditbin receipt/dispatch observation survives.

Read installed investigation/SKILL.md; the legacy role file is absent. Read
primary [Go deadline documentation](https://pkg.go.dev/net#Conn) and
[RFC 9293 section 3.7](https://www.rfc-editor.org/rfc/rfc9293.html#section-3.7).
A deadline affects pending and future reads, and TCP preserves an ordered byte
stream without a correspondence between application write/read boundaries.
No existing incident report was treated as proof of the failed run.

Barn's setter does store binary mode and then wake the reader. The reader clears
the deadline before selecting the current mode; a simple missing-wakeup claim
is false. Source alone has not established a lost-wakeup interleaving.
Canonical Toast network.cc:547 appends the next binary read to its existing
partial input stream, dispatches it, then resets the stream. Barn's ReadChunk
does not read its separate lineBuf. That is a separate behavior to verify,
not an explanation established for this CI payload.

## Experiments

The parent authorized a temporary diagnostic YAML under .tmp. It asserts setup
return 1, connected-player membership and binary mode before sending the same
payload, then polls a test-owned completion property with a bounded deadline.
It does not change Barn or retry CI. The first managed invocation incorrectly
passed a YAML path outside _tests, which the CLI forwarded to pytest rather
than selecting a packaged suite; collection failed before server startup.
After reading cli.py and the supported --candidate-root/--moo-suite-root options,
the corrected documented managed invocation selected that YAML and admission.

```text
Toast observed producer: 2 passed, 1 warning in 10.71s
Barn observed producer: 2 passed, 1 warning in 11.78s
```

Artifacts: .tmp/toast-binary-observed.xml and .tmp/barn-binary-observed.xml,
with separately named admission JSON. These passing runs demonstrate that the
additional observations work; they do not establish the original failure cause.

The controlled experiment splits the eight-byte producer into two sends separated by
the secondary transport's normal quiet read. It tests the fixture's assumption
that the native verb name arrives in one binary input chunk. A failure on Toast
would establish that this assertion is not invariant under valid segmentation,
not prove segmentation actually occurred in CI.

## Controlled split result and repair proposal

The controlled-split diagnostic verifies setup, logged-in membership and binary
mode, then sends audi and tbin separately. The transport's ordinary quiet read
separates those sends. Canonical Toast produced the same empty seen list:

```text
expected [''], actual []
1 failed, 1 passed, 1 warning in 11.78s
```

Preserved producer: .tmp/binary-dispatch-split-native.yaml. Raw oracle XML:
.tmp/toast-binary-split.xml and its admission JSON. This establishes a defect
in the test's native-verb-name assumption under a valid TCP producer. No packet
capture survives from the failed CI run, so its actual packetization is unknown.

A replacement observation uses a test-player-filtered do_command hook, saves
and restores that verb's metadata, code and existence, and concatenates the
delivered argstr chunks. The exact expected content remains auditbin without
CR/LF, so it checks binary dispatch while allowing legal read boundaries.
The diagnostic includes strong setup/mode expectations and bounded completion
observation. It does not add a new runtime layer or change Barn.

```text
Toast controlled-split raw-byte observation: 2 passed, 1 warning in 10.69s
Barn controlled-split raw-byte observation: 2 passed, 1 warning in 10.46s
```

The producer is .tmp/binary-dispatch-chunks.yaml; evidence is
.tmp/toast-binary-chunks.xml and .tmp/barn-binary-chunks.xml with separately
named admission JSON. The primary
eval connection is excluded by the hook's explicit player comparison.

Smallest durable repair: update the existing binary-mode conformance case in
gap_followups_toast_oracle.yaml to observe raw command bytes through this existing
hook, preserve the exact original eight-byte unsplit send, assert setup and
mode, and wait on the test-owned value. Preserve the split negative witness.
Re-run its canonical managed oracle and Barn comparison, then new PR CI on the
actual changed source. Do not label the original run a flake or claim its hidden
transport history was established.

## Next step

The comparison and diagnosis handoff are complete. Coordinator owns the durable
fixture repair and new exact-head CI. No production code, published test, CI
state, review or merge was changed by this investigator. Separate partial-text
transition behavior remains source-only and must be oracle-verified before any
claim of a Barn defect or implementation change.
