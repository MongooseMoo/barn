# Binary codec regression investigation

## Goal and state

Explain the third trial's correct-codec mixed and one-byte-special regressions
before selecting another implementation. Investigation only: production source
and frozen binaries must remain unchanged. Parent granted an exclusive diagnostic
window; only the new explicit BenchmarkRawToBinary producer may be profiled.

## Facts

- The coder records passing focused correctness tests and controlled n10 trial3
  medians: mixed/1024 12.31 to 15.93 microseconds; mixed/4096 51.68 to 67.90;
  special/1 42.27 to 49.56 nanoseconds. These are coder evidence, not yet an
  independently reproduced diagnostic attribution.
- Old and candidate codec binaries are recorded under the system TEMP directory,
  not this checkout's .tmp. Their exact paths and hashes must be checked before
  any diagnostic execution. The original transport comparator was never built.
- The advertised investigator role file is absent; the discovered investigator
  TOML points at the same absent file. The parent-supplied investigation protocol
  and task constraints govern this work; no extra role instructions were read.

## Competing theories

1. Short printable-run Builder.Write calls cost more than the original per-byte
   WriteByte path. Predict additional copy/memmove work for mixed byte patterns,
   while long printable runs benefit.
2. Tilde escaping lost the old literal ~7E path and now uses general byte-to-hex
   formatting. Predict different formatting work even for one tilde byte.
3. Inlining, call overhead or binary layout explains some or all differences.
   Predict generated-code differences outside the run-copy/tilde operations.

## Tests and observations

Frozen hashes match the coder report: baseline 4CEB3D58C0F83E888BDEA34991749741DE334CFAAC826FA6BD58D06F060D37BA;
trial3 9C61F2FA579E7B17F71B30C99ACDCE7B4AA269DFA316242E6ED1A39BA2196B1A.
Both are under C:/Users/Q/AppData/Local/Temp. No binary was built or modified.

Generated-code inspection with go tool objdump establishes:

- Baseline encodeBinaryBytes calls AppendRawByteToBinary per byte. Its tilde
  branch emits literal ~7E with MOVW/MOVB stores and no generic string conversion.
- Trial3 AppendRawToBinary emits runtime.slicebytetostring followed by a
  runtime.memmove for every escaped byte, including tilde.
- Baseline printable branch appends via a direct MOVB instruction. Trial3's
  preceding printable-run Write emits runtime.memmove even when the run has
  only one/two bytes. Mixed benchmark repeats ab, escape, c, escape, escape, d,
  escape; there are three short printable-run copies per eight input bytes.
- The installed Go 1.27.1 strings.Builder source confirms Write appends a slice;
  WriteByte appends one byte. Those generated paths differ as observed.

These are source/generated-code facts, not measured attribution percentages.
They support theories 1/2 and make an entirely layout-only explanation less
plausible, but do not yet isolate each contribution.

One broad TEMP inventory search encountered unrelated inaccessible directories
and was interrupted before diagnostics. Recorded literal binary names were
then resolved directly with Test-Path. No output from that search is evidence.

## Next action

The predicted focused profile test is complete. No production change is selected
or implemented by this investigator. Parent decides the next coding design.

## Focused diagnostic test and raw results

Prediction before execution: trial3 mixed samples should expose extra short-copy
work, and special/1 (exactly tilde) should expose generic formatting work absent
in baseline. Four sequential processes used the matching fixed binary,
GOMAXPROCS=1 and Windows processor affinity mask 16. Flags:

```text
-test.run=^$
-test.bench=^BenchmarkRawToBinary/mixed/bytes=1024$
-test.benchmem -test.benchtime=2s -test.count=1
-test.cpuprofile=<owned investigation profile>
```

The other selector was ^BenchmarkRawToBinary/special/bytes=1$. Every process
returned PASS/exit zero. Instrumented stdout (not a new uninstrumented verdict):

```text
old mixed/1024:    193603 iterations, 12818 ns/op, 5368 B/op, 10 allocs/op
trial3 mixed/1024: 142945 iterations, 16533 ns/op, 5368 B/op, 10 allocs/op
old special/1:     63752978 iterations, 43.54 ns/op, 8 B/op, 1 alloc/op
trial3 special/1:  52050800 iterations, 50.14 ns/op, 8 B/op, 1 alloc/op
```

Raw profiles are binary-codec-old-mixed.cpu, binary-codec-trial3-mixed.cpu,
binary-codec-old-special1.cpu and binary-codec-trial3-special1.cpu beside this
file. No profile was overwritten. go tool pprof -top -nodecount=16 with each
matching frozen executable returned:

