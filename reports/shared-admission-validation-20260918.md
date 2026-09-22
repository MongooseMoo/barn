# Shared admission implementation and validation

Historical record, superseded for merge readiness on 2026-09-21. The old helper
scripts below bypassed the current managed entrypoint and have been removed.
Their reported baseline failures must not be used to excuse failures under CI's
workflow. Use the [managed command](../docs/mongoose-conformance.md) and
[current full-suite and benchmark evidence](../experiments/2026-09-21-waif-prune-watermark.md).

Branch: `fix/mongoose-workload`. Baseline: `83751cc`.

## Delivered behavior

Foreground, background and independent system invocations now share runtime admission. Each principal pays measured execution occupancy, including conflict retries, with commit-gate wait recorded separately. Nested synchronous calls borrow the owner's reservation; real suspension and completion release it before dependent work runs. Configurable global/principal caps and class/group weights retain background service under foreground load.

The commit gate grants FIFO shared/exclusive capabilities. Transaction exemption requires a live grant from its own store. Only unstarted entry-exclusive waits are cancellable; existing mid-activation transaction semantics remain unchanged. Debug metrics expose service, gate wait, queue and maintenance totals.

Forced input uses a nonblocking per-connection mailbox, avoiding a producer blocked behind its own admission reservation. Completion result roots remain pinned through callbacks. Checkpoints drain admission and retain the sweep/start barriers across task capture and store serialization, preventing an old continuation from being saved alongside its later mutations.

No conformance assertions or profiles were changed. Repeatable commands live in `scripts/test-shared-admission.ps1` and `scripts/recheck-conformance-failures.ps1`; configuration and ownership details are in `plans/barn-transaction-scheduling-design.md`.

## Verification

After the checkpoint repair, `go test ./...` exited 0 (`.tmp/admission-checkpoint-go.log`). Broad race command:

```text
go test -race ./internal/admission ./internal/commitgate ./engine ./server ./task ./db/store
ok github.com/MongooseMoo/barn/internal/admission 1.570s
ok github.com/MongooseMoo/barn/internal/commitgate 1.463s
ok github.com/MongooseMoo/barn/engine 189.055s
ok github.com/MongooseMoo/barn/server 2.385s
ok github.com/MongooseMoo/barn/task 1.694s
ok github.com/MongooseMoo/barn/db/store 3.838s
```

Deterministic regressions reproduced and repaired callback-result collection and execution between checkpoint task/store capture. Focused tests also exercise cap-one nested work, cancellation, queued-task preservation, forced-input flooding and FIFO gate ownership.

Canonical managed WSL Toast checks, with unchanged assertions and admission checks:

- Dump/exec regression selection: 6 passed.
- Forced input and task scheduling audit: 40 passed in 83.70s.
- Connection lifecycle and checkpoint operational suites: 28 passed in 75.49s.

Full rebuilt Barn matrix command: `./scripts/test-shared-admission.ps1 -SkipGo -FullConformance -RunDir .tmp/shared-admission/checkpoint`.

- Cap 1, run `20260918_182853`: `79 failed, 12875 passed, 24 skipped in 503.99s`. Failed list exactly matches the 79 failures reproduced on baseline in focused run `20260918_182040`.
- Cap 16, run `20260918_183720`: `80 failed, 12874 passed, 24 skipped in 510.46s`. All 80 failures are in the 81 reproduced on baseline in focused run `20260918_182920`; no unmatched failures remain. Both `audit_waif_values_in_suspended_vm_survive_restart` and `suspended_task_resumes_once_after_restart` passed.

The checkpoint regression was then strengthened to require the competing goroutine to reach admission, preventing a vacuous pass; ten race-enabled repetitions passed (`ok github.com/MongooseMoo/barn/server 1.209s`). This final change is test-only. The matrix script exited 1 after collecting both complete runs, as designed for non-green conformance results.

The initial pre-barrier runs had one additional checkpoint consistency failure each: WAIF continuation replay at cap 1, and suspended-task duplicate replay at cap 16. Both expectations were verified against the canonical Toast oracle before the repair. Baseline comparisons are focused reproductions of candidate failures, not complete baseline-suite comparisons. The suite is not green, and these checks do not establish live Mongoose latency or throughput improvement.

## Review and remaining limits

Fable reviewed admission, requested corrections, and accepted those corrections in `reports/fable-admission-final-20260918.md`. Its subsequent checkpoint/root-handoff follow-up stopped with HTTP 429 (`model_requires_usage_credits`, "You're out of usage credits") before a verdict. That incomplete draft is retained separately. The Gemini fallback did not start its review: `IneligibleTierError: This client is no longer supported for Gemini Code Assist for individuals.` Neither failure is an independent acceptance of the checkpoint follow-up.

Local checkpoint audit: the server's final checkpoint precedes `runtime.Stop`; hooks execute outside the admission pause; pause drains reservations without holding sweep/start locks; physical leases are deliberately not drained because suspended intrinsic Eval retains one while awaiting readmission. A suspended Eval changes its private resume state before readmission but executes MOO only after readmission; finalization takes the same sweep/start barriers. These are source findings, not a substitute for the unfinished independent review.

Checkpoint serialization currently pauses admission through disk writing because snapshot WAIF payloads remain mutable. The principal ledger retains historical principals and scans them when settling service. Forced-input queues are unbounded. Direct Eval/store mutation retains existing isolation limitations. Subsequent [live Mongoose measurements](mongoose-admission-measurement-20260918.md) found a severe responsiveness regression at admission/GOMAXPROCS 1, mitigated by admission capacity 2 on one Go execution slot. Those measurements also exposed startup and checkpoint failures in both builds; this implementation has not earned live-workload performance acceptance.
