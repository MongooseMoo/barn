# Current campaign checkpoint 42

- Managed run `31018225886` completed red, yielding actionable oracle proof rather than a false green.
- Quality job passed.
- All three stock pinned Toast profiles built, admitted, and ran the complete packaged collection; each had exactly one failure: `language/anonymous.yaml::queued_tasks_returns_valid_anonymous_for_wizards`.
- Exact oracle result in all three profiles: expected two owned tasks but unfiltered `queued_tasks()` returned five total server tasks; the first was the test's delayed `c` task and the second was an unrelated `Input to EVAL` task.
- Narrow fix now in working tree: capture both fork task IDs, filter `queued_tasks()` to those IDs, assert only the two owned records, and kill the captured IDs. This repairs isolation rather than changing expected values for global contamination.
- Stock profile counts before the fix: 64-bit outbound on 11,551 passed / 35 skipped / 1 failed; outbound off 11,536 / 50 / 1; 32-bit outbound on 11,531 / 55 / 1.
- Example profile provenance toggle was exact but dormant Toast code failed compilation at three stale C-to-C++ points. No runtime cases executed in that profile.
- Issue 47 explicitly requires feature cases to execute in explicit pinned Toast profiles, so deletion, permanent absence branches, or skip exceptions are not acceptable shortcuts.
- Next: obtain adversary decision on a declared compiler-compatibility profile versus a separately pinned Toast source fix; verify the filtered queued-task fixture with the managed WSL command; then commit and rerun exact-head CI.
