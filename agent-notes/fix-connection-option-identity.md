# Connection option identity

## Goal

Preserve connection options across player switches by keying the existing
Session option store with immutable physical connection IDs. This is independent
of the still-unfixed same-task deferred switch/boot projection.

## State

Canonical managed Toast retains binary mode one after a switch. Baseline Barn
returned zero at the new player even in the following task. Its option
map was keyed by player; SwitchPlayer changes that player mapping without moving
the option bag. Source and raw witnesses are preserved in the campaign and
companion lifecycle research reports. The selected key/caller/cleanup change is
implemented; the real-MOO regression reproduces the old zero result and passes
after the change. Four server lifecycle/output-lock regressions pass, necessary
fixtures expose explicit IDs, and repeated A/B samples are saved in reports.
The first implementation's common input/held-input timings regressed despite
unchanged allocations. Those raw artifacts remain preserved; they are not
measurements of the revised implementation. The coder report records existing
input/HTTP producers and the exact selected revision.
Parent selected the exact revision written in that report: pass
producer IDs through input/read/collector calls; keep current-player connection
resolution only in cold cancellation, which lacks a physical producer ID. No
HTTP identity metadata is added. The revised implementation and its focused
correctness checks are complete.
The explicit-ID signatures, two production input callers, read_http producer and
discovered direct test callers are edited. New tests cover no-manager hot probes
and replacement cancellation. After the compilation hold released, focused
builds exposed an omitted caller: builtinRead still passes player to the helper.
Parent selected and implementation retained builtinRead's existing admission
result and its ConnectionID, without a second lookup or changed error order.
Focused builtin cases pass (1.687s), including zero manager hot probes, exactly
one read admission lookup, and replacement cancellation. Six server cases pass
(1.541s), including real ordinary/forced-input FIFO flush and stale replacement
cleanup.
Full observed focused commands/output are saved in
reports/system-layering-option-identity-revised-focused.txt. Parent granted the
exclusive timing window after output-lock released it. Original and selected
benchmark binaries were built; selected source was restored exactly, with
matching pre/post source fingerprints. Ten balanced AB/BA pairs completed using
fixed affinity mask 4 and GOMAXPROCS=1. Raw output/order/fingerprints and benchstat
are saved in revised-base/candidate/manifest/benchstat report artifacts.
Significant residual regressions remain: default input 39.83 to 41.55 ns
(+4.32%, p=0.001), stored binary notify 130.0 to 135.2 ns (+3.96%, p=0.000).
Stored input improves 70.48 to 69.38 ns (-1.57%, p=0.000); allocation counts
are unchanged. Worker stopped further changes and released the timing window.
Parent retains this direct design for correctness review with the measured
nanosecond costs explicit. It fixes proven physical-socket option loss; it is
not performance-neutral. No cache, index or additional connection state is
selected to chase those costs. Independent source analysis is active; nothing
is committed or published, and full required CI remains outstanding.

## Decisions and why

Keep Session as the option-store owner and its current mutex. Use immutable
Connection.ID as the key; expose that existing identity through the builtin
connection interface without an alias or fallback. Adapt actual callers that
already hold a connection, so ordinary notify and input reads do not gain a
second manager lookup. Internal player-owned held-input operations must resolve
their actual connection explicitly, with lock-order review. That first choice
caused measured hot-path overhead. The implemented revision changes heldInputEnabled
to take a physical ID and threads existing producer IDs through HandleHeldInput,
collectHTTPWakeupsLocked and prepareHTTPRead. Cancellation alone resolves the
current player connection inline; all player-owned state and queues stay put.

Do not add last-producer identity metadata to retained HTTP state: replacement
changes the manager mapping before another producer need run, while stale
disconnect intentionally preserves that state. Such metadata would become
stale during cancellation or require an unnecessary lifecycle rebind operation.

Use a real blocked SendNotification producer in the option-read liveness test,
with the existing shutdownPeer/TCPTransport helpers, rather than manually
locking Connection.mu. PR 646 separates metadata/output mutexes; a field-based
fixture would silently stop holding the output owner after that change. Wait
actual physical Write entry, read the option through real Eval, then close and
join both workers with cleanup registered first. This preserves the test's
meaning across that independent refactor without a production compatibility path.

Delete options for the actual physical ID on disconnect, using the existing
input event's ConnID. A stale displaced connection must not clear a replacement's
state. Keep full Session.Close clearing. Do not transfer/copy bags by player,
move options under the output mutex, add GetByID solely for convenience, migrate
held commands/HTTP buffers, or change deferred recipient/effect semantics.

