# Issue #270 checkpoint — 2026-10-07

- Updated #270 to deferred-finalization progress; original dispatcher work already landed in #398.
- Worktree `C:/Users/Q/code/barn-issue-270`, branch `fix/270-deferred-gc-progress`, baseline `34e6622`.
- New idle and overlapping-execution regressions both fail for their intended liveness gaps (`red_test_exit=1`); production is unchanged.
- Managed WSL Toast finalization selection is running (session 9634), with canonical admission; no terminal result yet.
- Next: finish oracle verification, implement an independent deadline-driven maintenance rendezvous, then lifecycle/race/conformance gates and paired measurements. No publication or merge authorized.

## Implementation checkpoint — 2026-10-07 evening
- Corrected Makefile Toast output from `.out` to documented `.new`; managed oracle rerun: 135 passed, 61 skipped.
- Red contracts and measurement plan committed as `274419e`; idle/overlap regressions pass with the new deadline worker (production edits uncommitted).
- Worker pauses admission outside task leases, uses nonblocking GC barriers, wakes after enqueue/startup release, and joins on Runtime.Stop.
- Shutdown fixtures assigned queues without locking/registration; migrated them to AdoptPendingFinalizations. Current check needs the now-unused kernel import removed.
- Next: safety/cancellation/root tests, paired retention/latency runs, full static/Go/race gates and managed conformance; still no publication/merge authority.

## Verification checkpoint — 2026-10-07 late evening
- Branch commits: `274419e` red contracts/plan, `5ae0bc6` production/safety tests, `114dd5f` paired measurement evidence; measured harness blob `21c7e32da501a8a975d755f4ad654129553e4ebc` is unchanged.
- Five alternating pairs retain 1000 baseline anonymous objects versus 0 candidate after 500 ms idle; anonymous command median paired cost +4.0%, allocations unchanged, no production speedup claimed.
- Full Go tests, vet/build, pinned staticcheck v0.8.1, Python bench-driver tests and store/engine/types race checks pass. Installed staticcheck v0.7.0 was incompatible with Go 1.27.
- Full managed Barn conformance is active in session 14689; suite tracked tree `7f05b70`, Linux binary SHA256 `343bb689...7722cc2`. Await its terminal inventory before changing code.
- Next: append final conformance evidence and commit the record; scope/source review complete. Generated gate logs/XML remain local; no push, PR, merge, or issue closure authorized.

## Delivery checkpoint — 2026-10-07
- Full managed Barn conformance completed: 12670 passed, 420 skipped, one JUnit-format warning; `managed_conformance_exit=0`.
- Final local head `e46c433` on `fix/270-deferred-gc-progress`; source tree is unchanged from tested/measured production commit `5ae0bc6` (`tested_source_diff_exit=0`).
- Record: `barn-issue-270/experiments/2026-10-07-deferred-gc-progress.md`, including full Go/race/static/managed results and five paired raw samples.
- #270 is updated and remains open; implementation is committed locally, with publication/merge unperformed. No remaining local verification work.
- This checkpoint stays uncommitted in the original checkout; generated diagnostics and the baseline worktree are preserved locally.

## Concurrent assessment checkpoint — 2026-10-08
- User accepted the concurrent comparison and asked what to do if latency is bad; no publication or merge authority added.
- Frozen plan and opt-in identical harness committed as `cf45a6f`; production tree remains unchanged. Five alternating pairs are running in session 3667.
- Initial pairs show existing rapid-allocation sweep stalls in both trees; longer-slice candidate pauses need the complete paired inventory before a conclusion.
- New raw samples are under `barn-issue-270/experiments/2026-10-08-deferred-gc-concurrent-evidence`; baseline copy is intentionally untracked.
- Next: collect the terminal result, assess added admission versus sweep delay, preserve evidence, and record a concrete merge recommendation and fallback.

## Concurrent decision checkpoint — 2026-10-08
- All ten frozen paired samples exited 0; longer-slice command maxima crossed the declared latency guard in pairs 3–5. Recommendation is to hold merge.
- Candidate profile attributes 1.85 s cumulative CPU to the frozen-candidate router, including 1.09 s checking already-handled IDs; evaluate an exhausted-list early exit first.
- New opt-in harness race run and engine vet exited 0. Assessment/evidence committed as `cb4c607`; production and managed gate trees are unchanged.
- Raw Windows logs preserve CRLF; their check passed with per-command `cr-at-eol`, and the assessment source check passed separately. Profile binary/data remain local.
- Next implementation step: preserve callback/context/frozen-root semantics while reducing repeated scans, then repeat the identical workload before publication or merge.

