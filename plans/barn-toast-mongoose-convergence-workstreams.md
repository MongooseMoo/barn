# Barn/ToastStunt/Mongoose Convergence Plan

## Goal

Make Barn run the real Mongoose workload with exact Toast-compatible behavior,
and keep it compatible by converting every discovered delta into a durable
conformance test.

The required loop is:

1. observe a concrete Mongoose failure or divergence;
2. reduce it to the smallest useful conformance scenario;
3. prove the unchanged scenario passes the WSL Toast oracle;
4. prove it fails on Barn before changing Barn;
5. make the smallest Barn production change that closes the delta;
6. prove the unchanged scenario passes on Barn;
7. run the relevant managed suite and commit the conformance and Barn slices in
   their respective repositories.

Tests in their final state pass on both Toast and Barn. Before the Barn fix, the
new test must pass on Toast and fail on Barn. If Barn already passes a genuine
candidate test, keep it as coverage and do not invent a Barn source change.

This is a plan only. No current Mongoose regression was reproduced while
writing it; reproducing the present break through the managed harness is an
explicit execution milestone.

## Repositories And Authority

- `C:/Users/Q/code/barn` owns Barn implementation, runtime profiles, local
  diagnostics, and Barn-side verification.
- `C:/Users/Q/code/moo-conformance-tests` owns durable behavioral truth.
- `C:/Users/Q/code/mongoose` supplies the real workload and its database/client
  behavior. It is a discovery source, not a substitute for conformance.
- WSL Toast is the behavioral oracle. Do not substitute a Windows Toast binary.

For ordinary Toast behavior, use:

- `/root/src/toaststunt/build-release/moo`
- stock Toast commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`

For Mongoose `PROMOTE_NUMBERS` behavior, use the pinned WSL Mongoose build:

- `/root/src/toaststunt-mongoose/build-release/moo`
- detached worktree commit `72e3c7f96ce7a41fdeba793aef8818dc4408072e`

The managed WSL oracle wrapper is `scripts/run_toast_wsl.sh`.

## Recovered Records

The useful prior records are:

- `notes/mongoose-differential-2026-07-01.md`: the detailed successful July 1
  Mongoose differential campaign;
- `notes-mongoose-promote-and-login.md`: `PROMOTE_NUMBERS`, trusted-PROXY, and
  `read()`-based login findings;
- `notes/observability-slog-2026-07-11.md`: the structured logging, metrics, and
  pprof implementation record;
- `src/moo_conformance/_tests/builtins/yin_semantics.yaml`: durable `yin()`
  coverage in the conformance repository;
- `src/moo_conformance/_tests/builtins/connection_name_semantics.yaml`: durable
  basic connection-name coverage;
- `src/moo_conformance/_tests/audit/connection_lifecycle_toast_oracle.yaml`:
  partial trusted-PROXY and login lifecycle coverage.

The existing live-login tools are part of the recovered authority, not examples
to recreate:

- `cmd/moo_client/main.go` and the built `moo_client.exe` drive a real socket,
  preserve partial prompt bytes, and provide `-banner-wait`, `-inter-cmd`, and
  idle `-timeout` controls;
- `reports/read-login-verifier.md` records the proven Mongoose settings
  `-banner-wait 3000 -inter-cmd 2500` and the exact trusted-PROXY -> account ->
  password interaction;
- `notes/mongoose-differential-2026-07-01.md` records the July 1 fresh database
  fetch and repeated live use of `moo_client.exe`;
- `cmd/toast_oracle` and `scripts/wsl_oracle.sh` are emergency-expression tools.
  They do not replace `moo_client.exe` for login, connection, or room-render
  behavior.

Do not scrape notes to reconstruct a client, replace `moo_client.exe` with an
ad hoc socket script, or infer the login conversation from a nearby database.
Run the existing tool and observe the selected fixture first.

### Conformance repository boundary (user correction, 2026-07-13)

`moo-conformance-tests` must know nothing about Mongoose. Mongoose fixtures,
profiles, names, login flows, environment hooks, accounts, passwords, and other
credentials must never appear in its tests or test-support code.

Live Mongoose runs are discovery and deployment evidence in the Barn
repository only. Every discovered behavioral issue must be minimized onto the
bundled conformance `Test.db` before it becomes a durable cross-engine test. If
a behavior cannot be reproduced faithfully with `Test.db`, stop and report the
blocked reduction; do not put a Mongoose-specific test into the conformance
repository.

The July 1 campaign landed six Barn fixes but only two corresponding conformance
suites. The real trusted-PROXY -> account login -> room render path, startup
responsiveness, burst input, and most gameplay behavior were not captured as a
single durable cross-engine gate.

## Current Profile And Process Gaps

### Mongoose profiles do not enable Mongoose semantics

`profiles/barn/profiles.json` names Mongoose profiles, but their config files
only set `OUTBOUND_NETWORK`. They do not set `PROMOTE_NUMBERS = 1`.

Barn already publishes `option.PROMOTE_NUMBERS` in profile manifests. However,
the conformance profile gate currently requires only
`option.OUTBOUND_NETWORK`. A strict-mode target can therefore be accepted as a
Mongoose comparison.

### Active oracle guidance has drifted

`CLAUDE.md` still contains a Windows Toast server command even though the WSL
oracle report and current project instructions require WSL Toast. The correct
procedure must live in tracked active instructions, not only in an untracked
side report.

### Existing PROXY coverage is incomplete

The current conformance row proves that a trusted `PROXY` prelude is cleared
before `do_login_command` runs. It does not prove:

- adoption of the announced client address;
- the Mongoose account-login conversation;
- successful room entry and rendering;
- input delivery while the Mongoose world is busy;
- burst input through `hold-input` and `read()`;
- reconnect and disconnect hooks in the real connection contract.

## Authoritative Fixture Identity

The earlier plan incorrectly treated `mongoose.db` matching a recorded checksum
as proof that it was fresh. A checksum proves identity, not provenance. All
July 13 managed results using this file are rejected:

- rejected file: `C:/Users/Q/code/barn/mongoose.db`;
- size: `100959239` bytes;
- SHA-256: `a9d167861eab56d62e9bd12ae1d47c5e6a858530020a5dcf174a0b104fb23db9`;
- rejection reason: no proof that the file was a fresh upstream
  `mongoose.db.new` rather than a Barn-written or otherwise evolved copy.

The recovered known-good control fixture is the file Claude fetched from
`mongoose@mongoose.world:~/mongoose/mongoose.db.new` on July 1:

- local path: `C:/Users/Q/code/barn/mongoose_fresh2.db`;
- size: `101244108` bytes;
- SHA-256: `33201970097d3d2d2bfc0d5f875f087d587601bf8255ef31ef19b416d65ac925`;
- provenance record: `notes/mongoose-differential-2026-07-01.md`;
- matching untouched WSL discovery copy: `/tmp/mg_in.db`.

Use this exact snapshot as the fixed login control. Re-hash it immediately
before every control run, copy it to a disposable run location, and never run
Barn or Toast directly against the source file.

The current upstream snapshot was fetched on July 13 with the exact source
`mongoose@mongoose.world:~/mongoose/mongoose.db.new` and Windows-native `scp`:

- fetched path: `C:/Users/Q/code/barn/.tmp/mongoose-refresh-20260713/mongoose.db.new`;
- size: `98434477` bytes;
- SHA-256: `b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.

