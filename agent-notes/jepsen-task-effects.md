# Jepsen task persistence and external effects (PR E)

## Goal
Plan section 4 / Delivery item 4 (first part) of `plans/jepsen-expansion.md`: controlled task persistence and externally visible effects in the Jepsen campaign, with a separate lifecycle checker, distinct witnesses, fixture and real-binary controls, and bounded CI coverage. One PR, `jepsen/task-effects`, against master after #747, #748, #749.

## State
2026-11-11: implemented and unit-tested; first CI run pending.
- Task lane (`--tasks`, `scripts/jepsen_checkpoint.py`): `plant` before every graceful checkpoint and before each generation's first live checkpoint (not in stage runs); `task_recovery` reads every plant's held state at each launch after PR C's recovery observation and before any release; `open_connections` (receiver + option connection, fresh programmers); `release` (gate, resume with `{token, "wake"}`, kill) and bounded wait, before admission. `task_checks` is the lifecycle checker.
- Effects lane (`effects` family): `effects_code`, `drain` at every observation, `receipts.jsonl`, `effect_checks`. `barn.commit_retries` from the debug endpoint is the conflict-coverage evidence.
- Control: elle-only fault `pending-effect-retry` (`engine/task_runtime.go` commit-conflict retry publishes the lost attempt's effects); `CONTROLS` entry without a stage; `control_result` also reads findings' `witness`. Stages job runs it with `--workloads effects`.
- `snapshot_checks` subtracts task-lane tasks (`held_task_count`) from both task counts.
- Tests: `scripts/test_jepsen_task_effects.py` (41) plus the adjusted `scripts/test_jepsen_checkpoint.py`; 152 OK on Windows Python.
- Invalid-task policy (added after CI run 42974481407): kind `invalid` in every plant; at the first graceful checkpoint `invalidate` flips the first 64-hex line after task tid's header in the suspended section (its first frame's fingerprint, before the builtin-layout hex, as gen-001/input.db of that run shows), rewrites the sidecar header digest, and launches from that pair. log_findings expects DROPPED_TASK_MESSAGE naming that task_id and requires it; task_checks reports `invalid-task-policy` if it is restored or runs.
- CI run 42974481407 (first push): effects lane healthy (533 ok effects ops, 273 across the boundary, barn.commit_retries 29, no effect witness). Task lane: fork and cancel behaved; the five resumed kinds stayed queued 45 s after release (Barn defect below). Harness defect fixed: txn_code sent the pre-admission observation (keys of every family) to effects_code, which refused it; observations now read effects rows only (`observation=True`). Stages job: drop-append control's Elle check of gen-000.edn (17.9 KB) printed nothing for 15 min until the job timeout; same control took ~30 s in #749's run; not this PR's code path. MOO conformance: notify no_flush D5 test from conformance #190 fails on every Barn PR (not implemented in Barn).

- CI run 42977648003 (second push): invalid-task drop logged and accepted; a live plant recovered held through a checkpoint-crash; effects clean (330 ok, 159 boundary, commit_retries 12). Task lane red only from the Barn resume defect. Fixed a checker label: leftovers of an incomplete release captured by a later checkpoint are `lost-task`, not `ran-before-release`.
- Barn fixes (team-lead: separate PRs, #750 harness only): #752 `fix/restored-suspension-resume` (open). Second defect reported, decision pending: recycled object slots are lost from max_object() after load (Toast keeps them; next create() reuses the recycled number), so the recycle scenario will witness `recycled-reference-valid` until fixed.
- Stages job times out twice at the drop-append control: Elle 0.1.11 never returns on that run's gen-000.edn (148 ops; reproduced >240 s with and without plotting; #749's 112-op history takes 4 s). Owner decision pending with team-lead.
- Conformance branch `jepsen/held-task-restart` stays local (team-lead: Q decides).

## Decisions and why
- Held tasks use a timed `suspend(3600)` released by `resume(id, value)`: an indefinite suspend resumes with 0 at load (Toast D2, #747).
- A delayed fork cannot be resumed, so its body polls a gate property (read-only) before recording.
- task_local and notify-to-another-player go through wizard-owned helper verbs (wizard-only builtins); every other scenario verb is owned by the nonwizard programmer.
- Every launch re-reads every plant: a launch from an older pair can legitimately hold a plant again (rollback), and it is released again; a plant is held, completed or absent, never partial.
- Effects rows follow the irreversible boundary, so the transaction stays atomic for Elle; `E.pre` written before the boundary makes the boundary publish and flush the pre notification.
- PR D's layout stays seven objects; effects objects live in `layout["effects"]`.

## Surprises
- Barn defect (reported to team-lead, decision pending): resume() of a restored timed suspension waits until the original deadline; `engine/task_load.go:74` sets StartTime to the saved deadline and `resumeLocked` (`task/task.go`) does not reset it, while `readyDeadlineLocked` takes the later of StartTime and WakeTime. Toast runs it at once. Conformance tests: moo-conformance-tests branch `jepsen/held-task-restart`, `server/restart_held_task_semantics.yaml` (Toast 4/4 pass, Barn 3 fail). Until fixed, the task lane reports `lost-task` for every resumed kind.
- `r`, `w`, `f` are builtin object properties (`add_property(o, "r", ...)` is E_INVARG).

## Next step
Push, open the PR, watch both Jepsen jobs; read the campaign's task/effect tables and a gen-*/input.db for the invalid-task check.
