# Binary input stream implementation

## Goal and state

Implement the selected shared pending-input builder correction and reuse the
existing binary byte encoder. Parent disposition: retain the restored original
private byte algorithm/shared append, bulk buffered transport correction and
canonical producer consolidation for final review. No further speculative codec
rewrite. Source analyst found no actionable correctness defect. Current scope
is six production files, six direct existing-test migrations and one new focused
server test file, plus notes/evidence. No publication by this worker.

Fresh parent-managed candidate on Linux Go 1.27.1 passed the same three
canonical-proven cases plus admission: 4 passed, 2 deselected in 10.31s.
Read XML records four tests, zero errors/failures/skips; admission schema 2
records four passed probes. Rechecked all 12 manifest source hashes: zero
mismatches. Identities and raw evidence stay in .tmp/binary-native-source.json,
.tmp/barn-binary-current.xml and .tmp/barn-binary-current-admission.json.
Cases cover partial ASCII prefix, literal-tilde prefix and external byte encoding.
This is current feature evidence before integration with the newer default.

Follow-up investigation establishes normalized decoded loop 43/43 and private
helper 193/193 instructions, no differences after documented relocation
normalization. The current append is already inlined: a proposed additional
codec-call mechanism is disproved. This does not establish binary/runtime
equivalence or clear the measured frozen Windows costs. Larger ASCII codec
+20-21%, larger mixed +5-6%, and one-byte transport +13.96% remain unexplained;
allocations unchanged. No neutral/global/Linux performance claim is made.

Parent will commit this feature, integrate current default including merged
#647, resolve changed HTTP test signatures preserving physical IDs and canonical
codec names, then repeat the managed selection on the integrated source.
Required exact-head CI/final verification remain pending. This final worker
update changes documents only; all raw trials, binaries and profiles are retained.

## Historical evidence and trial record

Read actual transport, encoder and production binary consumer. Added one focused
server test file reusing newTestTransport/fakeConn and tcpFuzzConn. Prefixes are
installed by real ReadInput returning an OOB IAC WILL command, not by assigning
private state manually. The equivalent ASCII benchmark resets the same reader
and consumes the actual ReadChunk result on both sides.

Explicit Windows/amd64 Go 1.27.1 focused RED:

```text
ReadChunk = "tbin", <nil>; want "auditbin"
ReadChunk = "tbin", <nil>; want "~tbin"
ReadChunk = "~\x00\r\n\xff", <nil>; want "~7E~00~0D~0A~FF"
concatenated chunks = "~\x00\r\n\xff", want "~~7E~00~0D~0A~FF"
TestTCPReadChunkRetainsPrefixOnInitialError PASS
FAIL github.com/MongooseMoo/barn/server 1.406s
```

Command selects only TestTCPReadChunkPreservesPendingText,
TestTCPReadChunkEncodesExternalBytes, TestTCPReadChunkRetainsPrefixOnInitialError
and TestTCPReadChunkIndependentReadBoundaries from the new file. No broad local
test run occurred.

## Next step

The parent must decide whether and how to revise the selected design. No code
expansion, alternate implementation, commit, push, PR or merge was attempted.

## Selected implementation and GREEN

ReadChunk appends newly read bytes through the existing byte encoder into
lineBuf, obtains its string and resets the pending builder after a successful
chunk. Initial read errors still return before builder mutation, and only
already-buffered bytes are consumed after the first read. Existing text prefix
bytes are not re-encoded. The existing encoder body is unchanged; it was renamed
and exported, and all three existing call sites now use that owner. No CR/telnet
flags, filtering, limits, queues, wrappers or codec copies were changed.

```text
go test ./server -run '^TestTCPReadChunk(PreservesPendingText|EncodesExternalBytes|RetainsPrefixOnInitialError|IndependentReadBoundaries)$' -count=1 -v
TestTCPReadChunkPreservesPendingText PASS
TestTCPReadChunkEncodesExternalBytes PASS
TestTCPReadChunkRetainsPrefixOnInitialError PASS
TestTCPReadChunkIndependentReadBoundaries PASS
ok github.com/MongooseMoo/barn/server 0.973s
```

