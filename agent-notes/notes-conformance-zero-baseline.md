# Conformance zero-baseline prerequisite

## Scope

Restore Barn master to zero MOO conformance failures before PR #37 is rebased.
This note is an uncommitted handoff record and must not be staged, committed, or
pushed.

## Current authority and repository state

- Read `AGENTS.md` and the full tracked
  `plans/barn-toast-mongoose-convergence-workstreams.md`.
- Barn is on clean-tracked `master` at
  `fec44e7ab195faa2f0a0d9e84aa6fc2fceacac85`.
- `moo-conformance-tests` is on clean-tracked `main` at
  `aa312b976804df05c9f9e43a02443a49697d91f3`.
- Barn origin is `git@github.com:MongooseMoo/barn.git`.
- Conformance origin is
  `git@github.com:MongooseMoo/moo-conformance-tests.git`.
- The documented ordinary Toast authority is the managed WSL binary
  `/root/src/toaststunt/build-release/moo`.
- The documented managed command runs from
  `/mnt/c/Users/Q/code/moo-conformance-tests` with `uv run moo-conformance`
  and the server command
  `/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}`.

## CI observations to prove unchanged

The latest conformance main failed these rows against Barn master:

- `read_stdin::read_stdin_a_prefixed_input_returns_e_nacc`: expected `E_NACC`;
  Barn returned the structured error
  `[E_NACC, "Move refused by destination", 0, ...]`.
- `exec::queued_tasks_shows_suspended_exec`: expected `1`, got `0`.
- `exec::multiple_simultaneous_execs_work`: expected `1`, got `0`.

The rows already exist in:

- `src/moo_conformance/_tests/builtins/read_stdin.yaml`
- `src/moo_conformance/_tests/server/exec.yaml`

No Barn source, test, branch, issue, or PR action has been taken.

## Toast result and blocker

The first exact managed oracle command was:

```powershell
wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc 'uv run moo-conformance --server-command="/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}" -k "read_stdin_a_prefixed_input_returns_e_nacc" -v'
```

The harness collected one exact row but skipped it:

```text
test_yaml_conformance[read_stdin::read_stdin_a_prefixed_input_returns_e_nacc] SKIPPED
1 skipped, 11578 deselected
```

The unchanged row declares `skip_if: "missing builtin.read_stdin"`.
`test_conformance.py` implements that gate by evaluating
`function_info("read_stdin")` and treating an unsuccessful result as a missing
builtin. The documented stock Toast oracle therefore cannot supply the required
passing proof for this row.

This is a blocking oracle result under the assigned workflow. No `exec` Toast
row, Barn conformance row, Barn root-cause inspection, issue mapping, branch,
source change, commit, push, or PR was attempted.

## Next action

The objective authorized Go TDD when Toast proof is impossible for an optional
builtin, so work resumed.

## Resumed authority and Barn results

Stock WSL Toast passed both unchanged supported rows separately:

- `exec::queued_tasks_shows_suspended_exec`: 1 passed.
- `exec::multiple_simultaneous_execs_work`: 1 passed.

Barn `master` at `fec44e7ab195faa2f0a0d9e84aa6fc2fceacac85` was rebuilt with:

```powershell
go build -o barn.exe ./cmd/barn/
```

The first documented Windows managed-harness invocation could not start because
the preceding WSL `uv` run had created a Linux `.venv`, and Windows `uv` could
not remove its `lib64` entry. The exact failure was reported before changing
the invocation. The same managed harness was then run with a separate generated
Windows environment at `.venv-windows-baseline`; no existing environment was
deleted.

Current Barn results, each run separately:

- `read_stdin::read_stdin_a_prefixed_input_returns_e_nacc`: failed exactly as
  CI reported. Expected `E_NACC`; actual value was
  `[E_NACC, "Move refused by destination", 0, [["#-1", "", "#8", "#-1", "#8", 1]]]`.
- `exec::queued_tasks_shows_suspended_exec`: passed 1/1.
- `exec::multiple_simultaneous_execs_work`: passed 1/1.

The two `exec` CI premises are therefore non-reproducible on the freshly built
current master and are closed without source or test work.

## Live issue ownership blocker

Live GitHub open-issue bodies were searched for `read_stdin`, `stdin`, `E_NACC`,
`Move refused by destination`, and the two `exec` row names/behavior.

- No open issue mentions `read_stdin`, `E_NACC`, or `Move refused by
  destination`.
- The only open issue returned for `stdin` is #23. Its body owns the different
  defect where three-argument `exec(cmd, input, env)` discards its input. It
  does not own the `read_stdin()` structured-exception mismatch.

