# Connection output lock coder report

## Scope and selected plan

Worktree: C:/Users/Q/code/barn-connection-output-lock.
Branch: fix/connection-output-lock.
Selected plan: campaign reports/system-layering-connection-lock-scout.md.

Move only SendNotification, Buffer, Flush and BufferedOutputLength to outputMu;
preserve metadata mutex, Close interruption, direct Send, diagnostics, FIFO and
accounting. No asynchronous writer or effect boundary changes.

## Evidence

Initial git status --short produced no entries. Source/test inspection and
RED/GREEN/REFACTOR evidence will be appended here as work proceeds.

## RED

Command: go test ./server -run '^(TestSwitchPlayerInterruptsBlockedNotification|TestConnectionMetadataProgressesDuringBlockedNotification)$' -count=1

```
--- FAIL: TestSwitchPlayerInterruptsBlockedNotification (3.00s)
    connection_output_lock_test.go:42: shutdown did not finish replacement while old output is blocked within the overall bound
--- FAIL: TestConnectionMetadataProgressesDuringBlockedNotification (3.00s)
    connection_output_lock_test.go:78: shutdown did not finish connection metadata during output within the overall bound
FAIL github.com/MongooseMoo/barn/server 7.076s
```

Both tests use actual TCPTransport.WriteOutput through existing shutdown helpers,
wait for physical write entry, and register independent close/join cleanup.
The source-field audit confirms no production output queue accesses outside
connection.go; existing connection_buffer_test.go accesses are sequential.
Parent granted exclusive timing window for before/after n10 captures.

## GREEN and refactor

The four-method outputMu change passed both new regressions. Before layout
refactor, actual unsafe.Sizeof(Connection{}) changed 328 to 336 bytes:

```
--- PASS: TestSwitchPlayerInterruptsBlockedNotification (0.00s)
--- PASS: TestConnectionMetadataProgressesDuringBlockedNotification (0.00s)
Connection struct bytes: 336
ok github.com/MongooseMoo/barn/server 1.023s
```

Refactor groups loggedIn with existing listener/print/outbound bools, removing
the newly introduced padding footprint without changing any field semantics.
Baseline size test output was 328 bytes; baseline n10 raw samples are preserved
in system-layering-connection-output-lock-before.txt. Constructor samples retain
448 B/op and 3 allocs/op. Baseline timing batches were sequential, CPU count one,
and host frequency was not controlled; no performance conclusion yet.

## Final selected implementation and timing stop

Production diff is exactly the four output-method lock substitutions, one
outputMu, its flushLocked ownership comment, and loggedIn regrouping. Direct
Send/command response, Close, all write/accounting/diagnostic code and task
effect boundaries are unchanged. No method takes both connection mutexes.

```
--- PASS: TestSwitchPlayerInterruptsBlockedNotification (0.00s)
--- PASS: TestConnectionMetadataProgressesDuringBlockedNotification (0.00s)
Connection struct bytes: 328
ok github.com/MongooseMoo/barn/server 1.026s
```

Command: go test ./server -run '^$' -bench '^BenchmarkConnectionOutputLock$' -benchmem -benchtime=300ms -count=10 -cpu=1

Raw reports: system-layering-connection-output-lock-before.txt,
system-layering-connection-output-lock-after.txt, and
system-layering-connection-output-lock-benchstat.txt. After raw capture also
contains preceding focused GREEN output. Benchstat parses benchmark lines.

```
Send          2.576n -> 2.817n +9.38% (p=0.000 n=10)
Notification  23.58n -> 24.02n +1.84% (p=0.000 n=10)
BufferFlush   30.87n -> 29.99n -2.83% (p=0.000 n=10)
NoFlushFlush  36.62n -> 36.37n -0.68% (p=0.012 n=10)
NewClose      315.0n -> 308.1n -2.16% (p=0.007 n=10)
```

All output cases remain 0 B/0 allocations; constructor remains 448 B/3
allocations. Actual struct size is unchanged at 328 bytes. The unchanged Send
control moved materially, and constructor batches have 10-23% intervals, so
these sequential host-unpinned samples do not establish regression attribution,
constructor speedup or performance neutrality. The selected stop rule applies
to the measured notification increase. Parent was informed and timing window
released; no alternate implementation or rerun attempted.

