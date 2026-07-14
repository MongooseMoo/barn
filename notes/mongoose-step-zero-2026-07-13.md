# Mongoose step-zero live observation — 2026-07-13

Goal (Q): follow plans/barn-toast-mongoose-convergence-workstreams.md IN ORDER,
step zero = run the existing moo_client against the selected fixture (Toast
control first, then Barn), prove one full iteration, then harden the plan so
the next agent cannot deviate.

## Verified today, in order

1. Uncommitted conformance test `audit_forked_try_except_rebases_handler_ip`
   (+27 lines, task_scheduling_toast_oracle.yaml) matches the plan's step-1
   spec verbatim. Harness supports `assert_log.not_contains` (schema.py:263,
   runner.py:591). NOTE Q feedback: absence-only stream assertions are the
   streetlight blindspot; the load-bearing assertion here is the positive
   `expect value: E_INVARG`.
2. WSL Debian alive. Stock Toast HEAD aecc51e94...; mongoose worktree HEAD
   72e3c7f96...; mongoose binary sha256 a748a93644fe... — ALL match plan pins.
3. Fixture .tmp/mongoose-refresh-20260713/mongoose.db.new re-hashed:
   b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69 (matches).
4. **CODEX'S WALL FOUND**: launched WSL mongoose Toast on 9480 (disposable
   /tmp/mg_ctl.db). Windows→WSL localhost:9480 = CONNECTION REFUSED even
   though `ss` inside WSL shows moo listening. .wslconfig networkingMode=nat.
   FIX: connect to WSL NAT IP from `wsl hostname -I` → 172.17.144.45. Q wants
   this PINNED IN THE PLAN (connectivity diagnosis section).
   Also found leftover codex-era Toast on 9771 (.tmp/mongoose-live-research-
   20260713/toast.db) — cleanup candidate, NOT mine, ask before killing.
5. **Toast control GREEN** (172.17.144.45:9480, moo_client -banner-wait 3000
   -inter-cmd 2500 -timeout 15, PROXY prelude/q/canefan): player #249, MCP
   line `#$#mcp version: 2.1 to: 2.1`, full Codex's Lab render, exits line,
   known benign SQLite traceback. All plan anchors reproduced.
   Oddity: one "Access Denied" + re-prompt before Password (probably blank
   line after PROXY); login still succeeds — same shape as recorded runs.
6. Barn built from master (b78f76e + tracked-dirty plan only) →
   .tmp/mongoose-convergence/barn.exe; fixture copied to mg_run.db; running
   on 127.0.0.1:9481 with -promote-numbers.
7. **Barn red reproduced TODAY**: logs/latest.jsonl has `panic in task`,
   verb=server_started this=0, `index out of range [171] with length 32` —
   exactly the plan's recorded delta (ExtractForkBody keeps parent absolute
   exception-handler IPs; fork body 32 instrs, handler IP 171).

## In flight
- bljxb7wuf: moo_client login against Barn 9481 (expect authenticated
  Welcome! but NO MCP line/render per plan record).

## Next (plan slice, in order)
- Step 2: Toast-green managed harness row (the exact uv run command in plan).
- Step 3: Barn-red managed row (same panic in managed log).
- Step 4: bytecode unit test + ExtractForkBody rebase fix (subtract bodyIP
  from OP_TRY_EXCEPT/OP_TRY_FINALLY absolute targets; nothing else).
- Steps 5-7: go test ./bytecode ./vm ./scheduler; rerun row green; full
  task_scheduling family; commit conformance + barn separately.
- Step 8: rerun THIS moo_client login on rebuilt Barn → require MCP line,
  #249, Codex's Lab render, no panic.
- Task 7: harden plan — pin WSL NAT connectivity section, moo_client-first
  step zero, exact commands + expected outputs + failure→action table.

## Connectivity wall — PINNED (this is what killed codex)
- Windows→WSL localhost forwarding DEAD VM-wide (fresh python listener:
  127.0.0.1 → 000, NAT IP 172.17.144.45 → 200). WSL VM up 4d17h.
- Harness error "Server did not start accepting connections within 30s" is a
  LIE in this state — server starts fine, the localhost dial path is dead.
- `wsl --terminate Debian` does NOT fix it (relay lives in the shared utility
  VM, kept alive by docker-desktop distro). Full fix = `wsl --shutdown`, which
  KILLS Q'S LIVE RADIO (azuracast, up 4d) + accessmap → DO NOT do without Q.
- Harness `--moo-host` existed but a guard forbade it with --server-command;
  ManagedServer already accepted/used host. Removed the guard
  (plugin.py:166-169, uncommitted in moo-conformance-tests) — managed WSL
  Toast now dialed at NAT IP.
- Step-2 rerun with --moo-host 172.17.144.45: got PAST accept-wait (was 30s
  fail, now 9.2s), new failure = TimeoutError in transport.py:521 _receive()
  recv — connected but response-marker protocol stalled. Reading _receive:
  waits for '-=!-^-!=-' PREFIX/SUFFIX marker lines around eval output.
  CURRENT BLOCKER: why no marker response over NAT-IP connection when raw
  moo_client login to same-style server works. Hypotheses: (a) login step
  before eval got no response (wizard connect?), (b) per-connection host
  differs somewhere in transport, (c) something about the harness's login
  script env mechanism (reports/toast-oracle-wsl.md) not sent.
