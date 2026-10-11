WAIF coder checkpoint 2026-09-21: isolated correctness worktree; no commits or database edits.
Implemented initial immutable images, store-domain ownership, private staging, coarse mixed validation/publication, journal removal, snapshot alias freezing.
Initial focused store RED was undefined new APIs; current `go test ./db/store -run '^TestWaif' -count=1` output: `ok github.com/MongooseMoo/barn/db/store 0.726s`.
Engine focus temporarily blocked by parent's in-progress StartEval API; parent notified.
Next: retained-image semantic GC traversal, VM/engine regressions, concurrent staging refinement, broader unit/race verification.

WAIF checkpoint: semantic GC now walks retained WAIF images; checkpoint and containment keep their distinct committed/private views.
Focused store tests passed; deterministic engine concurrent increment test output `ok github.com/MongooseMoo/barn/engine 0.149s` with committed value held at zero while both writes stage.
Full types/store/VM/kernel/builtins packages passed; full engine test still running while parent repairs intrinsic eval routing.
New nested-eval/suspend test currently fails E_PERM because the fixture lacks programmer permission; correcting fixture before asserting behavior.
Next: suspend/irreversible tests, race run, history/publication cleanup and independent review handoff.

WAIF checkpoint: scoped engine concurrency, nested eval/suspend and irreversible-boundary tests pass; focused race packages types/store/VM/engine pass.
Full engine rerun now bounded at three minutes with JSON output outside Git; still awaiting terminal evidence.
Scout found historical GC traversal accidentally reused for checkpoint pending-root ownership.
Deterministic new RED: TestWaifHistoricalEdgeDoesNotRemoveCheckpointPendingRoot fails `historical edge removed pending root: []`.
Next: separate committed serialization walkers from retained semantic-GC walkers, verify RED goes GREEN, report final scope to parent and scout.

WAIF checkpoint: checkpoint pending-root RED fixed by current-only ownership walks; retained images remain semantic GC roots only.
Read-only cache lifecycle RED reproduced recycle count zero; completion/suspension now release old txn before finalization and focused engine passes0.494s.
Full types/store/VM packages pass; latest focused race outputs store2.026s, VM1.231s, engine1.231s.
Preserved reference-graph memo on scalar WAIF writes via image-reference comparison; RED scalar invalidation test now passes types0.465s/store0.729s.
Parent owns full engine/conformance/benchmark runs; coder preparing report and awaiting scout re-review, with no commits or DB handling.

WAIF checkpoint: completed caller audit across Store/VM GC, checkpoint, shutdown and pending-finalization paths.
Committed-only Store APIs now serve VM shutdown canonicalization; current vs retained traversal is explicit throughout ownership paths.
Direct/persistent WAIF and anonymous historical-child regression cases pass; focused packages raw store0.760s/types0.443s/VM0.139s/engine0.148s.
Coder report written and inspected in correctness worktree reports/waif-transactions-coder-20260921.md; final race run session80987 awaiting terminal output.
Next: return scoped completion, with parent responsible for complete exact-source CI, managed conformance and benchmarks.

Prerequisite prune-race fix completed in barn-history-prune-20260921; local commit 853cba51f8d623353b37328f68f4160591bbb24b contains only Store WAIF source and its test.
Deterministic RED paused the floor scan after shard0, then observed Release return or block on store.mu before resuming publication: obsolete image and registry1pendingtrue remained.
Fix advertises weak history and pendingtrue before the floor sample, removes unneeded history afterward; focused GREEN0.610s and race/history tests1.909s, exit0.
Parent's untracked waif_prune_watermark_test.go remains untouched and uncommitted; no optimization implemented, no merge/push/database handling.
Next: parent freezes prerequisite baseline and prepares independent optimization experiment contracts.
