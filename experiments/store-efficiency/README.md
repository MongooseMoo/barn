# Store efficiency experiments

## Selected implementation

The change avoids copying data a transaction does not modify:

- Keep property-table maps absent while empty. Nonempty maps retain the original len-sized copy, which compacts maps after deletion.
- Allocate read-dependency maps on their first insertion, using the existing lazySet helper. Validation dependencies remain unchanged.
- Detach builder-supplied non-verb backing at numbered-object admission. Simple scalar/property publications then share unchanged fields. Property writes still copy the live property table; staged tables cannot replace live state because another transaction may have changed a different property.
- Keep full copying for live inheritance walks, relationships, definitions, renames, deletions, verb edits and recycling. Preserve the existing created-object and anonymous-object paths. Existing verb-backing ownership handling is unchanged.

## Final store measurements

Windows amd64, AMD Ryzen 9 5950X, Go 1.27.1, GOMAXPROCS=4, GOGC=100. Each binary ran in a separate process; baseline/candidate order alternated over ten paired rounds, 150ms per selected benchmark. Both used identical benchmark code via Go's build overlay. The baseline production code is integrated master after PR #597.

See `selected-delivery-base.txt`, `selected-delivery.txt` and `selected-benchstat.txt` for raw observations and statistics. These are microbenchmark results, not production throughput claims.

| Operation | Baseline | Selected | Time change | Allocated bytes change |
| --- | ---: | ---: | ---: | ---: |
| 128-slot scalar publication | 5.018 us | 2.977 us | -40.68% | -59.10% |
| 128-slot property publication | 7.695 us | 5.632 us | -26.81% | -24.80% |
| 1024-slot scalar publication | 23.628 us | 3.717 us | -84.27% | -91.42% |
| 1024-slot property publication | 41.56 us | 25.89 us | -37.71% | -31.74% |
| Clear loaded table clone | 91.89 ns | 37.23 ns | -59.48% | -50.00% |
| Sparse map after deletions | 1.160 us | 1.132 us | indistinguishable | unchanged |

The four publication time comparisons have p=0.000 as displayed by benchstat, n=10. This notation is the tool's rounded output, not a claim of zero probability. Variance is visible in the raw report; no timing improvement is claimed for the sparse-map case.

Publication fixtures use the actual loaded-property layout with private and clear slots plus populated children/contents. Scalar writes toggle an object flag; property writes replace one value. Setup is outside timing, so admission/load cost is measured separately with the real-database harness.

## Experiments and rejected variants

- `base.txt` / `clone.txt`: initial bulk maps.Clone experiment. Clear-table cloning saved one allocation, but nonempty-map timings were mostly inconclusive.
- Go 1.27's maps.Clone duplicates existing map tables. That retains capacity after property deletion, unlike the previous store clone. Bulk cloning was rejected; the selected implementation keeps only empty-map allocation elimination. Sparse/empty deletion cases were added to the shared benchmark fixture.
- `lazy-clone.txt` / `lazy-lazy.txt`: isolated lazy-map change. Read-only transaction allocations fell from 17 to 11 and 2864 to 2576 bytes; time fell 19.07% in that experiment (n=10, p=0.019). Write-path timing differences were inconclusive.
- `wide-base-wide.txt` / `wide-lazy-wide.txt` / `wide-final.txt`: publication-sharing experiment, including held readers. The filename `wide-final` denotes that experiment's third variant, which still included bulk maps.Clone; it is not the selected implementation. Use the `selected-*` files for delivered timing claims.

## Real-command measurement scope

The existing engine TestMongooseRealWorkload harness connects real players without passwords, dispatches real commands, and waits for terminal output acknowledgement. Runs use the same immutable Mongoose checkpoint and sound catalog, fresh processes, GOMAXPROCS=4, and the server's GOGC=400. Raw command/exception output stays in the local `.tmp` evidence directory; public files contain only metric lines. Throughput uses completed commands divided by actual elapsed time, including drain. Allocation ratios divide process-wide allocation deltas by completed commands; they include background work and are not attributed to individual commands or store operations.

The initial unchanged-baseline mixed-command run (`real-0-base.txt`) had three failures at four players and two at eight players. All recorded command failures were player 71's home command raising E_NACC. Read-only dump inspection found home points to object 62, named Generic gun, whose inherited acceptable verb returns zero. No store defect is inferred, and the fixture was not changed. The Go harness printed PASS, but these failed cohorts are excluded from performance claims.

Follow-up experiments compare the full command mix at one player and the existing look-only workload at one, four and eight players. These are explicitly narrower experiments; they cannot establish a successful multiplayer mixed-workload comparison. Each comparison has three alternating baseline/candidate pairs, with fresh worlds. The look experiment runs player levels in order within each world. The files are `mix1-{0,1,2}-{base,delivery}.txt` and `look-{0,1,2}-{base,delivery}.txt`.