The fetched file was copied separately for Toast and Barn; neither engine ran
against the fetched source path. Its unchanged WSL Mongoose Toast login passed,
so this identity is the current convergence fixture. A later upstream fetch is
a new identity and must repeat this exact fetch, hash, disposable-copy, and
Toast-baseline sequence before use.

## Corrected Execution State (2026-07-13)

Milestones 0 and 1 are committed. The first Milestone 2 attempts are invalid
because they used the rejected fixture and a reconstructed login invocation.
They are not evidence that Mongoose fails on Toast and must not be used to
justify a Barn change.

The recovered Claude workflow was rerun successfully on July 13 with:

- WSL Mongoose Toast source commit
  `72e3c7f96ce7a41fdeba793aef8818dc4408072e`;
- `/root/src/toaststunt-mongoose/build-release/moo`, executable SHA-256
  `a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098`;
- a disposable copy of `mongoose_fresh2.db`;
- `moo_client.exe -banner-wait 3000 -inter-cmd 2500 -timeout 15`;
- the trusted-PROXY prelude followed by the account and password prompts.

The result was a successful connection as player `#249` and a complete room
render. Stable observed anchors were:

- `[Toint Town; Zemilda's Tea House]`;
- `The interior of the tea house is warm and close`;
- `You can go northwest.`

A later `Confunc failed: This database is not open.` traceback occurred after
the room render in optional sound/database code. It does not negate successful
login or room control, but it is a separate candidate delta only after it is
reproduced through the Toast-first row.

### Current upstream differential (2026-07-13)

The fixed control was rerun first and passed unchanged on the pinned WSL
Mongoose Toast binary. Player `#249` connected and the complete Zemilda's Tea
House render contained all three recorded anchors above.

The same login-only invocation was then run on a disposable copy of the current
upstream snapshot. It used only the existing `moo_client.exe`,
`-banner-wait 3000 -inter-cmd 2500 -timeout 15`, the trusted-PROXY prelude, the
account, and the password. No `look`, `@test`, selection response, reconstructed
login command, or additional wait was added. WSL Mongoose Toast passed:

- player `#249` connected;
- `[[Trojanovich plaza; Daystrom Annex]; Codex's Lab]` rendered completely;
- `You can go west, northeast (closed), and north (closed).` appeared;
- the later optional SQLite `This database is not open` traceback appeared;
- during a three-minute observation, repeated `#4143:cycle "Already cycling"`
  and optional SQLite batch-insert errors were visible to the connected wizard.

Windows Barn was built from `b78f76e0595409de2f77e62d6c396083e5fb97f5`
with the existing tracked dirty plan only. Its managed profile manifest recorded
the current fixture checksum and both `option.OUTBOUND_NETWORK=true` and
`option.PROMOTE_NUMBERS=true`. The unchanged login-only invocation authenticated
and printed `Welcome!`, but did not emit Toast's MCP line, connected-player line,
or room render before the 15-second idle timeout.

Barn's structured log contains the preceding concrete delta:

```text
panic in task: runtime error: index out of range [171] with length 32
task_id=1783980338 this=0 verb=server_started
vm.(*VM).executeLoop -> scheduler.(*Scheduler).runTask
```

No additional Barn error appeared during the three-minute observation. The
current active behavior row is therefore the startup fork panic, not the later
`look` or `@test $pbt` symptoms from the manual transcript.

The triggering source is already localized. Current `#0:server_started` lines
14-21 fork a zero-delay body containing `try`/`except`. Barn extracts that body
with `bytecode.Program.ExtractForkBody`, but the extracted program retains the
parent program's absolute exception-handler instruction pointer. When the fork
body handles an exception, `vm.HandleError` assigns parent IP `171` inside the
32-byte extracted program and the unchecked fetch at `vm/vm.go:273` panics.
Relative jump operands are not part of this delta.

## Windows-To-WSL Connectivity (read this before any managed WSL Toast run)

Verified 2026-07-13. Two independent walls sit between the Windows harness and
a WSL-hosted Toast. Both produce misleading errors. Diagnose in this order
before concluding anything about Toast, Barn, the harness, or the fixture.

### Wall 1: localhost forwarding to WSL can be dead

Symptom: the harness fails with `Server did not start accepting connections
on port N within 30.0s`, or a direct client gets `connection refused` on
`localhost`/`127.0.0.1`. The message is a lie in this state: the server is
up; the dial path is dead.

Proof procedure (run exactly this before believing any other theory):

```powershell
wsl -d Debian -u root -e bash -c "ss -tlnp | grep <port>"   # server IS listening
wsl -d Debian -u root -e hostname -I                        # NAT IP, e.g. 172.17.144.45
```

If the listener exists but Windows cannot reach `127.0.0.1:<port>`, localhost
forwarding is broken VM-wide. Facts established 2026-07-13:

- a fresh trivial listener reproduces it, so it is not Toast-specific;
- `wsl --terminate Debian` does NOT fix it — the relay lives in the shared
  WSL utility VM, which stays up while any distro (docker-desktop) runs;
- the only full reset is `wsl --shutdown`, which kills the user's live
  Docker containers (azuracast radio, accessmap). NEVER run `wsl --shutdown`
  without the user's explicit approval;
- the working route is the WSL NAT IP from `wsl hostname -I`, passed to the
  harness as `--moo-host <NAT-IP>`. `ManagedServer` accepts a non-localhost
  host (guard removed 2026-07-13).

### Wall 2: Toast reverse-DNS stalls connections from the Windows side

Symptom: the connection is accepted but the banner/login output arrives ~10s
late; the harness dies with `TimeoutError` in `transport.py` `_receive`, or a
manual login takes ~20s instead of instantly.

Cause: Toast reverse-DNS-resolves every incoming connection. From the WSL NAT
gateway (the Windows host, e.g. `172.17.144.1`) that lookup hangs.

Fix: `scripts/run_toast_wsl.sh` now appends the gateway to WSL `/etc/hosts`
(idempotent, re-asserts after distro restarts because WSL regenerates
`/etc/hosts`). If a manual Toast launch bypasses the wrapper, apply the same
line first:

```bash
gw=$(ip route show default | awk '{print $3}')
grep -q "^$gw " /etc/hosts || echo "$gw windows-nat-gateway" >> /etc/hosts
```

### Failure-to-action table

| Observation | Action | Do NOT |
|---|---|---|
| "did not start accepting connections within 30s" on a WSL server | Run the Wall 1 proof; use `--moo-host <NAT-IP>` | Conclude Toast/the harness is broken; rewrite the server command |
| `connection refused` on localhost, listener present in WSL | Same as above | `wsl --shutdown` without user approval |
| recv `TimeoutError` after a successful accept | Check Wall 2; verify the hosts entry exists in WSL | Raise harness timeouts; blame the test |
| `invalid-comparison: manifest runtime_os differs` | You paired a WSL oracle manifest with Windows Barn: omit `--oracle-profile-manifest` for the deployment lane | Edit manifests to lie about `runtime_os` |
| Row fails on Toast | The test is wrong; fix the test, never run Barn | Patch Barn |

## Structured Observability

Barn now has the diagnostics needed for this campaign:

- every run writes readable stderr and structured JSONL;
- `logs/latest.jsonl` is the stable current-run path and older runs rotate;
- records carry fields such as `conn_id`, `task_id`, `player`, `this`, `verb`,
  `error`, `err`, `traceback`, `frames`, and `go_stack`;
- an uncaught MOO error is one record containing the player-visible traceback,
  structured frames, and source lines;
- `barn_logs` filters a run, expands tracebacks and Go stacks, and exits nonzero
  if the selected records include an error;
- `/debug/vars` exposes task, connection, GC, panic, exception, and checkpoint
  counters;
- `/debug/pprof/` exposes profiles on a localhost-only endpoint;
- `/debug/loglevel` changes the running server's log level without restarting
  it.

These are diagnostic tools, not an oracle. A plausible JSON record, counter, or
profile does not replace a Toast-passing conformance test.

Some log message text is already a conformance contract. Preserve asserted text
byte-for-byte and add structured attributes rather than rewording it.

## Git Accountability

Before editing in either repository:

1. record branch, HEAD, and tracked-file status;
2. identify the exact intended files for the active slice;
3. do not combine multiple experimental source slices in one worktree;
4. finish each slice as either a committed kept improvement or a full revert;
5. commit conformance changes in `moo-conformance-tests` and Barn changes in
   `barn` separately;
6. stage exact paths only and preserve unrelated untracked files.

Experiment records required by this plan are repository records, not chat-only
summaries.

## Milestone 0: Repair The Durable Control Surface

Reconcile the useful untracked Mongoose/oracle records into this tracked plan
and the active project instructions.

Required work:

- replace stale Windows Toast commands with the WSL-managed workflow;
- record the exact Barn SHA, both Toast SHAs, selected database checksum, and
  connection contract for each run;
- document the environment variable used to provide the uncommitted login
  script without recording credentials;
- remove or clearly supersede conflicting process instructions;
- commit the documentation-only correction before behavioral source work.

Acceptance criteria:

- another agent can start the exact managed oracle workflow using tracked files
  alone;
- no active instruction directs Barn conformance work to Windows Toast;
- the selected fixture and engine identities are explicit.

## Milestone 1: Make Profiles Truthful

Add explicit Mongoose profiles rather than reusing strict profiles with a
Mongoose name.

Required work:

- add Mongoose config files with `PROMOTE_NUMBERS = 1` and an explicit outbound
  networking value;
- include `option.PROMOTE_NUMBERS` in each Mongoose profile's expected features;
- require promotion compatibility in the conformance profile gate;
- define stock WSL Toast and WSL Mongoose Toast oracle manifests;
- fail closed when promotion metadata is missing or mismatched.

Use two validation lanes:

1. WSL Toast versus WSL Barn for OS-matched semantic conformance;
2. Windows Barn against the same tests and fixture for the real deployment
   proof.

This avoids lying about `runtime_os` while still proving the Windows target.

Acceptance criteria:

- a strict Barn profile cannot masquerade as Mongoose-compatible;
- a missing or mismatched promotion feature prevents tests from starting;
- the selected database checksum and runtime options appear in both manifests.

## Milestone 2: Capture The Present Break

Start from a proven live observation, identify the smallest server behavior it
depends on, and reproduce that behavior with bundled `Test.db` through the
managed harness. Do not modify the conformance transport unless an independent
`Test.db` row proves a transport defect.

For every observation:

1. state the implementation-independent behavior;
2. build the smallest `Test.db` setup that preserves the triggering workload or
   state;
3. run the exact row on the documented stock WSL Toast oracle;
4. run the unchanged row on pre-fix Barn;
5. keep it only when Toast is green and Barn shows the concrete delta;
6. record Mongoose-specific discovery details only in Barn-local records.

The first reduced scenario is scheduler/input fairness. It uses six finite
background tasks in `Test.db` and requires a fresh eval to be admitted before
the whole ready batch starts. Stock WSL Toast passed, Barn `8fe7e6a` failed
with expected `1` and actual `0`, and corrected Barn passed. The durable test is
conformance commit `8de1c22`; the Barn fix and investigation record are commit
`b78f76e`.

The test must pass WSL Toast first. If Barn already passes, keep the test as
coverage and do not patch Barn. If Barn fails, record the exact failed assertion
as the active behavior row.

Acceptance criteria:

- the Barn-local discovery observation is recorded without placing Mongoose or
  credentials in the conformance repository;
- the exact managed stock-Toast command, `Test.db` checksum, focused test name,
  and pass result are recorded;
- the same unchanged test produces a concrete Barn result;
- `src/moo_conformance/transport.py` has no change for this row;
- no Barn production change precedes the Toast result and Barn red proof.

## Milestone 3: Diagnose The Red Barn Run

Run Barn through the managed harness with its structured log directed to a
stable run directory.

Diagnostic sequence:

1. run `barn_logs -level error`;
2. correlate `conn_id`, `task_id`, player, verb, traceback, frames, and source;
3. snapshot `/debug/vars` before the PROXY prelude, during login, and after the
   observed timeout or failure;
4. if the server is alive but stalled, collect pprof from the already-running
   process;
5. raise the running server to debug only when a named decision requires more
   detail, then turn it back down.

Add instrumentation only when current evidence cannot distinguish two concrete
hypotheses. Extend existing `slog` or metric choke points; do not introduce a
new logging facade, sender, adapter, or service container.

Acceptance criteria:

- the failing ownership boundary is supported by evidence;
- diagnostics are preserved with the run record;
- logs and profiles are not presented as substitutes for the conformance row.

## Milestone 4: Close One Delta At A Time

Every behavior row follows this state machine:

`Mongoose observation -> reduced test -> Toast green -> Barn red -> Barn fix -> Barn green -> full gate`

Rules:

- require a reduced `Test.db` test for every conformance behavior;
- if reduction would erase the behavior or workload condition, stop and report
  the blocked reduction instead of adding a Mongoose-specific conformance test;
- patch the real Barn ownership boundary, not scattered symptoms;
- run focused Go tests, the focused conformance row, the relevant managed suite,
  and `git diff --check`;
- commit the conformance test and Barn fix separately before selecting another
  source slice;
- if two consecutive slices on the active target produce no kept improvement,
  stop and report that result instead of widening the search.

The first behavior family is boot, login, and connection behavior because it
gates every later Mongoose action:

1. startup responsiveness under restored background tasks;
2. trusted-PROXY metadata rewrite;
3. account-login `read()` sequence;
4. burst input and `hold-input` behavior;
5. reconnect and disconnect hooks.

After that family closes, proceed one family at a time:

- `look`, movement, contents, exits, and room rendering;
- parser shortcuts, `huh`, and command dispatch;
- `@who`, `@display`, `@props`, `@verbs`, and object inspection;
- telnet negotiation and packet-boundary behavior;
- MCP, GMCP, and out-of-band traffic;
- task scheduling, suspended reads, queued tasks, restart, and persistence;
- remaining promotion-sensitive comparison, sorting, map, and collection
  semantics.

For raw telnet behavior, split the significant bytes across separate
`send_bytes` steps so parser state across reads is part of the regression.

## Milestone 5: Make "Amazingly" Measurable

Functional compatibility comes first. Once the boot/login gate is durable,
measure the same fixture and script on WSL Mongoose Toast and Barn.

Baseline metrics:

- database load to listening;
- PROXY prelude to first banner;
- complete login time;
- command latency during startup jobs;
- `look` and movement latency;
- settled CPU and memory;
- task and connection liveness;
- checkpoint duration.

Do not choose performance targets before measuring Toast. Record the baseline,
then define the Barn acceptance threshold in the repository before optimizing.

For each performance slice:

1. name one metric and one hypothesis;
2. capture the before measurement and relevant profile;
3. change one production surface;
4. rerun the same benchmark and conformance gate;
5. commit a measured improvement or fully revert the slice.

A profiler-looking result is not proof of a bottleneck. A performance change is
not kept unless it improves the named metric while preserving behavior.

## Completion Criteria

The workstream is complete only when:

- Barn-local live checks cover the deployment workload without exporting its
  fixture, login contract, or credentials into `moo-conformance-tests`;
- every accepted semantic delta has a Toast-passing conformance test;
- every conformance test uses bundled `Test.db` and contains no Mongoose
  knowledge;
- Barn passes the expanded managed `Test.db` suite under truthful strict
  profiles;
- Windows Barn passes the separate Barn-local deployment lane;
- trusted-PROXY metadata, account login, burst input, and connection lifecycle
  are durably covered;
- gameplay and persistence behavior families have no open accepted rows;
- performance targets derived from Toast are met or explicitly deferred by the
  user;
- no accepted behavior exists only in chat, memory, untracked notes, an old
  log, or an ad hoc reconstructed command;
- every kept source slice and required experiment record is committed.

Milestones 0 and 1 are complete. The first Milestone 2 `Test.db` reduction and
Barn fix are committed.

### Slice executed 2026-07-13: forked-`try` handler rebase

The `server_started` forked-`try` slice below was executed end to end on
2026-07-13. Live step-zero observation: Toast control on a disposable copy of
fixture `b9bc254...` rendered Codex's Lab as player `#249`; Barn from
`b78f76e` authenticated but produced no MCP line or render, with the
structured-log panic `index out of range [171] with length 32` in
`server_started`. Toast-green (managed row, 3.49s), Barn-red
(`expected 'E_INVARG', but got 0`), unit regression red→green
(`bytecode/program_test.go`), fix in `ExtractForkBody` only, family
`task_scheduling_toast_oracle` 22/22 green. The step-8 login gate re-check on
rebuilt Barn is the remaining acceptance item for this slice.

Step 8 was run twice on the rebuilt Barn (3 and 10.5 minutes uptime,
fresh disposable copy, hash re-verified): the `server_started` panic is gone
and authentication completes through `(***) WELCOME! (***)`, but no MCP line,
no connected-player line, and no room render arrive. The step-8 gate remains
OPEN and no further Barn change was made for it, because it is a new
behavior row, not the closed one.

### Active behavior row (pinned 2026-07-13): post-authentication silence

Observation, live and current, same fixture both engines:

- Toast control: MCP line, `Q (#249)` connection line, complete room render;
  continuous in-db heartbeat activity visible to a connected wizard;
- Barn (with the forked-try fix): authenticated login then silence; the
  world is nearly idle (`barn.tasks_started` 25 after 10 minutes,
  `tasks_live` 2) where Toast churns continuously. A later Barn
  `#1584:bf_call_function` line 9 `E_RANGE` was captured as
  `call_function("mapdelete", [#3516 -> ["client" -> "Mongoose Client",
  "version" -> "0.1"]], #249)`, but its timestamp follows the client timeout,
  EOF, and connection close. It is a post-disconnect cleanup error and cannot
  cause the earlier post-authentication silence.

Candidate dependencies to isolate (in the Milestone 4 loop, one at a time,
reduced onto `Test.db`, Toast first — no Barn debugging before a red row):
`#0:user_connected` dispatch and its forked confunc chain, the in-db
scheduler heartbeat's survival (task_id stability across suspend, fork
cadence), and the first pre-timeout task or verb-lifecycle divergence after
authentication. The post-disconnect `mapdelete` error is not part of this
active row.

### Slice executed 2026-07-13: nested fork after suspended heartbeat

