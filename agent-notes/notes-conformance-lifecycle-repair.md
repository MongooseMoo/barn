# Conformance lifecycle repair checkpoint

- Scope: implement the managed lifecycle repair in a fresh isolated worktree of `C:\Users\Q\code\moo-conformance-tests` from current `origin/main`.
- Allowed ownership: `src/moo_conformance/server.py`, `runner.py`, necessary plugin/test integration, and focused tests.
- Exclusions: no Barn code changes, profiles, semantic YAML tests, CI workflows, historical Markdown, push, or PR.
- Confirmed failure chain: omitted `--server-db-dir` left `Anon1.db` unavailable; the runner disconnected; `ManagedServer.restart` stopped Barn and then failed copying the missing fixture; cached default database state remained trusted; subsequent cases cascaded into roughly 190 connection refusals.
- Required behaviors: restart/fixture failure cannot poison cached database state or cascade; expected `wait_for_port=False` exits are distinct from unexpected server death while transport is required; preserve the first return code and server-log tail; packaged execution stops after a root lifecycle failure without blanket pytest `-x`; suite requirements are enforced before `prepare_suite_environment`.
- Current repository observation: the primary checkout is on `agent/checkpoint-restart-controller` with many unrelated untracked artifacts, so it must not be edited or cleaned. Origin is `git@github.com:MongooseMoo/moo-conformance-tests.git`.
- Next action: fetch `origin/main`, create a separate worktree and branch, read current instruction files and implementation/tests, write focused failing regressions, then implement and run focused plus full harness gates.

## Isolation and current-code findings

- Created clean worktree `C:\Users\Q\code\moo-conformance-tests-lifecycle-repair` on branch `agent/managed-lifecycle-repair` at `1878f5620b4a5aa75cccdef1383cd01ffc7f943c`, exactly matching current `origin/main`.
- Current `YamlTestRunner._ensure_suite_server_db` disconnects before restart and updates `_active_server_db_path` only after successful restart, but its first-use shortcut can cache `managed_server.db_path` without proving the managed process is alive or the requested database was successfully started.
- Current `ManagedServer.start` mutates `db_path`, temp/copy paths, manifest/log handles, and process state incrementally before copy/start/wait can fail; `wait_for_port=False` has no explicit expected-exit observation, while transport-required death diagnostics are only emitted inside `_wait_for_port`.
- Current canonical test order is declared skip, then `prepare_suite_environment`, then `_enforce_suite_requirements`; this violates the required requirement-first ordering and allows missing `server_db_dir` to trigger lifecycle work before the declared config requirement is enforced.
- Current plugin already terminates packaged execution when canonical admission fails via `session.shouldfail`, but it has no equivalent root lifecycle failure signal.
- The pre-existing `agent/checkpoint-restart-controller` branch differs from `origin/main` only in checkpoint-output adoption behavior in `server.py` and related tests; it is a separate concern and remains untouched.
- Next action: inspect focused test conventions and packaged execution hooks, add red tests for transactional start/restart state, terminal lifecycle failure propagation, expected exit-only handling, and requirement ordering, then implement the narrow repair.

## TDD and implementation checkpoint

- Red proof: `uv run pytest -q tests/test_managed_server.py tests/test_requirements.py tests/test_admission.py` initially failed collection because the new tests imported the not-yet-implemented `ManagedServerLifecycleError` and `_record_managed_lifecycle_failure`.
- Green proof: after implementation, the same focused command passed all 128 tests in 11.61 seconds.
- Modified worktree files: `src/moo_conformance/server.py`, `runner.py`, `plugin.py`, `test_conformance.py`, plus focused tests `tests/test_managed_server.py`, `test_requirements.py`, and `test_admission.py`.
- Implemented architecture: typed and latched first lifecycle failure; stable first return code and 8 KiB log tail; expected no-wait exits remain valid until transport is demanded; staged database copies and delayed state commit; fixture validation before disconnect; last-known-good runner DB cache retained across failed switch; packaged-only session termination for typed lifecycle roots; static config requirement preflight before managed environment preparation while DB-specific dynamic probes still run afterward.
- Current gates: `git diff --check` and focused Ruff passed. Focused pytest passed. `uv run mypy ...` could not spawn because the clean worktree environment was created without dev extras, so this is an environment setup item rather than a type-check result.
- No blockers. Next action: install/sync declared dev extras, run mypy and inspect the full diff for lifecycle cleanup/edge cases, then run the full repository pytest gate and the documented managed harness gate.
