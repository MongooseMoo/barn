# Task-local literal-skip repair checkpoint

- Scope: only `src/moo_conformance/_tests/builtins/task_local.yaml` in the clean `moo-conformance-task-local-skips` worktree; no harness changes, commits, or pushes.
- Oracle: canonical WSL Toast `/root/src/toaststunt/build-release/moo`, repository commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`.
- Managed baseline command: `uv run --frozen pytest --pyargs moo_conformance --moo-suite-path=builtins/task_local.yaml --server-command="/root/src/toaststunt/build-release/moo --no-ipv6 --ipv4 127.0.0.1 --outbound {db} {db}.new {port}" --server-db-dir=/mnt/c/Users/Q/code/.worktrees/moo-conformance-task-local-skips/src/moo_conformance/_db/startup --moo-host=127.0.0.1 --fail-on-unexpected-skip --strict-markers --junitxml=<temp>/task-local.xml -q -ra`.
- Baseline result: 20 collected, 13 passed, 7 literal skips. Skip split: one async parent resume, five stale dynamic-verb placeholders, one current-task kill timeout.
- First candidate result: 20 collected, 18 passed, 2 failed, zero skips. The async resume, self-kill secondary-connection proof, command verb, and three direct verb-call cases executed successfully.
- Failure 1: `server_verb` notification plus eval return was parsed as one combined value. Adjustment: the recycle hook writes the observed task-local value to an isolated property, asserted in the next managed step and deleted in cleanup.
- Failure 2: exact five-second suspend sequence exceeded the primary transport socket's fixed timeout. Adjustment: execute that long call on an existing managed secondary connection, wait 5.5 seconds, then assert its returned output; no harness timeout or semantics weakened.
- Current state: YAML adjustment applied but not yet rerun.
- Blocker: none known.
- Next action: rerun the exact focused managed Toast command; inspect any red; then run focused schema/unit verification and report exact totals to the parent.