## ASCII comparison and STOP

Both source versions ran exactly:

```text
$env:GOMAXPROCS='1'; go test ./server -run '^$' -bench '^BenchmarkTCPReadChunkASCII$' -benchmem -benchtime=300ms -count=10
```

The same benchmark file uses identical reader and bufio.Reader resets, consumes
the actual method output, and checks the equivalent printable-ASCII result.
No special-byte output from the incorrect original is used as a performance
baseline. Windows/amd64, Go 1.27.1, Ryzen 5950X. Sequential original/candidate
batches, no other campaign agent compiling or timing concurrently; host frequency
and external load were not controlled. These limits prevent a production-level
latency claim, but the measured common-path regression triggers the agreed stop.

```text
benchstat reports/system-layering-binary-ascii-original.txt reports/system-layering-binary-ascii-candidate.txt
                         original       candidate       delta
bytes=1                  58.77ns        66.28ns         +12.78% (p=0.000 n=10)
bytes=1024               6.222us        8.324us         +33.78% (p=0.000 n=10)
bytes=4096               23.41us        30.99us         +32.39% (p=0.000 n=10)
B/op                     8/3320/12536   8/3320/12536    unchanged
allocs/op                1/9/12         1/9/12          unchanged
```

Complete raw samples and comparison are adjacent in the three
system-layering-binary-ascii-*.txt files. This comparison establishes a timing
regression, not a compiler/root-cause explanation. No speculative bulk encoder
or other machinery has been added. The CR-only observer remains excluded and
unsettled. Required broader CI and managed post-change conformance remain
unperformed by this worker.

## Read-only revision proposed for parent selection

The parent selected and this worker implemented the exact written revision,
including canonical consolidation and deletion of duplicate producers. The
existing-correct codec baseline was captured first. Current state: STOPPED again
for significant special/mixed codec regression; see measured results below.

### Exact transport algorithm

Keep `func (t *TCPTransport) ReadChunk() (string, error)`.

1. Call `t.reader.Peek(1)`. On error return `"", err` immediately, before any
   lineBuf append/reset. This is the only operation allowed to fill the reader.
2. Set `n := t.reader.Buffered()` and obtain `data, err := t.reader.Peek(n)`.
   Return an error before append if present. n is positive after step 1 and
   bounded by the existing bufio buffer, so this Peek cannot fill it again.
3. Call `builtins.AppendRawToBinary(&t.lineBuf, data)` exactly once. Only new
   bytes are encoded; the old text prefix remains unchanged.
4. Call `t.reader.Discard(n)` and check its error. The installed Go bufio source
   guarantees success without an underlying read for n <= Buffered(). Do not
   loop or ask for another byte. The data view is consumed before Discard and
   is not retained after the call.
5. Obtain `chunk := t.lineBuf.String()`, reset lineBuf, return chunk and nil.

Installed primary source inspected: Go 1.27.1 src/bufio/bufio.go Peek at line
143, Discard at 177, Buffered at 336. Peek fills only when buffered length is
less than requested; Discard explicitly guarantees no underlying read within
Buffered(). No transport caller uses UnreadByte/UnreadRune. CR/telnet state,
filters, limits, text EOF behavior and scheduling remain untouched.

### Exact encoder signatures and algorithm

Canonical definitions live together in builtins/crypto.go:

```go
func AppendRawToBinary(result *strings.Builder, data []byte)
func EncodeRawToBinary(data []byte) string
```

AppendRawToBinary is the sole raw-byte classification/escaping loop. Track the
start of a printable run. Scan bytes by index; a byte needs escaping iff it is
below 32, above 126, or '~'. On an escape, Write the preceding data slice once,
WriteString(encodeByteHex(byte)), then advance the run start. After scanning,
Write the trailing slice once. A fully printable input therefore takes one
builder Write rather than one builder call per byte. No per-byte callback,
temporary encoded string, second raw buffer, cache, unsafe conversion or generic
dispatch. Existing encodeByteHex remains the single ~XX formatting owner;
~7E is its ordinary output for '~', so no separate tilde formatting rule.