The campaign requires an existing truthful issue association and a `Closes #N`
PR phrase. No existing open issue owns the only reproducible baseline repair,
so work stops before branch creation, Barn source diagnosis, Go regression,
patch, commit, push, or PR.

## Required next action

The user explicitly removed issue association from this prerequisite. Work
resumed on branch `agent/read-stdin-enacc`; this repair must not use a closing
phrase.

## Go TDD and diagnosis

The focused scheduler integration regression
`TestReadStdinErrorResumesAsLiteralValue` drives the real `read_stdin()` builtin
through a controlled pipe, suspension, task wakeup, and VM resumption. The
pre-production result is red:

```text
read_stdin result flow = 4, want FlowReturn
result={E_NACC, "Move refused by destination", 0, {}}
```

Exact command:

```powershell
go test ./scheduler -run '^TestReadStdinErrorResumesAsLiteralValue$' -count=1
```

The concrete ownership boundary is the task-to-VM wake path:

- `builtins.readStdinValue` correctly produces the literal
  `types.NewErr(types.E_NACC)`;
- `ProcessStdin.readOne` correctly resumes the waiting task with that value;
- `VM.SetResumeValue` converts every error-typed wake value into
  `resumeError`, causing the VM to raise and construct the four-element
  exception list;
- ordinary explicit `resume(task, E_*)` still requires that exception behavior,
  while `read_stdin()` requires the error value as its return.

The in-progress production correction marks only `read_stdin` wake errors as
literal values on the existing task wake state and passes that distinction
through the existing `VM.SetResumeValue` helper. No interface, helper, or
adapter is being added.

Intended committed files currently are:

- `builtins/signatures.go`
- `scheduler/task_runtime.go`
- `scheduler/task_runtime_test.go`
- `scheduler/eval.go`
- `task/task.go`
- `vm/vm.go`
- `vm/bytecode_execution_test.go`

## Current blocker and next action

The `read_stdin` correction is implemented and focused green:

- `TestReadStdinErrorResumesAsLiteralValue`: pass.
- `TestBytecodeErrorResumeRaisesIntoSavedExcept`: pass, preserving explicit
  `resume(task, E_*)` exception behavior.
- `go test ./builtins ./scheduler ./vm -count=1`: pass.
- unchanged managed
  `read_stdin::read_stdin_a_prefixed_input_returns_e_nacc`: pass 1/1.

The first complete managed suite is not green:

```text
13 failed, 11449 passed, 117 skipped in 294.16s
```

Eleven failures are invocation-only unexpected-skip rejections for startup
repair suites requiring `--server-db-dir`; the next complete command must
provide the documented canned database directory before using
`--fail-on-unexpected-skip`.

The two behavioral failures are the same `exec` rows originally reported:

- `exec::queued_tasks_shows_suspended_exec`: expected 1, got 0.
- `exec::multiple_simultaneous_execs_work`: expected 1, got 0.

They are order/load dependent:

- complete 23-row `exec::` family passes on stock Toast;
- complete 23-row `exec::` family passes on Barn;
- `error_dump_persistence:: or exec::` passes 24/24 on Barn;
- `dump_persistence or exec::` passes 34/34 on Barn.

This rules out the immediately preceding persistence/restart suites. Current
hypotheses are an earlier suite leaving active task state or full-run
load/latency allowing one-second Windows `sleep` fixtures to complete before
the test's 0.1-second queue observation.

No publication is authorized until the full managed suite has zero failures.
Next: use the conformance CLI's tracked file/directory selectors to narrow the
pre-`exec` category prefix without rerunning the complete suite, identify the
first causal predecessor or load threshold, then add the smallest Go
regression/fix for that existing Barn boundary.

## Exec prefix reduction

The suspected simple load/age explanation is not sufficient:

- `builtins + generated_builtins + language + exec`: 10,842 passed, 107
  skipped in 68.38s.
- `audit + builtins + generated_builtins + language + exec`: 10,969 passed,
  107 skipped in 240.60s.
- every non-audit category before `server` plus `exec`: 11,183 passed, 115
  skipped in 83.24s.
- the six `server` YAML files lexically before `exec` plus `exec`: 30 passed
  in 13.18s.

The next decisive command is the exact discovery prefix ending at
`server/exec.yaml`: all earlier category directories, the six earlier server
files, and `exec.yaml`, with all post-`exec` tests excluded. If that reproduces
the two failures, the reduction has proven accumulated cross-prefix state and
can bisect the interacting prefixes without conflating post-`exec` failures.

## Final verification and publication

The user corrected the verification authority to the repository-managed
wrapper. The interrupted direct harness process tree was terminated and
verified absent before running the exact managed command:

