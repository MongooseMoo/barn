# Binary input stream

## Current merge instruction

Goal: complete Q's new instruction to merge the Barn and MCT campaign PRs.
State: Q resumed this bounded delivery after the off-design handoff. Barn 648
is open with nine HTTP conformance failures; MCT 180 has one failed required
Toast profile. Both require repair/verification before normal merge.
Decisions and why: preserve the handoff and measured costs as evidence; the
new instruction authorizes completing these concrete published PRs, without
claiming performance parity or restarting the wider campaign.
Surprises: the CLI PR's required 64-bit outbound-on profile failed while its
other profiles and quality checks passed; cause is not yet known.
The real blocked-reader regression reproduced the missing disable wake and
subsequent statement syntax error; symmetric wake passed. Managed frozen Barn
HTTP selection reproduced all nine failures; the repaired TCP selection passed
26 cases with admission. Analyst then found that a generic wake cancels a
WebSocket read fatally. Whole-repo callers show this connection wake is used
only by the binary-option builtin; shutdown uses CloseNow. The revised bounded
repair restricts the existing wake to BinaryTransport, tests the actual
WebSocket setter, and preserves direct transport cancellation coverage.

MCT original failure remains unexplained. Its trusted lane does not execute
the changed CLI; candidate same-profile CI passed, and focused canonical move
cases passed. The failed required full job is being repeated as a recurrence
probe, not a claimed fix. Original artifacts are retained separately.

The final owner guard and real WebSocket 0/1 regressions are complete. Combined
TCP/WebSocket/direct cancellation selector passes (server 1.583s). Independent
delta analysis found no remaining actionable defect. The frozen final Linux
source, including corrected comments, passes managed HTTP/admission: 26 passed,
one deselected in 12.54 seconds. Lossless evidence JSON preserves ten raw
XML/admission captures plus all seventeen changed Go-source hashes and the
final executable identity. Existing benchmark costs remain unchanged claims.

Next step: publish the scoped repair, obtain required exact-head checks and
independent final verification, then normal merges and local integration.
Preserve unrelated dirty work and raw artifacts.

## Design-departure stop

Goal: preserve the correctness evidence and original no-regression constraint.
State: PR 648 is open and conformance is red (nine HTTP failures). Execution
stopped after root found that accepting measured slowdowns departed from the
campaign plan. The earlier parent disposition to retain this candidate despite
one-byte transport +13.96% and codec regressions is superseded by this notice.
No merge is authorized by that disposition.

Decisions and why: the written campaign requires an unavoidable correctness
slowdown conflict to go to Q before merge. Disclosure of costs is not acceptance.
Surprises: focused canonical Toast HTTP/admission passes 26 cases, whereas Barn
full CI fails nine. Source supports a possible reader-mode wake omission, but
no deterministic reproduction or causal proof has been obtained.

Next step: work is stopped under Q's off-design handoff instruction. The root
handoff is C:/Users/Q/code/barn-system-layering/agent-notes/refactor-system-layering.md.
It records source, departures, affected states and undo/redo decisions. Raw
profiles, binaries, XML and admission evidence remain preserved. No production
code changed during this stop.

## Goal

Preserve partial text input when binary mode starts, and encode new external
binary bytes at the MOO boundary using the existing shared encoder.

## Current state and disposition

Parent retains the correctness fix and original restored byte algorithm for
final review. ReadChunk keeps Peek(1), only Buffered() bytes, prefix-owned
lineBuf, encoding only new bytes, Discard, String/reset. Canonical string
producer and actual caller consolidation remain; no further speculative rewrite.
Source analyst found no actionable correctness defect.

Fresh parent-managed Linux Go 1.27.1 candidate passed the three canonical-proven
cases plus admission: 4 passed, 2 deselected in 10.31s. XML records 4 tests,
zero errors/failures/skips; admission schema 2 has four passed probes. The
12 captured source hashes still match. Raw identities/evidence remain in
.tmp/binary-native-source.json, .tmp/barn-binary-current.xml and
.tmp/barn-binary-current-admission.json. This is pre-default-integration evidence.

Follow-up generated-code comparison found normalized loop 43/43 and helper
193/193 instructions with no differences; append is already inlined. The extra
codec-call explanation is disproved. Frozen Windows codec larger-ASCII +20-21%,
mixed +5-6%, and one-byte transport +13.96% costs remain explicit/unexplained.
These are not cleared, neutral, or target-Linux performance claims. Allocations
remain unchanged. Full raw measurements/history and investigation are preserved.