```text
old mixed:    total CPU samples 2430ms; memmove 330ms flat (13.58%);
              Builder.WriteByte 360ms cumulative (14.81%);
              encodeByteHex 500ms cumulative (20.58%).
trial3 mixed: total CPU samples 2300ms; memmove 430ms flat (18.70%);
              Builder.Write 860ms cumulative (37.39%);
              encodeByteHex 550ms cumulative (23.91%).
old tilde:    total CPU samples 2710ms; no encodeByteHex or slicebytetostring
              entry among the shown top 16; disassembly proves tilde bypasses
              both functions, independently of sampling.
trial3 tilde: total CPU samples 2560ms; encodeByteHex 400ms cumulative (15.62%);
              slicebytetostring 340ms cumulative (13.28%);
              memmove 210ms flat (8.20%).
```

Different iteration counts and GC work prevent subtracting these percentages
to claim an exact causal share of the uninstrumented slowdown. The profiles
support the predicted mechanisms, not a machine-independent cost model.

## Generated-code mechanism

The old capacity-available printable WriteByte branch stores the new length
and one byte directly:

```text
builder.go:97  MOVQ DX, 0x10(AX)
builder.go:97  MOVB BL, -0x1(DX)(R8*1)
```

The new short slice Write also rewrites buffer pointer/capacity and has
GC-write-barrier paths plus runtime.memmove. These operations occur even when
the slice has one or two bytes. The old tilde branch directly stores its
constant output:

```text
builder.go:114 MOVW $0x377e, 0(R8)(BX*1)
builder.go:114 MOVB $0x45, 0x2(R8)(BX*1)
```

The candidate instead constructs ~XX through hex digit lookup and invokes
runtime.slicebytetostring then runtime.memmove. Thus a wholly layout-only theory
does not explain away the observed added work. Inlining/layout may affect the
remaining point estimates; these probes do not isolate every cycle or GC phase.

## Smallest evidence-earned recommendation

Retain the bulk buffered transport read and canonical caller consolidation,
but restore the original byte encoding body as a private helper. Have the
single AppendRawToBinary loop call that helper for each input byte, preserving
its literal ~7E and direct WriteByte paths. EncodeRawToBinary uses that same
append owner. Actual single-integer encode_binary producers can call the
private byte helper directly. No copied encoding rule or old-name facade.

This removes the two demonstrated new costs using existing behavior instead
of a new short-run threshold, second scan, Grow policy or extra API. It gives
up the trial's large-ASCII codec allocation/timing gains; those were optional
optimizations, while prefix preservation and external binary encoding remain
required corrections. Confidence is high that the added short-copy/tilde work
is removed; performance parity still requires the existing full codec matrix.

Then measure actual original ReadChunk against the corrected bulk-reader path
under the same benchmark and fixed-binary pairing. Removing per-byte ReadByte
can compensate for encoding work, but that is a prediction, not yet measured.
Do not call the original transport regression resolved without that comparison.

A hybrid long-ASCII fast path could retain the codec win, but it needs a new
classification/prepass/threshold policy and more run-length cases. It is not
the minimum correction supported here. No implementation, commit, publication,
new executable, broad test or managed conformance run was performed.

## Follow-up: residual restored-helper cost

Parent resumed this investigation after the coder implemented the recommended
original byte helper behind the shared append loop. Coder's new controlled
results still show ASCII/1024 +21.42%, ASCII/4096 +20.31%, mixed/1024 +6.12%,
mixed/4096 +4.93%. Actual-original transport comparison now exists: one-byte
input +13.96%, with no detected larger-size difference. These are recorded
measurements, not yet an independently established explanation.

Latest frozen codec/transport candidate is
%TEMP%/barn-binary-codec-restored-byte.test.exe, recorded hash
ABFE5722E835894F99789E2BF8A430C9979FDB1CEDCCCE4CFA81FF77D64C3CB3.
Actual-original transport comparator is
%TEMP%/barn-binary-transport-original.test.exe, recorded hash
4DAE934C1560BAE54927FD3DE590284FA114022DC70C3A98D5C0904766A1320C.

The original helper body is now present in current crypto.go. The earlier
short-run/generic-tilde explanation therefore no longer suffices for the
remaining regression. Competing hypotheses for this phase:

1. Moving the loop into a caller-owned Builder append function changes alias,
   register, pointer/capacity or inlining work despite the same byte helper.
2. The original local-builder loop and current wrapper/append pipeline have
   different call boundaries or per-iteration spills/reloads.
3. Binary layout or measurement variability explains residual timing without
   an added repeated operation. Inspect generated work before attributing cost.