- NOTE: earlier failed managed run may orphan a WSL toast on port 45041 —
  died with Debian terminate, moot.

## Managed-gate progress (2026-07-13 18:05-18:15)
- DNS STALL FOUND AND FIXED: Toast reverse-DNS-resolves incoming connections;
  from the Windows NAT gateway (172.17.144.1) the lookup hung ~10s+ and
  tripped the harness's 2-3s socket timeouts (recv TimeoutError transport.py
  :521). Proof: stock Toast+Test.db login via NAT IP took 22s; after adding
  `172.17.144.1 windows-nat-gateway` to WSL /etc/hosts → instant (0.2s).
  Self-healing fix committed into scripts/run_toast_wsl.sh (idempotent
  append; /etc/hosts regenerates on distro restart so wrapper re-asserts).
- STEP 2 GREEN: audit_forked_try_except_rebases_handler_ip PASSED on managed
  WSL stock Toast (1 passed in 3.49s) with --moo-host 172.17.144.45.
- PLAN BUG: step-3 command passes --oracle-profile-manifest (linux) against
  Windows Barn → profile gate correctly refuses runtime_os mismatch. The
  deployment lane (Milestone 1's lane 2) must OMIT the oracle manifest.
  Fix plan text in hardening pass.
- STEP 3 RED (as required): same row on pre-fix Windows Barn:
  "expected 'E_INVARG', but got 0" (fork panicked pre-handler; panic already
  captured live on 9481: index out of range [171] with length 32).
- Uncommitted changes so far:
  - moo-conformance-tests: plugin.py guard removal (--moo-host now allowed
    with --server-command); yaml test (+27 lines, was already there).
  - barn: scripts/run_toast_wsl.sh hosts-entry self-heal.
- STEP 4 IN PROGRESS: encoding facts pinned:
  - OP_TRY_EXCEPT operands: numClauses, per clause {numCodes, codes...,
    var+1, handlerIP hi, lo} — handlerIP ABSOLUTE (vm/control.go:190).
  - OP_TRY_FINALLY operands: 2-byte ABSOLUTE finally IP (control.go:234).
  - OP_JUMP/AND/OR/LOOP: RELATIVE offsets (vm.go:520-534) — do NOT touch.
  - Walker to reuse: bytecode/disassemble.go instructionOperandCount.
  - ExtractForkBody at bytecode/program.go:72; caller scheduler/
    task_factory.go:213. Nested-fork double-rebase is CORRECT (compounds:
    orig - outerBodyIP - innerBodyIP = right coords in inner program).
  - Next: RED unit test in bytecode/ (compile fork+try/except, extract,
    execute or inspect IPs), then rebase fix in ExtractForkBody.

## Slice execution complete through step 6 (18:25)
- STEP 4 DONE: bytecode/program_test.go (2 hand-assembled-bytecode tests,
  RED pre-fix with exact unrebased absolutes: 21/11, 37/30, 47/40, 57/50)
  → rebaseAbsoluteHandlerIPs in bytecode/program.go (reuses
  instructionOperandCount walker; rebases OP_TRY_EXCEPT clause IPs +
  OP_TRY_FINALLY IP; jumps untouched) → GREEN.
- STEP 5 DONE: go test ./bytecode ./vm green; ./scheduler has ONLY the
  documented pre-existing red TestReview_IDCollision... (since 800fece,
  July 1 notes; my diff is bytecode-only). git diff --check clean.
- STEP 6 DONE: rebuilt barn.exe → managed row PASSED (3.89s); full
  task_scheduling_toast_oracle family 22 passed in 30.61s.
- PLAN UPDATED (Q asked before commits): new "Windows-To-WSL Connectivity"
  section (wall 1 localhost forwarding + wall 2 reverse-DNS +
  failure→action table + never wsl --shutdown without Q [radio]); step-2
  command now sets --moo-host from `wsl hostname -I`; step-3 command drops
  --oracle-profile-manifest (deployment lane, gate refuses cross-OS);
  executed-slice record added above the recipe.
- NOW: step 7 commits. Conformance repo: (1) yaml row, (2) plugin.py
  --moo-host guard removal. Barn repo: (1) run_toast_wsl.sh DNS self-heal,
  (2) bytecode fix + test + notes + plan. Msg files in scratchpad
  (commit_conf_row.txt, commit_conf_host.txt). Ward gotcha: adopt alone,
  never chained.
- THEN step 8: kill stale barn 9481 (old binary), fresh disposable fixture
  copy, rebuilt barn, settle, moo_client PROXY/q/canefan → require MCP line
  + #249 + Codex's Lab render + no server_started panic.

## Servers up right now
- WSL Toast mongoose 9480 (/tmp/mg_ctl.db, log /tmp/mg_ctl.log) — MINE.
- WSL Toast 9771 — codex leftover, not mine.
- Barn 9481 (.tmp/mongoose-convergence/mg_run.db) — MINE.
