# Toast reads a Barn-written database (issue #976)

## Goal

A machine that writes a database with Barn, loads that file in Toast, and
compares the whole world between the two engines (objects: flags, parents,
location, contents; properties: value/owner/perms incl. clear; verbs:
names/owner/perms/args/code; counts of queued and suspended tasks). It fails
when Toast refuses the file, when anything differs, or when the checkpoint
holds a task in Barn's frame form. Runs (1) on generated worlds every nightly
fuzz cycle via `scripts/fuzz_report.py`, (2) on the pinned Mongoose dump as its
own workflow, counts only.

## State

Sources read (CLAUDE.md, AGENTS.md, plan, scripts/fuzz_differential.py,
scripts/bench_differ.py, scripts/fuzz_report.py, scripts/fuzz_compute.cjs,
.github/workflows/fuzz.yml, native-checkpoint-census.{yml,sh},
internal/fuzzseed/world.go, cmd/moo-fuzz-source, internal/dbtool/offline_checkpoint.go,
engine/task_factory.go TaskSnapshots, Toast server.cc emergency_mode). Building.

Machine load before work (WSL Debian, 8 CPUs): `load average: 0.89, 1.36, 2.60`.

## Decisions and why

- Barn writes the file through its production checkpoint writer:
  `barn offline-checkpoint` (dbtool.WriteOfflineCheckpoint -> Runtime.TaskSnapshots
  -> dbformat.WriteCheckpoint), the same calls server.go dumpCheckpoint makes.
  `barn dump` was not used: dbtool.go:684-720 writes `NewWriter(...).WriteDatabase()`
  from the store only, with no tasks.
- No existing command runs MOO code and then writes a checkpoint, so generated
  worlds need one: `offline-checkpoint --run FILE` runs FILE as a background task
  of the first wizard until no task is ready, then writes the checkpoint.
- Toast reads the file in emergency mode (`-e`); `quit` makes Toast write its own
  copy (server.cc emergency_mode `quit` -> start_ok 0 -> db_shutdown), whose
  `N queued tasks` / `N suspended tasks` / `N interrupted tasks` lines
  (tasks.cc 1949-1993) are Toast's count of what it loaded.
- Mongoose dump is loaded by the Mongoose Toast build (MongooseMoo/toaststunt
  `72e3c7f96ce7a41fdeba793aef8818dc4408072e`, PROMOTE_NUMBERS on, options.h
  sha256 `6c855f6b...` = committed file, checked in WSL) because the plan
  (lines 43-47) names it the Mongoose oracle.

- `fuzzseed.ToastWorld` shares WorldHistory's renderer (`worldHistory(choices, toast)`).
  WorldHistory output is unchanged: sha256 over 5000 random choice records was
  `a666ad13...4d20` both before (git stash of world.go) and after the refactor.
- Toast's out.db task headers are read by appending a grep to bench_differ's
  toast lane script; the two lanes come from lane_scripts with separate copies.
- Object range: the written file's run of object headers `#0`, `#1`, ...
  (`# N recycled`); the walk's first record checks Barn's max_object() equals it.

- Nightly: three steps appended to fuzz.yml's ToastDifferential job, after the
  differ's reporter: `Compare Barn-written worlds in Toast` (id worlds, runs
  whatever the differ found), `Upload Barn-written world evidence`
  (artifact fuzz-failure-ToastReadsBarnDatabase), `Open world failure issues`
  (fuzz_report.py target ToastReadsBarnDatabase). The report's
  `## divergence` block has fuzz_differential's shape, so fuzz_report.py
  classifies it unchanged (test_toast_reads_barn_db ReportTest).
  fuzz_compute.cjs accept: either comparison step may fail, each must agree
  with its own results.json outcome; nothing else in the job may fail.
- Mongoose: `.github/workflows/toast-reads-mongoose-checkpoint.yml`, census
  pattern (fixture path and SHA-256 read from goodput-series.yml, copy in
  RUNNER_TEMP, removed at the end). Builds the Mongoose Toast from
  profiles/toast/mongoose-wsl-mongoose.json (ref + options.h checksum). The
  step prints only the script's stdout (counts, digests, Barn's format
  strings); stderr goes to a private file.
- Barn-form reasons in database mode are printed only as Barn's own Go format
  strings (`public_reason`): `frame %d (#%d:%s): <inner literal>`; anything
  else is `(not a fixed string in the Go source; not shown)`.
- Toast repairs: Toast prints VALIDATE phase banners on every load; any other
  `VALIDATE:` line is a repair and is a finding.

## Results

### Generated worlds, built-in seeds (scripts/testdata/toast-world-seeds), stock Toast

`python scripts/toast_reads_barn_db.py worlds --out .tmp/trbd-1` (load before: `1.90, 1.75, 2.22`):

```
- outcome: success
- worlds: 11; compared: 11; failed: 0
- harness errors: []
- unicode worlds (a string above 127): 1
- base database: `...Test_conf.db` sha256 `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`
- toast binary: `/root/src/toaststunt/build-release/moo` sha256 `61dd1799...13dd` version `ToastStunt version 2.8.0_7`
```
Every world: 11 objects, 34 walk records, tasks barn wrote/barn read/toast read = [1 queued, 1 suspended].
Wall 1m59s on Windows (each step is a `wsl` call); per-step cost on a Linux runner not yet measured.