EncodeRawToBinary has an existing real string-returning producer contract. Its
body creates a local builder, calls AppendRawToBinary and returns String().
It is not a retained alias under an old name: transport appends to an existing
builder while file/HTTP/Unicode/exec/crypto producers require a fresh string.
No new string-specialized API is needed. Existing encodeBinaryBytes and
encodeBinaryStr names/bodies are deleted, and all actual callers use the
canonical string-returning owner. AppendRawByteToBinary is deleted.

encode_binary string arguments call AppendRawToBinary(&result, []byte(v.Str())).
Its existing integer validation remains; each valid integer uses a local
`[1]byte{byte(v.Int())}` passed as a slice to the same bulk owner. Recursive list
and quota logic stay unchanged. This avoids a second byte predicate. The
base64_decode inline duplicate loop becomes EncodeRawToBinary(decoded).
encodeAllBinaryStr is intentionally retained: six hash/HMAC binary-output
producers require ALL bytes escaped, which is a different verified contract.

### Complete direct caller migration

- server/transport.go: ReadChunk uses append owner; existing telnet formatter
  remains on EncodeRawToBinary.
- builtins/crypto.go: base64_decode, encode_binary string/integer producers,
  random_bytes; delete byte helper and duplicate encodeBinaryStr.
- builtins/fileio.go: delete encodeBinaryBytes and move EncodeRawToBinary's
  definition to crypto.go; file_read, file_readline, file_readlines use the
  canonical string owner.
- builtins/system.go: exec stdout/stderr conversions change names only;
  existing CRLF normalization remains.
- builtins/network.go: folded header values, normal header values, completed
  chunked bodies, request bodies and response bodies change names only.
- builtins/unicode.go: encode_chars conversion changes names only; quota
  and target-encoding behavior remain.
- Existing tests/benchmarks referring to removed names migrate directly:
  fileio_pool_bench_test.go, fileio_readline_buffer_test.go,
  file_conversion_bench_test.go, pool_review_test.go,
  http_buffer_consumption_test.go. review_io_test.go has comments to update.

No forwarding definitions under deleted names. No unrelated producer behavior
or check changes. This expands the original production scope by exactly three
files (system.go, network.go, unicode.go) for mechanical owner migration.

### Regression and measurement plan after selection

Keep the four new transport regressions and ASCII benchmark unchanged. After
the codec baseline capture and API implementation, add a
focused test to the same owned new test file covering every byte through the
public canonical codec with a literal expected alphabet of escapes, mixed ASCII
runs, empty input, and a literal existing prefix through AppendRawToBinary.
The assertion must not copy the production predicate as its oracle.

Add BenchmarkRawToBinary in that same file, before modifying the existing
correct string codec: sizes 1/1024/4096; printable ASCII, repeated raw
`{'~',0,'\r','\n',255}`, and mixed `{'a','b',0,'c','~',255,'d','\n'}`.
Use literal encoded pattern expectations, actual EncodeRawToBinary output,
b.Loop, SetBytes and ReportAllocs. Construct expected output outside timing by
indexing literal per-pattern-byte encoded tokens for the selected raw length;
do not truncate encoded text at a raw-byte offset or invoke the production codec.
Record existing-correct codec samples before implementing the revision, then
record candidate samples with the identical benchmark. This comparison bounds
every migrated string producer for special/mixed bytes without comparing the
incorrect old raw ReadChunk output on those inputs.

For ASCII transport, compare revised ReadChunk against the original old
ReadChunk benchmark already captured (identical reader resets). Capture a fresh
original batch in a disposable baseline worktree if host/session conditions
have changed materially; copy no production codec or old method into tests.
No timing overlaps with other campaign workers. All comparisons use n>=10,
benchmem, GOMAXPROCS=1, identical benchtime and benchstat, recording CPU/Go/host
limits. Significant regression stops implementation for the parent again.
Do not add a bulk Grow policy or second scan speculatively; let these first
measurements establish whether the direct run-copy implementation meets the
selected requirement. Run only explicit tests/benches from the owned new file
locally; existing migrated tests require normal CI. Managed post-change MOO
cases plus admission remain parent-owned.

