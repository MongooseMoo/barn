# Cross-engine generated suspended-stack lane (issue #975)

Resumed 2026-11-21 by a new agent after the previous one hit a usage limit;
plan: merge origin/master, finish the lane, verify, commit.

## Goal

A nightly lane that takes generated suspended-stack programs, runs each on one
engine up to its suspensions, checkpoints, restarts on the other engine, resumes
the tasks, and compares the final result with the same program run with no
restart. Both directions (Barn writes/Toast resumes, Toast writes/Barn resumes).
Findings are reported as issues through `scripts/fuzz_report.py`.

## State

Committed on branch `fuzz/cross-engine-generated` (local, not pushed), merged
with origin/master on 2026-11-21:
- `scripts/cross-engine-session.sh`: one managed session in one direction;
  `scripts/cross-engine-checkpoint.sh` now calls it four times instead of
  holding the engine commands itself.
- `scripts/fuzz_cross_engine.py`: renders the corpus with
  `fuzz_differential.render_choices` (now taking the generator name), writes a
  JSON-as-YAML suite per chunk of 25 programs, runs each chunk both ways,
  reads `--junitxml`, writes `report.md`/`results.json`; exit 0/1/2 like the
  differential. `--minutes` stops starting chunks; `--seed` orders programs.
- `scripts/fuzz_report.py`: the divergence regex accepts any two
  `- label: ` result lines, and an entry's `- signature: ` line replaces the
  input path as the issue signature (one issue per failure class).
- `scripts/build-toast-oracle.sh [MANIFEST]`: the pinned Toast build, run by
  ci.yml's cross-engine job, the differential job, the new job and
  toast-check.yml (with `mongoose-0ff6fd7-wsl-testdb.json`). It replaced the
  `.github/actions/toast-oracle` composite action; see Surprises.
- `.github/workflows/fuzz.yml` job `cross-engine` (name
  `ToastCrossEngineCheckpoint`), and `scripts/fuzz_compute.cjs` accept judges
  it like the differential (COMPARISONS list).
- Tests: `scripts/test_fuzz_cross_engine.py` (8, pass in WSL),
  `scripts/test_fuzz_compute.py` (+2; 34 pass on Windows, where node is),
  `test_fuzz_report.py` 35 and `test_fuzz_differential.py` 37 pass in WSL.
  The Windows Python fails 5 report and 21 differential tests with and
  without these changes (`write_text() got an unexpected keyword argument
  'newline'`), so those run in WSL.

## Decisions and why

