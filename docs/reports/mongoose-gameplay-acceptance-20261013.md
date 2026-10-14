# Offline Mongoose gameplay acceptance, October 13, 2026

This is a recorded local acceptance summary, not a new performance comparison.
The source baseline was `9c705a1a8ff90243202742f9b8d0af9743933eb8`.
Use [the runner guide](../mongoose-gameplay-runner.md) to reproduce the scenarios
with a fresh output directory; the original raw cohorts remain in the recording
workspace's ignored `.tmp` directory. Snapshots, credentials and private logs
are excluded from this source record.

Pinned inputs:

| Input | SHA-256 |
|---|---|
| October 12 23:49 Mongoose checkpoint | `fab6957acffa1db8bc1c4b911fb53a8e8d6e605f4de2a31b9f8e4fa874c3200d` |
| Real sound catalog | `ba15182e25c60e9795c10f8f9479095ff4d22ea9bf2a03d7d061446cfca241e4` |
| Accepted instrumented Toast executable | `f96c79de3ac701557c7fe71e2a9be42816c188d6d085afba1a7b4b3c4e7c7a01` |

Both01, with both diagnostic profilers off, completed one real connected
ordinary-player flight/turn/brake/cutoff sequence and one catalogue Lumos/Nox
light/extinguish pair. Magic had three attempts, including one natural fizzle.
The physical, admission, payment, refill and recovery checks passed. Its final
manifest and command results recorded:

```text
status: complete
flight.completed_iterations: 1
magic.completed_iterations: 1
shutdown.exit_code: 0
source_database/executable/controller_sources/sound_catalog/owned_shutdown: true
```

Flight's action window was 3.180644918s wall / 3.182204075s main-thread CPU;
magic's was 3.082636184s / 3.077598100s. Whole-process high-water RSS was
604636KiB. These include observer and scheduled background work; they are
neither exclusive action costs nor evidence of lower memory or faster engines.

Diagnostic02 also completed both scenarios with valid definitions-mode MOO
sampling and required native main-thread user CPU sampling. It recorded:

| Capture | Accounted MOO periods | Unknown or missing | Native samples |
|---|---:|---:|---:|
| Flight | 747 | 0 | 588 |
| Magic | 664 | 1 | 510 |

Its final status was complete, owned shutdown exit0, all five final integrity
checks true, and finalization_errors empty. Perf's raw requested SIGINT exit
remained `-2`; the collector retained successful disable acknowledgements,
verified ownership, nonempty data and report results. No target was signalled
by the native collector.

Offline symbol resolution checked recorded executable SHA/build ID, mapping
offsets, ELF load segments and finite GNU symbol bounds. It resolved 476/588
flight samples and 413/510 magic samples; shared-library and unresolved weights
remain in every denominator. Exact source lines were unavailable.

| Native self-IP function/subset | Flight | Magic |
|---|---:|---:|
| `db_fixup_owners(long)` | 105/588, 17.86% | 102/510, 20.00% |
| `moo_profile::refresh_stack(...)` | 70/588, 11.90% | 60/510, 11.76% |
| All identified profiler-specific symbols | 115/588, 19.56% | 105/510, 20.59% |

These are instrumented-run estimates. The profiler subset is not total overhead
or an off/on penalty. Native/MOO capture boundaries differ. The isolated world
retains inherited background tasks and native errors; full external-service
parity, repeated measurements, Barn acceptance and heap attribution remained
unproven in these cohorts. Failed cohorts were preserved rather than counted as
productive work.

The final tooling regression run recorded `Ran 94 tests in 0.152s / OK` in WSL.
Later commit-audit checks are recorded separately from this historical result.