```powershell
.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788
```

Managed result:

```text
11451 passed, 128 skipped in 292.03s
Failed tests: 0
```

Artifacts:

- `reports/runs/20260730_031132/pytest.log`
- `reports/runs/20260730_031132/summary.json`

Additional gates:

- `go test ./... -count=1`: pass.
- `git diff --check`: pass.

Published baseline:

- branch: `agent/read-stdin-enacc`
- commit: `f457786` (`fix: return read_stdin errors as values`)
- ready PR: https://github.com/MongooseMoo/barn/pull/46
- initial CI: `Build & unit tests` pending; `MOO conformance suite` pending.

Only the seven intended source/test files were committed. This note,
pre-existing unrelated `CLAUDE.md` changes, and the existing untracked
workspace files remain uncommitted.

## PR #46 exec-gate follow-up

Independent authoritative reruns are red with the same two rows:

- local managed run `20260730_032404`: 11,449 passed, 128 skipped, 2 failed;
- Actions run `30530134551`: 11,436 passed, 128 skipped, 2 failed.

The failures remain:

- `exec::queued_tasks_shows_suspended_exec`;
- `exec::multiple_simultaneous_execs_work`.

Preserved Actions JUnit timing materially changes the diagnosis. Both failed
rows finish in about 0.108-0.109 seconds, while their `sleep 1` children should
live for about one second. The immediately adjacent kill/resume/task-stack
rows also pass at about 0.108-0.110 seconds. Therefore the failure is not
premature subprocess completion: the exec tasks still exist and are suspended
when observed.

The failing YAML returns a boolean combining exact `queued_tasks()` length,
task id, executable label, and order. Since focused exec runs pass and the
same rows fail late in full runs, the remaining proven boundary is the
contents/order of the global queued-task view, most likely an additional live
task retained from an earlier test. The next action is to identify the exact
retained task/lifecycle owner from preceding suites and establish a
deterministic focused Go regression for that leak before any production edit.
No unchanged full-suite rerun is authorized.

## Independent PR #46 review (2026-07-30)

### Findings and observations

- The independent reviewer reconfirmed `AGENTS.md` and all 1,531 lines of the
  tracked convergence plan before PR action.
- Live PR #46 is open, ready rather than draft, mergeable, and targets
  `master` from `agent/read-stdin-enacc` at
  `f4577865a1f7c19a1406128e8aaec2f54a9b1735`.
- The live and local diffs contain exactly the seven published files listed
  above.
- The wake/resume distinction is set only by `read_stdin()` and is consumed
  through the three existing task-to-VM resume sites.
- Ordinary error resumes still use the existing exception path when
  `WakeErrorAsValue` is false.
- No new helper, interface, adapter, or alternate runtime path was introduced.
- Focused independent replays passed:
  - `go test ./scheduler -run '^TestReadStdinErrorResumesAsLiteralValue$' -count=1`
  - `go test ./vm -run '^TestBytecodeErrorResumeRaisesIntoSavedExcept$' -count=1`

### State and blocker

- No substantive review finding or current blocker.
- The shared worktree still contains the unrelated tracked `CLAUDE.md`
  modification and extensive pre-existing untracked files; none has been
  changed, staged, or committed by the reviewer.

### Next action

- Run `go test ./... -count=1`.
- Run `git diff --check`.
- Run the exact repository-managed full conformance command from Barn root:
  `.\scripts\run-conformance.ps1 -Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788`.
- Inspect and wait for every configured PR #46 check; approve and merge only if
  every local and remote gate is green.

### Terminal independent gate results

- `go test ./... -count=1`: passed.
- `git diff --check`: passed.
- Exact managed run ID: `20260730_032404`.
- Exact managed result: `2 failed, 11449 passed, 128 skipped in 292.26s`.
- Local failures:
  - `exec::queued_tasks_shows_suspended_exec`: expected `1`, got `0`.
  - `exec::multiple_simultaneous_execs_work`: expected `1`, got `0`.
- Local artifacts:
  - `reports/runs/20260730_032404/pytest.log`
  - `reports/runs/20260730_032404/summary.json`
- Configured PR #46 checks are terminal:
  - `Build & unit tests`: success.
  - `MOO conformance suite`: failure in Actions run `30530134551`, job
    `90830247718`.
- Remote conformance result: `2 failed, 11436 passed, 128 skipped in 243.87s`.
- The remote failures are the same two assertions as the local managed run.
- The green-only gate is blocked. No approval or merge action was taken.

## Retained queued-task diagnosis checkpoint (2026-07-30)

### Findings and observations