## Surprises

At this task's original base, Connection.mu was held during physical output;
PR 646 now separates that output owner. This bounded correction keeps Session
ownership. A post-switch bag transfer also has a window
between player publication and hook/input-reader observations; stable keys remove
that transfer requirement.

The first implementation's player-owned held-input helper gained actual connection resolution.
Ten-sample medians show ordinary input 39.69 to 47.33 ns/default and 68.13 to
79.15 ns/stored. Held-input probes rise 12.16 to 15.71 ns/default and 25.57 to
30.98 ns/stored. These are fixture measurements without production manager-lock
contention. Default notify has no significant difference; stored binary notify
rises 1.76%. Allocation counts are unchanged. The coder report preserves raw
samples, fixture failures/repairs, commands and statistical limitations.

## Next step

Independent source analysis found a real stale-setter cleanup race: a wizard
task can retain a resolved socket while its disconnect lane removes/forgets it,
then recreate the closed ID's bag. The correction below is implemented and its
delta analysis found no new actionable defect.

Selected correction: expose the real Connection's existing ctx.Done through
Done() on the builtin interface, without a new lifetime field. On natural
disconnect, call its existing cancel after manager unlock and before Forget.
The private Session setter takes the captured physical ID and lifetime channel,
checks that channel while holding the existing option mutex, and returns false
without creating/updating if closed. It returns true after a live mutation.
The builtin preserves its existing normal zero result when skipped and does not
wake input or drain commands for a mutation it did not perform. Reads remain
ID-only; no tombstone, closed-ID registry, extra lock or manager lookup is added.
Raw store-only test/benchmark callers can supply a nil lifetime channel; the
sole production setter supplies the real connection channel.

First add the analyst's deterministic real-manager capture/pause regression:
pause admitted set_connection_option after actual lookup, suppress the target's
disconnect hook, process real disconnect, resume/join setter, assert normal zero
and no closed-ID binary entry. Register idempotent release/join cleanup first.
Capture RED before the correction and GREEN afterward. Add the inverse/live
ordering assertion as needed, preserve existing error order, and adapt actual
fake Connection interfaces explicitly. Normal server shutdown already cancels
connections before Session.Close; dbtool has no ConnManager. No new global
Session-closed semantics is selected from this per-socket race.

The real-manager regression is written in the existing server identity test
file. It registers idempotent release/join cleanup before launch, captures the
actual connection, suppresses its disconnect hook, processes real disconnect,
then resumes and checks the normal zero result and absence of its binary bag.
After parent released diagnostics and granted focused compilation, behavioral
RED reproduced the recreated closed-ID binary bag (server 1.062s), saved in
reports/system-layering-option-identity-stale-setter-red.txt. The selected Done
interface, real context method, natural-disconnect cancel-before-Forget, and
setter check under the existing mutex are implemented. Builtin skips ancillary
wake/drain on a rejected mutation while preserving normal zero. Fakes expose
explicit never-cancelled lifetimes; raw store test/benchmark calls pass nil.
The same exact regression is GREEN after correction (server 1.020s). Related
named selections pass: builtins 1.161s, seven server cases 1.553s, engine
blocked-output fixture 1.425s. Unit coverage proves no update or recreation for
closed lifetime and normal-zero/no wake/no drain/no forced input on skipped
builtin mutation. Full raw outputs are saved in lifetime-builtins/server/engine
green report files. The focused window is released; no Go commands remain for
this worker. No timings were rerun: earlier source hashes/performance evidence
describe the measured pre-lifetime revision, with residual costs still explicit.
Hot option-read source did not change in the lifetime correction, but the exact
final binary was not benchmarked. No performance-neutral claim is made.

Parent replaced the manual output-mutex fixture with the actual blocked TCP
SendNotification producer described above. Its explicit test passes (server
1.473s), proving the option read completes while physical output is still
blocked and the writer joins after Close. This remains meaningful across PR
646's separate output mutex. Only this edited-file test was run by parent.

reports/system-layering-option-identity-evidence.json preserves twelve raw
captures losslessly with decoded UTF-8 SHA256 checks, including source identities,
pair order, behavioral RED/GREEN and precursor timings. Original files remain.
Integrate current default containing PR 646 before publication, then finish
required exact-head PR CI/final review.
Preserve the original/revised timing evidence and disclose residual costs in the
reviewable result. No further measurement or alternate design is selected.
Keep same-task pending views and output redesign outside this bounded fix.
Only explicitly named tests from edited test files have run locally.