## Bulk revision implementation and second STOP

Implemented exactly the selected Peek/Buffered/Peek/append/Discard/String/reset
sequence and one run-copy encoder. Six production files changed; six existing
test files contain direct removed-name migrations. The new server test file
holds regression tests and both benchmarks. No old forwarding definitions remain
under encodeBinaryBytes, encodeBinaryStr or AppendRawByteToBinary; the Go-source
search returned no matches. encodeAllBinaryStr is unchanged except its comment.

Focused command selected only this new file's tests:

```text
go test ./server -run '^(TestTCPReadChunk(PreservesPendingText|EncodesExternalBytes|RetainsPrefixOnInitialError|IndependentReadBoundaries)|TestRawToBinaryLiteralOracle|TestAppendRawToBinaryPreservesPrefix)$' -count=1 -v
TestTCPReadChunkPreservesPendingText PASS
TestTCPReadChunkEncodesExternalBytes PASS
TestTCPReadChunkRetainsPrefixOnInitialError PASS
TestTCPReadChunkIndependentReadBoundaries PASS
TestRawToBinaryLiteralOracle PASS
TestAppendRawToBinaryPreservesPrefix PASS
ok github.com/MongooseMoo/barn/server 1.149s
```

Frozen benchmark binary source composition:

- `%TEMP%/barn-binary-codec-baseline.test.exe`: existing-correct old codec,
  stopped v1 per-byte ReadChunk, new literal codec test and both benchmark
  definitions before adding the new append-prefix test. Used ONLY for codec
  benchmarks, never for original transport performance claims. SHA256:
  `4CEB3D58C0F83E888BDEA34991749741DE334CFAAC826FA6BD58D06F060D37BA`.
- `%TEMP%/barn-binary-codec-candidate.test.exe`: selected bulk path and canonical
  producer consolidation, same benchmark definitions, plus append-prefix test.
  SHA256: `B91F60AB7E814F61B863180041AC80F606D2BD43046164285EFD8D8F5D5DF55C`.

Both built using `go test ./server -run '^$' -c -o <named-binary>`. Existing-
correct codec n10 samples were first captured before production edits in
system-layering-binary-codec-original.txt. After candidate compilation, the
decisive comparison uses ten alternating AB then BA pairs of frozen binaries,
each invoked with the same settings:

```text
$env:GOMAXPROCS='1'
$benchProcess = Get-Process -Id $PID
$benchProcess.ProcessorAffinity = 16
& <named-binary> '-test.run=^$' '-test.bench=^BenchmarkRawToBinary$' '-test.benchmem' '-test.benchtime=300ms' '-test.count=1'
```

Windows/amd64 Go 1.27.1 Ryzen 5950X; affinity mask16 (one logical processor),
exclusive campaign compilation/timing window. CPU frequency, thermal behavior
and unrelated host processes remain uncontrolled. Initial unquoted dotted test
flags were rejected before any benchmark; the corrected quoted command above
produced the samples. That rejected usage output is not benchmark evidence.

```text
benchstat reports/system-layering-binary-codec-interleaved-original.txt reports/system-layering-binary-codec-interleaved-candidate.txt
shape/size        old median    bulk median    delta (n=10)
ASCII/1           41.69ns        42.97ns        +3.07%  p=0.015
special/1         42.36ns        62.11ns       +46.64%  p=0.000
mixed/1           42.19ns        43.73ns        +3.65%  p=0.000
ASCII/1024         4.903us        0.937us      -80.88%  p=0.000
special/1024      20.55us        31.96us       +55.52%  p=0.000
mixed/1024        12.51us        16.98us       +35.73%  p=0.000
ASCII/4096        18.096us        3.618us      -80.01%  p=0.000
special/4096      85.84us       135.40us       +57.73%  p=0.000
mixed/4096        52.85us        73.04us       +38.21%  p=0.000
```