Two prerequisite/coverage-only conformance commits landed first. Commit
`0ebcf88` makes the trusted-PROXY lifecycle rows trust the actual connected
peer instead of assuming localhost, and commit `0df1dac` proves both Toast and
Barn dispatch `user_connected` on a first login. The latter rules out the basic
hook dispatch itself without inventing a Barn change.

The next reduced row is
`audit_background_task_id_stable_across_suspend_and_fork` in
`task_scheduling_toast_oracle.yaml`. It uses only bundled `Test.db` (SHA-256
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`)
and asserts that a background task keeps the fork-assigned `task_id()` across
`suspend(0)`, creates a nested zero-delay fork, yields to it, and then resumes.
The exact focused oracle command was the plan's managed WSL Toast command with
selector `-k audit_background_task_id_stable_across_suspend_and_fork`; stock
Toast `aecc51e` passed 1/1. The complete managed Toast family passed 23/23.

Pre-fix Windows Barn `87d67fa` failed the unchanged row with expected
`[1, 1, [1, 1]]` and actual `[1, 1, []]`. Stable diagnostic runs are under
`.tmp/mongoose-convergence/heartbeat-red-20260713-2145` and
`.tmp/mongoose-convergence/heartbeat-red-debug-20260713-2147`. `barn_logs`
reported no error-level record: the task silently stopped progressing.

The failing ownership boundary was `scheduler.runTask`. Its bounded inline
handling of a forked task's `suspend(0)` resumed the VM, but if that continuation
yielded `FlowFork`, it skipped `drainForks`, fell through the terminal branch,
and marked the parent completed while the nested child and remaining parent
continuation were still pending. Unit regression
`TestForkedTaskRequeuesAcrossSuspendAndCreatesNestedFork` reproduced the red
state (`completed`, expected `queued`). The production change is confined to
`scheduler/task_runtime.go`: drain an inline-resume `FlowFork`, then return the
result to normal suspend/queue handling so the ready child owns the next turn.

Post-fix proof: the unit regression passed, the unchanged managed Barn row
passed 1/1, and the complete managed Barn family passed 23/23. Conformance
commit is `6a4b416`. `git diff --check` passed. The exact local package gate
`go test ./bytecode ./vm ./scheduler` remains red only at the pre-existing,
tracked unconditional review regression
`TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`; that task-ID
allocator defect is not bundled into this behavior slice. The live Mongoose
login-only gate was rerun after Barn commit `7e9a1f7`.

The step-8 rerun used a disposable copy at
`.tmp/mongoose-convergence/barn-m4-after-2/mongoose.db`; both the source and
copy freshly hashed to
`b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.
Barn used managed profile `barn-windows-mongoose-outbound-on`, and the existing
`moo_client.exe` used only the trusted-PROXY/account/password conversation with
`-banner-wait 3000 -inter-cmd 2500 -timeout 15`. Authentication again reached
`Welcome!` and `(***) WELCOME! (***)`, but Barn emitted no MCP line, no player
`#249` connection line, and no Codex's Lab room render. `barn_logs -level error`
reported no error-level record in
`.tmp/mongoose-convergence/barn-m4-after-2/logs/latest.jsonl`. The heartbeat
slice therefore closes a real scheduler delta but does not close the live gate;
post-authentication silence remains the active behavior row, and no additional
Barn production change is bundled into this slice.

### Coverage executed 2026-07-13: `user_connected` continuation after fork

The next reduced `Test.db` row was
`audit_user_connected_continues_after_zero_delay_fork` in
`connection_lifecycle_toast_oracle.yaml`. It models the relevant control-flow
shape of the current Mongoose hook without importing any Mongoose object,
fixture, credential, or profile knowledge: first login dispatches
`user_connected`, the hook creates a zero-delay child, the parent continues
after `endfork`, and both activations record completion.

Fresh authority checks observed stock Toast HEAD `aecc51e`, executable
`/root/src/toaststunt/build-release/moo`, version `2.7.3_5`, and bundled
`Test.db` SHA-256
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
The focused managed WSL Toast row passed 1/1, and current Windows Barn passed
the unchanged row 1/1. The complete managed connection-lifecycle family then
passed 21/21 on both engines. Conformance commit is `60ca66c`.

Per the plan, this is coverage only: no Barn source change was made. The result
rules out loss of the parent activation at a top-level zero-delay fork and loss
of that immediate child. The live gate remains open, so the next reduction must
preserve a deeper dynamic handler/confunc dependency or the independently
observed `call_function` argument that produced Barn-only `E_RANGE`; it must
again pass Toast before Barn runs.

### Coverage executed 2026-07-13: dynamic `user_connected` handler

The next reduced row was
`audit_user_connected_dynamic_handler_continues` in the same lifecycle family.
It models the current hook's pre-fork dynamic dispatch boundary: a first-login
`user_connected` activation invokes another object's same-named verb through
`handler:(verb)(@args)` under an `ANY` error-catching expression, then continues
the parent hook. The row contains only generic `Test.db` objects and markers.

The first Toast draft was correctly rejected with both markers unset because
it omitted the MOO backtick/apostrophe error-catching delimiters; Barn was not
run on that invalid draft. After correcting the unchanged behavior to
`` `handler:(verb)(@args) ! ANY' ``, fresh authority checks again observed
stock Toast `aecc51e`, ToastStunt `2.7.3_5`, and `Test.db` SHA-256
`1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`.
The corrected focused row passed 1/1 on both managed WSL Toast and current
Windows Barn, and the complete connection-lifecycle family passed 22/22 on
both. Conformance commit is `80f19f2`.

This is another committed coverage reduction, not an unproductive slice, and
it authorizes no Barn source change. It rules out the dynamic handler call and
continuation boundary itself. The next reduction stays in the same live
`user_connected` chain and must preserve the synchronous post-fork permission
change plus nested confunc call before considering the separate `E_RANGE` row.

### Coverage executed 2026-07-13: post-fork permissions and confunc chain

The next reduced row is
`audit_user_connected_confunc_calls_continue_after_fork_and_setting_task_perms`
in `connection_lifecycle_toast_oracle.yaml`. A first-login `user_connected`
activation creates a zero-delay child, continues synchronously, changes task
permissions to the login player, invokes both the player's location and player
`confunc` verbs, and records the final parent continuation. It contains only
generic objects created in bundled `Test.db`.

The initial draft covered the permission and confunc calls but omitted the
post-fork control-flow dependency, so it was strengthened before commit. The
first strengthened Toast assertion incorrectly required the child to retain
`#0` permissions; Toast returned the intended child marker while using a
different inherited permission value. The final row asserts child execution
without over-specifying that unrelated value, while requiring the parent and
both confunc calls to observe the login player's permissions.

The focused final row passed 1/1 on managed WSL Toast and unchanged Windows
Barn. The complete managed connection-lifecycle family passed 23/23 on both
engines. Conformance commit is `5ee57e9`. This is coverage only and authorizes
no Barn production change. The next independent reduction is the observed
`call_function` argument that produced Barn-only `E_RANGE`; it must pass Toast
before Barn runs.

### Coverage executed 2026-07-13: indirect SQLite invalid handle

Fixture inspection confirmed that `#3882:is_open` calls
`call_function("sqlite_info", this.handle)`, and the earlier live trace recorded
handle `2`. Existing conformance covered direct `sqlite_info(999999)` but not
error propagation through `call_function`, so
`sqlite_info_invalid_handle_through_call_function` was added to `sqlite.yaml`
with the generic expression `call_function("sqlite_info", 2)`.

The final focused row passed 1/1 on managed WSL Toast and unchanged Windows
Barn. The complete SQLite-selected family passed 91/91 on both engines.
Conformance commit is `73a178e`. This genuine candidate is retained as
coverage and authorizes no Barn production change; it proves the July 1
SQLite-handle call is not sufficient to reproduce the current Barn-only
`E_RANGE`.

The subsequent temporary diagnostic captured the exact failing call as
`mapdelete([#3516 -> ["client" -> "Mongoose Client", "version" -> "0.1"]],
#249)`. The diagnostic record is at `23:32:49.097`, after client EOF and
connection close at `23:32:49.089` and `23:32:49.090`. The temporary source
diagnostic has been removed. This call is therefore excluded from the active
login-silence row rather than promoted into a conformance slice.

Next steps for the active row:

1. Run the existing managed Barn login-only gate with the pinned client timing
   and capture the structured task and verb lifecycle from authentication
   through the client timeout; do not add post-login commands.
2. Compare that pre-timeout interval with the already-established Toast live
   control and identify the first behavior that Toast performs but Barn does
   not, or the first Barn task that stops before producing output.
3. Reduce only that exact pre-timeout behavior onto generic `Test.db`, run the
   focused row on managed WSL Toast first, then run the unchanged row on Barn.
4. Keep coverage only if Barn passes. If Barn fails, add the smallest unit
   regression, implement the smallest production fix, run the named family and
   full gates, commit the kept slice, and rerun the live login-only gate.

### Slice implementation 2026-07-14: anonymous caller identity

The managed Barn login-only gate was repeated before changing conformance or
production code. It reproduced authentication through `(***) WELCOME! (***)`
followed by silence. Barn's existing execution trace then identified the first
pre-timeout exception: after `CONN LOGIN` at trace line 894,
`#1289:user_connected` created an anonymous MCP session and called its inherited
`initialize_connection`; the nested `this:send()` raised `E_PERM` at line 934,
before the MCP notification or room render. The later `mapdelete` error remained
post-disconnect and unrelated.

The implementation-independent reduction is
`anonymous_nested_this_call_preserves_caller_identity` in
`language/anonymous.yaml`: an anonymous instance calls an inherited outer verb,
which calls `this:inner()`, and the inner verb requires both
`typeof(caller) == ANON` and `caller == this`. Managed stock WSL Toast passed
the focused row 1/1. Pre-fix Windows Barn failed the unchanged row with expected
`[1, 1, 1]` and actual `[1, 0, 0]`.

Unit regression `TestBytecodeAnonymousNestedThisCallPreservesCallerIdentity`
failed at the same caller-type assertion before the fix and passed afterward.
The production change is confined to `vm/op_verb.go`: when binding the nested
verb's `caller` local, preserve the current context's non-object `ThisValue`
instead of always rebuilding it as `types.NewObj(currentFrame.This)`. Normal
object callers retain the existing object value.

Post-fix proof so far: the focused managed Barn row passed 1/1; the managed
anonymous family passed 88 with 7 established skips on both WSL Toast and
Windows Barn; `git diff --check` passed. The exact local package gate remained
green in `bytecode` and `vm` and red only at the already-recorded scheduler
regression `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`.
The conformance commit is `dca934b`. The post-commit live login-only gate is the
remaining acceptance item for this slice.

The slice recipe below is retained because it is the template for every
following slice. Execute it exactly:

1. In `moo-conformance-tests`, add only
   `audit_forked_try_except_rebases_handler_ip` to
   `src/moo_conformance/_tests/audit/task_scheduling_toast_oracle.yaml`. Its
   setup, assertion, and cleanup are:

   ```moo
   try
     add_property(#0, "audit_fork_except_result", 0, {#0, "rw"});
   except (E_INVARG)
     #0.audit_fork_except_result = 0;
   endtry
   fork (0)
     try
       raise(E_INVARG);
     except e (ANY)
       #0.audit_fork_except_result = e[1];
     endtry
   endfork
   suspend(0);
   return #0.audit_fork_except_result;
   ```

   Expect the value `E_INVARG`, assert that the server log does not contain
   `panic in task`, and delete `#0.audit_fork_except_result` in cleanup. Do not
   add Mongoose names, objects, fixture data, or profile knowledge to the test.

2. Run that unchanged row on managed stock WSL Toast from
   `C:/Users/Q/code/barn`:

   ```powershell
   $wslIp = (wsl -d Debian -u root -e hostname -I).Trim()
   uv run --project ..\moo-conformance-tests moo-conformance `
     --moo-host $wslIp `
     --server-command "wsl -d Debian -u root -e env TOAST_MOO=/root/src/toaststunt/build-release/moo bash /mnt/c/Users/Q/code/barn/scripts/run_toast_wsl.sh {db} {port}" `
     --server-db C:/Users/Q/code/moo-conformance-tests/src/moo_conformance/_db/Test.db `
     --oracle-profile-manifest C:/Users/Q/code/barn/profiles/toast/stock-wsl-testdb.json `
     --target-profile-manifest C:/Users/Q/code/barn/profiles/toast/stock-wsl-testdb.json `
     -k audit_forked_try_except_rebases_handler_ip
   ```

   `--moo-host` is required while Windows→WSL localhost forwarding is broken
   (see the connectivity section above; the failure without it is a
   misleading "did not start accepting connections").

   Keep the row only if this command exits zero with the named test passed.
   Any skip, startup failure, connection failure, or failed assertion stops the
   slice; record the exact command and output and do not run Barn.

3. Build pre-fix Barn and run the unchanged row through the managed Windows
   Barn profile:

   ```powershell
   go build -o .tmp/mongoose-convergence/barn.exe ./cmd/barn
   uv run --project ..\moo-conformance-tests moo-conformance `
     --server-command "C:/Users/Q/code/barn/.tmp/mongoose-convergence/barn.exe --db {db} --listen tcp://127.0.0.1:{port} --checkpoint-interval 0 --config C:/Users/Q/code/barn/profiles/barn/outbound-on.conf --profile-id barn-windows-testdb-outbound-on --profile-manifest {manifest}" `
     --server-db C:/Users/Q/code/moo-conformance-tests/src/moo_conformance/_db/Test.db `
     -k audit_forked_try_except_rebases_handler_ip
   ```

   Do not pass `--oracle-profile-manifest` here: this is Milestone 1's
   deployment lane (Windows Barn), and the profile gate correctly refuses a
   cross-OS oracle pairing (`runtime_os` linux vs windows). The Toast-green
   evidence comes from step 2's separate run, not from a paired manifest.

   The required pre-fix result is the named row failing because the forked
   exception handler does not set the marker, with `panic in task` or the same
   out-of-range failure in the managed server log. If Barn passes unchanged,
   commit the Toast-passing coverage only and stop without a Barn source patch.

4. After Toast-green and Barn-red are recorded, add a bytecode unit regression
   covering a fork body containing `try`/`except`, then change only
   `bytecode.Program.ExtractForkBody` in `bytecode/program.go`. Rebase the
   absolute handler targets encoded by `OP_TRY_EXCEPT` and `OP_TRY_FINALLY` by
   subtracting the extracted body's parent `bodyIP`. Do not change relative
   jump operands, add a fallback bounds check to `vm.executeLoop`, or introduce
   an interface, adapter, alternate VM path, or Mongoose special case.

5. Run exactly these local gates:

   ```powershell
   go test ./bytecode ./vm ./scheduler
   git diff --check
   ```

6. Rerun the unchanged managed Barn row from step 3. It must pass with no panic.
   Then run the complete managed task-scheduling family by replacing the final
   selector with `-k task_scheduling_toast_oracle`. Both commands must exit
   zero before keeping the Barn source slice.

7. Commit the Toast-passing conformance row in `moo-conformance-tests`. Commit
   the bytecode regression, `ExtractForkBody` fix, and this Barn-side run record
   in `barn` as a separate commit. Stage only the named files.

8. Rebuild Barn and repeat the exact trusted-PROXY/account/password login-only
   run on a disposable copy of
   `b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.
   The gate is closed only when Barn emits the MCP line, player `#249`
   connection, complete Codex's Lab render, and no `server_started` panic.
   Do not send `look`, `@test $pbt`, or any other command until this gate passes.
