# ToastDifferential: tell an OOM kill from the harness's time-out

## Goal
Found while chasing #801 (Q 2026-11-15: "The hang that keeps coming back
seems rather important"): the fuzz guest's kernel log shows `barn` processes
of the ToastDifferential lane (listener 1) memcg-OOM-killed at 12.1 GB
anon-rss on 2026-11-14 17:57, 22:28 and 2026-11-15 05:17. The report listed
them as "timed out (harness killed the engine)".

## State
- Cause: `scripts/fuzz_differential.py` treated every exit 137 as the
  harness's kill (TIMEOUT_EXITS). Under coreutils timeout an engine killed by
  the OOM killer also makes the lane shell exit 137.
- Fix: run_streaming records whether it ran the kill; engine_runner marks
  exit 137 with no harness kill, before the bound, as `outside_kill`;
  run_bounded records that probe as KILLED (counted as aborted, like
  TIMED_OUT), and the report lists it under "killed".
- Run 43612256659: the killed Barn probes were F02634
  `while n (l + E_NACC + n + n) endwhile` (Toast `*Aborted*`) and F02750.
  Toast's OP_ADD on lists (execute.cc:1492, listconcat list.cc:277) checks no
  value limit, and neither does Barn's Concat (types/list.go:304): the list
  doubles each pass on both engines. Toast hits fg_seconds 2 first; Barn
  copies faster (memmove vs a var_ref per element) and passes 12 GiB (2^29
  elements x 24 bytes) before its seconds check. Same semantics as Toast;
  not changed here.

## Next step
- Merge; the next ToastDifferential report shows these under "killed".
