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

Built, merged with origin/master (2026-11-21), committed locally; not pushed,
no PR. Generated worlds run in fuzz.yml's ToastDifferential job each cycle and
the accept job judges that step with its own results; the Mongoose workflow
builds the 0ff6fd7 Toast (the live game's) and prints counts only. Unit tests
pass. On the Mongoose dump the machine reports 8 differing P records, all
explained by findings 6 and 7 below (engine differences, recorded, not fixed
here), so the Mongoose workflow will be red until those are fixed in Barn.
See "Resumed 2026-11-21" for what was verified and how.

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
  at profiles/toast/mongoose-wsl-mongoose.json's implementation_ref, PROMOTE_NUMBERS on, options.h
  sha256 `6c855f6b...` = committed file, checked in WSL) because the plan
  (lines 43-47) names it the Mongoose oracle. SUPERSEDED 2026-11-21: the
  workflow now builds 0ff6fd7 (profiles/toast/mongoose-0ff6fd7-wsl-testdb.json),
  the build the live game runs; see Resumed.

- `fuzzseed.ToastWorld` shares WorldHistory's renderer (`worldHistory(choices, toast)`).
  WorldHistory output is unchanged: sha256 over 5000 random choice records was
  `a666ad13...4d20` both before (git stash of world.go) and after the refactor.
- Toast's out.db task headers are read by appending a grep to bench_differ's
  toast lane script; the two lanes come from lane_scripts with separate copies.
- Object range: the written file's run of object headers `#0`, `#1`, ...
  (`# N recycled`); the walk's first record checks Barn's max_object() equals it.
  CHANGED 2026-11-21: anonymous objects continue that run, so it is the walk's
  upper bound and the check is that Barn's max_object() is not past it.

- Nightly: three steps appended to fuzz.yml's ToastDifferential job, after the
  differ's reporter: `Compare Barn-written worlds in Toast` (id worlds, runs
  whatever the differ found), `Upload Barn-written world evidence`
  (artifact fuzz-failure-ToastReadsBarnDatabase), `Open world failure issues`
  (fuzz_report.py target ToastReadsBarnDatabase). The report's
  `## divergence` block has fuzz_differential's shape, so fuzz_report.py
  classifies it unchanged (test_toast_reads_barn_db ReportTest).
  fuzz_compute.cjs accept: either comparison step may fail, each must agree
  with its own results.json outcome; nothing else in the job may fail. Since
  the 2026-11-21 merge this is a second COMPARISONS entry on the
  ToastDifferential job (key ToastReadsBarnDatabase) beside master's
  ToastCrossEngineCheckpoint job.
- Mongoose: `.github/workflows/toast-reads-mongoose-checkpoint.yml`, census
  pattern (fixture path and SHA-256 read from goodput-series.yml, copy in
  RUNNER_TEMP, removed at the end). Builds the Mongoose Toast from
  profiles/toast/mongoose-0ff6fd7-wsl-testdb.json (ref + options.h checksum;
  was mongoose-wsl-mongoose.json until 2026-11-21). The
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
  does not. Classified on the 0ff6fd7 rerun (Resumed): findings 5, 6, 7.
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
5. Toast orders a map's WAIF and anonymous-object keys by memory address
   (0ff6fd7 src/utils.cc compare(), `case TYPE_WAIF` / `case TYPE_ANON`:
   `(uintptr_t)lhs.v.waif < ...`), so the order of those keys in toliteral()
   is not part of the file and cannot match Barn's. 47 of the 55 differing P
   records on the dump were exactly this (each a character permutation of the
   other, all in records holding WAIFs, all equal after the fix).
