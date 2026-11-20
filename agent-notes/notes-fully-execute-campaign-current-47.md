# Current checkpoint 47

- PR #49 remains NO-MERGE. Exact head is now `4548d60` after two managed Toast red runs exposed GC-measurement contamination introduced by the failure-isolation frame.
- Run `31022805832` at `c62d82b` failed only `empty_list_is_green` and `empty_map_is_green` in all three Toast profiles; all other packaged cases passed. Both expected 1 and returned 0.
- Commit `6627107` moved those two `gc_stats()` measurements after `endtry`, retaining failure cleanup. Managed run `31023465439` proved that unwinding alone was insufficient: the identical two failures remained in all profiles, with no other failures.
- Exact managed totals in the second red run: 32-bit 11,529 passed/49 skipped/2 failed; 64-bit outbound-off 11,533/45/2; 64-bit outbound-on 11,548/30/2.
- Current commit `4548d60` adds `run_gc()` immediately after each cleanup frame, restoring the intended ordering: unwind cleanup, collect, create empty list/map, inspect purple roots. Local 335 harness units and diff check passed.
- Next action: wait for managed run on `4548d60`; do not merge or manually launch Toast/Barn. If green, obtain fresh exact-head review, audit, normal-merge PR #49, and verify two-parent merge. If red again, use an investigator before further changes.
- PR #48 remains clean/local-only at `adc097d` and must not be pushed until PR #49 is properly merged.
- Preserve dirty original checkout and untracked Barn notes. Do not read historical project Markdown.