ASCII 1024 allocations improve 9 to 1 and bytes/op 3320 to 1024; ASCII 4096
allocations improve 12 to 1 and bytes/op 12536 to 4096. Special/mixed allocation
counts and bytes/op are unchanged. Complete raw samples and statistics are in
system-layering-binary-codec-interleaved-{original,candidate,benchstat}.txt.

STOP as selected: no optimization, alternate fast path or further timing after
this statistical result. Parent notified and timing window released. The planned
separate actual-original ReadChunk baseline was NOT built or measured: the
codec comparison triggered the stop first. No claim that the original ASCII
transport regression is resolved. Required CI and post-change managed MOO
verification remain unperformed. No commit, publication or merge.

## Third trial: skip output-free writes (measured, STOP)

The parent selected exactly two guards in AppendRawToBinary: write the
preceding run only when start<i and the trailing run only when start<len(data).
The observed all-special loop otherwise invokes Write on an empty slice once
per escape and again after the loop. These calls have no output purpose.
Hypothesis: removing them may reduce special/mixed overhead. This has not been
measured and is not an attribution of the earlier timing regression.

No other byte classification, formatting, transport, producer consolidation or
test change. No Grow, second scan, alternate API or additional optimization.
Both old frozen binaries and all raw artifacts remain intact. Code prepared;
no compilation or timing before the parent's grant. Then run focused tests and
freeze `%TEMP%/barn-binary-codec-empty-guard.test.exe`; compare this binary to
the same fixed existing-correct baseline with alternating AB/BA, affinity16,
GOMAXPROCS=1, 300ms, n10. A persistent significant regression stops the third
trial for source/diagnostic investigation, not another guessed optimization.

The parent granted exclusive compilation/timing. Exact focused test selector
from the bulk trial above was rerun, with all selected tests passing:

```text
TestTCPReadChunkPreservesPendingText PASS
TestTCPReadChunkEncodesExternalBytes PASS
TestTCPReadChunkRetainsPrefixOnInitialError PASS
TestTCPReadChunkIndependentReadBoundaries PASS
TestRawToBinaryLiteralOracle PASS
TestAppendRawToBinaryPreservesPrefix PASS
ok github.com/MongooseMoo/barn/server 1.054s
```

Frozen `%TEMP%/barn-binary-codec-empty-guard.test.exe` was built with the same
go-test -c command and contains exactly trial2 plus the two selected guards.
SHA256: `9C61F2FA579E7B17F71B30C99ACDCE7B4AA269DFA316242E6ED1A39BA2196B1A`.
Baseline binary is the same fixed existing-correct codec binary documented
above. Both were run for ten balanced AB/BA pairs, affinity mask16,
GOMAXPROCS=1, 300ms, count1 per process using the unchanged benchmark producer.
Same Go/CPU/host limits as trial2; no other campaign Go compilation or timing.

```text
benchstat reports/system-layering-binary-codec-trial3-original.txt reports/system-layering-binary-codec-trial3-candidate.txt
shape/size        old median    trial3 median    delta (n=10)
ASCII/1           42.20ns        42.74ns         ~       p=0.128
special/1         42.27ns        49.56ns       +17.25%   p=0.000
mixed/1           42.14ns        43.09ns        +2.24%   p=0.014
ASCII/1024         4.946us        1.181us      -76.13%   p=0.000
special/1024      20.39us        20.88us        +2.42%   p=0.011
mixed/1024        12.31us        15.93us       +29.39%   p=0.000
ASCII/4096        17.929us        4.644us      -74.10%   p=0.000
special/4096      84.83us        87.57us         ~       p=0.089
mixed/4096        51.68us        67.90us       +31.39%   p=0.000
```

ASCII allocation/byte improvements and unchanged special/mixed allocations
match trial2. Full raw samples/statistics are in
system-layering-binary-codec-trial3-{original,candidate,benchstat}.txt. The earlier
v2 artifacts and both frozen v2 binaries are preserved.

STOP: significant mixed and one-byte-special regressions persist. Parent
notified, exclusive timing window released. No further implementation guess or
timing. The required next step is source/diagnostic investigation before another
change. The actual-original transport comparator remains unbuilt/unmeasured,
and there is no claim the initial transport ASCII regression is resolved.