- No further managed conformance run has been made; rerunning unchanged code is
  still forbidden.
- `scheduler/task_load.go` does not support the initial theory that checkpoint
  restore loses fork identity. `loadSuspendedTask` explicitly restores every
  suspended snapshot with `IsForked = true` and restores `VerbName`; therefore
  an anonymous suspended eval-scaffold fork remains eligible for the existing
  `task.Manager.GetQueuedTasks` filter after restart.
- The two exact YAML predicates are stricter than merely finding the exec task:
  `queued_tasks_shows_suspended_exec` requires an exact list length of one plus
  the child id and `executables/sleep` label, while
  `multiple_simultaneous_execs_work` requires exact lengths of four and two and
  exact order before and after two kills.
- `builtinQueuedTasks` obtains the process-global task-manager view, sorts it by
  `StartTime`, `QueueSeq`, then id, and wizard calls retain every entry returned
  by the manager. `Task.ToQueuedTaskInfo` supplies the executable label only
  when `IsExecSuspended` and `ExecCommandName` are both set.
- The current branch remains at `f457786`; the only tracked worktree change is
  the unrelated pre-existing `CLAUDE.md` modification.

### State and blocker

- No deterministic red has been established yet, so no production code has
  been edited.
- The original restart/fork-identity mechanism is rejected by direct code
  evidence. A retained task can still explain both failures only if it escapes
  the manager's eval-scaffold filter (for example, a different task kind or
  lifecycle transition), and that exact path remains to be proved.

### Next action

- Trace registration, terminal cleanup, connection-close, kill, and checkpoint
  transitions for the HTTP lifecycle tasks immediately preceding the exec
  family.
- Build a focused Go regression only after identifying the exact retained task
  shape that makes the YAML boolean false while the exec subprocess remains
  suspended.

### Proven evaluator-state mechanism

- The retained entry is the currently executing top-level eval task, not a task
  leaked by an earlier HTTP test or checkpoint.
- `EvalCommandOutput` registers its task in the process-global task manager.
  The task begins as `TaskCreated`. When the eval executes `suspend(0.1)`,
  `builtinSuspend` changes that task to `TaskSuspended`.
- The positive-delay branch in `EvalCommandOutput` waits while driving ready
  fork tasks, then calls `bcVM.Resume()` directly. It never changes the eval
  task from `TaskSuspended` back to its executing state.
- A later `queued_tasks()` call therefore sees the current eval task plus the
  suspended exec children. The exec processes can be alive and correctly
  labeled while both YAML tests still return false because their exact lengths
  are one too large.
- This also explains why the neighboring exec kill/resume/task-stack tests pass:
  none asserts the exact global queued-task list length.

### Immediate next action

- Add a focused scheduler regression that runs a wizard eval containing
  `suspend(0.1); return length(queued_tasks());` and requires zero, proving that
  the resumed evaluator is running rather than waiting.
- Observe the deterministic red before changing `scheduler/eval.go`.

## Authoritative evaluator experiment result

- The evaluator-state regression did fail deterministically with `{1, 1}` and
  went green after setting the eval task to `TaskRunning` before VM resume.
- Relevant scheduler/builtin packages and both read-stdin resume regressions
  passed with that experiment.
- The exact managed wrapper run `20260730_034815` nevertheless produced the
  identical authoritative result: 11,449 passed, 128 skipped, and the same two
  exec rows failed. There was no reduction in the named conformance failures.
- Per the controlling scope, the evaluator-state test and its one-line
  production change were fully removed. `scheduler/eval.go` and
  `scheduler/eval_fork_test.go` now match committed PR #46 exactly; tracked
  status is again only the unrelated `CLAUDE.md` modification.
- The run's temporary managed `server.log` was removed with the harness temp
  directory. The preserved evidence is
  `reports/runs/20260730_034815/pytest.log` and its summary.

### Remaining proven boundary

- Before entering `exec`, a zero-delay fork is `TaskQueued`; manager filtering
  includes it, but queued-task slot 2 is an integer start time.
- After entering `exec`, it is `TaskSuspended` with `IsExecSuspended` and
  `ExecCommandName`, so manager filtering includes it and slot 2 is
  `executables/sleep`.
- Therefore the unchanged YAML boolean can still fail with the correct list
  length if the fork has not entered `exec` by observation time. The preserved
  JUnit duration proves only that the test returns after about 0.1 seconds; it
  does not prove the child reached the exec builtin or that its subprocess was
  running.

### Next action

- Establish a focused scheduler test that distinguishes the zero-delay child
  remaining queued/unlabeled from reaching exec-suspended/labeled under the
  same eval-driven scheduling path. Do not add arbitrary waits or rerun the
  full suite.

