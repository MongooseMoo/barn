# Sync pool campaign checkpoint
- User authorized profiling, experiments, implementation where worthwhile, and PR publication; now asks to investigate all four candidates.
- Candidates: commit scratch, unique bookkeeping, file-read buffers, and finalization scratch; whole transaction pooling has lifetime hazards.
- Baseline HEAD and origin/master: 3c0f5105e30dd557734deac200ff0fec51faae8f; tracked main-tree changes absent at initial check.
- CI_RUNNER_LABELS currently reports ["self-hosted","Linux","X64","barn"]; no experiment worktree or source changes yet.
- Next: compare prior records, prepare isolated experiments and sealed benchmarks, profile all four candidates, retain only measured wins.

## Independent experiments underway
- Campaign branch/worktree perf/pool-campaign at C:/Users/Q/code/barn-pool-campaign; campaign record committed through 8f71a8e.
- Workers /root/commit, /root/builtins, /root/finalization own isolated worktrees; user clarified all worthwhile candidates, not one winner.
- Builtins baseline profiles show file string growth allocations larger than read buffer; authorized separate conversion-preallocation follow-on within measured scope.
- Measurement slots serialized: finalization baseline currently granted, commit baseline next, parent Mongoose profile next, then builtins paired confirmations.
- Main checkpoint remains uncommitted; no PR yet. Next verify each frozen gate, independently review/holdout, combine wins, run CI and publish.

## Baseline profiles and review
- Commit scratch rejected pre-implementation: about 1400/22634 B per 8-object commit (6.19 percent ceiling); published clone allocations dominate. Imported evidence only and reusable benchmark into campaign.
- Builtins completed ten paired runs each; analysis pending. Finalization source 6b72d0a ready for measurement next. Conversion preallocation follow-on prepared.
- Real Mongoose baseline profile passed: 184 commands/s, zero failed/uncaught, 2.30 MB/command at one player; profile includes startup allocations.
- Independent reviewer found missing same-class/different-content ownership coverage; no source bug yet. Reviewer tests/holdouts next after frozen measurements.
- Campaign HEAD 651575c includes isolated five-pair application driver; no PR or integration source yet. Serialized measurement slot moves from builtins to finalization.

## Independent gates
- Unique and file-read pooling passed ten-pair allocation gates (~76.85 and 19.91 percent savings); finalization passed ~12.95 percent bytes and ~16.5 percent runtime improvement.
- Reviewer test commit 91c5612 passed normal/race ownership tests; 60 holdout processes completed, analysis pending. Original source remains frozen.
- Combined text/binary preallocation rejected: mixed text64KiB regressed ~25 percent. Binary-only refinement prereg dc92217, source1afd45b, queued for final confirmation.
- Parent application baseline binary and disposable DB live in campaign .tmp; generated SQLite fixture moved safely under .tmp. No production source integrated yet.
- Next grant binary refinement measurement slot, review holdout gates, integrate all survivors, run five application pairs/full tests/CI, publish PR without merging.

## Combined validation running
- Campaign now includes all three passing pool changes, independent reviewer tests/60 holdout outputs, retained-heap snapshots, and negative conversion evidence/patches; HEAD 29485c7.
- Both conversion attempts rejected (text +25 percent runtime, binary-only +10.7 percent); no conversion source is active. Full Windows suite only fails TestDivergenceRuntimeIntrospection, identical E_QUOTA reproduced in unchanged baseline; vet passed.
- Five-pair Mongoose run active session44958 via frozen driver, fresh fixture per process; no concurrent benchmarks. Pair0 completed, pair1 candidate running.
- Pair0 candidate16-player throughput unusually low (73/s vs baseline364/s), zero failed/uncaught; collect all preregistered pairs before interpreting, no selective exclusions.
- Next finish application gate/diagnostics, run final checks and Linux CI, publish only if combined evidence supports it. No PR yet.

## Builtins experiment checkpoint
- Assigned independent unique scratch and file_read buffer experiments from baseline 3c0f510.
- Separate worktrees and frozen evaluator records will isolate both changes.
- Performance measurements require the parent's serialized measurement slot.
- Next: prepare representative builtin harnesses, baseline profiles, and preregistrations.

## Finalization experiment checkpoint
- Assigned finalization scratch experiment in barn-pool-finalization from baseline 3c0f510.
- Candidate is temporary root collection; direct accumulation must preserve deduplication and rooting.
- Performance measurements require the parent's serialized slot; no measurements made yet.
- Next: inspect lifecycle ownership, prepare tests and frozen evaluator, then measure baseline.

## Finalization ownership review
- Existing collector batches anonymous IDs before waifs; direct append could alter ordering.
- Prefer bounded pooled scratch with unchanged traversal/append helpers and cleared waif references.
- Development benchmark is PendingWaifLocals; PlainFrameFinalization and TemporaryFrameLifecycle remain holdouts.
- Waiting for serialized profiling slot; next prepare behavioral and scratch-reset contracts before preregistration.