6. A property defined with a builtin property's name (Toast include/db.h
   BUILTIN_PROPERTIES: name, owner, programmer, wizard, r, w, f, a, location,
   last_move, contents): Toast's property_info() raises E_PROPNF
   (src/property.cc:85 `if (!h.ptr || db_is_property_built_in(h)) return
   make_error_pack(E_PROPNF)`), Barn returns the defined property's info
   (builtins/properties.go:71 findPropertyForRead, no builtin-name check), and
   Barn's `o.(p)` then reads the builtin. Toast logs `DB_WARNING: Property #N.x
   has a reserved name` at load (src/db_file.cc:276). 7 P records on the dump
   (the walk's P record of each affected object raises on Toast). Reproducer:
   Test_conf.db with #2's propdef/propval counts (1-based lines 94-95, both
   `0`) replaced by `1`, `location`, `1`, `0`, `5`, `3`, `1`, run through
   run_case (scratch reserved.py): Toast
   `=> {"P2", "raised", E_PROPNF, "Property not found"}`, Barn
   `=> {"P2", {{#2, "location", {#3, "r"}, 0, "#-1"}}}`.
7. A value past a size limit with `max_concat_catchable` unset (shown with
   `max_list_value_bytes`; which limit the dump's record hit was not checked):
   Toast aborts the task (src/list.cc make_space_pack:
   `make_abort_pack(ABORT_SECONDS)` unless SVO_MAX_CONCAT_CATCHABLE;
   include/options.h 359-362), Barn raises a catchable E_QUOTA (no production
   Go code reads max_concat_catchable; only engine/server_options_*_test.go
   set it). 1 P record on the dump (Toast `*Aborted*`, Barn
   `{"P..", "raised", E_QUOTA, ...}`), so that object's properties are not
   compared. Reproducer on Test_conf.db, stock lanes (scratch quota.py), Toast
   0ff6fd7 with `;;` inputs vs `barn eval`:
   `add_property($server_options, "max_list_value_bytes", 200, {#3, "r"}); return load_server_options();`
   then `r = {}; try for i in [1..100] r = {@r, i}; endfor return length(r); except e (ANY) return {"raised", e[1]}; endtry`:
   Toast `=> *Aborted*`, Barn `=> {"raised", E_QUOTA}`.

Which the machine compares now:
1. ancestors() order: not compared (definers are sorted before the walk prints them).
2. barn eval's WAIF spelling: not compared (values go through toliteral()); a
   WAIF's own property values are not compared either.
3. call_function lowering: compared (V records carry verb_code). Not seen on
   0ff6fd7: 0 differing V records on the rerun, where 72e3c7f gave 6.
4. Indefinite-suspend start time: not compared (task counts only).
5. WAIF/anonymous map-key order: not compared (same_record sorts each map's run
   of those keys before comparing P values); everything else in the value is.
6. property_info on a builtin-named property: compared, reported (7 records).
7. max_concat_catchable abort vs E_QUOTA: compared, reported (1 record); the
   affected object's properties themselves go uncompared while it stands.

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

The lead publishes the branch (PR body draft at the end of this note). After
that: findings 6 and 7 are Barn fixes for their own branches, each starting as a
Toast-passing conformance test; the Mongoose workflow stays red until both land.
Not verified: any run on a Linux CI runner (all runs here were on the Windows
workstation through `wsl`), the workflow's wall time there, and the nightly
cycle end to end.

## Resumed 2026-11-21

Resumed by a new agent after the previous one hit a usage limit. Plan: merge
origin/master, read both commits, finish what Next step lists.

- On resuming, the fuzz.yml steps, the accept change, the Mongoose workflow
  and the unit tests already existed in the two commits; "Next step" above was
  stale. What remained: the merge, the Mongoose pin, a Mongoose run with the
  final walk, and a two-copies check covering the Mongoose step.
- Merged origin/master (merge commit). Conflicts, both sides kept:
  cmd/moo-fuzz-source (master's structured-ascii generator and --builtins,
  this branch's toast-world generator); fuzz_differential.render_choices keeps
  this branch's generator parameter with master's structured-ascii as its
  default, so the differ's call is master's; fuzz_compute.cjs accept keeps
  master's builtin census check and this branch's two-comparison check;
  test_fuzz_compute.accept() takes both `builtins` and `worlds`. Verified:
  `go build ./...`; `go test ./cmd/moo-fuzz-source ./internal/fuzzseed`;
  `go test ./internal/dbtool -run OfflineCheckpoint` ok;
  `python -m unittest discover -s scripts -p test_fuzz_compute.py` 34 OK,
  `-p test_toast_reads_barn_db.py` 11 OK, `-p test_fuzz_differential.py` 38 OK.
- Toast pin for the Mongoose workflow changed from mongoose-wsl-mongoose.json
  (72e3c7f, the plan's line 46) to mongoose-0ff6fd7-wsl-testdb.json: the lead's
  brief names it, docs/mongoose-conformance.md:63 calls 0ff6fd7 "the commit the
  live game runs", and toast-check.yml checks the same dump against it. Finding 3
  was seen on 72e3c7f, which lacks the AST builtins (0ff6fd7's manifest has
  `feature.ast: true`); it is rechecked on 0ff6fd7 below. WSL build present:
  `/root/src/toaststunt-mongoose-0ff6fd7/build-release/moo`, HEAD = the profile's implementation_ref,
  options.h sha256 `d9739c2d...` = manifest. Load then: `6.42, 9.20, 5.85` (8 CPUs).
- Two copies, database mode (the Mongoose workflow's command): two
  `toast_reads_barn_db.py database Test_conf.db --toast <0ff6fd7 moo>
  --barn-linux .tmp/trbd-bin/barn_linux --out .tmp/trbd-db-copy-{a,b}` started
  together as PowerShell jobs, while the Mongoose run below was also running
  (three at once; load after `11.14, 9.81, 6.35`). Both: `outcome: success`,
  `findings: []`, 8 objects, 25 records, tasks [0,0] on all three counts, 0
  repairs, identical file digests, EXIT 0. The dump itself was not run twice
  at once (too heavy for a loaded workstation); the code path is the same and
  every engine process works in its own mktemp directory under a per-run --out.
  Toast 0ff6fd7 binary sha256 `1c0e5128...5a53`, version `ToastStunt version 2.8.0_1`.
- Merged branch, generated worlds (`toast_reads_barn_db.py worlds --barn-linux
  .tmp/trbd-bin/barn_linux --out .tmp/trbd-merged`, stock Toast, load before
  `10.51, 9.83, 6.50`): `outcome: success`, `worlds: 11; compared: 11; failed: 0`, EXIT 0.
- Seen on the dump run: Barn's writer also writes `barn.db.new.waifids`
  (db/format/waif_identity_sidecar.go:15, read back only when its header
  digest matches the database). The machine copies only `barn.db.new` to every
  reader, so Barn's walk and readback load without the sidecar, as Toast does:
  the comparison is like for like, and whatever the sidecar carries is not compared.
- Mongoose run 1 on 0ff6fd7 (`toast_reads_barn_db.py database <goodput-series
  FIXTURE> --toast /root/src/toaststunt-mongoose-0ff6fd7/build-release/moo
  --barn-linux .tmp/trbd-bin/barn_linux --out .tmp/trbd-mongoose-0ff6fd7`,
  01:27-01:41, EXIT 2): input sha256 `626bd284...25e7` (= FIXTURE_SHA256), Barn
  wrote 432301704 bytes, all three copies the same digest; tasks barn wrote /
  barn read / toast read all `[93, 12]`; 0 Toast repairs; `outcome:
  inconclusive`, harness error `barn's max_object() is not #289697, the file's
  last object header`. Cause (harness, not engines): both engines' max_object()
  is #287625; the file's header run goes on to #289697 because write_db_file
  (0ff6fd7 src/db_file.cc 1060-1072) writes the anonymous objects after the
  permanent ones as further batches numbered on from them, and the line
  before `#287626` in Barn's file is the batch count `2072`. object_count()
  counted those too. Fix: object_count() is the walk's upper bound (anonymous
  numbers read as invalid on both engines), the harness check is now "Barn's
  max_object() is not past the last header", Toast's M0 record is compared
  like every other record, and the report gives `object headers` and
  `objects` (Barn's max_object()+1) separately. New unit test
  test_object_count_includes_anonymous_batches; 12 OK. Anonymous objects
  themselves are not walked (no number reaches them).
- Second merge of origin/master (11 commits behind): clean, merge commit.
  After it: `go build ./...` ok; unittest `test_toast_reads_barn_db.py` 12 OK,
  `test_fuzz_compute.py` 34 OK, `test_fuzz_differential.py` 38 OK; `go test
  ./cmd/moo-fuzz-source ./internal/fuzzseed` and `./internal/dbtool -run
  OfflineCheckpoint` ok.
- Workflow linters (ci.yml:142-143 runs them on fuzz.yml and fuzz-controller.yml
  only): `go run .../actionlint@v1.7.12` on fuzz.yml, fuzz-controller.yml and
  toast-reads-mongoose-checkpoint.yml: exit 0, no output. `uvx --from
  zizmor==1.32.0 zizmor --offline --no-progress` on CI's two files: "No findings
  to report (1 ignored, 27 suppressed)", exit 0; on the new workflow (outside
  CI's scope): "No findings to report (1 suppressed)", exit 0.
- Mongoose run 2 on 0ff6fd7, after the object_count fix (same command,
  03:37-~03:52, load before `10.10, 13.19, 13.87`, EXIT 1): input sha256 =
  FIXTURE_SHA256; Barn wrote 432301704 bytes, all three copies one digest;
  `object headers` 289698, `objects` (max_object()+1) 287626, 869095 records
  compared; tasks barn wrote / barn read / toast read all `[93, 12]`; 0 Toast
  repairs; `outcome: divergence`, differing records `{"P": 55}`, 0 V, 0 O.
  The Toast load log held one `DB_WARNING: ... has a reserved name` line.
  Classified privately by shape (scratch scripts printing counts and masked
  structure only): 47 records are character permutations of each other inside
  records holding WAIFs (finding 5), 7 are Toast `raised E_PROPNF` where Barn
  lists the object's properties (finding 6), 1 is Toast `*Aborted*` vs Barn
  `raised E_QUOTA` (finding 7). Each class was then reproduced on Test_conf.db
  or read in Toast source (findings 5-7).
- Fix for finding 5 in the machine: parse_literal/canonical/same_record in
  toast_reads_barn_db.py compare each P value with every map's run of WAIF or
  anonymous keys sorted; anything that does not parse stays a difference.
  Unit tests RecordTest (3 tests); `test_toast_reads_barn_db.py` 15 OK.
  Applied to run 2's saved transcripts (not a third dump run): 47 records now
  agree, 8 still differ (7 finding 6, 1 finding 7). The transcripts were then
  deleted; no copy of the dump or its transcripts is left under .tmp.
- Third merge of origin/master (PRs 1032, 1036 and others): conflicts in
  offline_checkpoint.go/_test.go, app.go, cmd/barn/main.go (master's
  --toast-loadable beside this branch's --run: WriteOfflineCheckpoint(out,
  path, options, omitSuspended, toastLoadable, run); master's new
  toast_loadable_test.go callers given `""`), fuzz.yml (this branch's world
  steps in ToastDifferential beside master's new cross-engine job; accept
  downloads both artifacts), fuzz_compute.cjs (master's COMPARISONS list kept;
  the worlds step is a second entry on the ToastDifferential job, and a job's
  comparison steps may each fail with a finding while no other step may;
  problems are de-duplicated), fuzz_differential.py (master's text),
  test_fuzz_compute.py (both helpers' arguments). Verified after it:
  `go build ./...`; `go vet ./internal/dbtool ./internal/app ./cmd/barn
  ./cmd/moo-fuzz-source`; `go test ./internal/dbtool ./cmd/moo-fuzz-source
  ./internal/fuzzseed` ok; unittest test_toast_reads_barn_db.py 15 OK,
  test_fuzz_compute.py 36 OK, test_fuzz_differential.py 38 OK; actionlint
  v1.7.12 on fuzz.yml, fuzz-controller.yml, toast-reads-mongoose-checkpoint.yml
  exit 0; zizmor 1.32.0 offline on CI's two files "No findings (1 ignored, 28
  suppressed)" exit 0, on the new workflow "No findings (1 suppressed)" exit 0;
  generated worlds on stock Toast with a fresh linux barn (load before
  `19.69, 21.32, 13.70`): `outcome: success`, `worlds: 11; compared: 11;
  failed: 0`, EXIT 0.

## PR body draft

Title: Check that Toast loads a Barn-written database and sees the same world

Closes #976.

`scripts/toast_reads_barn_db.py` has Barn write a database through its
checkpoint writer (`barn offline-checkpoint`, which gains `--run FILE` to build
a world first), loads identical copies of the written file in Barn (`barn
eval`) and Toast (emergency mode, no listener), walks every object, property
and verb on both, and compares the counts of queued and suspended tasks Barn
wrote, Barn read back, and Toast saved. It fails when Toast refuses or repairs
the file, when anything differs, or when Barn kept a task in its own frame
form. The order of a map's WAIF and anonymous-object keys is set aside: Toast
orders those by memory address (utils.cc compare()).

- Nightly: `fuzz.yml`'s ToastDifferential job runs it on generated worlds
  (`fuzzseed.ToastWorld`, rendered by `moo-fuzz-source --generator
  toast-world` from the cycle's FuzzWorldCheckpointEffects inputs plus 11
  committed seeds, one with strings above 127), uploads
  `fuzz-failure-ToastReadsBarnDatabase`, and files findings through
  `fuzz_report.py`. The accept job judges the step against its own results.
- Mongoose: `toast-reads-mongoose-checkpoint.yml` runs it on the
  goodput-series dump with the Toast the live game runs (0ff6fd7) and prints
  counts, digests and fixed text only.

Local results: 11 generated worlds agree on stock Toast; the Mongoose dump
loads in Toast with 0 repairs and the same task counts ([93, 12]) on all three
readers, and 8 property records differ, from two Barn differences this PR
records and does not fix: property_info() on a property named after a builtin
property (Toast E_PROPNF, Barn returns it), and a size limit hit with
max_concat_catchable unset (Toast aborts the task, Barn raises E_QUOTA). The
Mongoose workflow stays red until those are fixed.

Tests: `scripts/test_toast_reads_barn_db.py`, `scripts/test_fuzz_compute.py`,
`internal/dbtool` (`--run`).