Next probe prediction: compare frozen old correct codec and restored-helper
candidate symbols/calls, then inspect loop instructions and Builder escape
behavior. If a repeated added operation exists, it should be visible before
profiling. No production edits or further uninstrumented matrix is authorized.

### Follow-up generated-code results

All three recorded binary hashes match. No binary, production source or benchmark
was built or modified in this follow-up. go tool objdump shows the compiler has
already inlined AppendRawToBinary into EncodeRawToBinary. The latter owns a local
stack Builder in generated code, with the same 0x38-byte frame and loop register
pattern as baseline encodeBinaryBytes. BenchmarkRawToBinary.func1 makes one
outer codec call in both binaries: baseline calls encodeBinaryBytes after its
thin public wrapper is inlined; restored candidate calls EncodeRawToBinary.

Read-only normalized instruction comparison produced:

```text
Loop instruction counts: old=43 new=43
Byte instruction counts: old=193 new=193
Compare-Object -SyncWindow 0: no differences for either pair
```

Normalization removed source annotations/absolute instruction addresses and
INT3 padding; converted numeric jump targets to function-relative targets;
canonicalized the corresponding old/new loop and helper symbol names; and
canonicalized type-symbol relocation offsets. Register operands, stack offsets,
constant immediates, call targets and instruction order were retained. This is
decoded-work equivalence after relocation normalization, not identical binary
bytes, addresses, runtime behavior or a performance-parity verdict.

Consequences for competing hypotheses:

- The proposed extra caller-Builder alias/register/call work does not exist in
  the current compiled STRING codec. There is no remaining outer append call,
  extra per-byte operation, spill or larger frame in these compared functions.
- The original byte helper's literal tilde and direct printable byte paths are
  restored in generated code, not merely similar source.
- Current measured +20% ASCII codec timing remains a fact about the paired
  frozen Windows test binaries. This comparison does not explain it away or
  establish its cause. Callee code placement, caller/runtime/GC work and machine
  effects remain hypotheses. No further uninstrumented matrix or CPU profile
  was run during this follow-up.

### Primary evidence about the remaining hypotheses

Read primary Go discussions before considering layout as an explanation:

- [Go issue 60071](https://github.com/golang/go/issues/60071): a contributor
  reports AMD Threadripper Windows/amd64 append benchmark shifts across commits
  and suspects alignment; this is a relevant incident, not a proven cause for
  Barn or this processor/toolchain.
- [Go issue 80046](https://github.com/golang/go/issues/80046): Go maintainers
  discuss an arm64 regression affected by benchmark link randomization and
  explain package locality versus alignment-sensitive instability. It confirms
  that fixed machine code's placement can matter in Go benchmarks, while the
  architecture and experiment differ from this task.
- [Go issue 63678](https://github.com/golang/go/issues/63678) documents amd64
  instruction alignment work and cautions that alignment changes can also hurt
  routines. It provides a plausible mechanism, not a Barn diagnosis.

No padding, linker randomization, architecture flags, runtime setting or new
source optimization was tried. It would be unsupported to label the measured
codec delta as noise or alignment merely because normalized instructions match.

### Transport difference and candidate disposition

Actual-original ReadChunk disassembly has an initial Reader.ReadByte call,
then an inlined local-Builder WriteByte path. Current corrected ReadChunk has
two Reader.Peek calls, an inlined append loop that calls encodeByte per byte,
Reader.Discard, and member-Builder Reset with a GC-write-barrier path. Thus the
one-byte transport comparison does have additional operations compatible with
its measured overhead, unlike the compared string codec loop/helper. Exact
per-operation causal cost remains unmeasured.

Parent's candidate of removing AppendRawToBinary and retaining an original local
loop in EncodeRawToBinary would simplify the API and allow ReadChunk to return
the newly encoded string directly when no text prefix exists. With a prefix,
it would allocate the encoded chunk then append it to lineBuf, introducing an
extra temporary/string copy on that rare transition path. It must preserve
first-read error behavior and encode only new bytes. This can be an ownership
choice, but it is NOT an earned fix for the residual string-codec timing:
the compiler already emits that local-builder loop. It may trade the member
Builder reset/write work for another codec call on the common transport path;
no result here establishes which is faster. Do not select it as a performance
repair based on the disproved append-call hypothesis.

The supported disposition is to retain the original restored encoding algorithm
and avoid another speculative implementation. Report exact generated-work
equivalence, unchanged allocations and the unresolved frozen-binary timing
deltas separately. No global or target-Linux performance claim follows from
these Windows binaries. If performance parity is a required release condition,
it remains unverified; it cannot be earned by changing source on a false model.

Follow-up diagnostic window released. No production edit, executable build,
additional benchmark matrix or publication was performed.
