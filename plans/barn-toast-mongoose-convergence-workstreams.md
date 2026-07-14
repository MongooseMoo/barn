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
failed at the same caller-type assertion before the first fix and passed
afterward. That first implementation bound `caller` from the task-global
`vm.Context.ThisValue` and landed as Barn commit `283f76d`. Its post-commit live
login gate failed before the banner: every input reached
`#0:do_login_command` line 24 and raised `E_TYPE` while calling the selected
login verb. This proved that `Context.ThisValue` can retain an anonymous value
across later, ordinary object-call task boundaries and is not valid frame
ownership.

The unit regression was extended with a stale anonymous context followed by an
ordinary `#0` verb call. Commit `283f76d` returned `TYPE_ANON` for that call;
the required result is `TYPE_OBJ`. The corrected production change gives each
`StackFrame` its own optional `ThisValue`. Nested call and `pass()` frames copy
the actual non-object receiver, while the initial run, prepared verb, and eval
frames explicitly initialize it to `None`. `executeCallVerb` now binds
`caller` from the active frame only, so an earlier task context cannot
contaminate an ordinary object call.

Post-correction proof so far: the extended unit regression passed; the focused
managed Barn row passed 1/1; the managed anonymous family passed 88 with 7
established skips on both WSL Toast and Windows Barn; and `git diff --check`
passed. The exact local package gate remained green in `bytecode` and `vm` and
red only at the already-recorded scheduler regression
`TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`. The
conformance commit is `dca934b`, and the corrected Barn commit is `7c46984`.
Its post-commit live login-only gate emitted the MCP line for the first time,
proving the anonymous `send()` path was repaired, but still emitted no player
connection line or room render before timeout. The remaining live failure was
therefore narrowed to the continuation after MCP rather than attributed back to
the closed anonymous-caller delta.

### Slice implementation 2026-07-14: resumable `user_connected` hook

The trace-only repeat after `7c46984` used the same three login inputs and no
post-login command. `#0:user_connected` created its handler fork, completed the
MCP session and notification, then synchronously called `$wizinfo` at line 35.
That nested call reached `#56:suspend_if_needed(0)`. Barn subsequently ran all
forked handlers, but the parent activation never resumed at lines 37-63, so
neither `user.location:confunc(user)` nor `user:confunc()` ran before the client
timeout. The trace is under
`.tmp/mongoose-convergence/barn-frame-caller-trace-20260714-02`.

The existing generic row
`audit_user_connected_confunc_calls_continue_after_fork_and_setting_task_perms`
was strengthened with the missing dependency: after creating a zero-delay
child, the hook calls a nested generic verb that performs `suspend(0)`, then
must resume and reach both later confunc markers. Managed stock WSL Toast passed
the strengthened row 1/1. Pre-fix Windows Barn failed the unchanged row with
expected `[1, 1, 1, 1, 1]` and actual `[1, 0, 0, 0, 0]`: the child ran, while
the suspended nested call and every parent continuation marker were lost.

Unit regression `TestUserConnectedResumesAfterNestedSuspendWithPendingFork`
reproduced the same state before the fix and passed afterward. The ownership
error was in `server.InputProcessor.callUserHook`: it used the lightweight
`Scheduler.CallVerb` path, whose task and VM are intentionally throwaway after
the synchronous call returns. A `FlowSuspend` therefore discarded the live
parent continuation. The production change routes lifecycle hooks through the
already-existing `RunServerVerbTask`, which registers the task and preserves
its VM for normal scheduler resumption; no new runtime path or adapter was
introduced.

Post-fix proof so far: the focused managed Barn row passed 1/1; the complete
connection-lifecycle family passed 23/23 on both WSL Toast and Windows Barn;
the full `server` package passed; and `git diff --check` passed. The exact local
package gate remained green in `bytecode` and `vm` and red only at the
already-recorded scheduler ID-collision review regression. The conformance
commit is `09abeec`, and the Barn fix is commit `5045acb`.

