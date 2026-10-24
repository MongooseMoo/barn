# Integrate Mongoose tooling and repairs with current master

## Goal

Publish and merge the existing Mongoose tooling/repairs on current Barn master,
and publish the existing numeric-preposition fixtures to the shared conformance
suite. Q explicitly authorized implementation with "Make it so!" on October 21.
Smallest change: reuse the 23 existing repair commits, integrate the 26 newer
master commits, and reuse the two existing fixture files (96 lines). Initially
zero new runtime lines; approximately 20 handoff lines change. Any required
source reconciliation must follow the actual overlapping code.

## State

- Integration worktree: `C:/Users/Q/code/barn-mongoose-integration`; branch `integrate/mongoose-tooling`, starting at repair tip `225bc93b`. Fetched master is `704a1b204221380a78f536ffd8ce7246303f9b91`; primary checkout and old evidence worktrees remain untouched.
- PRs are Barn #484 and shared-suite #134. October 22: Barn head `922b6be4` passes conformance/Elle but fails golangci-lint QF1001 at internal/app/app.go:148. Q authorized reading/fixing that failure with "Yup". Smallest repair is one equivalent boolean line, expanding the negation; no new guard or local CI run.
- Shared-suite head `a885465f9480a6a609d3c868ef7f08a4ef7b61fe` has successful existing CI, including all three Toast profiles. Source review and exact-head merge checks remain to be completed.
- The following measurements and checks describe the October 18 source-bound cohort, not the integrated October 21 tree. CI on the new PR must establish its current result.
- Go repair head `05f3680ccfcddf6a94e0d73ebc57a725d6ac17d9`: numeric prepositions validated/canonicalized; parent `82c04213` buffers the WAIF identity sidecar. Earlier immutable-source sharing remains in the ancestry.
- Controller head `f08fd946d2442c2d6cbad9c20032ab48d1599d29`: ENODATA-only readiness-log retry within the existing deadline; errors remain visible in bounded metadata.
- Tested binary SHA256 `d47d6ace45f9115198141a49a83eb8ccdc8140c15e15f0083422a1044cfe5a72`, built at Go head 05f368. All 697 Go/PGO/module inputs match the later Python-only controller tree. Keep those identities separate.
- Actual cohort: `.tmp/barn-magic-startup-observer-repair-20261018-01`; one completed Lumos/Nox pair, six preceding fizzles, two negative controls, eight receipts/debits, nonwizard player, fixture cleanup, shutdown 0/unforced and all ten preservation checks true.
- Original controller exit 1/status `diagnostic-unvalidated` is immutable. Separate offline parsing/domain audit exits 0; do not rewrite the original validation field.
- Historical full Go/race, pinned Elle strict audit and managed canonical/Barn conformance receipts are retained. Run current CI on the PR, without repeating its steps locally.

## Decisions and why

- Fix semantic termination before optimizing more setter work: Mongoose enumerates numeric prepositions until native error. Canonical oracle rejects 15; unrepaired Barn accepted every bounded probe through 32. The repaired actual magic completes.
- Buffer sidecar writes, retaining checked flush/sync/close: one 48,000-record WSL-mounted diagnostic fell from 21.697s to 0.193s untraced with identical bytes; n=1, not a gameplay speed estimate.
- Keep failed historical cohorts and raw manifests intact. The first combined repair failed on ENODATA before gameplay; it nevertheless published an intact checkpoint. The new actual did not encounter ENODATA, so it proves no real-system error recovery.
- October 21 replaces the earlier keep-isolated scope: Q now requests reintegration, PR publication and merge if valid. Reuse existing CI and shared fixtures; do not introduce new validation machinery. Stop and ask Q on a failed step.

## Surprises

- Actual CPU labels: `#1007:s_run` 15.76/20.64 CPU-seconds (76.36%) in a 15s multithread capture. Root labels describe task entry, not nested current MOO frames. Weave roots total 0.05 sampled CPU-seconds.
- Remaining terminal errors: 76 E_INVARG at `#2478:schedule_flush`, one at `#592:exitfunc`, one E_INVIND at `#0:user_client_disconnected`; reasons unknown, no terminal weave/E_MAXREC. One undeliverable-traceback ERROR class occurs in both older cohorts; exact task origin unavailable.
- Peak observed RSS 3,627,148 KiB; after natural heap profile 717,180,503 in-use bytes. Property setSlot cloning is the largest flat retained allocation site, 114.36 MiB. Sampling/GC/allocator/capture scopes differ; no retained-memory improvement is established.

## Next step

Master integration completed without conflict at `6fb2839d98a0921211720c5ee42e0bac0d695193`, with parents `2f979b19` and fetched master `704a1b20`. Lint correction `fb892565` is pushed and its three Barn CI checks pass. On October 24, root verified shared-suite PR #134 was merged by ctoth at `2026-10-24T19:36:02Z` as `c8a485b061ce29b449f13e0892485b51490638da`, with two parents and all current-head checks successful. Suite main is now `be876f59bd30815df46b7a318dc3e1bb0e6cc614`, also containing the resume-unrun coverage from #132; numeric fixture content still matches reviewed `f38be333`.

The earlier pause followed the verifier naming nonexistent `task/persistence.go` in a multi-file search (exit 2), contrary to the existing path-discovery instruction. Root confirmed `Test-Path=False` and retained the incomplete review rather than claiming completion. Q resumed work with "Keepagoin" on October 24; the same verifier is completing the existing convergence pass using discovered paths. The existing ledger's pass 1 covers 104 files/139 hunks. No replacement review, new guard or local CI run is added.

Publish this handoff-only update so final Barn PR CI runs after the shared fixtures reached suite main. Finish the existing current-head review gate, inspect final CI's actual suite checkout and numeric-case results, and merge PR #484 only if valid. Its earlier green run predates the merged numeric coverage and does not establish that final check. Preserve all
failed history and avoid a new gameplay speed or memory claim. Once integration
is merged, regroup around work rooted at `#1007:s_run` and property-slice lifetime
as separate bounded investigations; this integration adds no further optimization.

Evidence lives in the report/results and `.tmp/final-repair-review-20261018`: report SHA256 `b0781b13b1bac7a34ae0b9fea07cb3eba225a442f4ae1e81a1ac9842857b23e0`, audit summary `52f95285782bd81d7c2f185537cee5fd83eb8f0dd763a06e77cb63dff1df1886`. Original DB SHA256 `fab6957acffa1db8bc1c4b911fb53a8e8d6e605f4de2a31b9f8e4fa874c3200d`. Published pair: database `73a026ca627a311d4656de5c24717a3e31fa98e1772d492409e15627c4425d5d`, sidecar `ce2e44d033eb8834e7fc092bcb68a32cfbcdfa045701221bb1d2f1d6bbad28b0`.