Unicode: the text-utf8 world stores `"ü€😀é"` in a property, an object name and
verb code. Toast printed the same bytes as Barn in every walk record (no
difference). `length()` of the same value is 2 in Toast for `"é"` (bytes) and 1
in Barn (characters): by design, and not part of the walk.

### Planted failures (real engines; scratchpad plant.py monkeypatches run_wsl)

- value: `sed` renames `System Object` in Toast's private copy after the three
  copies' digests matched (`bed129e6...9457`, 4409 bytes each). Output:
  `FINDING walk` / `1 walk records differ; first O0` /
  toast `{"O0", "Planted Object", ...}` vs barn `{"O0", "System Object", ...}`.
- task: world `n = 1; m = n; while ((n = suspend(100)) == 0) endwhile ...`. Output:
  `FINDING barn-form task`, `FINDING toast refused the file`, reason
  `frame 0 (#0:): which variables Toast holds unbound at the call depends on the path into its block`;
  Toast: `READ_ACTIV: Bad PC for suspended task.` ... `DB_LOAD: Cannot load database!`, exit 1.

### Two copies at once

Two `worlds` runs started together (`.tmp/trbd-copy-a`, `.tmp/trbd-copy-b`, same
barn binary, while the Mongoose run and CI jobs were loading WSL, load
average 14-21): both `outcome: success`, `worlds: 11; compared: 11; failed: 0`,
`EXIT 0`. Private mktemp directories per engine process and a per-run `--out`.

### Local FuzzWorldCheckpointEffects cache (30 records) plus seeds

`.tmp/trbd-cache`: `outcome: success`, `worlds: 41; compared: 41; failed: 0`.

### Mongoose dump, first run (counts only; old walk)

Writer: `read: 93 queued, 12 suspended or interrupted tasks, 10 connections`,
`wrote: 93 queued, 12 suspended or interrupted tasks, 10 connections`; readback
identical; Toast wrote `93 queued`, `12 suspended`, `0 interrupted`. Writer JSON
log: 0 WARN records (no Barn-form task). 289698 objects, 869096 records on each
engine, both lanes exit 0. Differing records: P 24938, V 6. The script then
crashed in go_formats (fixed; formats are now built before the long run).
Classified privately by shape only:
- 20028 P: same (definer, name) set, definers in a different order; names
  within each definer in the same order. Cause: ancestors() order (finding 1).
- 4841 P: identical once WAIF spelling is normalized (finding 2).
- 62 P: values differ beyond WAIF spelling; 7 P: Barn lists properties Toast
  does not. Not yet classified; rerun with the final walk pending.
- 6 V: verb code calls a builtin the Mongoose Toast lacks (`parse_ast`,
  `validate_string_lines`): Barn's verb_code shows the call, Toast's shows
  `call_function("parse_ast", ...)` (finding 3).
- The 84 Toast `VALIDATE:` lines were all `Done through #N ...` progress, not
  repairs; REPAIR_RE now excludes them.

## Findings

1. Barn's `ancestors()` order differs from Toast's for multiple parents.
   Reproducer on copies of Test_conf.db, stock Toast emergency mode vs `barn eval`:
   `c = create(#1); b = create(#1); a = create(c); o = create({a, b}); return {ancestors(o), a, b, c};`
   Toast `=> {{#10, #8, #1, #9}, #10, #9, #8}` (depth first); Barn
   `=> {{#10, #9, #8, #1}, #10, #9, #8}` (breadth first). The walk now sorts definers.
2. `barn eval` prints a WAIF as `<waif #8>`; Toast emergency mode prints
   `[[class = #8, owner = #3]]`; `toliteral()` agrees on both. Reproducer:
   `c = create(#1); add_property(c, ":v", 1, {#3, "rw"}); add_verb(c, {#3, "xd", "new"}, {"this", "none", "this"}); set_verb_code(c, "new", {"return new_waif();"}); w = c:new(); return {w, toliteral(w), {w}, w.v};`
   Toast `=> {[[class = #8, owner = #3]], "[[class = #8, owner = #3]]", {[[class = #8, owner = #3]]}, 1}`,
   Barn `=> {<waif #8>, "[[class = #8, owner = #3]]", {<waif #8>}, 1}`.
   The walk now compares property values through toliteral(), which shows a
   WAIF's class and owner only: WAIF property values are not compared.
3. Verbs that call builtins Barn has and the Mongoose Toast lacks load in Toast
   as `call_function("name", ...)` calls (6 verbs on the dump). They load, but
   fail at run time on Toast.
4. Indefinite suspend start time (below, Surprises).

## Surprises

- First try at sorting definers used `sort()`: Toast runs it threaded, so every
  P input printed `=> *Suspended*` and Toast's saved copy held 12 suspended
  tasks; the machine failed all 11 seeds on `task counts differ` (`toast read
  [1, 12]`). Replaced with an insertion sort in MOO.

- Barn writes an indefinitely suspended task's start time as `4611686018427387904`
  (2^62, task/task.go:915 IndefiniteSuspendStartTime); Toast writes `INTNUM_MAX`
  (tasks.cc enqueue_suspended_task `when.tv_sec = INTNUM_MAX`). Toast loads Barn's
  value as a timed wake at 2^62 s (queued_tasks() shows `4611686018427387904`
  where a Toast-written file shows 9223372036854775807). The task still never wakes
  on its own. Not compared by the machine (it compares task counts). Reproducer:
  seed `action-0`, `grep -A1 'suspended tasks' barn.db.new`.

## Next step

Unit tests, fuzz.yml step, accept job, Mongoose workflow and run, two copies.
