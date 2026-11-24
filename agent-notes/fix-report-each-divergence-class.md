# Fuzz reporter: one issue per divergence class

## Goal

Q 2026-11-24 chose "One per class": each distinct failure class in a
comparison lane's report gets its own issue and repair, instead of only the
cycle's first divergence.

## State

- Cause: scripts/fuzz_report.py `report()` matched only the first entry of
  `## divergence (N)`. Run 45396507340: 722 ToastDifferential divergences
  became one comment on #1124; every other class was hidden while #1124 sat
  unrepaired.
- Change: `report()` returns a list, one finding per class in first-seen
  order (signature `target|divergence|class`). An entry's class is its
  `- signature:` line; scripts/fuzz_differential.py now writes `toast <r> /
  barn <r>` (result_class: value, compile, or raised E_X), and
  scripts/toast_reads_barn_db.py writes the sorted finding kinds.
  scripts/fuzz_cross_engine.py already wrote one. `summarize()` and
  `memory_reports()` take the list; tests index `[0]` where one finding is
  expected.
- Tests OK (Windows, Python 3.13): fuzz_report, fuzz_differential (incl. a
  producer-to-reporter class test), toast_reads_barn_db, fuzz_cross_engine,
  memory_check, fuzz_compute, fuzz_builtin_census.

## Decisions and why

- Class signatures replace the old first-corpus-file signature, so open
  issues filed under the old one (#1124) get no more comments; the class gets
  a new issue once.

## Next step

After merge: the first cycle opens one issue per class, deduplicated by
signature across cycles.