- One generated YAML test per retained input, in a temporary suite root under
  `.tmp/`, run through the managed harness with `--restart-server-command`.
  The harness starts each test on a fresh database copy on the
  `--server-command` engine (`moo-conformance-tests` runner.py
  `_ensure_suite_server_db`, the "Every test begins on the --server-command
  engine with a fresh database" comment), so one test per program gives the
  fresh database the generator needs, with no harness change.
- The no-restart reference runs inside the same test, before the checkpoint,
  on the first engine: copy A runs to completion (a MOO loop resumes every
  suspended task the program made), its `#0.suspended_stack` property is
  deleted, then copy B starts, settles at its suspensions, `dump_database()`,
  restart on the other engine, the loop resumes B's tasks, and the last step
  compares A and B with `equal()` (case-sensitive). The compared value is the
  root task's return (or its error code and message) plus every property of the
  program's test object, with object-valued properties replaced by a marker
  (A and B create different object numbers). The root result itself holds no
  object numbers: every generator kind returns integers, lists, errors and
  booleans (`internal/fuzzseed/suspended_stack.go` nest/inFinally/moveHook).
- Tasks to resume are found with `queued_tasks()` minus the ids queued before
  the program started; `resume()` errors (a forked task that has not run) are
  caught. Toast's `queued_tasks()` lists waiting forked and suspended tasks for
  a wizard (`git show origin/master:src/tasks.cc` bf_queued_tasks, 2502-2600).
- The harness is told the result through `--junitxml`; the script turns each
  failing generated test into a `## divergence` block that
  `scripts/fuzz_report.py` already parses.

## Results so far

- Before PR 886 (Barn built from the merge's first parent, `.tmp/barn-pre886`,
  sha256 `30a3e0cf...`), four seed inputs (three one-layer `eval` stacks, one
  `verb`), WSL load average 2.34 at start, 8 cores:
  `{"divergence": 6, "match": 2}`. Barn writes, Toast resumes:
  `expected value 1, but got ['no restart', [['return', [[1, [1, 0]]]], []], 'restarted', [['return', [[1, 0]]], []]]`
  (Toast lost eval()'s `{1, value}` wrap, PR 886's export bug). Toast writes,
  Barn resumes: Barn did not boot, `load database: restore suspended tasks:
  suspended task 1288614929: native frame 3: native builtin continuation
  requires conversion` (PR 886's import bug). The verb seed matched both ways.
- Master (Barn at the branch's base, the merge of PR 972, `bin/barn` sha256 `9a903def...`), same
  four seeds, load 2.08: `{"divergence": 1, "match": 7}`. FINDING 1: Toast
  writes, Barn resumes, choices `0000010200`, program
  `r1 = call_function("eval", "suspend(); v = 0; return {1, v};");` (the full
  program is in `.tmp/master-seeds-out/report.md`): Barn does not boot,
  `level=ERROR msg="Barn failed" err="load database: restore suspended tasks:
  suspended task 963952496: native frame 3: native builtin continuation
  requires conversion"`. PR 886 converts the eval frame's continuation only
  when the caller is paused at `eval(...)` itself (its description, "Import");
  through `call_function("eval", ...)` the caller is paused at call_function.
  Not fixed here (Barn VM defect, out of this branch's scope).
- The nightly's retained corpus (run 44852833568 artifact
  `fuzz-corpus-FuzzSuspendedStackCheckpoint`) holds 3529 inputs; at about 4 s
  per test per direction that is about 8 hours, so the lane needs a time
  budget and rotates which inputs it runs.

- 2026-11-21, after merging origin/master (PRs 979, 982, 983, 985): conflicts
  in `fuzz_differential.py` (master hard-coded `--generator
  structured-ascii`; `render_choices` keeps its generator argument and now
  defaults to `structured-ascii`) and `test_fuzz_compute.py` (the accept
  helper takes both `cross_engine` and master's `builtins`). Master's new
  `toast-check.yml` held a third copy of the Toast build; the
  Toast build is shared (now `scripts/build-toast-oracle.sh`) and toast-check.yml
  uses it with `mongoose-0ff6fd7-wsl-testdb.json`.
  Unit tests after the merge: Windows `python -m unittest test_fuzz_compute`
  36 OK; WSL `bash -lc` (go on PATH only in a login shell)
  `python3 -m unittest test_fuzz_cross_engine` 8 OK, `test_fuzz_report` 35
  OK, `test_fuzz_differential` 38 OK. WSL load average 10.35 on 8 cores
  while they ran.

- 2026-11-21, merged branch (Barn `bin/barn` built in WSL, sha256
  `32f6e118...`; Toast `/root/src/toaststunt/build-release/moo` sha256
  `61dd1799...`, the manifest's `binary_path`; suite
  `mct-cross-engine-generated` fast-forwarded to origin/main, PR 249), 8
  programs: the 4 `.tmp/eval-corpus` seeds plus `.tmp/hook-corpus`, the
  shortest program of each new hook kind from PR 982 found in the nightly
  corpus (`recreate`, `call_function("create")`, `call_function("move")`,
  `call_function("recycle")`). WSL load 18.64 at start, 15.70 at end, 8
  cores; 64 s. `--out .tmp/merged-out`: `{"divergence": 5, "match": 11}`,
  `problems: []`. cf-move, cf-recycle, verb and the two plain eval seeds
  match both ways.
  - FINDING 1 still present: toast-first, choices `0000010200`
    (`call_function("eval", ...)`), Barn does not boot: `restore suspended
    tasks: suspended task 1646851400: native frame 3: native builtin
    continuation requires conversion`.
  - FINDING 2: Barn writes, Toast resumes, `recreate` (choices
    `000010e38d53bab33300`) and `call_function("create")` (choices
    `c9c9b1cfa79e9e9eff6dc9c9c9640090`): Toast does not boot on Barn's
    checkpoint: `*** READ_ACTIV: Bad PC for suspended task.` /
    `READ_VM: Bad activ number 0` / `READ_TASK_QUEUE: Bad suspended task vm,
    count = 1` / `DB_LOAD: Cannot load database!`. Barn logged `Toast could
    not load this checkpoint: task kept in Barn's frame form ... recreate:
    stock Toast does not resume a create, recreate or recycle continuation
    from its checkpoint`. This is PR 982's deliberate refusal,
    `vm/persistence.go:341-371` (`ErrToastLosesObjectContinuation`): Barn
    writes the task in its own frame form because stock Toast's own
    bf_create/bf_recreate/bf_recycle task data is read back wrong
    (objects.cc 546-554, 962-972 per that comment, which I did not re-read).
    The consequence the lane shows: one such suspended task makes the whole
    checkpoint unloadable by Toast, not just that task.
    Recreate program (create is in `.tmp/merged-out/report.md`):
    ```moo
    o = create(#-1);
    add_property(#0, "suspended_stack", o, {o.owner, "r"});
    p1 = create(#-1);
    x1 = create(#-1);
    add_property(o, "p1", p1, {o.owner, "rw"});
    add_property(o, "x1", x1, {o.owner, "rw"});
    add_property(o, "h1", 0, {o.owner, "rw"});
    add_verb(p1, {o.owner, "rxd", "initialize"}, {"this", "none", "this"});
    c = set_verb_code(p1, "initialize", {"v = {0, suspend()};", "$suspended_stack.h1 = {1, v, parent(this) == $suspended_stack.p1};"});
    if (c) raise(E_INVARG, "initialize does not compile", c); endif
    recycle(x1);
    results = {};
    r1 = recreate($suspended_stack.x1, $suspended_stack.p1);
    v = {r1 == $suspended_stack.x1 && parent($suspended_stack.x1) == $suspended_stack.p1, $suspended_stack.h1};
    results = {@results, v};
    return results;
    ```
  - FINDING 3: Toast writes, Barn resumes, the same two programs: Barn does
    not boot, `restore suspended tasks: suspended task 1540572287: native
    frame 3: native builtin continuation requires conversion` (recreate) and
    `... 1574951315 ...` (create). Barn does not import Toast's
    bf_create/bf_recreate continuation; per the persistence.go comment,
    Toast's own data for it is wrong, so what Barn should do with it is
    open. Not fixed here.
  - These three are expected to recur each cycle until fixed; the lane
    reports them as findings, one issue per failure class.

## Surprises

- PR 1032's "Audit discovery workflows" step (ci.yml:140-143) went red, exit
  12: zizmor 1.32.0's `self-repository` audit flagged `uses:
  ./.github/actions/toast-oracle` at fuzz.yml:346 and :446 and wants
  `$/.github/actions/toast-oracle` (its own `--fix=all` writes that). GitHub
  shipped `$/` in July 2026 and per secondary sources it needs runner
  2.336.0+; every workstation runner's newest `_diag` log reports 2.338.2.
  But the same CI step runs actionlint v1.7.12, the newest release, which
  rejects `$/` ("specifying action \"$/.github/actions/toast-oracle\" in
  invalid format because ref is missing", exit 1; rhysd/actionlint issues
  711 and 780 are open). No `uses:` form passes both, and neither check may
  be loosened, so the composite action became
  `scripts/build-toast-oracle.sh`, run by a `run:` step in all four places.
  It fetches the manifest's ref with `git fetch --depth 1` (the repository is
  public) and checks HEAD equals the ref. 2026-11-21 results: the script in
  a WSL scratch directory, WSL load 8.47, built `toaststunt/build/moo` in
  76 s with `options.h: OK` (exit 0, binary sha256 `e49c8a09...`; the
  workstation's `/root` build is a different binary, `61dd1799...`, so the
  build is not byte-reproducible across machines); `uvx --from
  zizmor==1.32.0 zizmor --offline --no-progress .github/workflows/fuzz.yml
  .github/workflows/fuzz-controller.yml` exit 0 ("No findings to report");
  `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` exit 0 on
  fuzz.yml + fuzz-controller.yml and exit 0 on ci.yml + toast-check.yml;
  `python -m unittest test_fuzz_compute` 36 OK. Other workflows still
  use `./` actions (tested-tree, elle-cli, conformance-test); those files are
  not audited by zizmor and I did not change them.

- A new job in `fuzz.yml` would be counted as a native lane by
  `scripts/fuzz_compute.cjs` accept (`NOT_LANES`, line 41) and fail the cycle
  with "no census"; the accept logic must learn about the lane either way.

## Time budget

The campaign step starts no chunk after 20 minutes (`--minutes 20`); a chunk
is 25 programs, measured at about 4 s per program and direction (8 programs
both ways in 64 s at WSL load 18), so a started chunk adds about 200 s: the
step ends within about 24 minutes, inside the job's 60-minute timeout with
the Toast and Barn builds. Each cycle orders the corpus by
sha256(run id + program name), so a different part of the corpus runs each
cycle.

## Verified and not verified (2026-11-21)

Verified: the unit tests listed under Results; the merged branch's lane on
8 programs both ways (FINDINGS 1-3); two copies run at once in WSL with
separate `--out` on the 4 eval seeds, each exit 1 with
`{"divergence": 1, "match": 7}` and `problems: []` (FINDING 1), WSL load
19.3 to 19.6: no collision, and exit status 1 for a divergence. The four
workflow files parse as YAML; actionlint (actionlint-py 1.7.12) printed
nothing on fuzz.yml, toast-check.yml and ci.yml, but its exit status was
not captured (a second run to capture it was refused by the permission
system).

Not verified: the job on a real GitHub runner (the corpus artifact download,
the failure reporter publishing
issues, accept on a real cycle); toast-check.yml with the action; the
lane over more than these 8 programs on the merged branch.

Known limitation: the failure class is direction plus failing step, so
FINDING 1 and FINDING 3 (both "toast-first: the other engine did not boot on
the checkpoint") share one issue signature, and `fuzz_report.py` files only
the first divergence of a report.

## Next step

The lead publishes the branch (push, PR against master). FINDINGS 1-3 are
Barn/Toast defects for separate work.
