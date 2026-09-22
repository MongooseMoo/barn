# Mongoose lifecycle throughput investigation

Goal: recover the live parallel throughput lost between original 65a7e21 and
correctness repair 4b8733a without weakening transaction, lifetime or input semantics.
Databases, credentials, raw live transcripts and profiles remain outside Git.

## Facts

- Full repaired conformance: 12524 passed, 420 skipped. Nine live worlds accepted.
- At 16 clients repaired disjoint throughput was 587.27 versus 711.63 ops/s;
  read throughput was 644.83 versus 702.14 ops/s. Three-world medians only.
- Parallel workers invoke a test-owned matrix_eval verb, not intrinsic StartEval.
- Runtime now releases and creates a cleanup snapshot after every slice.
- Earlier floor-shard experiment identified registration contention and eager
  transaction maps/finalizers; that is historical evidence, not current attribution.

## Competing hypotheses and triage plan

1. Cleanup transaction churn: CPU/allocation profiles should attribute a material
   cost to BeginSnapshot/registration or lifecycle cleanup.
2. Lost execution concurrency: execution traces should expose scheduler, admission
   or lock waiting rather than additional useful interpreter CPU.
3. Short-run noise/background work: longer unchanged paired workloads should
   reduce or erase the gap. This would kill a regression-specific optimization claim.

Budget: three diagnostic probes, at most two isolated single-variable experiments.
Stop after two experiments miss their metric gate; record profiler-backed reasons.
Development: 16-client read/disjoint/hot workloads, 2000 iterations, longer runs.
Holdout: fresh 8-client parallel workloads, excluded from tuning; independent
verifier runs them once if a candidate earns promotion.
Profiling measurements are diagnostic only. Confirmation requires five paired
fresh-world runs against the committed correctness baseline, frozen evaluator,
at least 5 percent gain on the selected primary metric and paired interval above
zero, with no correctness failure. Candidate preregistration precedes source edits.

## First probe

Use the existing frozen matrix workload and assertions, with a diagnostic-only
external wrapper attaching CPU and execution traces during each parallel case.
One fresh world per original/repaired binary, 16 clients, 1500 operations/client.
Collect synchronization/scheduler profiles from Go traces and allocation profiles.
No source change is part of this probe. Raw results: private lifecycle-profile directory.

## Probe findings and selected experiment

Two diagnostic fresh worlds completed with zero recorded errors. Profiled rates
(original / repaired, ops/s) were read 737.21 / 743.41, disjoint 463.55 / 681.48,
contended 550.78 / 304.76. CPU/trace collection changes execution and these are
not acceptance timings. They do not establish a uniform repair regression.

The repaired read CPU profile attributed 4.17 percent cumulative CPU to transaction
release, including 2.76 percent to WAIF history pruning. Every release acquired
the global store write lock while history was pending, even with an unchanged
oldest reader. The three-second trace attributed about 0.99 blocked goroutine
seconds to release. Shared task enumeration and verb lookup also consumed CPU,
but this probe did not establish them as new repair costs. No scheduler, task
enumeration or cleanup-snapshot change was made.

Inspection exposed a prerequisite publication/last-reader race: publication could
sample an old reader before announcing pending history; the departing reader
could then miss cleanup. Commit 853cba5 announces pending history before sampling
the floor. A deterministic regression controls the reader-registration shard
barrier and proves the stale history is removed; it failed before the fix.

The selected experiment caches the floor of a completed full WAIF prune, skips
only equal nonzero floors and invalidates on every publication. This preserves
explicit older/future snapshot timestamps and prompt last-reader cleanup.
Preregistration, every development pair, validation output and final decision
belong in [the experiment record](../experiments/2026-09-21-waif-prune-watermark.md).
The experiment compares against the corrected 853cba5 baseline; its results must
not be presented as a fresh direct comparison against original 65a7e21 or master.
