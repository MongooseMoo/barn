# Current checkpoint 48

- PR #49 exact head is now `a738975`; it remains NO-MERGE pending managed runtime proof.
- Third managed red run `31024044356` at `4548d60` again failed exactly two identities in every profile: `empty_list_is_green` and `empty_map_is_green`, expected 1 but returned 0; every other executable case passed.
- After three failed runtime repairs, implementation froze and investigator `/root/investigate_gc_cleanup_frame` separated the cause from guesses.
- Direct read-only verification against pinned WSL Toast exact commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`: `bf_run_gc()` only requests collection; server main loop later calls `gc_collect()`. Toast's own `test_garbage_collection.rb` empty list/map contracts use `run_gc(); suspend(0);` before `gc_stats()`.
- Current commit `a738975` adds only `suspend(0)` after each post-cleanup `run_gc()`. This retains the 24-case failure-isolation architecture and restores the scheduler boundary required by the Toast oracle. Local 335 harness units and diff check passed.
- Next action: wait for the new managed three-profile run. If green, fresh exact-head review, then audit and normal-merge PR #49 with two-parent verification. If red, stop and use the investigator's external per-test teardown alternative; do not manually launch Toast/Barn.
- PR #48 remains clean/local-only at `adc097d`, awaiting a valid PR #49 merge.
- Preserve dirty original checkout and untracked Barn notes; do not read historical project Markdown.