The feature is committed and current default, including merged 647, is integrated.
Three HTTP fixture conflicts were resolved by retaining both the physical-ID
arguments and canonical codec name. Exact edited-file selections pass on the
combined source: builtins 1.445s and server 1.107s. Fresh integrated managed
selection again passes all three established cases plus admission (four tests,
10.16s), with four passed probes and thirteen unchanged captured Go hashes.
Required exact-head PR CI and final verification remain.

reports/system-layering-binary-input-evidence.json losslessly packages sixteen
raw text captures, four diagnostic profiles, both managed source/result summaries
and focused integration output. Decoded hashes match originals. Private raw
admission/XML and frozen binaries remain under .tmp; no performance-neutral or
all-caller performance claim is made.
Worker changed documents only for this handoff; no new code/Go/benchmark work.

## Historical trial sequence

The canonical managed suite passes three behavior cases plus admission. The
same selection on Barn fails all three cases: ASCII prefix loss, literal-tilde
prefix loss and missing binary escaping. Raw captures remain in the checkpoint
checkout, and the single research producer lives in the conformance checkout.
The current selected bulk revision is present across six production files,
with direct migrations in six existing test files and one new focused test file.
Focused regressions captured prefix/encoding failures before the changes and
now pass; initial EOF already preserved the prefix before either change.
Original and candidate printable-ASCII n10 samples and benchstat output are
captured in reports/system-layering-binary-ascii-{original,candidate,benchstat}.txt.
The first per-byte implementation was stopped for equivalent ASCII regression:
chunks: +12.78% at one byte, +33.78% at 1024 bytes, +32.39% at 4096 bytes
(p=0.000, n=10 each). Allocations and bytes per operation are unchanged.
The parent then selected the revised plan recorded in the coder report: Peek(1), then
Peek/Discard only Buffered() bytes; shared bulk run encoder with one predicate;
canonical consolidation of actual file/exec/HTTP/Unicode/crypto producers.
The parent requested consolidation for this proposal, including deletion of
encodeBinaryBytes/encodeBinaryStr aliases and duplicate loops. The parent has
selected that exact plan. The parent released an exclusive timing window.
Existing-correct codec n10 baseline was captured before codec revision and
the literal oracle passes on that version. The selected bulk implementation
and direct caller consolidation are now present. Six focused new-file tests pass.
Ten alternating AB/BA correct-codec benchmark pairs have been captured with
CPU affinity mask16 and GOMAXPROCS=1. STOP again: the selected bulk revision
regresses correct-codec special bytes +46.64% to +57.73% and larger mixed bytes
+35.73% to +38.21% (p=0.000 n=10). Larger ASCII improves about 80%, but that
does not clear the special/mixed regression. Full raw samples/statistics are in
reports/system-layering-binary-codec-interleaved-{original,candidate,benchstat}.txt.
The parent was notified and timing released. No further design change. The codec
baseline includes stopped v1 ReadChunk but is NOT a transport baseline. A
separate actual-original ReadChunk comparison remains unperformed because the
codec regression triggered the required stop first.
No alternate fast path added.

## Decisions and why

Use TCPTransport.lineBuf as the existing pending input owner. Encode only newly
read binary bytes, preserving already-cleaned text. Rename/export the current
shared bulk run encoder and migrate existing callers; delete duplicate owners.
Keep first-read errors before mutation and consume only already-buffered bytes.
Do not change CR/telnet flags or text limits: that separate CR observer did not
establish an oracle contract.

## Surprises

MOO binary strings contain encoded text, so returning raw binary bytes is a
separate correctness failure from dropping the old text prefix. Re-encoding the
whole combined input would corrupt a literal tilde retained from text mode.
The unchanged shared encoder body on the selected per-byte path produced a
material ASCII timing regression in this benchmark. This is a measurement,
not attribution to any particular compiler decision. Host frequency was not
controlled; samples were sequential batches with GOMAXPROCS=1.

## Historical third-trial stop

