# Original-checkpoint player lag diagnosis

The original Mongoose world can now be replayed with its saved work restored.
The diagnostic capture found a missing wait-accounting path, which is corrected.
This in-process capture reproduces terminal delays without socket I/O. It does
not establish that player first-output lag is fully solved.

## Inputs and fidelity

The frozen November 5 checkpoint and deployment sound catalog are identified by
their hashes in each `.tmp/player-lag-*` manifest. No source input is opened for
writing. The original checkpoint reports:

```text
queued restored=1080/1080 suspended restored=49/49
native restoration census: 70 frames
```

Native saved positions now convert over the existing compiler IR into the
existing VM, with its verifier retained. No source effects are replayed. Reduced
managed Toast producers cover suspension across calls, inherited command
environment and unsupported byte-string iteration. Pending native-finally and
builtin continuation states remain explicit unsupported errors.

Toast account preparation changed 415 saved scheduling rows, including a fork
deadline from 109290153959993 to 416145977. The harness now refuses that output
and preserves the rejected preparation. Earlier bootstrap-based comparisons do
not establish original-checkpoint performance. No control credentials are
available in MONGOOSE_LOGIN_SCRIPT for an original-checkpoint TCP capture.

## Runtime mutex comparison

Both builds contain the same native-restoration/compiler fixes and upstream
code. Executable source differs only in the three runtime mutex-repair files;
the candidate also has the deterministic blocked-scheduler regression. These
are sequential instrumented runs, one pair, not a statistical speed claim.
Each measured window is 30 seconds after a two-second warmup; levels share a
live disposable runtime, so differing command counts can change subsequent
world state. Full console, runtime JSON and profiles are retained in
`.tmp/player-lag-paired-r1/{control,candidate}`.

| Players | Control goodput/s | Candidate goodput/s | Control p99 ms | Candidate p99 ms | Control max ms | Candidate max ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 43 | 43 | 119.11 | 124.32 | 243.09 | 343.69 |
| 4 | 88 | 80 | 622.44 | 667.83 | 985.21 | 1205.13 |
| 16 | 183 | 275 | 1158.98 | 834.42 | 2061.52 | 1913.32 |

```text
control completed=1290/2683/5629 failed=0/10/81 unsettled=0/0/0
candidate completed=1292/2447/8443 failed=0/9/119 unsettled=0/0/0
```

Application exceptions remain failures. Native `home` E_NACC and `@who`
E_PROPNF frames are preserved. The mutex repair permits an unrelated VM start
while a scheduler handoff is blocked; the mixed workload pair does not prove a
universal latency improvement.

## One slow command and the missing wait

After 60 seconds of warmup, the mutex-disabled trace capture completed:

```text
completed=9358 failed=136 unsettled_at_return=0
p99=756.77ms max=1450.09ms
--- PASS: TestMongooseRealWorkload (104.60s)
```

Its terminal tool output was truncated and is named `console.partial.txt`.
Complete JSON logs, trace, block/CPU profiles and measurement-only MOO profile
are in `.tmp/player-lag-warmed-block`.

Task 2146093051, `#261:say`, provides a concrete producer:

```text
observed task span 1442.878ms; 174 slices
VM attempts 175; total 690.231ms; max 12.652ms; ticks 1883122
exclusive commit_gate 3 observations; total 5.529ms
task-scoped trace synchronization blocking 523.52ms
  StoreTxn.Commit 327.53ms; commitTaskRowsOnly 320.93ms
task-scoped trace scheduler latency 60.59ms
SQLite operations 2
```

The task-scoped trace profiles are retained beside the raw trace. The interval
spans observations of this task; it is not the socket first-output interval.
VM wall, synchronization and scheduling intervals overlap and are not additive
CPU measurements. CPU sampling attributed approximately 0.57s cumulatively to
executeLoop for this task. MOO profiles expose root verb labels, not task-ID
labels; a task-ID MOO filter returned no samples and was rejected.

In the original MOO, `#1:forbids` calls `yin()` for each handler at line 21.
`#1:can_be_seen_by` calls viewer and target forbids at line 9. Speech also
computes per-observer titles through `#35:say_action` line 27 and
`#2700:players_seeing_differently`. Barn's existing `yinCommitTicks=10000`
policy splits long nonforked work. That policy has not been changed here.

`commitTaskRowsOnly` acquired the shared gate but skipped the counters and
observer used by ordinary commits. The correction gives both paths one
acquisition/accounting owner. Runtime `shared_commit_gate` records the available
task/hook/command identity and, for admitted work, calls the existing admission
observer so the wait no longer counts as execution service. Sweep hooks emit
the diagnostic phase without an admission scope. Exclusive waits remain `commit_gate`. Gate order,
validation and publication rules are unchanged.

The real task-row regression was RED with counters/observer all zero, then
GREEN. The runtime regression was RED with no phase, then:

```text
TestTaskRowCommitWaitReachesRuntimeDiagnostics: PASS
ok github.com/MongooseMoo/barn/engine 1.306s
```

The subsequent original-world diagnostic smoke restored every recorded task
and settled its 1/4/16-player cohorts. The existing CLI now exposes an actual
long `say` task's shared waits:

```text
barn_logs -dir .tmp/player-lag-accounted-smoke/logs -runtime -task 2145966056 -json
shared_commit_gate: count=172 total_ns=306350900 max_ns=30618800
commit_gate: count=2 total_ns=3093900
slice: count=174 total_ns=1156499100
command_records=0 invalid_records=0 terminal_exception_records=0
```

Full console, manifest, JSON and the retained task summary are in that capture
directory. No socket first-output records are expected from this in-process
driver. The 16-player window completed 2325 commands, failed 33 and left none
unsettled; p99=961.41ms/max1444.69ms. Its 27330 shared observations total
31.304313s across concurrent work, matching the store counters for that window.
That aggregate must not be attributed to this individual task.

Review also found standalone admitted and sweep-owned hooks missed the phase.
They now use the same observer, with hook/owner identity and nil-scope handling.
Both hook endpoints were RED with observation count zero, then the joint root
and hook regressions passed with `-race` (`ok engine 1.469s`). This final hook
coverage correction has focused verification; the smoke predates it. Canceled
shared acquisitions remain outside this successful-acquisition accounting.

## Capture failure preserved

The preceding Windows Go 1.27.1 capture enabled mutex profiling and stalled
after 77.3 seconds. It never produced a terminal cohort. Its partial trace and
logs remain in `.tmp/player-lag-warmed-trace`. A Delve attachment reached EOF
without capturing stacks and the target exited afterward; there is no debugger
stack verdict for that process.

Upstream golang/go#80982 describes a Windows mutex-profiler/GC deadlock, with
the #81145 backport in Go 1.27.2. Local Go 1.27.1 source lacks that fix. The
controlled retry without mutex profiling completed. This is consistent with
the upstream defect; it is not proof that this process had the identical stack.
The workload now rejects enabled mutex collection on Windows Go 1.27.0/1 before
heavy loading. Documentation directs block/trace collection on this toolchain.

## Acceptance limits

No new publication, exact-head full PR CI, pinned Elle verdict or live deployment
is established. Native restoration and the wait-accounting fix have focused
regressions and original-world diagnostic evidence. First socket output on the
unmodified checkpoint still needs a schedule-preserving credential preparation
or an existing disposable-world control login. Preserve that distinction before
changing runtime policy based solely on terminal-completion latency.
