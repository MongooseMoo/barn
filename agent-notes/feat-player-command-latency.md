# Player command latency

## Goal

Q authorized bounded socket read-ahead and command response diagnostics with
"Make it so!". Preserve per-connection execution order and existing scheduling
policy. Do not infer that contention, $sh.handle or read coupling causes the
reported lag without a measured incident.

## State

PR #614 is open: https://github.com/MongooseMoo/barn/pull/614 . Q authorized
publication with "yup, do it." The branch has been pushed. First PR CI found
two metrics metadata-contract failures because unassociated records included
command_id:0. The correction emits command_id only for actual command-associated
work; the original strict metadata tests are unchanged. A new regression covering
runtime phases, threaded lifecycle and terminal exceptions first failed with
"unassociated work claimed command identity" on the actual runtime record.
CI/Elle final verdicts for the correction remain pending.

Implemented bounded transport read-ahead (16 outstanding events, 5 MiB budget,
one decoded input may wait outside the budget), diagnostic command identity,
receipt/task/committed-output/first-write records, a 100 ms pending record with
admission and task context, and barn_logs command filtering/summary. Diagnostic
identity stays out of checkpoints and unrelated fork contexts.

Focused new tests passed: server TestPlayerReaderReadsAheadWhileExecutionStaysOrdered,
TestPlayerReaderBufferBoundsAndCancellation, TestPlayerCommandDiagnostics;
metrics TestCommandDiagnosticDistinguishesFailedAndUnrelatedOutput and
TestCommandDiagnosticDisabled; barn_logs TestCommandSummaryCorrelatesInputAndPhases.
Managed WSL Toast input completion and capability admission returned:
3 passed, 13244 deselected, 1 warning in 238.19s. The warning concerns JUnit
record_property compatibility. Oracle evidence is under /tmp/barn-player-latency-toast*.
Final focused race-detector command:

`go test -race ./server ./metrics ./cmd/barn_logs -run '^(TestPlayerReaderReadsAheadWhileExecutionStaysOrdered|TestPlayerReaderBufferBoundsAndCancellation|TestPlayerCommandDiagnostics|TestCommandDiagnosticDistinguishesFailedAndUnrelatedOutput|TestCommandDiagnosticDisabled|TestCommandSummaryCorrelatesInputAndPhases)$' -count=1 -v`

Raw result:

```text
ok  github.com/MongooseMoo/barn/server 2.562s
ok  github.com/MongooseMoo/barn/metrics 1.619s
ok  github.com/MongooseMoo/barn/cmd/barn_logs 1.621s
```

The real diagnostic test emitted pending at elapsed_ns=100151300, with its
suspended task and admission snapshot, then first_output at elapsed_ns=103221400.
It also checked a discarded retry produces no output, one published response
after retry, and error-handler task/output correlation. These are pre-publication
local results, not candidate conformance or isolation verdicts. CI runner variable
selects self-hosted Linux X64 barn; CI/Elle are triggered by a PR, not branch push.

Isolated worktree from origin/master. Existing master changes remain untouched.
Read the input dispatcher, transports, connection output, runtime task factories,
eval lifecycle, admission controller, task context, commit-deferred notifications,
workload diagnostics and Elle instructions. Read all 14 page images of Dapper
through the paper-process light workflow; text notes and collection records are
under papers/Sigelman_2010_Dapper. Managed Toast input-completion check is complete.

## Implementation plan

1. Let the socket reader enqueue ahead while its existing connection worker
   continues to execute input in order. Bound outstanding transport events and
   bytes per connection; apply backpressure to that reader, never the shared
   dispatcher or forced-input producer. Preserve the initial banner, timeout and
   disconnect ordering. A single decoded line can be retained while waiting for
   byte capacity; transport MAX_LINE_BYTES remains unchanged.
2. Under existing diagnostic opt-in, assign input identities with monotonic
   receipt timestamps. Log dequeue, task association, first successful output
   write and processing/terminal outcomes. Preserve identity across retry and
   suspension but not unrelated forks. Direct replies carry explicit identity;
   committed notifications carry it to their destination connection. Prefixes,
   suffixes and output to other players cannot satisfy the response measurement.
3. Report slow pending inputs before they complete with admission and existing
   task summaries as competing-work context. Use existing task phase records,
   gate evidence and profiles for detailed diagnosis; do not attribute global
   counter differences to one command. Summarize response timings in barn_logs.
4. Add focused regressions for read-ahead/order/backpressure/shutdown and for
   delayed, buffered, unrelated, failed and absent output. Only newly written
   test files may run locally. Record verification output and keep limitations
   explicit. No deployment requested.

## Decisions and why

Dapper pp.3-4 supports explicit identity through asynchronous callbacks; p.13
explains why tracing the slow request alone misses competing queue work. Use
existing logs and task owners, with no new scheduling policy or tracing service.

## Surprises

The metadata fetch helper could not resolve the primary Dapper PDF URL. Its
title search returned unrelated physics papers, which were rejected. Retrieved
the independently verified Google PDF directly and read local images instead.
No assertions from search snippets are used as paper evidence.

One read probe incorrectly named nonexistent builtins/async.go. Located and read
builtins/threaded.go instead. Per Q's rule zero, added a literal-read-path guard
to C:/Users/Q/.codex/hooks/verify-read-paths.ps1 and configured it in the existing
hooks.json PreToolUse list. Direct checks returned exit 2 for the missing file
and exit 0 for the real file. This global hook change is outside the Barn commit.

An error-output callback initially tried calling a concrete Connection method
through builtins.Connection; the test build rejected it. Read the manager's
typed connection maps and GetConnection producer, then used its concrete
Connection at the server callback boundary. Focused race tests passed afterward.

Final review found notify(player, "", 0, 1) counted as a response despite
writing no text or newline. Added a native-command regression using that exact
MOO notification; it failed with "zero-byte notification counted as a printed
response". The output boundary now drops diagnostic attribution for that
zero-byte notification while preserving its original transport behavior.
The focused TestPlayerCommandDiagnostics regression then passed with -race:
`ok github.com/MongooseMoo/barn/server 2.408s`.

## Next step

Validate and publish the metadata-contract correction, then watch PR #614's CI
and repair any remaining failures. The PR's checks and retained workflow
artifacts are the authoritative candidate conformance and isolation evidence.
No merge or deployment was requested. Do not claim real-world lag resolved or
deployment performed from these diagnostics or tests.
