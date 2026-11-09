# Live Mongoose observations and CPU profiles

Linked `.tmp` artifacts and sibling worktrees are retained local recording
evidence, not checkout contents. The [tracked gameplay acceptance summary](reports/mongoose-gameplay-acceptance-20261013.md)
and this guide preserve provenance and reproduction instructions.

Two tools answer different measurement questions.

## Passive observations on the current binary

From this worktree:

```powershell
python scripts/sample_live_moo.py --duration 5 --interval 0.05 --out .tmp/active-verbs.json
```

The default target is the known production binary, pinned by SHA-256 and source
revision. It needs process-memory read permission (normally root on this host),
matching clean source/build headers and a compiler. It compiles a tiny layout
helper in a temporary directory, reads interpreter metadata through read-only
procfs, and checks task/frame/name identity repeatedly. No debugger, signal,
target-memory write or restart is involved. Captures never overwrite evidence.

Output includes accepted observations, rejected reads and caller stacks. It has
caught real ship-cycle electronics/cache invalidation and test cleanup. Reads
remain non-atomic; stable-read counts favor long-lived activations and **cannot
be converted into CPU percentages**. The top cached PC may be stale; its current
source line remains unknown. See the original checkout's
[slow-Mongoose report](../why-mongoose-slow.md) for the recorded observations.

## True CPU sampling on an instrumented Toast binary

The isolated Toast implementation is at
`C:\Users\Q\src\toaststunt-cpu-profiler`. Its
[feature guide](../../../src/toaststunt-cpu-profiler/docs/Features/cpu-profiler.md)
describes native main-thread CPU sampling, coherent semantic publication,
controls, loss accounting and bounds. It has not been deployed to production.

Export `generate_json(moo_cpu_profile("stop"))` from that server and save the JSON
object without the surrounding telnet or Ops response envelope. Then:

```powershell
python scripts/report_moo_cpu_profile.py profile.json --out profile.md --folded profile.folded
```

The reporter verifies that all recorded categories reconcile, CPU-clock-derived
expirations fit the complete denominator, native and MOO self counts remain separate,
and verb/builtin fractions agree with raw counts. It rejects passive captures,
invalid or empty captures, inconsistent totals and missing folded-stack weights.
The folded file includes unknown, transition, profiler and outside-interpreter
weights; omitting them would make flamegraph percentages misleading.

Tables show self and inclusive CPU estimates by retained program generation.
Inclusive rows overlap. Generation IDs are local to a capture. The folded labels
replace semicolon separators inside names with the full-width character `；`;
the original JSON preserves exact metadata.
Unknown observations can contain entire omitted verbs, so structural losses
can make rankings incomplete. These shares describe the instrumented run.

The current implementation is an offline diagnostic prototype: one five-pair
cohort measured active/disabled CPU medians of 1.254x for interpreter loops,
1.568x for tiny verb calls and 1.000x for synchronous native work. See the Toast
guide for ranges, raw evidence, ownership checks and memory/export limits.
An additional five-round fixed-work matrix measured disabled/stock medians of
1.007x for interpreter loops, 0.984x for tiny calls, 0.998x for native work and
1.049x for cheap builtins. Active/stock medians were 1.266x, 1.550x, 1.008x and
3.165x respectively. These are instrumented Test.db fixture measurements;
sub-one ratios do not establish improvements to stock execution. All rounds,
including a noisy cold round, remain in the
[complete matrix](../../../src/toaststunt-cpu-profiler/.tmp/stock-disabled-matrix-20261013-01/complete.json).
A live
semantic capture requires an instrumented binary; production has not been
replaced or restarted by this work.

Neither tool measures network RTT or foreground response latency. A production
diagnosis needs those measurements alongside CPU, scheduling and memory data.
