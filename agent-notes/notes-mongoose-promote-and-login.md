# Mongoose: promote-to-float + read()-login — session state (2026-06-22)

Detailed running log: `notes/promote-to-float-toggle.md`. This is the root-level summary.

## Original task (Q)
Capture the toaststunt master↔mongoose branch differences so Barn can run Mongoose
*configurably* (default OFF), without breaking conformance with true MOO. Q's hypothesis:
the permissive promote-to-float behavior is what lets you log in as `q canefan` on the
mongoose branch but not master.

## VERIFIED FINDINGS
1. **Mechanism = promote-to-float** (PROMOTE_NUMBERS in options.h). Confirmed by source diff
   AND live eval on three engines (master/strict, mongoose/promote, Barn):
   - `1+1.0`: master E_TYPE | mongoose 2.0 | Barn E_TYPE
   - `5>2.5`: E_TYPE | 1 | E_TYPE ;  `1==1.0`: 0 | 1 | 0 ;  `2.0*3`: E_TYPE | 6.0 | E_TYPE
   - => Barn == stock MOO == master. The mongoose branch promotes.
2. **Q's login premise = DISPROVEN.** Login behaves identically on master vs mongoose
   binaries. master loads+runs mongoose.db fine. The login difference was (a) a STALE db
   and (b) the trusted-proxy contract — NOT promotion.
3. **Old mongoose.db was stale (66MB).** Fetched fresh from
   `mongoose@mongoose.world:~/mongoose/mongoose.db.new` → `mongoose_fresh.db` (100MB). Had
   to clear a stale WSL known_hosts entry (server rekeyed).
4. **Login contract (verified on Toast, fresh db):** localhost is in
   `$server_options.trusted_proxies` → must send `PROXY TCP4 203.0.113.5 127.0.0.1 50000
   <port>` FIRST, then username `q`, password `canefan` → connected. Uses read()-based
   $account_login.
5. **Promotion DOES matter at runtime (the real payoff):** on every look/connect,
   `#9501:cluster_by_proximity` line 23 `best_dist = threshold + 1` (float + int) throws
   `Type mismatch (expected float; got integer)` on strict; works under promote. Hit during
   `#0:user_connected` room render. CONCRETE justification for the toggle + a clean
   conformance reduction target.

## TEST APPARATUS (WSL)
- master/strict moo: `~/src/toaststunt/build-release/moo` (origin/master).
- mongoose/promote moo: worktree `~/src/toaststunt-mongoose/build-release/moo` (built; had to
  copy master's `version_options.h` into build dir — shallow fetch broke its perl-gen).
- emergency eval syntax = `;EXPR` (NOT `;return ...;` — wsl_oracle.sh doc is stale).
- kill servers with `pkill -x moo` (NOT `pkill -f build-release/moo` — that matches+kills
  the launching bash). Keep the `sleep` in the SAME wsl call as the launch or the detached
  server gets reaped.
- Servers left UP: Toast master 9471, Toast mongoose 9472, Barn 9450 (all fresh db).

## PLAN (Q approved: "both, login first")
### Thread 1 — read()-based login (GAUNTLET in progress)
Root cause (reports/barn-read-login-gap.md): login hook ran as unregistered throwaway task
via scheduler/call_verb.go CallVerbWithArgstr, no FlowSuspend handling → read() suspend
dropped. Fix = Option A: run login hook as a registered resumable task; defer loginPlayer
to task completion.
- Coder round 1: implemented; happy-path GREEN; live login matched Toast.
- Analyst round 1: NO-MERGE — BLOCKER (orphan task swallows input on recycled connID) +
  2 MAJORs (parallel do_login_command spawn in resume window; tests only cover happy path).
- Coder round 2 (DONE): ResumeReadingTask runs resumed task synchronously (kills the race
  window); one-login-task-per-conn guard (IsTaskLive); CancelLoginTasksFor(connID) kills all
  conn tasks on disconnect; dispatch-error no longer falls back read-less; CleanupFinishedTasks
  on 5s tick. 5 RED→GREEN tests (single, two-read, parallel-spawn, disconnect-mid-read,
  verb-raise). `go test ./server ./scheduler ./task ./builtins -race` ok. Live login = Toast.
- Analyst round-2: all NO-MERGE findings genuinely fixed (verified by disabling each fix →
  RED). -race clean. 2 non-blocker MINORs documented.
- VERIFIER: **MERGE** (agent af1db301cec4f0654, reports/read-login-verifier.md). Independently
  cleared all 6 gates incl. RED-without-fix sabotage of ResumeReadingTask + CancelLoginTasksFor,
  and live PROXY→q→canefan login. Conformance YAML untouched. Thread 1 COMPLETE.
- Change is UNCOMMITTED on master (~349 insertions across scheduler/task_factory.go,
  task_runtime.go, server/{connection,input_login,input_processor}.go, task/task.go,
  server/read_login_test.go). Commit boundary decision pending with Q.
- COMMITTED on master: `2dc2eb8` "server: run login hook as a resumable task so read()
  works pre-auth" (7 files, 605 insertions incl. test). Thread 1 DONE+COMMITTED.

### Thread 2 — promote_numbers toggle (GAUNTLET in progress, UNCOMMITTED)
Scout: reports/promote-numbers-scout.md — only bytecode VM live (operators.go funcs dead);
DON'T cherry-pick exp branch (diverged + always-on); add PromoteNumbers bool to
kernel.TaskContext (default false), set at 5 scheduler sites; VM reads vm.Context.PromoteNumbers.
- Coder r1 (agent a46014d3205399ef7): implemented. -promote-numbers CLI flag. Gated
  op_arith.go (Add/Sub/Mul/Div/Mod/Pow) + op_compare.go (compareValues+param, executeEq/Ne).
  Conformance default-OFF: 3871 pass / 0 fail / 131 skip (zero regression). Live: flag
  controls cluster_by_proximity Type mismatch. reports/promote-numbers-coder.md.
- Analyst (agent aca20095918107fef): NO-MERGE. reports/promote-numbers-analyst.md:
  - MAJOR M1: executePow int^(neg int) not gated — under promote Toast returns FLOAT pow
    (2^-1→0.5, 0^-1→+Inf, no E_DIV). Scout plan was wrong (strict vs PROMOTE do_power).
  - MINOR m1: in/sort/map-key equality not promote-aware (intentionally deferred = partial).
  - CLEAN: strict/default-OFF byte-identical; all 5 callers of compareValues pass promoting();
    all 5 context sites set flag; perf hot path untouched; tests genuinely guard; other
    promote semantics match Toast numbers.cc.
- Authoritative pow semantics verified live on 9472: 2^-1→0.5, 10^-2→0.01, 0^-1→inf.0,
  2^3→8 (non-neg stays int).
- Coder r2: fixed executePow under promote (int^neg-int → float pow). RED→GREEN, live
  differential matches Toast 9472 table.
- VERIFIER: **MERGE** (agent a54eb55ac4393daba, reports/promote-numbers-verifier.md). All
  gates passed: default-OFF byte-identical (arith conformance subset 402/0/4), promote
  matches Toast, M1 fix RED-on-disable, tests real guards, plumbing clean.
- COMMITTED on master: `ca6b0fa` "vm: add configurable promote_numbers option (default
  off)" (12 files, 468 insertions incl. test). Thread 2 DONE+COMMITTED.

