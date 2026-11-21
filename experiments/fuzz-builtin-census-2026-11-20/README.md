# First builtin census (issue #978)

Produced by `scripts/fuzz_builtin_census.py` on the workstation, 2026-11-20.

Inputs:

- Differential evidence: artifact `fuzz-failure-ToastDifferential` of Nightly
  fuzz run 44852833568 (dedicated-continuous, head 12a7ba9e), its
  `results.json` (sha256
  `54dadfd7c47b9cb8e1d1010eebd383a4693173b8dc9f5dd652181cfb4f759fc3`):
  8630 eligible rows, 7936 completed pairs, outcome `divergence`.
- Toast: `/root/src/toaststunt/build-release/moo` in WSL Debian, the oracle at
  stock commit 03af4fa2 (`plans/barn-toast-mongoose-convergence-workstreams.md:39-40`).
  `function_info()` lists 238 builtins. Toast's source at that commit has 243
  `register_function` names; the five not in the build are `anon`,
  `background_test`, `finished_tasks`, `malloc_stats`, `read_stdin`.
- Conformance: a fresh clone of MongooseMoo/moo-conformance-tests at 8816b3f0,
  plus this repository's `tests/mongoose-conformance`.

Result (`builtin-census.json`): 5728 compared programs (completed on both
engines and compiled on both); 154 builtins called by at least one of them;
238 called by at least one conformance test (upper bound: the suite's
name-before-parenthesis scan); no builtin at zero on both, so the committed
`scripts/fuzz_zero_coverage_builtins.txt` lists none.

`builtin-census.tsv` has one row per builtin: compared programs, conformance
tests. Its 84 rows with zero compared programs, grouped by why the
differential never compares them (conformance test count in brackets):

- Generator exclusion, `internal/fuzzseed/structured.go:55-71,98` (37):
  every `file_*` (29), `argon2`, `argon2_verify`, `call_function`, `exec`,
  `getenv`, `log_cache_stats`, `read_http`, `run_gc`.
- Differential exclusion, `scripts/fuzz_differential.py:112-132` (21):
  `ctime`, `frandom`, `ftime`, `kill_task`, `load_server_options`,
  `object_bytes`, `queue_info`, `queued_tasks`, `random`, `random_bytes`,
  `resume`, `salt`, `seconds_left`, `server_version`, `suspend`, `task_id`,
  `task_stack`, `ticks_left`, `time`, `usage`, `value_bytes`.
- Both exclusions (6): `db_disk_size`, `dump_database`, `memory_usage`,
  `read`, `reseed_random`, `shutdown`.
- Capability never drawn (`structured.go:95-99` draws only `config.Core`):
  SQLite (9) `sqlite_*`; Network (5) `curl`, `listen`, `listeners`,
  `open_network_connection`, `unlisten`.
- Nothing excludes them (6, Core and public): `verb_info` [178],
  `descendants` [74], `parents` [37], `task_perms` [13], `trunc` [10],
  `spellcheck` [3]. In run 44852833568's rows, `spellcheck` appears once (a
  divergence: Toast compile error, Barn value), `trunc` and `descendants`
  once each as `unknown`, and the other three not at all.