The parent selected exactly one controlled third-trial change: skip preceding
run Write when start==i and trailing run Write when start==len(data). Those
empty writes have no output purpose. The parent granted exclusive timing;
focused regressions pass and the separately named trial3 binary is frozen.
Ten balanced AB/BA codec pairs against the fixed existing-correct baseline are
captured. STOP: mixed1024 +29.39%, mixed4096 +31.39%, special1 +17.25%
(p=0.000 n=10 each). Larger ASCII improves about 74-76%, but mixed regression
persists. Complete raw/stats are in reports/system-layering-binary-codec-trial3-
{original,candidate,benchstat}.txt. Timing window released and parent notified.
No original transport comparator was built/measured because this stop came first.
Hypothesis only: removing output-free builder
calls may address some measured special/mixed overhead. No timing attribution
has been established. Existing byte predicate/format, transport, consolidation
and tests stay exact; all previous binaries and raw artifacts are preserved.
Focused tests pass and the separately named third-trial binary/raw evidence are
preserved. Source/diagnostic investigation is required before another code
change. No additional optimization, API, Grow, second scan or timing undertaken.
Publish only after parent review; full required CI and final verification remain.

## Selected correction after structured investigation

The investigation is complete in investigations/binary-codec-regression.md.
Generated code and narrow frozen-binary profiles support two introduced costs:
short printable slices use memmove/barrier work instead of direct byte stores,
and tilde goes through generic conversion/copy instead of the original literal.
Profile percentages do not establish exact causal shares of benchmark deltas.

Keep Peek(1), Buffered/Peek(n), shared prefix owner, Discard(n), String/reset and
all caller consolidation. Replace the run-copy loop with the original private
encodeByte body, unchanged including literal ~7E and WriteByte. The one
AppendRawToBinary loop calls that helper per byte; EncodeRawToBinary uses that
same append owner. encode_binary's real integer producer calls the private
byte helper directly, removing the temporary one-byte array. No exported byte
API, duplicate rules, forwarding old names, threshold, prepass or Grow policy.

Run the explicit new-file correctness cases, then compare the full existing
correct-codec matrix with fixed original/candidate binaries and ten balanced
same-affinity pairs. Record per-case costs honestly. Build the actual original
ReadChunk comparator as previously documented, restore selected source exactly,
and measure the ASCII transport matrix too. Do not infer transport parity from
codec results. The original comparator's new codec is unused by old ReadChunk;
record that source composition. Preserve all earlier failures/binaries/profiles.
No managed post-change conformance or publication until parent review.

Correction implemented after reading the full investigation and its selected
recommendation. Restored the original private encodeByte body unchanged and
used it from the sole AppendRawToBinary loop and actual integer producer.
Transport and canonical caller consolidation remain intact. No new tests or
benchmark changes. Parent granted exclusive timing. Focused correctness passes
and separately named restored-byte binary is frozen. Ten balanced codec pairs
show residual larger-ASCII cost +20-21% and larger-mixed cost +5-6%; allocations
unchanged. No further implementation changes. Actual-original ReadChunk was
temporarily restored, its comparator frozen, then selected source restored
exactly. Ten balanced original/current ASCII transport pairs are captured;
statistics show 1024/4096 no significant difference, but one-byte +13.96%
(71.88 to 81.92ns, p=0.000 n10), all allocations unchanged. Exclusive timing
released. Residual codec and tiny-chunk costs are reported; no further change.
Complete corrected-codec and transport matrices/hashes/compositions are in the
coder report and binary-codec-restored / binary-transport-restored raw reports.
Await parent review; no commit, publication or managed post-change run by worker.

## Final verifier handoff state

Goal: verify the published binary-input change and its actual callers, raw
evidence and required CI, without changing implementation or publishing.

State: source and analyst/investigation review found no actionable correctness
defect. All sixteen archived text captures and four profiles match their raw
files; all thirteen changed Go files match the integrated managed-build source
manifest. Both native managed XML/admission summaries match private raw files.
Build/unit CI passed. Isolation CI passed with six healthy true verdicts,
twenty negative-control false verdicts and nonempty witnesses; every recorded
healthy path is nonzero. Full conformance failed: nine HTTP cases, eight with
fixture setup syntax errors and one expected E_INVARG receiving E_PROPNF.
Raw XML, admission, JSONL and profiles are retained in the verifier's private
artifact directory. The verifier verdict is NO-MERGE pending repair and CI.

Decisions and why: preserve the bounded correctness fix and canonical owner.
Do not treat normalized instructions as proof of timing parity. Recorded Windows
codec and one-byte transport regressions remain explicit limitations.

Surprises: the pinned Elle CLI logs verdicts as a path followed by a boolean,
rather than an EDN map; raw records and native exit codes agree. The verifier
report holds detailed evidence and remains separate from implementation files.

Next step: parent diagnoses and repairs the required conformance failures,
then obtains required exact-head CI. Do not merge the current red head.
