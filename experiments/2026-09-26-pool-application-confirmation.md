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

## Results (appended after all twenty processes completed)

Independent recommendation: publish the allocation-focused PR, subject to final
repository/CI checks. The final narrowed source clears the preregistered
application guard for this sixteen-player, fifteen-second setup. This is not a
general production-throughput guarantee or a causal attribution of the entire
observed difference to pooling.

All ten alternating pairs completed. Every process ended with exit=0, and every
measurement reported failed=0 and uncaught=0. No replacement pairs, extensions,
early termination or exclusions. In particular pair 8's slower candidate
(292/s versus baseline 318/s) remains in the analysis. The full runner log and
all twenty original logs are preserved in the adjacent
2026-09-26-pool-application-confirmation-evidence directory.

The sealed independent analyzer was unchanged from its pre-run SHA256
0CDCE11CB09586FB8681D980D9D7427853DBB0E4D3DA77CF2F2E9154FC14346D.
Its complete result is results.json. Raw primary output:
```
verdict NONINFERIOR
paired goodput ratio median 1.123825099631551
95% interval [1.0348222545262322,1.1679006968641115]
independent analyzer exit=0
```
The lower bound exceeds the frozen 0.95 threshold. Baseline goodput median is
283/s, range [163,322]; candidate median is 320/s, range [189,341]. These unpaired
medians differ from the paired-ratio calculation. Every pair's goodput ratio:
```
1.010638 1.159509 1.708543 1.196429 1.039344
1.139373 1.129371 1.118280 0.918239 1.059006
```

Diagnostic paired ratios (not additional promotion gates):

| Metric | Median ratio | 95% interval |
| --- | --- | --- |
| Reported bytes/command | 0.990058 | [0.974244,0.998850] |
| Reported allocations/command | 0.981096 | [0.953300,0.994703] |
| p50 command latency | 0.948886 | [0.859758,1.046566] |
| p99 command latency | 0.934537 | [0.699573,0.997309] |
| Abort percentage | 0.987507 | [0.928800,1.024279] |

The host snapshot (host.json) records CPUPercent=100 at
2026-09-26T23:11:23.5443890-06:00, free physical memory 31876948 KiB of
134125572 KiB. This was not an isolated host. Time-based command prefixes,
background work, changed realized command mix, and end-window drain remain
limitations described in the initial application record. Allocation/command
therefore does not measure identical work on both sides; latency percentiles
are diagnostic, not production latency promises.

The original application candidate used broader finalization pooling and a
different 1/4/16-player sequence with three-second windows. It was slower in
every sixteen-player pair. This final run changes both source and measurement
setup, so the change between those studies cannot establish how much the
refinement, longer windows, isolated player level or host load contributed.
Application CPU/memory profiles captured before narrowing describe the initial
candidate; they are not final-source profiles. The initial unfavorable data and
the rejected experiments remain in the repository.

The recommendation rests on independently measured focused allocation savings,
correct ownership/reset/bounds and race checks, preserved small-anonymous
behavior, and this final setup's noninferiority result. Do not advertise a
general 12.4% application speedup from this experiment.

## Raw archive and reproduction

To keep the PR file list reviewable, raw-logs.zip contains all twenty untouched
stdout/stderr logs. raw-log-manifest.json records each original filename, byte
length and SHA256. Every archive entry was checked against its original bytes
before committing. The original .tmp logs remain untouched. The evidence
directory also includes host.json, runner.txt and results.json.

Extract into a fresh directory and verify hashes against the manifest, then run
the frozen analyzer with a new output filename (it refuses to overwrite):
```
Expand-Archive -LiteralPath experiments/2026-09-26-pool-application-confirmation-evidence/raw-logs.zip -DestinationPath .tmp/confirmation-reproduce
python experiments/pool-application-confirmation-analyze.py .tmp/confirmation-reproduce .tmp/confirmation-reproduced-results.json
```