Source audit: output fields have no production accesses outside connection.go.
Metadata is independently serialized; direct output bypasses already existed.
Existing partial-failure/accounting tests are untouched and have not been run
locally, per the explicit-new/edited-file restriction. Required CI and analyst/
verifier work remain with the parent. No commit, push or PR was made.

## Selected follow-up measurement proposal

Parent selected interleaved, affinity-controlled exact base/candidate comparison.
No saved benchmark executable was created by this worker: go test used its
ephemeral executable. No further compile/timing has run after that selection.

After the next timing/build grant:

1. Parent creates a detached baseline worktree at this task's actual base. Add
   the same single focused test/benchmark producer through apply_patch. Production
   code stays the exact base; do not copy any production implementation.
2. Build each server test executable once with go test -c, preserving binaries
   under this task's .tmp. Record actual base/head source identities, production
   diff, benchmark source hash, go version, binary hashes, and build commands in
   an untracked artifact manifest rather than task notes.
3. Run those fixed binaries with identical -test.run=^$, explicit
   BenchmarkConnectionOutputLock selector, -test.benchmem, -test.benchtime=300ms,
   -test.count=1 and -test.cpu=1. Pin both to the same available Windows processor
   affinity using cmd start /b /wait /affinity, capturing raw stdout through the
   tool rather than shell redirection. Confirm both exits before advancing.
4. Use ten pairs with balanced alternating order: AB, BA, AB, BA, AB, BA, AB, BA,
   AB, BA. Save each raw output and actual order through apply_patch. No other
   benchmark or compiler jobs during captures. Parent coordinates the window.
5. Benchstat base/candidate samples per case; retain the unchanged Send control
   and all constructor samples. A moving control or remaining significant
   notification increase remains unresolved and is reported without a neutral
   claim or source redesign. Existing initial samples remain preserved.

Required analyst/verifier and PR CI are still outstanding. Worker is stopping
now to release its slot, with exact implementation preserved and no publication.

## Detached baseline preparation

Parent created C:/Users/Q/code/barn-output-lock-base. Both actual HEAD identities
match; the baseline production connection.go diff is empty. Only the focused
test/benchmark producer is untracked there. It was added through apply_patch;
no production code was copied. Exact source hash comparison:

```
candidate benchmark SHA256 32D032C1CAAD9E740145A0045DFC46471026AA184AD16A57969AB6F1626956B5
baseline benchmark SHA256  32D032C1CAAD9E740145A0045DFC46471026AA184AD16A57969AB6F1626956B5
git diff --no-index benchmark producers: exit 0
git diff --exit-code baseline connection.go: exit 0
```

The .tmp/output-lock-benchmark-manifest.json artifact records actual base and
candidate parent, both production source hashes, benchmark hash, planned build
commands, arguments and balanced pair order. Toolchain/binaries/actual order are
unset until the grant. No new build, test or benchmark was executed. Analyst
source report was read and records no actionable defect, with timing/CI still
outstanding. Candidate production code remains unchanged.

## Fixed-binary interleaved result

Parent granted exclusive build/timing. Both test executables were built once
from the prepared baseline and selected candidate. Toolchain: go1.27.1
windows/amd64. The manifest records full binary/source hashes and build commands.
The documented START /B /WAIT /AFFINITY 10 restricts both programs to logical
CPU 4; the available parent mask was 0xffffffff. Each binary used -test.cpu=1,
-test.benchtime=300ms and the explicit benchmark selector; no tests ran.

Actual order was ABBAABBAABBAABBAABBA, ten balanced pairs, all twenty exits zero.
Raw per-run files were saved immediately under .tmp/output-lock-interleaved-*.
Combined raw and benchstat are reports/system-layering-output-lock-interleaved-
base.txt, -candidate.txt and -benchstat.txt. The source and binary hashes matched
their pre-capture identities when checked afterward. Production code was not
changed, and no extra comparison was run. The exclusive window is released.