## Selected correction after structured investigation (prepared)

Read investigations/binary-codec-regression.md in full, including generated-code
observations, fixed-binary profile evidence, limitations and minimal correction.
Parent selected that recommendation. Restored the original private encodeByte
body unchanged: literal ~7E, encodeByteHex for nonprintables, WriteByte for
printable bytes. AppendRawToBinary is one loop over that owner; encode_binary's
actual single-integer producer calls it directly without a temporary array.
Canonical string producer/caller consolidation and bulk buffered transport stay
unchanged. No exported byte API, threshold, prepass or Grow. No test/benchmark
changes. Source prepared only while the option worker owns compilation/timing.

After grant, run explicit focused new-file tests, freeze correction under a
new name, then compare its full codec matrix to the same fixed existing-correct
baseline using balanced affinity n10. Report residual per-case costs. Build
actual-original ReadChunk comparator by temporary exact method restoration,
freeze its binary, restore selected method, and compare same-producer ASCII
transport with balanced affinity n10. The comparator's canonical codec is
unused by original ReadChunk; record source composition and all binary hashes.
Preserve all earlier raw evidence/profiles/binaries. No further speculative
change if substantial regressions remain; no publication.

## Parent current-default integration verification

Parent committed the selected source and integrated current default containing
646/647. Three HTTP fixture conflicts combine the already-owned physical ID with
the canonical codec name; their assertions and parsing logic stay unchanged.
The current-default-relative HTTP diff is exactly those three name changes.

Explicit edited-file integration output:

```text
go test ./builtins -run '^(TestHTTPBufferedMalformedAndFrontInsertion|TestHTTPAppendAndCancellationAfterConsumption|TestHTTPLargeAppendAfterConsumptionReleasesNewAllocation)$' -count=1 -v
ok github.com/MongooseMoo/barn/builtins 1.445s
go test ./server -run '^(TestTCPReadChunk(PreservesPendingText|EncodesExternalBytes|RetainsPrefixOnInitialError|IndependentReadBoundaries)|TestRawToBinaryLiteralOracle|TestAppendRawToBinaryPreservesPrefix)$' -count=1 -v
ok github.com/MongooseMoo/barn/server 1.107s
```

Parent verified the main package, built an isolated Linux Go 1.27.1 Barn and
repeated the same three canonical-proven behavior cases plus capability admission
through the documented managed harness. The separately named integrated binary,
source capture, XML and admission JSON remain in .tmp.

```text
4 passed, 2 deselected, 1 warning in 10.16s
XML: all four named cases PASS; no failures/errors/skips
admission: schema 2, all four probe statuses passed
captured current-default-relative Go hashes: 13 unchanged after execution
```

The warning is the harness's existing record_property/xunit2 warning. The CR-only
observer remains deselected and unsettled. Lossless publication archive
system-layering-binary-input-evidence.json contains sixteen raw text captures,
four diagnostic profiles, both managed source/result summaries and the full
focused integration output. Decoded UTF-8/base64 hashes match originals. Raw
private admissions/XML stay local; no secrets are copied into the public archive.
Required PR CI and final verifier remain outstanding. Timing limits and the
normalized-work comparison remain separate; no performance-parity claim follows.

## Investigation-earned correction: measured result and handoff

Parent granted exclusive compilation/timing. Only the selected private helper
restoration was implemented; predicate, literal tilde output and WriteByte body
match the original helper. No further optimization after the residual costs.

Focused same six-test selector returned:

```text
TestTCPReadChunkPreservesPendingText PASS
TestTCPReadChunkEncodesExternalBytes PASS
TestTCPReadChunkRetainsPrefixOnInitialError PASS
TestTCPReadChunkIndependentReadBoundaries PASS
TestRawToBinaryLiteralOracle PASS
TestAppendRawToBinaryPreservesPrefix PASS
ok github.com/MongooseMoo/barn/server 1.482s
```