| Workload / players | Baseline commands/s median (range) | Selected commands/s median (range) | Baseline / selected median bytes ratio | Baseline / selected median allocation ratio |
| --- | ---: | ---: | ---: | ---: |
| Full mix / 1 | 44.24 (41.27–45.92) | 45.80 (42.91–47.41) | 6,866,248 / 6,601,526 | 63,395 / 61,716 |
| Look / 1 | 29.12 (28.77–29.44) | 28.35 (27.69–29.90) | 11,795,871 / 11,774,409 | 100,489 / 99,942 |
| Look / 4 | 69.46 (60.88–76.92) | 68.64 (68.20–69.81) | 9,169,692 / 9,012,119 | 79,942 / 77,319 |
| Look / 8 | 170.72 (165.63–204.57) | 178.26 (135.07–192.18) | 3,282,514 / 3,333,271 | 26,413 / 28,272 |

All throughput ranges overlap; three worlds do not establish an end-to-end speedup. Full-mix process-wide allocation ratios were 3.86% lower in bytes and 2.65% lower in count at the medians, while eight-player look ratios increased. These observations are not statistically established production efficiency gains. The concrete speedups are the store microbenchmarks above.

Across these follow-up measured windows, baseline completed 8,005 commands and selected completed 8,475, with zero failed or unsettled commands in either lane. The eight-player look windows recorded two baseline and one selected process-wide uncaught exceptions. Private raw logs identify forked `s_run` tasks reporting `E_FLOAT` through `hull_temperature`; this counter is independent of command failures. Another baseline background exception occurred outside its measured window. No background exception was suppressed, and no whole-world error-free claim is made.

Across the six fresh worlds per lane, median load time was 14.560s baseline versus 14.535s selected (ranges 14.113–14.737s and 14.183–14.834s). Post-load allocated heap was 544 versus 541 MiB, but in-use heap increased from 690–696 to 701–702 MiB. Reserved heap varied from 1059–1228 versus 1115–1139 MiB. Post-window forced-GC allocated heap varied from 592–825 versus 587–772 MiB across differing completed workloads, so this does not establish a retained-memory improvement. Admission copying has no demonstrated load-time regression in this sample, but does shift allocation/layout costs.

## Provenance

Measurements used Go 1.27.1 on both Windows amd64 and Debian WSL Linux amd64, kernel 6.18.54.1-microsoft-standard-WSL2. Fixture SHA-256 values were unchanged before and after the runs:

| Artifact | SHA-256 |
| --- | --- |
| Mongoose checkpoint (`mongoose.db.new`, 221639217 bytes) | `ee84f1e848a4a2d22c2bcce1340d19f9cec7bdb752b416355d6ad4831e8f4f79` |
| Sound catalog (`sound.sqlite`) | `250b9b69fae2dad6788061a1abb029332f564b92e630514c93181030f7277ca9` |
| Baseline Windows store test binary | `435ccb9e93c6d61998348da21af3b8dbde8f0548cd282afe5affd8478515d2ef` |
| Selected Windows store test binary | `418a677feaa76f32c5ffb8e9a85e3ec859927e31830da94a3f2904ef0e460d0f` |
| Baseline Linux engine test binary | `8ad85fb22d19051271a0464cc06e972ab0e867e8a773e6d56c726a235f8a9e09` |
| Selected Linux engine test binary | `4ad757c91908d36fe1a0cc7f5c40e036e4fe7b2b86ffed6edfa9a84e1f063e9f` |

## Reproduction

Build baseline and candidate test executables with `go test -c ./db/store`, giving the baseline the candidate's benchmark-only files through a JSON `Replace` overlay passed with `-overlay`. This keeps one benchmark source and avoids modifying baseline production code. Use identical benchmark filters, GOMAXPROCS and GOGC, alternate executable order, and retain every stdout result. Run `benchstat` on the paired output files.

The final publication filter selects BenchmarkWidePublication at slots128/slots1024 with readerfalse. The clone filter selects LoadedClear, MapSparse, MapEmpty, LoadedGoneSparse and LoadedGoneEmpty at slots1024. Earlier files retain small-transaction and held-reader experiments.

For real commands, compile `go test -c ./engine` inside Debian WSL, run each executable from its checkout's engine directory, and set BARN_MONGOOSE_BENCH=1, BARN_MONGOOSE_DB, BARN_MONGOOSE_SOUND_DB, BARN_MONGOOSE_GOGC=400 and BARN_MONGOOSE_MEMSTATS=1. The full-mix experiment uses BARN_MONGOOSE_PLAYERS=1; the look experiment also sets BARN_MONGOOSE_ONLY=look and BARN_MONGOOSE_PLAYERS=1,4,8. Keep the default two-second warmup and eight-second measurement. Do not run builds or CI alongside these measurements.

Authored ownership/adoption regressions cover the changed publication and dependency behavior. The PR must also pass its normal unit/race, MOO conformance and pinned Elle workflows; those results are recorded by GitHub. Performance evidence does not substitute for those checks.