## Router implementation checkpoint — 2026-10-08
- User said Go; bounded router optimization is committed as `0aa7805`, with contract tests and frozen comparison plan committed first as `90a6183`.
- The five-line exhausted-list exit retains routing order/context and frozen-root behavior; focused VM and engine lifecycle checks exited 0.
- Five base/pre/optimized triples are active in session 65180 with identical harnesses; early samples remove request-count scaling and reduce rapid-allocation stalls.
- A live CPU snapshot showed unrelated Docker/Node load, so wall-clock responsiveness claims must retain that qualification; no user processes were changed.
- Next: collect the full comparison, run final Go/static/race and managed conformance gates, and record the resulting merge recommendation. No publication/merge authority added.

## Router measurement and verification checkpoint — 2026-10-08
- All thirty samples from five base/pre/optimized triples exited 0; the declared guard has zero optimized crossings in either scenario.
- Fixed 4096-request routing drops from 53–68 ms/op pre-router to 33–36 us/op optimized, with five allocations/op and the same acknowledged callback digest.
- Optimized long-case command maxima are 4.748–16.518 ms; rapid-allocation maxima remain 499.742–698.861 ms, below both matched controls. Shared-host load limits production inference.
- Full Go/static/build/Python and store/engine/VM/types race checks exited 0. Managed canonical Toast returned 135 passed, 61 skipped; full Barn conformance is active in session 23278.
- Next: preserve measured evidence, collect the full managed terminal summary, and finalize the source/record audit without publishing or merging.

## Router delivery checkpoint — 2026-10-08
- Full managed Barn conformance completed: 12670 passed, 420 skipped, one JUnit-format warning; `managed_conformance_exit=0`, JUnit zero failures/errors.
- Final local head `2077956`; production `0aa7805` is unchanged by later record commits (`final_tested_source_diff_exit=0`). Full Go/static/build/Python/race/oracle gates passed.
- Five-triple comparison has no declared guard crossings; exhausted-list routing no longer scales with already-handled requests. Bulk recycle pauses and shared-host inference limits remain documented.
- Record: `barn-issue-270/experiments/2026-10-08-deferred-gc-router.md`; all thirty samples are committed. Local gate logs/XML/profile binaries and original untracked user material are preserved.
- Scoped implementation is complete and suitable for PR/exact-head CI next. No PR, publication, or merge performed; this original-checkout note stays uncommitted.

## Publication and merge checkpoint — 2026-10-08
- User asked "shouldn't it be merged?"; publication and normal merge are now authorized and underway.
- PR https://github.com/MongooseMoo/barn/pull/429, head `2077956b983d430aebb658d22fc36e872ad0b1ee`, closes #270. Remote master remains `34e6622`; scope and ten commits audited.
- CI run `37815020268` initially failed before checkout because shared Debian `/dev/null` was a regular file. Existing workflow guard caught it. Restored only that device as character 1:3, mode 666; read/write probe passed, same-head failed jobs rerun once.
- Retry: Build & unit tests succeeded; MOO conformance is running. `CI_RUNNER_LABELS` already selects the eight online Barn Linux runners.
- Merge queue requires merge commits; wait for exact-head checks, enqueue with `--merge --match-head-commit`, verify queue CI, two parents, and issue closure. No merge yet.

## Merge completion — 2026-10-08
- PR #429 merged at `2026-10-08T17:30:38Z`; #270 closed one second later.
- Merge commit `8fbf3abef7747ccd9b01acf80632705f566a76b3` has parents `34e6622b8db8e476603f6ee806cdc12268a04c45` and PR head `2077956b983d430aebb658d22fc36e872ad0b1ee`. Fetched origin/master matches it; PR head ancestry exit 0.
- Exact PR CI `37815020268` and merge-group CI `37816181387` both succeeded. Each full managed conformance run returned 12685 passed, 420 skipped, one warning; CI reports fifteen more passes than the earlier local run.
- Queue tree equals PR tree (diff exit 0), preserving all ten issue commits. Push-triggered master CI `37817123204` is running on the same already-tested merge commit; no extra source changes.
- Publication, required checks, normal merge, and issue closure are complete. Original checkout and all untracked material are preserved.