## BOTH THREADS COMPLETE (2026-06-22)
- `2dc2eb8` login (read-based account login resumable)
- `ca6b0fa` promote_numbers toggle (default off, -promote-numbers flag)
To run Mongoose on Barn: `barn.exe -db mongoose_fresh.db -port NNNN -promote-numbers`,
log in via PROXY prelude + q/canefan.

## DEFERRED FOLLOW-UPS (not blockers; for later)
- promote mode is PARTIAL: `in`/sort/map-key equality not promote-aware (`1 in {1.0}`→0).
- `barn.exe -eval` debug CLI doesn't honor -promote-numbers (server path does).
- trusted-proxy-from-UNtrusted-IP hardening (PROXY only from trusted source) — secondary.
- login: non-read() suspend in a login verb drops a concurrent line (safe, intentional).
- Other mongoose-branch divergences (catalog in promote-numbers-scout.md §6): extended
  bytes 128-254, telnet SUBNEG state machine, json human error msgs, utils.cc coercion,
  server.cc OOB-prefix. Implement per-need with the same toggle pattern.
- Conformance YAML for PROMOTE mode needs profile/feature gating (separate workstream).

Divergence catalog (Q wanted, in scout report §6): PROMOTE_NUMBERS, utils.cc generic
compare/equality coercion, extended-bytes 128-254 (exec/decode_binary/fileio/list/pcre),
telnet SUBNEG state machine, json.cc human error messages, server.cc OOB-prefix activity.

### Thread 2 — promote_numbers toggle (AFTER login merges)
Default OFF (conformance-safe). Reuse exp/barn-options-profile-workstream options/profile
infra. Gate promotion in vm/op_arith.go (Add/Sub/Mul/Div/Mod/Pow int↔float) +
vm/op_compare.go (compareValues + executeEq/Ne); tree-walker vm/operators.go also.
Conformance tests: strict `1.5+1`→E_TYPE, promote→2.5 (reduced from cluster_by_proximity).
Also audit the OTHER mongoose-branch divergences (telnet state machine, extended chars
128-254, json error messages, pcre cache/JIT) — Q wants them catalogued.

## Agents (resumable via SendMessage)
- Coder: aa04a953c066d8647
- Analyst: a36108a2adc9dbba4 (re-review running)
- Investigator/Scout (login gap): produced reports/barn-read-login-gap.md

## Cleanup debt (end of session)
- WSL temp dbs (/tmp/mg_*.db), worktree ~/src/toaststunt-mongoose, running servers.
- Barn-dir scratch: barn_mg_fresh.db, mongoose_fresh.db (keep fresh db; it's the good one).
- Stray analyst probe files (analyst cleans its own; verify none left).