## Finalization preregistration
- Baseline profile attributes 12.46% of allocated bytes to temporary root-slice collection.
- Contracts/evaluator sealed at d0c04c3; preregistration committed at d171cfe.
- Frozen gate: ten paired runs, allocation reduction lower interval >=10%, no >5% runtime regression.
- Next: implement bounded scratch reuse, run correctness contracts, request paired measurement slot.

## Builtins measurement checkpoint
- Unique and file_read pooling passed frozen allocation/runtime gates; records and retention snapshots committed separately.
- Combined text/binary exact-capacity conversion failed mixed-text runtime; binary-only confirmation also failed large printable binary runtime.
- Final CPU diagnosis completed; no further refinement or source promotion by worker.
- Next: commit evidence-only negative records and hand hashes to parent for integration review.

## Application paired run complete
- All ten application processes exited zero with no failed or uncaught commands.
- Independent review found median goodput ratios 1.094/0.893/0.900 for 1/4/16 players; sixteen-player candidate was slower in every pair.
- Three pooling changes remain integrated; application performance safety is unresolved under competing host load.
- Next: preserve all measurements, preregister controlled confirmation, and finish diagnostic profiles before PR publication.

## Application regression diagnosis
- Original ten logs and independent analysis committed at 5b2d8fa; short-window drain and state-history confounders documented without dismissing losses.
- Candidate memory and baseline CPU profiles exited zero; candidate CPU profile is still running in session 2287.
- Finalization worker prepared 13 gap benchmarks at 000d0fe; waiting for serialized measurement slot.
- Next: measure small/anonymous/oversized/concurrent roots, then one frozen longer sixteen-player confirmation with no replacement runs.

## Narrow finalization refinement
- Ten diagnostic pairs exposed zero-allocation anonymous Value regressions: one root +53%, eight roots +15%; Frame and mixed parallel paths improved.
- Worker owns serialized slot to test one refinement: pool direct WAIF Value only; retain original Value containers/anonymous path and pooled Frames.
- All three diagnostic application profiles completed; builtins worker is committing raw attribution evidence, without normalized throughput claims.
- Longer isolated sixteen-player script sealed at 94dafdb; final source/binary freeze waits for refinement result and independent review.

## Final source frozen for confirmation
- Narrowed source ea251f7 passes all fourteen paired benchmark guards; former anonymous Value regressions removed, primary bytes reduced 12.94%.
- Independent review074bb8d confirms normal/race/selection tests and recomputed gate; build/vet/source diff check all exited zero.
- Final engine binary SHA25612A4BDF1529E48BB66BA102832017EE1E9C8D6518D41BF675409F4BDB14DE1B4; prereg205fdb5 and analyzer074bb8d frozen.
- Next: run all twenty longer application processes without overlapping campaign workloads, preserve results, then publish PR and check exact-head CI.

## Confirmation seven pairs complete
- Session95178 runs frozen application confirmation; fourteen processes through pair6 finished exit0 with failed/uncaught zero.
- Raw logs are .tmp/application-confirmation; no source changes since ea251f7 and no analysis before all twenty logs.
- Host snapshot23:11 CPU100%; preserve uncontrolled-load limitation and every observation; eight Barn CI runners online, one busy with unrelated work.
- Next: complete remaining six processes, ask independent reviewer to run sealed analyzer, archive evidence, then PR and exact-head CI.

## PR346 published with final evidence
- All twenty final processes exited0; sealed independent result NONINFERIOR, paired ratio1.123825 CI[1.034822,1.167901], no failed/uncaught commands.
- Final evidence544f7a4 includes verified twenty-log zip and all observations; original unfavorable runs and refinement diagnosis preserved.
- Draft PR346 opened at head544f7a4794b5a6c1fca6dcfa9c93ba69dd6b4c63; base advanced to53c08a8 via unrelated memory_usage PR343; PR is mergeable with only three production files changed.
- CI run36297540201 has Build/unit and MOO conformance IN_PROGRESS; next inspect exact-head checks, resolve scoped failures if any, mark ready after success; never merge.

## PR346 ready and verified
- Exact head544f7a4794b5a6c1fca6dcfa9c93ba69dd6b4c63 CI completed SUCCESS: Build/unit1m36s and MOO conformance7m31s.
- Managed conformance raw summary:12643 passed,420 skipped,1 warning in392.98s; benchmark-driver suite19 tests passed.
- PR body includes measured gains, rejected variants, retention/host limits and exact-head CI; ready exit0, isDraft=false,state=OPEN,mergeStateStatus=CLEAN.
- Campaign worktree clean; no merge performed. All authorized investigation, implementation, experiments and PR publication complete.
