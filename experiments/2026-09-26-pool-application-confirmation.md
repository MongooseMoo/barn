# Final application confirmation preregistration

The original five short-window pairs remain in `2026-09-26-pool-application.md`.
Their sixteen-player candidate was slower in every pair. Subsequent focused
diagnosis found anonymous-only Value pooling overhead and led to one narrower
finalization refinement; this confirmation evaluates that final source, not the
original candidate. It does not replace or erase the original measurements.

Production source is frozen at `ea251f7ffbb88112d3ed9337671da9df0b97e919`:
unique and file-read pools plus pooled Frames/direct WAIF Values. Other Value
shapes retain their original collection path. Baseline is `3c0f510`.
The evaluator remains engine/mongoose_real_bench_test.go blob
`3ac942cb3d5589b7a588dfb354ec0d0ac0627045`; runner script
pool-application-confirm.ps1 blob `d70982a275fba912cdc5eb9c89e3cc70e2b234a1`.

SHA256 seals:

- Baseline executable: `5C61B43174045818574EF37E031166122D22CA0449B4469FE6C67A78D3A45F11`
- Final executable: `12A4BDF1529E48BB66BA102832017EE1E9C8D6518D41BF675409F4BDB14DE1B4`
- Disposable database: `489FF8D14884392DCFBA6CD88D407E53A2140C03D4F0CC4B1B19CF2AFD8A031E`

Run ten alternating AB/BA pairs, each in a fresh process and private directory,
with sixteen players only, two-second warmup, fifteen-second measurement,
GOMAXPROCS=4, default command mix, repairs and number promotion enabled. The
longer window reduces drain sensitivity; isolated player level removes prior
one/four-player state. Keep normal process priority and affinity. Host load is
uncontrolled; do not disturb unrelated processes. No campaign workloads or CI
overlap timing. Preserve all observations, failures, and complete raw logs.

Primary statistic: median paired candidate/baseline goodput ratio, percentile
bootstrap 95% interval with 20,000 resamples and seed 20260929. Upper interval
bound below 0.95 means clear material loss. Lower bound at least 0.95 supports
no greater than 5% loss for this setup. Anything else is inconclusive. New
failed/uncaught commands stop promotion for diagnosis. Do not call an
inconclusive result no regression. Command mix/counts, retries, allocations,
and latencies are diagnostics subject to the original harness limitations.

This is the final confirmation: no source changes, replacement pairs, sample
extensions, or selective exclusions after observing results. An infrastructure
failure leaves the confirmation incomplete and must be reported explicitly.
The independently prepared analyzer will be committed and sealed before the
first process starts.

Command from the campaign root:

```powershell
./experiments/pool-application-confirm.ps1 -Baseline .tmp/engine-baseline.exe -Candidate .tmp/engine-final.exe -Database .tmp/mongoose.db -OutputDirectory .tmp/application-confirmation
```