Frozen `%TEMP%/barn-binary-codec-restored-byte.test.exe` contains the current
selected full source and unchanged benchmark/test producer. SHA256:
`ABFE5722E835894F99789E2BF8A430C9979FDB1CEDCCCE4CFA81FF77D64C3CB3`.
Same fixed old correct-codec baseline was used; its stopped v1 ReadChunk is
irrelevant to these codec-only measurements. Balanced AB/BA n10, affinity16,
GOMAXPROCS=1, 300ms, exact frozen binary commands as earlier trials. Go/CPU/host
limits unchanged; no other campaign Go job during this exclusive window.

```text
benchstat reports/system-layering-binary-codec-restored-original.txt reports/system-layering-binary-codec-restored-candidate.txt
shape/size        old median    corrected median    delta (n=10)
ASCII/1           42.33ns        41.37ns              ~       p=0.280
special/1         42.45ns        41.13ns            -3.10%    p=0.009
mixed/1           42.00ns        41.60ns              ~       p=0.280
ASCII/1024         4.883us        5.929us          +21.42%    p=0.000
special/1024      20.54us        21.04us              ~       p=0.052
mixed/1024        12.46us        13.22us            +6.12%    p=0.000
ASCII/4096        18.28us        21.99us           +20.31%    p=0.000
special/4096      85.96us        88.73us            +3.23%    p=0.015
mixed/4096        52.68us        55.28us            +4.93%    p=0.000
```

All allocation counts and bytes/op match baseline. The large-ASCII codec gains
from the rejected run-copy trials are intentionally absent; residual larger-
ASCII codec regression remains measured and is not claimed resolved. Restoring
the helper substantially reduced special/mixed regression but did not establish
parity across all cases. No further speculative change followed.

As explicitly requested, then froze an ACTUAL ORIGINAL transport comparator:
temporarily restored ReadChunk byte-for-byte from HEAD's original implementation,
including separate local builder and original ReadByte/Buffered loop. During
that temporary source state `git diff --exit-code -- server/transport.go`
emitted no diff, and the restored method was read directly. Built
`%TEMP%/barn-binary-transport-original.test.exe` with the same go-test -c command.
SHA256: `4DAE934C1560BAE54927FD3DE590284FA114022DC70C3A98D5C0904766A1320C`.
Other source, including canonical corrected codec and tests, remained current;
that codec is UNUSED by original ReadChunk. No stale per-byte-v1 transport was
used as an original baseline.

Immediately restored the selected Peek(1)/Buffered/Peek/append/Discard/String/
reset method by apply_patch and inspected it before transport measurement.
The already frozen restored-byte binary is the transport candidate, so both
source compositions differ only in actual ReadChunk implementation for this
comparison. No correctness tests were run against the deliberately incorrect
original comparator. The final source contains the selected corrected method.

Used the existing unchanged BenchmarkTCPReadChunkASCII producer, identical
reader resets/output checks, ten balanced AB/BA pairs, affinity16,
GOMAXPROCS=1, 300ms. Only the benchmark selector changed from codec to
`-test.bench=^BenchmarkTCPReadChunkASCII$`.

```text
benchstat reports/system-layering-binary-transport-restored-original.txt reports/system-layering-binary-transport-restored-candidate.txt
size      original median    corrected median    delta (n=10)
1         71.88ns            81.92ns             +13.96%   p=0.000
1024       6.450us            6.471us                ~      p=0.494
4096      24.22us            23.92us                 ~      p=0.123
```

All allocations/bytes per operation unchanged. Large-ASCII transport samples
show no significant difference; the one-byte transport case remains about
10ns slower and significant despite wider sample dispersion. No blanket
performance-parity claim is supported. No special-byte transport timing uses
the incorrect old raw output as a baseline.

All raw samples/statistics saved under system-layering-binary-codec-restored-
{original,candidate,benchstat}.txt and system-layering-binary-transport-restored-
{original,candidate,benchstat}.txt. Earlier binaries/raw/profiles preserved.
Timing window released. Parent notified of residual per-case costs; no further
code change, broad local test, managed conformance, commit or publication.
Await parent selection/review. Required wider CI and managed MOO verification
remain outstanding.