```
Send          2.812n -> 2.809n       ~ (p=0.698 n=10)
Notification  23.70n -> 24.10n +1.67% (p=0.000 n=10)
BufferFlush   30.68n -> 29.76n -3.01% (p=0.000 n=10)
NoFlushFlush  36.32n -> 36.20n       ~ (p=0.127 n=10)
NewClose      271.4n -> 268.9n       ~ (p=0.481 n=10)
```

All output cases remain zero B/allocations. Constructor remains 448 B and
3 allocations. Actual struct size remains 328 bytes in both binaries. Constructor
timing intervals are still 15-16%, so no constructor speedup is claimed.

STOP: with the Send control stable, the significant notification increase
persists. No performance-neutral claim is supported. The parent was informed;
the exact implementation is preserved without alternate changes, commits or
publication. Parent must select the next action; analyst/verifier/PR requirements
remain open. Original sequential samples remain preserved rather than replaced.

## Selected empty-queue cleanup prepared

Parent selected a small source revision after the controlled result. In
SendNotification, while already holding outputMu, call flushLocked only if
len(outputBuffer)>0. The helper's empty case only assigns outputBuffer[:0] back
to the same slice and returns nil; it performs no diagnostics, byte accounting
or transport work. Skipping that no-op preserves the ordinary empty-queue
behavior. The nonempty call and entire error/diagnostic branch remain unchanged.
All other locks, layout, queue operations and transport code remain exact.

No attribution is made for the measured 0.40 ns increase. The selected cleanup
is prepared through apply_patch only. No compilation/test/timing yet: the option
worker owns that window. Prior candidate binary and all raw remain preserved;
next grant will rerun the focused new regressions and exactly one fixed-base/
new-candidate balanced interleaved n10 comparison, stopping on persistent cost.

## Empty-queue guard controlled result

At the next granted window, both focused regressions passed:

```
ok github.com/MongooseMoo/barn/server 1.079s
```

Built .tmp/output-lock-candidate-v2.test.exe once. Original baseline and prior
candidate binaries remain untouched. Source/benchmark/binary hashes were checked
before and after the capture and match. New trial source/build/manifest and each
raw run are .tmp/output-lock-v2-*; combined output and benchstat are reports/
system-layering-output-lock-v2-{base,candidate,benchstat}.txt. Exactly ten balanced
pairs used ABBAABBAABBAABBAABBA, all twenty exits zero, same START /AFFINITY 10
and explicit benchmark selector/CPU1/300ms. No extra comparison or code change.

```
Send          2.818n -> 2.534n -10.06% (p=0.000 n=10)
Notification  23.70n -> 21.84n  -7.83% (p=0.000 n=10)
BufferFlush   30.60n -> 30.47n  -0.39% (p=0.027 n=10)
NoFlushFlush  36.15n -> 37.03n  +2.45% (p=0.000 n=10)
NewClose      226.2n -> 230.2n        ~ (p=0.314 n=10)
```

Constructor remains 448 B/3 allocations and struct size 328 bytes. All output
cases remain zero B/allocations. Constructor intervals are 23-29%; no timing
claim there. Notification cost is reduced in this comparison, but the measured
NoFlush+Flush increase prevents a neutral claim. The unchanged Send control also
moves materially. No attribution or normalized adjustment is invented for either.

STOP as selected: parent informed, exclusive window released, no extra trials,
architecture changes, commits or publication. All older results are retained.

## Parent disposition

Parent retains the lock split and empty-queue guard for the demonstrated
replacement/metadata liveness defect. The 36.15 to 37.03 ns NoFlush+Flush result
is an explicit bounded comparison cost, with no performance-neutral or workload
speed claim. No further micro-optimization/state is selected. Independent source
analysis found no actionable defect in either source delta; required PR checks
and final verifier remain outstanding.

Lossless publication artifact: system-layering-output-lock-evidence.json stores
all nine raw text captures as JSON strings with original SHA256s and both
measurement manifests. Each decoded capture's UTF-8 hash was checked against the
original. This preserves raw whitespace while keeping ordinary diff checks
intact. Original raw text, per-run files and binaries also remain in the worktree.