The post-commit live login-only gate passed on a fresh disposable copy under
`.tmp/mongoose-convergence/barn-resumable-hook-live-20260714-03`. The source
and disposable database both hashed to
`b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.
Using only the trusted-PROXY/account/password inputs and the pinned client
timings, Barn emitted the MCP line, `Q (#249)` connection line, the complete
`[[Trojanovich plaza; Daystrom Annex]; Codex's Lab]` render, and
`You can go west, northeast (closed), and north (closed).` The structured log
contains no `server_started` panic. The later optional SQLite
`This database is not open` confunc traceback matches the established Toast
control and does not precede or invalidate room entry.

The post-authentication-silence behavior row is therefore CLOSED. The next
unchecked Milestone 4 family is `look`, movement, contents, exits, and room
rendering, using the now-passing login control as its discovery entry point.

### Live coverage 2026-07-14: explicit `look` and room render

The first candidate in the next family used a fourth client command, `look`,
after the unchanged trusted-PROXY/account/password conversation. WSL Mongoose
Toast ran from the pinned executable on the disposable fixture under
`.tmp/mongoose-convergence/toast-look-control-20260714-04`; Barn `5045acb` ran
the unchanged script on the disposable fixture under
`.tmp/mongoose-convergence/barn-look-control-20260714-05`. Both copies hashed
to the current fixture identity.

Both engines produced a second complete Codex's Lab render after the explicit
command. The room title, three description paragraphs, contents, sleeping and
positioned players, player position, and
`You can go west, northeast (closed), and north (closed).` matched. This is
live coverage only: Barn passed the genuine candidate, so no conformance or
Barn source change is authorized. The next unchecked operation in this family
is movement through the open west exit.

### Active behavior row 2026-07-14: movement error propagation

The next live candidate sent `west` as the fourth command on fresh disposable
copies. WSL Mongoose Toast under
`.tmp/mongoose-convergence/toast-west-control-20260714-06` stood the player,
started walking west, then emitted the current fixture's SQLite failure with
the complete journey/travel call chain and the subsequent
`#0:handle_uncaught_error` notification. Barn `5045acb` under
`.tmp/mongoose-convergence/barn-west-control-20260714-07` emitted the same
stand and walk messages, logged `#1584:bf_call_function` `E_INVARG` at the
corresponding pre-timeout instant, but sent no traceback or uncaught-error
notification to the player before the client timed out.

The first generic reduction,
`uncaught_command_call_function_error_preserves_call_chain`, composes an outer
command verb with an inner `call_function` error. It passed focused and full
managed traceback runs on both engines (25/25 family results), so it is
coverage only and authorizes no Barn source change. Conformance commit is
`7d952ef`. It rules out ordinary bytecode command-call propagation. The next
reduction preserved the missing forked-task boundary.

### Slice implementation 2026-07-14: uncaught fork error dispatch

The generic row
`uncaught_forked_call_function_error_reaches_server_handler` creates a debug
command verb whose zero-delay fork calls a nested debug verb that fails through
`call_function`. Stock `Test.db` already defines `#0:handle_uncaught_error` to
write its formatted traceback argument to `server_log`, so the row asserts that
the forked call chain reaches that hook without adding fixture-specific code.

Managed stock WSL Toast passed the focused row 1/1. Pre-fix Windows Barn failed
the unchanged row because `forkboominner` never appeared in the server log.
The live trace already showed that Barn unwound the complete Mongoose chain
from `#3882::execute` through `#249:travel_to`; the loss occurred only when the
forked task reached scheduler completion.

Unit regression `TestUncaughtForkInvokesDatabaseErrorHandler` reproduced the
same boundary: a real zero-delay fork raised `E_TYPE`, but the database handler
received no arguments. `scheduler.runTask` unconditionally suppressed both
logging and fallback delivery for every `IsForked` exception and never invoked
the database handler. The production change now uses the existing resumable
`RunServerVerbTask` path to call `#0:handle_uncaught_error` with Toast's five
arguments: error code, message, value, stack, and formatted traceback. A truthy
return or suspension is treated as handled; a missing, false, or failed handler
falls back to the original traceback without recursively invoking itself.

Post-fix proof: the focused scheduler regression passed; the focused managed
Barn row passed 1/1; and the complete `error_traceback` family passed 26/26 on
both stock WSL Toast and Windows Barn. `git diff --check` passed. The exact
local package gate remained green in `bytecode` and `vm` and red only at the
already-recorded scheduler regression
`TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`. The
conformance commit is `8b6e561`; the Barn fix is `a83e103`.

The fresh live rerun used committed Barn `a83e103`, the existing
`moo_client.exe`, the unchanged trusted-PROXY/account/password inputs, one
additional `west` command, and a disposable copy under
`.tmp/mongoose-convergence/barn-west-handler-live-20260714-09`. Both source and
copy hashed to
`b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.
Barn emitted the same stand and walk messages as Toast, the complete
`$sqlite_db -> $sound_handler -> $Sound -> $waif -> $journey -> $gpo`
traceback, and the subsequent `#0:handle_uncaught_error` output before client
completion. The server log contains no `panic in task`.

The primary `look`, movement, contents, exits, and room-rendering operations
match for the current fixture. Exact convergence of the uncaught-error handler
arguments was later reopened by the parser-shortcut observation below; the
visible traceback alone did not prove the structured stack argument matched.
The next family remains parser shortcuts, `huh`, and command dispatch after
that shared handler contract is closed.

### Slice implementation 2026-07-14: connection-option snapshot race

The first parser-family candidate sent the generic unknown command
`codex-no-such-command`. WSL Mongoose Toast completed login and replied
`I don't understand that.` Barn did not reach the command: its process exited
during login first, with `fatal error: concurrent map iteration and map write`
in `builtins.getConnectionOptions` while `InputProcessor.HandleConnection`
read the `binary` option. The missing `huh` response is therefore not evidence
of a command-dispatch delta and must not be reduced as one.

The Go race regression `TestConnectionOptionsConcurrentReadWrite` failed on
pre-fix Barn with concurrent iteration in `getConnectionOptions` at
`builtins/network.go:207` and mutation in `setConnectionOption` at line 224,
matching the live fatal stack. The reader released `RLock` after retrieving the
shared per-player map but before copying it. The production fix keeps `RLock`
through the copy; no new storage path or abstraction was introduced.

The unchanged race-instrumented regression then passed. The complete
`builtins` and `server` package gates passed, and `git diff --check` passed.
Barn commit is `01b64de`.

The unknown-command candidate was repeated on committed `01b64de` with a fresh
disposable fixture under
`.tmp/mongoose-convergence/barn-huh-fixed-20260714-12`. The source and copy
both hashed to
`b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.
Barn completed login without a process crash and replied
`I don't understand that.`, matching the WSL Mongoose Toast control under
`.tmp/mongoose-convergence/toast-huh-control-20260714-10`.

Generic unknown-command and `huh` dispatch are already durably covered by
`server/command_parsing.yaml`, `command/parser.yaml`, and
`audit/command_parser_toast_oracle.yaml`. This genuine live candidate is
coverage only after the prerequisite crash fix; no parser source or
conformance change is authorized. The next unchecked candidate in the same
family is parser shortcut dispatch.

### Slice implementation 2026-07-14: uncaught handler stack values

The Mongoose single-quote command `'codex shortcut probe` dispatched correctly on both
engines and printed `You say, "Codex shortcut probe."`. Its subsequent fixture
SQLite failure exposed a remaining shared error-contract delta. Toast's
`#0:handle_uncaught_error` output resolved every structured stack frame to the
expected object and verb names. Barn's direct formatted traceback was correct,
but the database handler rendered each frame as
`vloc and prog are both < #0 -- what?`, proving that the fourth structured
stack argument was not equivalent.

The generic row `uncaught_forked_handler_stack_frames_are_complete` temporarily
replaces stock `Test.db`'s handler with a recorder, triggers a zero-delay
forked `call_function` error, and requires complete six-field inner and outer
frames. Stock WSL Toast passed 1/1. Pre-fix Barn failed only the programmer
field for both frames: every other identity and line field was correct, but
programmer was `#-1` instead of the verb owner/player.

The strengthened unit regression
`TestUncaughtForkInvokesDatabaseErrorHandler` reproduced programmer `#-1`.
`vm.snapshotActivationFrames` rebuilt live frames while hardcoding programmer
to `#-1` and concrete `ThisValue` to `None`, despite both values already being
owned by the VM/task activation. The production fix preserves `frame.ThisValue`
and the corresponding task activation's programmer and server-origin fields
while retaining the VM's live line/source data.

Post-fix proof: the focused unit and managed Barn row passed; the complete
`error_traceback` family passed 27/27 on both stock WSL Toast and Windows Barn;
and `git diff --check` passed. The exact local package gate remained green in
`bytecode` and `vm` and red only at the already-recorded scheduler ID-collision
review regression. Conformance commit is `c01a2ac`; Barn commit is `2b1be78`.

The fresh single-quote rerun used committed `2b1be78` and a disposable
fixture under
`.tmp/mongoose-convergence/barn-say-stack-fixed-20260714-15`. The database
again hashed to the authoritative identity. Mongoose's handler now resolved
the complete `$sqlite_db -> $sound_handler -> $Sound -> $waif -> $room ->
$player_class` chain instead of printing invalid frame warnings. Structured
stack identity is CLOSED.

### Slice implementation 2026-07-14: uncaught handler payload

The generic row
`uncaught_forked_handler_preserves_custom_message_and_value` temporarily
replaces `Test.db`'s handler with a recorder and raises
`E_INVARG, "custom uncaught message", {7, 8}` in a zero-delay fork. Managed
stock WSL Toast passed with the exact first three handler arguments
`[E_INVARG, "custom uncaught message", {7, 8}]`. Pre-fix Windows Barn failed
with `[E_INVARG, "Invalid argument", 0]`. The conformance commit is `c1cf7a0`.

The VM regression `TestUncaughtRaisePreservesExceptionValue` proved that
`raise()` already created the correct structured exception, but the uncaught
return replaced it with the annotated string `"E_INVARG (line 1)"`. The
strengthened scheduler regression independently proved that
`handle_uncaught_error` then substituted the error code's default message and
integer zero. The production change returns the four-element exception value
already built by `VM.HandleError` and passes its message and value fields to
the existing database handler path. No helper, adapter, or alternate runtime
path was added.

Post-fix proof: both focused regressions passed; the focused managed Barn row
passed; and the complete `error_traceback` family passed 28/28 on both stock
WSL Toast and Windows Barn. `git diff --check` passed. The exact local package
gate remained green in `bytecode` and `vm` and red only at the already-recorded
scheduler ID-collision review regression. Barn commit is `f6d2591`.

The fresh live single-quote rerun used committed `f6d2591`, the existing
`moo_client.exe`, the unchanged trusted-PROXY/account/password inputs, and a
disposable fixture under
`.tmp/mongoose-convergence/barn-say-payload-fixed-20260714-16`. The source and
copy both hashed to
`b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.
Barn emitted the MCP and player-connection lines, the complete Codex's Lab
render, `You say, "Codex shortcut probe."`, and both the direct traceback and
database handler output with `This database is not open`. Neither the generic
`Invalid argument` substitution nor invalid-frame warnings appeared. Uncaught
handler payload fidelity is CLOSED.

### Live coverage 2026-07-14: colon emote shortcut

The parser inventory establishes three server rewrites: leading double quote
to `say`, colon to `emote`, and semicolon to `eval`. The earlier single-quote
Mongoose command is not the server's double-quote rewrite and does not close
that parser row.

The colon candidate ran `:codex emote probe` after the unchanged login sequence
on fresh disposable copies. Pinned WSL Mongoose Toast under
`.tmp/mongoose-convergence/toast-emote-control-20260714-18` emitted
`Q codex emote probe`. Committed Windows Barn `e10a66d` under
`.tmp/mongoose-convergence/barn-emote-control-20260714-19` emitted the same
line, with the MCP, player-connection, room-render, and exit anchors intact.
Generic colon parsing and preposition reparsing are already covered by
`command/parser.yaml` and `audit/command_parser_toast_oracle.yaml`; no source
or conformance change is authorized.

### Live coverage 2026-07-14: double-quote say shortcut

The valid control ran `"codex double quote probe` after the unchanged login
sequence. Pinned WSL Mongoose Toast on the exact disposable database recorded
under `.tmp/mongoose-convergence/toast-doublequote-control-20260714-21`
emitted `You say, "Codex double quote probe."`. Committed Windows Barn
`fb742f7` under
`.tmp/mongoose-convergence/barn-doublequote-control-20260714-22` emitted the
same line after the same login and room-render anchors. The existing generic
`say_shortcut_quote` and `audit_say_shortcut_reparses_preposition` rows already
cover this rewrite; no source or conformance change is authorized.

### Live coverage 2026-07-14: semicolon eval shortcut

The final server rewrite ran `;return "codex eval probe";` after the unchanged
login sequence. Pinned WSL Mongoose Toast under
`.tmp/mongoose-convergence/toast-semicolon-control-20260714-23` emitted
`==> "codex eval probe"`. Committed Windows Barn `eead694` under
`.tmp/mongoose-convergence/barn-semicolon-control-20260714-24` emitted the same
line after the same login and room-render anchors. Generic semicolon dispatch
and reparsing are already covered by `audit_do_command_runs_before_semicolon_eval`
and `audit_eval_shortcut_reparses_preposition`; no source or conformance change
is authorized.

Unknown-command/`huh` dispatch and all three server rewrites now match their
live Mongoose Toast controls. The parser-shortcut/`huh`/command-dispatch family
is CLOSED.

### Slice executed 2026-07-14: `@who` and inherited `pass()` caller

The live WSL Mongoose Toast control on the fresh disposable fixture copy is in
`.tmp/mongoose-convergence/toast-who-control-20260714-25`: `@who` rendered the
player table, `Q (#249)`, `Codex's Lab`, the wizard marker, the one-player
total, and zero sunnet links. Committed Barn before this slice, captured in
`.tmp/mongoose-convergence/barn-who-control-20260714-26`, emitted no `@who`
server response; the transcript contained only the local client's completion
message, `Done.`. The focused trace in
`.tmp/mongoose-convergence/barn-who-trace-20260714-27` showed inherited
`#249:@who` returning `E_PERM` because `pass()` replaced the original player
caller with the defining object.

Three generic `Test.db` reductions were committed in order. Direct inherited
command caller coverage is `2b0ecae`; deep inherited caller coverage is
`9f0c5dc`; both already passed on Toast and Barn. The decisive row,
`audit_inherited_command_pass_preserves_player_caller`, is committed as
`d1f73b2`: Toast reported both the command-frame caller and the passed-frame
caller as the player, while pre-fix Barn reported the passed-frame caller as
the verb location. Barn commit `3155b46` makes `executePass` preserve
`frame.Caller` in the passed frame, built-in local, trace, and activation
frame, with `TestPassPreservesOriginalCaller` as the unit regression.

The managed `command_parser_toast_oracle` family passes 18/18 on both WSL
Toast and fixed Barn. `go test ./bytecode ./vm ./scheduler` passes `bytecode`
and `vm`; `scheduler` retains only the pre-existing independent task-ID
collision failure in
`TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`.
`git diff --check` passes.

The committed post-fix live replay is in
`.tmp/mongoose-convergence/barn-who-pass-fixed-20260714-28`, on a fresh copy
whose SHA-256 is
`B9BC25492BD56CB28BA0A63165F456C60417387E251391FBE8C97D7D79C9BB69`.
Barn now renders the same stable `@who` anchors as Toast. Its captured trailing
`Done.` is the local `moo_client` completion message from
`cmd/moo_client/main.go`, not server output. The `@who` target is CLOSED.

### Live coverage 2026-07-14: `@display`

The next object-inspection candidate ran `@display me` after the unchanged
login sequence on fresh disposable copies. Pinned WSL Mongoose Toast under
`.tmp/mongoose-convergence/toast-display-control-20260714-30` rendered
`Q (#249)` with player/programmer/wizard flags, parent
`Mongoose Wizard Class (#1191)`, location `Codex's Lab (#10132)`, the object
size and timestamp, and the finished delimiter. Committed Windows Barn under
`.tmp/mongoose-convergence/barn-display-control-20260714-31` rendered the same
stable lines. Both database copies had SHA-256
`B9BC25492BD56CB28BA0A63165F456C60417387E251391FBE8C97D7D79C9BB69`.
This is live coverage only: Barn passed the genuine candidate, so no
conformance or Barn source change is authorized. The `@display` target is
CLOSED.

### Live coverage 2026-07-14: `@props`

The next object-inspection candidate ran `@props me` after the unchanged login
sequence on fresh disposable copies. Pinned WSL Mongoose Toast under
`.tmp/mongoose-convergence/toast-props-control-20260714-32` and committed
Windows Barn under
`.tmp/mongoose-convergence/barn-props-control-20260714-33` both rendered the
`Properties for Q (#249:` heading and the same complete local property-name
grid, from `allbolted` through `welcome_message`. Both database copies had the
authoritative SHA-256. This is live coverage only: Barn passed the genuine
candidate, so no conformance or Barn source change is authorized. The
`@props` target is CLOSED.

### Slice executed 2026-07-14: `@verbs` numeric index termination

The final named object-inspection candidate ran `@verbs me` after the unchanged
login sequence on fresh disposable copies. Pinned WSL Mongoose Toast under
`.tmp/mongoose-convergence/toast-verbs-control-20260714-34` rendered the
`Verbs for Q (#249):` heading and the complete sorted local verb-name grid from
`@cleanup` through `xmoo_msg`. Committed Windows Barn under
`.tmp/mongoose-convergence/barn-verbs-control-20260714-35` instead raised
`E_QUOTA` (`Resource limit exceeded`) in
`$object_utils #52:accessible_verbs`, line 7, called from
`$old_prog #58:@verbs`, line 16. Both database copies had the authoritative
SHA-256.

The generic row `verb_info_numeric_index_past_end` creates one object with one
verb and calls `verb_info(object, 2)`. Stock WSL Toast returned `E_VERBNF`;
pre-fix Barn returned `E_RANGE`. That exact mismatch kept Mongoose's
`accessible_verbs` loop running past the final verb until it exhausted the
task resource limit. The conformance row is commit `d84d897`.

Barn commit `f6274a2` maps the existing store's numeric past-end `E_RANGE` to
Toast's public `verb_info` result `E_VERBNF`, with
`TestVerbInfoNumericIndexPastEndReturnsEVERBNF` as the unit regression. The
focused managed Barn row passes 1/1; the `verbs` selector passes 75 with 12
skipped on both stock WSL Toast and fixed Barn; the complete `builtins` package
passes; and `git diff --check` passes.

The committed post-fix live replay is in
`.tmp/mongoose-convergence/barn-verbs-fixed-20260714-36`. It renders the same
complete verb grid as Toast with no `E_QUOTA` traceback. `@who`, `@display`,
`@props`, and `@verbs` are now closed; the object-inspection family is CLOSED.

### Telnet coverage 2026-07-14: split login IAC negotiation

The existing generic row `audit_telnet_iac_stripped_from_login_input` was
strengthened so the login-text prefix, lone IAC byte, WILL-plus-option bytes,
and line suffix cross four separate `send_bytes` boundaries. The focused row
passes 1/1 on both stock WSL Toast and committed Windows Barn. Conformance
commit is `d3f74b2`; no Barn source change is authorized.

### Telnet coverage 2026-07-14: split IAC out-of-band delivery

The existing `audit_telnet_iac_delivered_as_oob_command` row splits the lone
IAC byte from WILL-plus-option bytes and asserts the complete binary-escaped
command delivered to `do_out_of_band_command`. The focused row passes 1/1 on
both stock WSL Toast and committed Windows Barn. This is existing generic
coverage; no conformance or Barn source change is authorized.

### Packet coverage 2026-07-14: binary chunk without newline

The existing `audit_binary_mode_dispatches_raw_chunk_without_newline` row
switches a connected player to binary mode, sends one raw ASCII chunk without
CR or LF, and asserts immediate command dispatch. The focused row passes 1/1
on both stock WSL Toast and committed Windows Barn. This is existing generic
coverage; no conformance or Barn source change is authorized.

### Telnet coverage 2026-07-14: fragmented subnegotiation

Conformance row `audit_telnet_subnegotiation_delivered_across_reads` splits a
NAWS command into lone IAC, SB plus option/payload, closing IAC, and SE sends.
Both stock WSL Toast and committed Windows Barn deliver the exact complete
binary-escaped command to `do_out_of_band_command` and pass 1/1. Conformance
commit is `7190a7e`; no Barn source change is authorized.

### Telnet coverage 2026-07-14: escaped IAC in ordinary text

Conformance row `audit_telnet_escaped_iac_stripped_from_login_input` splits
`IAC IAC` across reads inside ordinary login text. Toast establishes that the
escaped pair is stripped: both `args` and `argstr` contain `iac--login`. Barn
matches and both engines pass 1/1. Conformance commit is `36f7c5c`; no Barn
source change is authorized.

### Telnet coverage 2026-07-14: split two-byte command

Conformance row `audit_telnet_two_byte_command_delivered_across_reads` splits
`IAC NOP` across two raw sends. Both stock WSL Toast and committed Windows Barn
deliver the complete `~FF~F1` command to `do_out_of_band_command` and pass 1/1.
Conformance commit is `9569147`; no Barn source change is authorized.

### Packet coverage 2026-07-14: split CRLF terminator

Conformance row `audit_crlf_split_across_reads_dispatches_blank_lf` sends login
text, CR, and LF in separate raw writes. Toast establishes that CR dispatches
the nonempty line and the separately arriving LF dispatches one blank line;
the row records the two results without depending on concurrent completion
order. Barn matches and both engines pass 1/1. Conformance commit is `dbc98d9`;
no Barn source change is authorized.

### Telnet gate completed 2026-07-14

The complete managed `gap_followups_toast_oracle` family passes 16/16 on both
stock WSL Toast and committed Windows Barn. The cross-read coverage includes
login IAC stripping (`d3f74b2`), fragmented subnegotiation (`7190a7e`), escaped
IAC stripping (`36f7c5c`), split two-byte command delivery (`9569147`), split
CR/LF dispatch (`dbc98d9`), option-command OOB delivery, and binary chunks
without newlines. Telnet negotiation and packet-boundary behavior are CLOSED.

### OOB coverage 2026-07-14: textual MCP prefix dispatch

The existing `audit_oob_prefix_dispatches_do_out_of_band_command` row sends a
`#$#audit-oob alpha beta` command and asserts the exact three-token `args`
delivered to `do_out_of_band_command`. Both stock WSL Toast and committed
Windows Barn pass 1/1. This is existing generic coverage; no conformance or
Barn source change is authorized.

### OOB coverage 2026-07-14: held and disabled traffic

The existing `audit_connection_hold_and_oob_options` row proves that normal
input is held, OOB bypasses `hold-input`, and `disable-oob` changes the
held/released behavior. Both stock WSL Toast and committed Windows Barn pass
1/1. This is existing generic coverage; no conformance or Barn source change
is authorized.

### GMCP coverage 2026-07-14: option 201 subnegotiation

Conformance row `audit_gmcp_subnegotiation_delivered_across_reads` splits IAC,
SB plus option 201, a `Core.Hello` JSON payload, closing IAC, and SE across raw
writes. It asserts Toast's exact two-word `args` tokenization and quote-preserving
`argstr`. Both stock WSL Toast and committed Windows Barn pass 1/1.
Conformance commit is `88a8100`; no Barn source change is authorized.

### MCP/GMCP/OOB gate closed 2026-07-14

The complete managed `gap_followups_toast_oracle` family passes 17/17 on stock
WSL Toast and committed Windows Barn. The complete managed
`connection_lifecycle_toast_oracle` family passes 23/23 on both engines. This
closes MCP, GMCP, and out-of-band traffic.

### Task-scheduling gate closed 2026-07-14

Stock WSL Toast passed the complete managed `task_scheduling_toast_oracle`
family 23/23. Pre-fix Windows Barn passed 22/23; the only failure was
`audit_handle_task_timeout_invoked`. Barn invoked the truthy
`#0:handle_task_timeout` hook but discarded its result and continued into the
generic uncaught-error fallback, sending a traceback before the handler's
recorded result.

Unit regression `TestTruthyTaskTimeoutHandlerSuppressesGenericExceptionFallback`
reproduced one fallback send after a real compiled fork exhausted its tick
budget. The production correction makes the existing timeout hook report a
truthy or suspended result and uses that result to suppress the existing generic
handler and fallback path. No new runtime path was added. The focused regression
and unchanged managed row pass, and corrected Barn passes the complete family
23/23. The full `scheduler` package retains only the already-recorded independent
failure in `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`.
Barn commit is `b4b5af0`.

### Suspended-read gate closed 2026-07-14

The exact managed `read` family passes 7/7 on stock WSL Toast and committed
Windows Barn. The family covers argument errors, disconnected-player rejection,
nonblocking reads, and blocking `read()` suspension/resumption through both the
implicit and explicit player forms. No Barn source change is authorized.

### `queued_tasks()` gate closed 2026-07-14

The complete 19-case managed `queued_tasks` selector finishes with 18 passes and
one established anonymous-object skip on both stock WSL Toast and corrected
Windows Barn. Pre-fix Barn had one additional failure,
`exec::queued_tasks_shows_suspended_exec`.

The first cause was Barn's `exec()` resolver: it searched source-tree
`builtins/testdata/exec` locations instead of the managed server's
`executables/` directory, producing `E_INVARG`. Regression
`TestValidateAndResolvePathUsesServerExecutablesDirectory` proved the wrong
ownership. The production correction deletes the source-tree search and uses
only `executables/`.

The managed harness installed only extensionless POSIX fixtures, so after the
resolver correction Windows launched `sleep` unsuccessfully and the task left
the suspended queue before observation. The conformance package now installs
the seven existing Windows `.bat` counterparts alongside the POSIX files; Toast
continues to use the extensionless files and Barn's existing PATHEXT path uses
the batch files. The focused row passes, the complete selector matches, the full
Barn `builtins` package passes, and the managed-server harness passes 11/11.
Conformance commit is `11be5ee`; Barn commit is `d46be74`.

### `queue_info()` gate closed 2026-07-14

The complete 18-case managed `queue_info` selector passes 18/18 on stock WSL
Toast and committed Windows Barn. No Barn source change is authorized.

### `kill_task()` gate closed 2026-07-14

The complete 12-case managed `kill_task` selector finishes with 11 passes and
one established task-local skip on both stock WSL Toast and committed Windows
Barn. No Barn source change is authorized.

### `resume()` gate closed 2026-07-14

The complete 18-case managed `resume` selector passes 18/18 on stock WSL Toast
and committed Windows Barn. No Barn source change is authorized.

### `task_stack()` gate closed 2026-07-14

The complete 75-case managed `task_stack` selector finishes with 73 passes and
two established anonymous-object skips on both stock WSL Toast and committed
Windows Barn. No Barn source change is authorized.

Restart coverage is already included in the closed 23/23
`task_scheduling_toast_oracle` family through
`audit_suspended_task_survives_restart` and
`audit_pending_forked_task_survives_genuine_offline_restart`.

### Dump-persistence gate closed 2026-07-14

The complete two-case managed `dump_persistence` family passes 2/2 on stock WSL
Toast and corrected Windows Barn. Pre-fix Barn passed the inherited-property
case and failed `adjacent_floats_survive_dump_and_restart`: source literals
`1.0` and `1.0000000000000002` collapsed to one compiler constant, and the same
adjacent values collapsed to one map key.

The correction uses exact type-qualified IEEE float identity for compiler
constant deduplication and map hashing, while canonicalizing signed zero to
preserve MOO equality. Regressions cover both collapse points; the full `types`
and `bytecode` packages pass. Barn commit is `284c52e`.

### Promotion-aware list-membership gate closed 2026-07-14

The reduced `list_membership_promotes_integer_to_float` row passes 1/1 on the
pinned WSL Mongoose Toast build and current promotion-enabled Windows Barn.
The earlier record that Barn returned `0` for `1 in {1.0}` is stale for the
current tree, so no Barn source change is authorized. The row is gated by the
validated `option.PROMOTE_NUMBERS` profile feature and skips under the stock
strict profile. The conformance unit gate passes 59/59. Conformance commit is
`255121a`.

### Promotion-aware sorting gate closed 2026-07-14

Pinned WSL Mongoose Toast establishes that `PROMOTE_NUMBERS` does not extend to
`sort()` comparisons: `sort({2.0, 1})` returns `E_TYPE`. The unchanged reduced
row passes 1/1 on current promotion-enabled Windows Barn, and the complete
promotion suite passes 2/2 on both engines. No Barn source change is authorized.
Conformance commit is `c813cf8`.

### Promotion-aware map-key gate closed 2026-07-14

Pinned WSL Mongoose Toast establishes that `PROMOTE_NUMBERS` does not merge
integer and float map keys. The reduced row proves two-key length, independent
lookups, and a negative mixed-type `maphaskey()` result. It passes 1/1 on
current promotion-enabled Windows Barn, and the complete promotion suite passes
3/3 on both engines. No Barn source change is authorized. Conformance commit is
`31cf3a7`.

The plan's remaining promotion-sensitive comparison, sorting, map, and
collection semantics family is closed. Milestone 5 performance measurement is
the next unchecked phase.

### Active Milestone 5 performance gate 2026-07-14

The durable runner and pinned WSL Mongoose Toast baseline are recorded in
`experiments/2026-07-14-mongoose-performance-baseline.md`. Toast completed the
fixed workload with all required liveness anchors and a checkpoint. The Barn
acceptance thresholds were derived and recorded before measuring Barn.

The unchanged Windows Barn benchmark is now recorded in the same experiment.
Database load, PROXY-to-output, login, startup command, `look`, movement,
liveness, checkpoint, and CPU all pass the pre-recorded thresholds. The
connect-to-first-banner observation is not a failure: the client intentionally
waits 3000 ms before PROXY and Barn's banner follows the prelude, so the causal
plan metric is the passing 1 ms PROXY-to-first-output latency.

Post-settle RSS is the sole active target: Barn used 1882996736 bytes against
the 467460096-byte threshold. Saved Go counters establish heap ownership
(`HeapAlloc=847925488`, `HeapInuse=1089699840`, `HeapSys=1945698304`). Capture
a heap profile from the unchanged workload, test the recorded database
representation hypothesis, and do not switch metrics until RSS passes or two
consecutive slices produce no kept improvement.

The unchanged profile-bearing repeat reproduced 2010251264 bytes RSS. Its
forced-GC heap profile confirms database representation as the cause:
`types.NewMap` is the largest flat owner at 247.06 MB (30.32%), followed by
property-builder storage at 222.61 MB. The first slice is pinned to deleting
`goMap`'s redundant retained small-map storage while preserving map semantics.
Do not touch property or string storage in this slice.