## Exact exec shape and checkpoint reduction

- A temporary self-contained scheduler probe ran the exact exec YAML shape with
  a real long-running sleep fixture. It returned two entries:
  - task id 2, the current evaluator, with an integer start-time in slot 2;
  - task id 3, the `go` child, with slot 2 equal to
    `executables/sleep`.
- This proves the zero-delay child does enter the exec builtin and has the
  correct id and label. The temporary probe and imports were then fully
  removed; `scheduler/eval_fork_test.go` again matches committed PR #46.
- The full-only extra-entry mechanism is in `Scheduler.TaskSnapshots`.
  It checks `snapshot.VM != nil` before checking task state, so a killed task
  that retains its suspended VM is appended to the checkpoint's suspended
  collection. A checkpoint before the five-second terminal cleanup can
  therefore persist a killed task, and restart restores it as suspended.
- This is temporally consistent with the full run: fast cleanup kills from
  task/waif lifecycle tests can be followed by the late dump/restart families
  before the periodic cleanup removes their scheduler entries.

### Current state

- A focused regression,
  `TestTaskSnapshotsExcludeKilledSuspendedVMTask`, has been added in
  `scheduler/task_factory_test.go`.
- It creates a real VM task, runs it to timed suspension, kills it before
  cleanup, and requires both checkpoint snapshot lists to remain empty.
- The regression failed deterministically before the production edit:
  `checkpoint captured killed task: queued=0 suspended=1`.
- `TaskSnapshots` now excludes killed and completed snapshots before its
  VM-bearing classification.
- The focused regression and `go test ./scheduler -count=1` pass.
- The exact managed wrapper run `20260730_040319` is the zero-conformance
  baseline: 11,451 passed, 128 skipped, 0 failed in 291.28 seconds.
- Both previously failing rows passed:
  `exec::queued_tasks_shows_suspended_exec` and
  `exec::multiple_simultaneous_execs_work`.
- `go test ./... -count=1` and `git diff --check` pass.

### Next action

- Commit only `scheduler/task_factory.go` and
  `scheduler/task_factory_test.go`.
- Keep this checkpoint note and the unrelated `CLAUDE.md` change uncommitted.
- Push PR #46 and wait for CI without merging it.

## Independent PR #46 follow-up review

### Findings and current state

- PR #46 head is `83bb2761236acb2c68242458be8dbc043e597121`;
  it is open, non-draft, mergeable, and reports a clean merge state.
- The new diff is confined to `scheduler/task_factory.go` and
  `scheduler/task_factory_test.go`.
- The mechanism is confirmed: `TaskSnapshots` previously classified any
  snapshot with a VM as suspended before considering its terminal state.
  Killed or completed VM-bearing tasks still present before periodic cleanup
  could therefore be checkpointed, and restart loads non-queued snapshots as
  suspended tasks.
- The fix rejects exactly `TaskCompleted` and `TaskKilled` before VM
  classification. It adds no helper, interface, adapter, timing change,
  broader state exclusion, or conformance expectation change.
- The focused regression passed:
  `go test ./scheduler -run '^TestTaskSnapshotsExcludeKilledSuspendedVMTask$' -count=1`.
- `go test ./scheduler -count=1`, `go test ./... -count=1`, and
  `git diff --check` all passed.
- The exact managed conformance run `20260730_041719` passed with 11,451
  passed, 128 skipped, and 0 failed in 292.18 seconds. All `exec` rows passed.
- Live configured checks for CI run `30533583088` both pass:
  `Build & unit tests` and `MOO conformance suite`.
- No substantive review finding remains. Approval and merge have not yet been
  attempted.

### Next action

- Attempt normal approval if GitHub permits it, then merge PR #46 without
  bypass.
- Verify PR #46 is merged and update local `master` while preserving the
  unrelated `CLAUDE.md` modification and all untracked files.
- Keep this checkpoint note uncommitted.

### Final outcome

- GitHub rejected approval because the authenticated account is the PR author;
  no approval review was created.
- PR #46 was merged normally without bypass at
  `9d5239e668f8490ccdf93b01337e5e94935e53d0`.
- Live verification reports PR #46 as `MERGED`, with both configured checks
  still successful.
- Local `master` was fast-forwarded to the same merge commit. The configured
  pull-rebase path first refused to run with the unrelated unstaged
  `CLAUDE.md` change, so an explicit `git fetch origin` followed by
  `git merge --ff-only origin/master` performed the same fast-forward without
  stashing or modifying that file.
- Final tracked status is only `M CLAUDE.md`; this note and all other
  pre-existing untracked files remain uncommitted.
