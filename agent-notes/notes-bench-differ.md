# bench_differ notes (appended checkpoint 8)

See earlier checkpoints in git history / previous notes. Checkpoint 8 (2026-08-31):

- #251 merged to master (47bcce0). New scratch worktree `C:/Users/Q/code/barn-perf`,
  branch `agent/perf-regressions` off origin/master. NEVER edit Q's main checkout.
- Q: use Edit/Write tools, not bash heredocs, for file edits. Heredocs with
  backticks break under MSYS.
- Baseline BenchmarkVM (count=3) saved to `barn-perf/.bench-before.txt`.
- Fix A (in progress): tri-state `finalizable` cache on sliceList/goMap
  (types/list.go, types/map.go), `Value.MayHoldFinalizable()`, gate at
  OP_SET_VAR (vm/vm.go), early-return in collectPendingFinalizationsFromValue,
  collectDirectFinalizationRoots LIST/MAP, collectDirectWaifsForGC.
  NewEmptyList/NewEmptyMap start as finalizableNone; NewList/NewMap lazy.
- Fix B (next): cache Execution + 5 closures on VM in vm/op_misc.go
  executeCallBuiltin; rebind TaskContext/Task per call; PendingFinalizations
  set to cached closure or nil depending on Context.DeferredGC.
- DONE both fixes; build+vet green; full `go test ./...` green (pool_test dirtyVM
  taught about builtinExec/builtinPendingFinalizations; reset() zeroes struct).
- benchstat before/after (count=3): abs 88→32ms & 1.2M→25 allocs; tostr 191→74ms
  & 3.0M→600k allocs (matches July c4 baseline allocs); list_append 370→171ms;
  int_arith 95→74ms; geomean −35% time, −75% allocs. Absolute times still ~2x
  July numbers — machine was loaded (bench_differ running concurrently); alloc
  profile is restored, which is the real signal.
- list_append_10k is `l = {@l, i}` → inherent O(n) copy per iteration.
- Added types/finalizable_test.go. bench_differ run → .tmp/bench_differ_perf.
- Committed a2d9a1b on agent/perf-regressions, pushed. PR not yet opened.
- BLOCKER: `uv run moo-conformance` from Windows fails: the conformance .venv is
  now a *Linux* venv (created from WSL, uv 0.9.4, python 3.12.11, lib64->lib
  symlink Windows uv can't delete). Conformance must be run from WSL with a
  linux barn build: wsl -d Debian -u root bash -c "cd /mnt/c/Users/Q/code/moo-conformance-tests && uv run moo-conformance --server-command '/mnt/c/Users/Q/code/barn-perf/barn-linux -db {db} -port {port}'".
- Conformance from WSL needs a LOGIN shell: `wsl -d Debian -u root -e bash -lc ...`
  (uv is in /root/.local/bin). Run bsarpdtan in progress.
- Merge queue: Q approved. Created ruleset id 21976164 ("master merge queue",
  MERGE, ALLGREEN, max 3 build/merge). PR #252 (branch ci/merge-queue,
  worktree C:/Users/Q/code/barn-mq) adds `merge_group:` trigger — must land
  first or queue stalls. Open PRs 247/248/249 had auto-merge armed pre-queue;
  verify they enqueue after #252 lands. Classic protection still has
  strict=true + enforce_admins; leave as-is (queue satisfies it).
- Local full conformance vs perf binary (no CI profile flags): 79 failed /
  12463 passed. Failing files: moocode_parsing (64-bit shift roundtrips,
  skip_if "not feature.64bit"), wide_control_flow (60s-timeout huge-verb
  tests), object_authority (pinned_64_bit_lexer), int/obj dump persistence.
  Q: master should be zero-fail → suspect my invocation (CI passes
  --profile-id/--profile-manifest/--oracle-profile-manifest/-m markers/--config
  outbound-on.conf). Baseline run on master binary → barn-perf/.conf-master.txt;
  subset w/ tracebacks on perf binary → barn-perf/.conf-perf-subset.txt (both
  running concurrently — timeouts may be load flakes). --lf doesn't work
  (harness keeps no cache). Next: if master shows same 79 → rerun BOTH with
  exact CI command from .github/workflows/ci.yml.
- Merge queue bootstrap: #249 (pre-armed auto-merge) self-enqueued ahead of
  #252 but its group branch has old ci.yml → no merge_group checks → would
  stall 60 min. Dequeued #249 via GraphQL dequeuePullRequest (`gh pr merge
  --disable-auto` can't dequeue). TODO after #252 merges: `gh pr merge N
  --auto --merge` for 249, 247, 248 (247/248 showed UNKNOWN, auto=true).
  #252 PR-event CI green (conformance 7m17s) → CI master conformance is clean.
- Master binary bare-invocation baseline: 79 failed too (same files) → the 79
  are an invocation artifact (missing CI profile flags), not the perf change;
  0 perf-only failures among the 39 visible names. Rerunning perf binary with
  CI's exact flags → barn-perf/.conf-perf-ci.txt (task biel5ip7f).
- #252 MERGED via queue 02:27Z. #249 re-enqueued. #247 (conformance red) and
  #248 (build red) refused by enqueuePullRequest — genuinely failing PRs, not
  staleness; told Q.
- Perf binary w/ CI exact flags: 12533 passed, 1 failed =
  server_admin.yaml::load_server_options_basic (expected 0, got 1). Same
  failure on master binary locally; passes in CI → environment-dependent Barn
  divergence (Toast always 0). Not perf-related. Candidate issue.
- Perf PR opened + merge-when-ready armed (see gh). Worktrees: barn-perf
  (agent/perf-regressions), barn-mq (ci/merge-queue, can be removed).
- #253 = perf PR (rebased onto 4838edc, armed). #254 = load_server_options
  return 0 fix (Toast functions.cc:562 no_var_pack), worktree barn-lso, armed.
  barn-mq worktree removed. Q's checkout is on agent/issue-237-... (theirs).
- Remaining follow-ups: C4 one-arg tostr bypass (b806c9d) confirmation on top
  of #253; port 12 pr96 tests as coverage PR.
- #253 MERGED (perf). #254 merged (load_server_options). #255 = one-arg tostr
  bypass (worktree barn-tostr, −10% time/−33% allocs p=0.002), armed.
  Remaining: port 12 pr96 tests as coverage PR (saved in .tmp/leftovers/).
- Then: go build, go test ./types ./vm ./engine ./builtins, BenchmarkVM after,
  bench_differ, conformance run, PR against master.

## Checkpoint 9 (2026-08-31 21:xx) — next perf target: dispatch loop
- #255 MERGED. barn-tostr worktree + branch removed. New detached worktree
  `C:/Users/Q/code/barn-bench` @ origin/master 2348cfd for evaluation.
- bench_differ on master (repeats 7, .tmp/bench-master-20260831 in barn-bench):
  list_index 3.37x, nested_loop 3.18x, int_arith 3.17x, float_arith 2.95x,
  prop_access 2.27x, abs 2.00x, tostr 1.22x, list_append 1.15x, string_concat 0.27x.
  All values match Toast. (Pre-#253 was abs 4.93x, tostr 2.78x, list_append 2.93x.)
- CPU profiles in barn-bench/.tmp/prof/{int_arith_1M,nested_1k,list_index_1M}.cpu
  (+ vm.test binary). 19 allocs/op only → NOT allocation. ~85% is
  executeLoop+Execute+FetchByte+Pop = pure dispatch. ~11ns/op vs Toast ~3.5ns/op.
- Disasm: smeared, no single hotspot. Execute has a 0xf0-byte stack frame (one
  giant switch → big prologue/spills per opcode call). frame.IP lives in memory
  and is bumped via FetchByte (vm.frame→Program→Code deref each time) →
  store/load chains; loop line 323/324 (reload cur.IP, Code) = 220ms of 1.12s.
- Candidate fix: inline fast-path switch in executeLoop for the hot opcodes
  (immediate int, PUSH local, SET_VAR, ADD, FOR_RANGE_CHECK/NEXT, JUMP/LOOP,
  POP, IF) with code/ip cached in locals; fall through to Execute for the rest.
  Must preserve tick counting, VerbDebug error path, yield/tick-limit checks.
- **KEY FINDING**: int_arith_1M was 31.9ms at 7e4f27a (2026-07-15 baseline,
  experiments/2026-07-15-vm-dev-baseline-summary.txt, 11 allocs) and 33ms at
  fd17fa3 (c5 dispatch collapse). Today master = 70ms, 19 allocs. Same CPU
  (Ryzen 9 5950X), idle machine. ~2x dispatch regression across 690 commits.
  → Bisect first, redesign second. `git bisect run` in barn-bench (detached),
  script scratchpad/bisect_int_arith.sh (threshold 50ms, benchtime=10x),
  log barn-bench/.tmp/bisect.log, task bml9xg0tg. Remember `git bisect reset`
  in barn-bench afterwards.
- Bisect #1 and #2 (--first-parent) both "found" db8b9b0 (Jun 23) — BOGUS:
  neither fd17fa3 (c5) nor 7e4f27a is on master's first-parent line; they
  arrived via merge bc8008e (2026-07-15, "Merge branch 'master' into
  work/mvcc-concurrent-moo"). Pre-merge first-parent commits lack c5 → ~105ms.
- **int_arith "regression" is ENVIRONMENTAL, not code**: 7e4f27a checked out
  today = 61.7ms (July recorded 31.9ms, same 11 allocs, same 5950X, goos
  windows). Go 1.24.6/1.25.5/1.26.0 all ≈71ms on master. Power plan = High
  performance, % Processor Performance ≈131% under load. WSL barn lane shows
  same per-iteration cost → not Windows-only. Whole BenchmarkVM set is ~2x
  July uniformly (abs 13.6→30, tostr 36→61, list_index 80→160). Cause
  unknown (VBS/HVCI? BIOS? Hyper-V?) — Q's machine; report, don't chase.
- **REAL regression: list_append_10k** (`l = {@l, i}` 10k): July 1.02ms;
  7e4f27a today 1.9–4ms (1.47MB/op); master today 158ms. ~50x, code-caused.
  Next: first-parent bisect bc8008e..HEAD (320 commits, all descend from
  7e4f27a), threshold 20ms, script scratchpad/bisect_list_append.sh.
- Bisect #3 result: first bad = c0db35c (merge #161, 2026-08-09, 379ms);
  #253 (ae7f529) cut it to 158ms but not to 2ms. CPU profile on master:
  98% in Value.MayHoldFinalizable → sliceList.mayHoldFinalizable scan.
  ROOT CAUSE: derived headers (append/set/slice/Concat/InsertAt/DeleteAt,
  goMap set/delete) computed their state from the RAW `finalizable` field;
  an Unknown source yields Unknown children; SET_VAR then scans+caches on the
  *previous* header, which the already-derived new header never sees → O(n)
  scan per iteration. `{}` literal = NewList → Unknown, so the idiom never
  reached None. The peephole (compiler/lower.go:1604) and watermark append
  are intact.
- FIX (worktree C:/Users/Q/code/barn-fin, branch
  perf/finalizable-append-propagation off 8ad7d9b): added
  sliceList.finalizableState()/goMap.finalizableState() that resolve Unknown
  once (cached) and are used at every derive site; mayHoldFinalizable() now
  delegates. Also tightened finalizableAfterAdd: Maybe container stays Maybe
  (adding can't remove taint; previously fell to Unknown → rescan).
  Added TestMayHoldFinalizableDerivedFromUnscannedSource (list+map).
  RESULT: list_append_10k 158ms → 2.3–3.0ms; int_arith/list_index unchanged.
  types tests green. Full `go test ./...` green. Committed 1 commit on
  perf/finalizable-append-propagation, pushed, **PR #256** opened + armed
  (merge-when-ready). Poll task blp99tw3p; bench_differ on fix task b22u5gfpk
  (barn-fin/.tmp/bench-fin). Local conformance skipped — CI conformance is
  the gate on the PR; the change is cache-only.
- Cleanup after #256 merges: `git worktree remove --force
  C:/Users/Q/code/barn-fin && git branch -D perf/finalizable-append-propagation`;
  barn-bench (detached) can be removed too.
## Checkpoint 10 — dispatch fast path PLANNING (Q approved planning, not impl)
- Q asked about DCE/SSA; answered no (semantics: ticks, task_stack, Toast
  parity) and proposed dispatch fast path. Q: "yes, let's plan the dispatch
  fast path". Memory rule: resolve open decisions with Q before final plan.
- Bytecode facts (probe, removed): compiler ALWAYS emits *_WIDE control flow
  (JUMP_WIDE/JUMP_IF_FALSE_WIDE/LOOP_WIDE/FOR_RANGE_*_WIDE, 4-byte offsets).
  Self-add `x = x + e` → OP_STRING_APPEND (== executeAdd). Inner loops are
  6 ops/iter: FOR_RANGE_CHECK_WIDE, GET_VAR, GET_VAR|IMM, STRING_APPEND,
  SET_VAR, FOR_RANGE_NEXT_WIDE. Hot set also: PUSH const, POP, ADD/SUB/MUL/MOD,
  EQ/LT/GT, INDEX, FOR_LIST_LOAD, CALL_BUILTIN, GET_PROP.
- Invariants: vm.frame mutated only in pushFrame/popFrame (vm.go:50-63);
  vm.yielded set only in control.go:118, op_misc.go:100, persistence.go:147
  (suspend/fork/builtin paths) → never by the fast-path ops. Only
  CountsTick ops (LOOP*, FOR_RANGE_NEXT*, calls, PASS) change Ticks.
  Execute() has a 2nd caller: vm.Step (vm.go:483) — keep Execute intact.
- SPIKE in barn-bench (detached, disposable) vm/vm.go executeLoop: inline
  fast path for IMM, GET_VAR(bound), SET_VAR(clean prev), ADD/STRING_APPEND
  int+int, POP, JUMP_WIDE, JUMP_IF_FALSE_WIDE, FOR_RANGE_CHECK_WIDE int,
  FOR_RANGE_NEXT_WIDE int (ticks+limit via goto tickLimit), LOOP_WIDE.
  Falls through to existing Execute path for anything else (no IP mutation
  before commit). RESULT (vm tests green): int_arith 70.5→36.6ms, nested
  68.5→35.0, float 73→52.5 (float add not in fast path), list_index 162→123,
  abs 30→23, list_iter 95→~70, string_concat 1.7→1.15. bench_differ vs Toast:
  int_arith 3.17x→1.66x, nested 3.18x→1.66x, float 2.95x→2.16x, list_index
  3.37x→2.54x, prop_access 2.27x→2.05x, abs 2.00x→1.55x, tostr 1.22x→0.96x.
  Spike saved as .tmp/dispatch-spike.patch (Q's checkout, untracked) and
  scratchpad/dispatch-spike.patch; barn-bench reverted to clean master.
  Compiler emits ONLY *_WIDE control flow (lower.go emitJump→wideJumpOpcode);
  narrow forms = bytecode compat → slow path only. No tests pin the
  "TickLimit already exhausted at entry runs exactly one op" edge.
  Plan delivered to Q with open decisions (scope tiers, tick-check placement,
  parity test design, single vs split PR). Awaiting Q's answers.
- #256 MERGED; barn-fin worktree/branch removed.
- Q DECISIONS (via AskUserQuestion, which Q explicitly requested this time):
  (1) tick check: match Toast — check where ticks change; (2) scope: tier 1
  + extras that each earn >=3% by benchstat, one PR; (3) SET_VAR: "can we
  not factor out the non-DRY code?" → factor shared bookkeeping into small
  inlinable helpers (releaseLocal, wideOperand, countTick) used by BOTH the
  fast path and Execute; fast path then handles SET_VAR unconditionally.
- IMPL worktree: C:/Users/Q/code/barn-dispatch, branch perf/dispatch-fast-path
  off 6777216 (master after #256). Steps: helpers → fast path in executeLoop
  → generic path keeps its per-op tick-limit check (cheap next to Execute;
  fast path checks only at LOOP/FOR_RANGE_NEXT) → parity test (Run vs Step)
  → benchstat per tier-2 case → bench_differ → WSL conformance (CI flags)
  → PR + arm queue.
- TIER 1 DONE in barn-dispatch (uncommitted): helpers countTick/releaseLocal
  (vm.go), wideOperand (stack.go; ReadWide uses it), Run split into
  beginProgram+executeLoop, Step uses countTick, Execute SET_VAR uses
  releaseLocal. Fast path handles IMM, GET_VAR(bound), SET_VAR(always, via
  releaseLocal), ADD/STRING_APPEND int, POP, JUMP_WIDE, JUMP_IF_FALSE_WIDE,
  FOR_RANGE_CHECK_WIDE int, FOR_RANGE_NEXT_WIDE int<Max, LOOP_WIDE; generic
  path keeps per-op tick-limit check. Bench: int_arith 38.0ms, nested 37.0,
  float 54.9, list_index 120.6, abs 23.6, tostr 54, list_iter 63.6.
  vm/dispatch_parity_test.go: 22-program Run-vs-Step corpus + exact tick
  limit test — PASS. Full go test ./... running (task buyb7dyue).
- TIER 2 queue (each: add → benchstat count=6 vs tier-1 baseline → keep if
  ≥3% on some bench): float ADD; PUSH const; SUB/MUL int; MOD int (b!=0,
  b!=-1); LT/GT/EQ int; INDEX list×int in-range; FOR_LIST_LOAD non-pairs.
  Save tier-1 baseline to barn-dispatch/.tmp/bench-t1.txt first.
- Commits on perf/dispatch-fast-path: 989cfbe (tier 1 + parity test),
  702d115 (dense fastKinds[256] jump table + float ADD + PUSH). Lesson: a
  sparse `switch op` is a binary search — adding PUSH cost int loops +3-4%;
  dense kind table fixed it and gained more: vs tier-1 int_arith −12.9%,
  float −34%, nested −10.9%, list_index −7.2%, list_iter −8.9%.
  Absolute now: int_arith 33.1ms (master 70.5), float 35.0, nested 32.8,
  list_index 111.8, abs 22.4, tostr 51.4, list_iter 57.5.
- Added benchmarks while_lt_1M (139.5ms) and if_chain_1M (139.4ms) to
  perf_bench_test.go; baseline file .tmp/bench-t2d.txt (pre-group-A).
- Group A in progress (uncommitted): fkSub/fkMul/fkLt/fkGt/fkEq int/int
  inline; fkMod kind reserved, case pending reading executeMod int formula
  (sign semantics; guard b!=0 and b!=-1). Then: INDEX list×int,
  FOR_LIST_LOAD non-pairs. Benchfiles: t2d → t2e (group A).
- Group A inline (t2e) taxed int loops +7-10% (code size 5055→6362B, frame
  unchanged 0xd0) → moved to //go:noinline fastIntBinary (t2g): while −17%,
  if_chain −15%, list_index −6%, base +2-3%. Commit 3809b0d.
- Final group MOD (in fastIntBinary) + fastListIndex + fastForListLoad
  (both noinline). Machine got noisy (Everything indexer); used interleaved
  A/B binaries (.tmp/vmA.test vs vmB.test, 4 rounds ×count=2): list_index
  −9.4%, list_iter −7.7%, if_chain −3%, int/float/nested +3-4%. KEPT.
  Commit (4th) on perf/dispatch-fast-path; full go test ./... task b9nsjmt2z.
- Absolute vs master now ≈ int_arith 34.8 (70.5), float 37.6 (73), nested
  34.7 (68.5), list_index 95 (162), list_iter 54 (95), while 119 (?),
  if_chain 115 (?), abs 22.9 (30), tostr 53.8 (60).
- Full go test ./... green at 7901fe0. Branch pushed. bench_differ vs Toast
  (barn-dispatch/.tmp/bench-dispatch): int 1.55x, nested 1.61x, float 1.48x,
  list_index 1.96x, prop_access 2.03x, abs 1.50x, tostr 0.93x, values match.
  Master baseline for new benches: while_lt 185ms, if_chain 207ms.
- **PR #257** opened + armed (merge-when-ready). Poll task bwf16e4qi.
  WSL conformance (CI flags, CGO_ENABLED=0 linux build barn-dispatch/
  barn-linux) running: task bpkaeum30. Cross-compile needs CGO_ENABLED=0.
- Local conformance @7901fe0: 12534 passed / 0 failed / 420 skipped (13:02).
- **#257 MERGED** via queue (build + conformance green). Worktrees
  barn-dispatch and barn-bench removed; branch deleted. Only Q's checkout
  (master @ 6777216, behind by #257) and barn-issue-243-pr247 remain.
- Session total today: #252 #249 #253 #254 #255 #256 #257 merged.
- Next candidates: prop_access (2.03x — store/txn read path, not dispatch);
  FOR_LIST_LOAD missing releaseLocal bookkeeping (parity-preserved gap);
  environmental 2x slowdown on Q's box vs July; port 12 pr96 tests.
- Observation (not changed, parity-preserved): generic FOR_LIST_LOAD
  overwrites the loop variable without releaseLocal bookkeeping. Possible
  follow-up if anon/WAIF finalization inside for-in loops matters.
- Follow-up candidates: (a) the uniform ~2x environmental slowdown vs July on
  Q's machine (not code; VBS/HVCI/Hyper-V/BIOS suspects); (b) dispatch-loop
  inline fast path (~11ns/op → Toast ~3.5ns/op) is the remaining code gap
  (list_index 3.4x, nested/int/float ~3x); (c) port 12 pr96 tests.

## Checkpoint 11 (2026-09-01): the "2x environmental slowdown" is IDLEDISABLE

Resolved. `~/bug.txt` records `powercfg IDLEDISABLE=1` set 2026-08-24 as a
mitigation for CC6 idle-wake hard hangs on LP1. With idle disabled the Windows
idle thread busy-spins in C0 on every LP (measured: C1/C2/C3 residency exactly
0% at 97% "idle"), so every benchmark thread shares its physical core with a
spinning SMT sibling. Latency-bound code barely notices (dependent-add chain
still measures ~4.4 GHz); front-end/throughput-bound code — interpreter
dispatch, Barn and Toast alike — loses ~half. Indirect-dispatch microbench:
8.2 cycles per fully-predictable dispatch (healthy Zen 3: ~2–4).

Consequences:
- Absolute numbers recorded between 2026-08-24 and whenever IDLEDISABLE is
  reverted are NOT comparable to `experiments/2026-07-15-*` or anything
  earlier. `7e4f27a` int_arith_1M: 32.2 ms (July) vs 61.7 ms (today, same code).
- Same-day A/B comparisons and Barn/Toast ratios remain valid (both arms carry
  the same tax). Today's #252–#257 measurements stand.
- Latest master (5ebcc86) int_arith_1M = 34.8 ms *with* the tax; expect
  roughly half that once idle is re-enabled.
- CONFIRMED 2026-09-01 with Q's go-ahead, back to back on the same binary:
  IDLEDISABLE=1 (C2 residency 0%) -> 61.7 ms; IDLEDISABLE=0 (C2 87%) -> 31.6 ms
  (July recording: 32.2 ms); restored to IDLEDISABLE=1, C2 back to 0%.
- Mechanism caveat: a synthetic indirect-dispatch microbench only gained 18%
  (8.2 -> 6.7 cycles/dispatch) from the same toggle, so simple SMT-sibling
  front-end sharing does not fully explain why the real VM loop halves. Cause
  is proven; exact mechanism is not.
- Toggle recipe (powercfg's /flags get path-mangled by MSYS; run via
  `MSYS_NO_PATHCONV=1` or PowerShell; the setting is hidden until
  `powercfg /attributes sub_processor IDLEDISABLE -ATTRIB_HIDE`):
  `powercfg /setacvalueindex scheme_current sub_processor IDLEDISABLE 0; powercfg /setactive scheme_current`
  and `... IDLEDISABLE 1 ...` to restore. Verify with the
  `\Processor Information(_Total)\% C2 Time` counter, not the query.

## Checkpoint 12 (2026-09-01 ~01:00): PSIC written to NVRAM; awaiting reboot + IDLEDISABLE=0 test

State of the "2x machine slowdown" thread (all detail in ~/bug.txt and
~/notes/bios-virtualization.md "Run #3"):
- Cause proven: IDLEDISABLE=1 (8/24 CC6 hang mitigation). Same binary at
  7e4f27a: 61.7 ms with it, 31.6 ms without (July recording 32.2 ms).
  Mitigation restored after the test and is still active.
- Permanent fix chosen by Q: BIOS Power Supply Idle Control -> Typical Current
  Idle, PSIC alone (no PBO revert), VDB re-arm deferred. Applied blind via
  SCEWIN: exported 00:45, byte-exact edit (Edit tool re-encodes the file —
  use Python bytes replace; memory saved), imported 00:50. Import log said
  "System configuration not modified" but a 00:54 re-export read PSIC back
  from NVRAM as Typical Current Idle; diff vs pristine = only those lines.
  Takes effect at next POST. Q has NOT rebooted yet.
- Side fixes this session: System/Application event logs 20->512 MB;
  NvContainerLocalSystem 11-month crash loop (GFE/NVIDIA App duplicate
  plugin) fixed by moving two GFE leftovers to
  C:\ProgramData\NVIDIA\plugins-backup-20260901\LocalSystem; Q uninstalling
  GeForce Experience 3.24.
- Issues filed: #262 prop_access store path, #263 FOR_LIST_LOAD releaseLocal.
  No issue for the environment (Q's call).

Next step, after Q reboots: verify VirtualizationFirmwareEnabled still True,
NvContainerLocalSystem still Running, PSIC still Typical via export; then
IDLEDISABLE=0 in daytime with WSL/Docker up (rollback in the same command
line). Survive 22:30-00:30 = fix confirmed, and every Barn absolute number
roughly halves: expect master int_arith_1M ~17 ms, and re-baseline
experiments/ on that day. If it hangs: IDLEDISABLE=1 again; step two is
reverting bundle #2's PBO Scalar 4X / +50 MHz.

## Checkpoint 13 (2026-09-01 01:20): PSIC live, IDLEDISABLE=0, tax gone

- Q rebooted 01:15. Post-boot: SVM still on; PSIC read back from NVRAM by
  elevated export as *[00]Typical Current Idle (nvram-postboot-20260901.txt);
  GFE 3.24 uninstalled (took NvContainerLocalSystem with it; NVDisplay
  container healthy; 0 SCM failures since boot).
- 01:19 IDLEDISABLE=0 set with Q present. C2 residency 89.5% -> the CC6
  idle-wake path that hung the box on 8/23-24 is now being exercised with the
  PSIC fix in place. THIS IS THE TEST. Rollback if it hangs/reboots:
  MSYS_NO_PATHCONV=1 powershell -NoProfile -Command "powercfg /setacvalueindex scheme_current sub_processor IDLEDISABLE 1; powercfg /setactive scheme_current"
- First master (5ebcc86) numbers without the tax, noisy (Everything indexer
  rebuilding post-boot): int_arith_1M 19.2-20.6 ms (was 34.8 with tax),
  nested_1k ~20.4, list_index_1M ~57, while_lt_1M ~72-78. Clean re-measure
  pending once the indexer finishes; then re-baseline experiments/.
- 01:35 clean master (5ebcc86) numbers, idle enabled, ±1.5% across 4 runs,
  saved to experiments/2026-09-01-master-idle-enabled.txt (first honest
  same-conditions comparison with the July files):
    int_arith_1M 20.3 ms (with tax 34.8; July 7e4f27a 32.2)
    float_arith_1M 20.8 (37.6)   nested_1k 19.0 (34.7)   list_index_1M 55 (95)
    list_iter_1M 30 (54)   while_lt_1M 69 (119)   if_chain_1M 66 (115)
  Master is 1.58x faster than the July baseline on int_arith, apples to apples.
  (The indexer-wait loop in the bench script errored on multiple Everything
  processes and fell straight through; spread shows it did not matter.)

## Checkpoint 14 (2026-09-01 01:45): uncaptured levers, measured
- Toast oracle used by bench_differ (/root/src/toaststunt/build-release/moo)
  confirmed CMAKE_BUILD_TYPE=Release, -O3 -DNDEBUG. Ratios are honest. (The
  sibling /root/src/toaststunt/build is Debug+ASan — never point at it.)
- PGO (go test -c -pgo=cpu.pprof, profile from a 1 s BenchmarkVM run, 6 KB):
  geomean -4.1% vs baseline, n=4 interleaved: list_iter -6.4%, nested -5.4%,
  float -3.9% (p=0.029); int/list_index/while/if_chain within noise. Zero
  alloc change. Worth a real trial with a profile from TestMongooseRealWorkload
  and a default.pgo in cmd/barn — needs n>=6 and the real-workload gate.
- GOAMD64=v3: geomean -0.5%, nothing significant. Not worth the deploy constraint.
- Clock with idle enabled: 4.53 GHz-equivalent (was 4.42 with idle disabled);
  dispatch microbench 6.8 cycles (was 8.2). Loop calibration is approximate.
- Other uncaptured items listed for Q: allocs/program 11->19 since 7e4f27a
  (bisect + issue), GOGC/GOMEMLIMIT on the real-workload gate, Defender
  exclusions for build cycle time, RAM 2666->3200 (sighted-help only), do NOT
  apply negative Curve Optimizer given the Vmin crash theory, VDB re-arm.

## Checkpoint 15 — issue #262 prop_access attribution (2026-09-01)

Worktree `C:/Users/Q/code/barn-prop` off master 5ebcc86. Added four workloads to
`vm/perf_bench_test.go` (prop_access_1M, typeof_1M, prop_name_1M, prop_defined_1M)
plus `newBenchStore` (#0 named "bench" with int prop `benchprop`). Idle enabled.

| workload | ns/op | allocs/op | delta vs skeleton |
|---|---|---|---|
| int_arith_1M (skeleton) | 20 ms | 19 | — |
| typeof_1M | 68 ms | 25 | +48 ns / builtin call |
| prop_name_1M (`n.name`) | 82 ms | 1,000,020 | +62 ns, **48 B alloc per read** |
| prop_defined_1M (`n.benchprop`) | 104 ms | 19 | +84 ns direct findProperty |
| prop_access_1M (bench_differ) | 135 ms | 1,000,026 | ≈ 20+48+62, additive |

So #262 is three separate costs, none of them dispatch: builtin-call overhead,
`.name` boxing a string into a Value (alloc), and the direct-txn property walk.
Bench harness and bench_differ both use `Store.DirectTxn()` (vm.go:372); the
server's MVCC txn has a propResolve memo — measure separately before claiming
a server win. Next: cpuprofile each of the three.

### Checkpoint 15b — profiles + Toast comparison (Rule Zero done by source)

Toast (`db_properties.cc`, `functions.cc`, `execute.cc`): `.name` = `str_ref(name)`
(no alloc); protected check = `f->_protected` flag on the bf_table entry; property
lookup = hash-compare linear scan on the object's own propdefs/ancestors, and every
object's propval already holds inherited props. Barn divergences found by pprof:

1. `typeof_1M`: `Session.dispatch` 48% — `IsProtectedBuiltin(name)` does an atomic
   load + `map[string]bool` hash per call (Toast: flag on entry), and 88-byte
   `types.Result` zero/copy through maybeProtectedRedirect→dispatch→CallByID→
   executeCallBuiltin. `builtinTypeof` itself is 0.8%.
2. `prop_name_1M`: `types.NewStr(name)` allocates a 48 B strRep per read (620 ms of
   2.3 s); two RLocks (objectExists + objectName); `strings.ToLower` twice.
3. `prop_defined_1M`: `Store.findPropertyLocked` allocates `visited` map per call
   (runtime.rand seed + mapassign ≈ 300 ms) even though the object's own map
   already holds the inherited copy (copyInheritedPropertiesLocked, clear=true) so a
   non-clear self hit is final. MVCC `tx.findProperty` already has a memo and scratch;
   this is the direct-txn path (bench harness + bench_differ + dbtool eval).

Plan: (a) protectedSet{byName, byID} + entry.id, cheap guard before redirect;
(b) Object.nameVal boxed once at write time (setName helper on 6 write sites),
`StoreTxn.ObjectNameValue`; (c) self-first fast path in findPropertyLocked.
Verify with interleaved benchstat A/B, unit tests, conformance.

### Checkpoint 15c — fixes written in barn-prop (uncommitted)
- builtins: `protectedSet{byName, byID}`, `builtinEntry.id`, `Registry.protectedByID`,
  `Session.isProtectedEntry`; dispatch pre-checks ctx/ThisObj/protected before
  calling maybeProtectedRedirect (avoids 88 B Result zero+copy on happy path).
- db/store: `Object.nameVal` + `setName`/`nameValue` (staleness-guarded by pointer
  string compare); write sites: builder.SetName, Store.setObjectName,
  buildImageWithScalar, StoreTxn.SetObjectName, commit (live.setName);
  `Store.objectNameValue`, `StoreTxn.ObjectNameValue`.
- db/store/store_properties.go: findPropertyLocked self-first fast path.
- vm/op_property.go: `.name` uses ObjectNameValue.
Next: build, vet, unit tests (store/builtins/vm), name-consistency test, A/B benchstat.

### Checkpoint 15d — fixes green
Build/vet ok; `go test ./builtins ./db/store ./vm ./types` ok; `-race` on store+builtins ok.
New tests: `db/store/object_name_value_test.go` (every name write site keeps the box;
0 allocs/read; stale-box fallback), `builtins/protected_by_id_test.go` (byID == byName,
late registration falls back, nil refresh clears). A/B running: `vm_base.test`
(master + new bench file) vs `vm_new.test`, 6 interleaved rounds → benchstat.
Second worktree `barn-base` exists only for the baseline binary — remove after.

### Checkpoint 15e — clean A/B (n=6, interleaved, idle machine)
| workload | base | new | Δ |
|---|---|---|---|
| prop_access_1M | 126.6 ms | 94.2 ms | −25.6% (allocs 1,000,026 → 25) |
| prop_name_1M | 74.5 | 49.7 | −33.3% |
| prop_defined_1M | 100.3 | 83.9 | −16.4% |
| typeof_1M | 65.4 | 56.5 | −13.6% |
| builtin_abs_200k | 15.0 | 13.7 | −9.0% |
| tostr_200k | 32.9 | 29.8 | −9.4% |
| int_arith / list_index | ~ | ~ | controls |
Committed 2c9ce9c on `perf/prop-access-read-path` (worktree barn-prop). Full `go test ./...`
clean. Next: conformance (`scripts/run-conformance.ps1`), bench_differ rerun, PR.
(First A/B run was polluted by the concurrent full test suite — discarded.)

### Checkpoint 15f — conformance on worktree build
`scripts/run-conformance.ps1` (Test_conf.db, port 7788) on 2c9ce9c: 78 failed / 12464
passed / 412 skipped. All 78 are int-width/shift (moocode_parsing 32/64-bit shift
roundtrips), object_authority literal boundaries, integer/object dump persistence,
and vm/wide_control_flow — nothing touching properties or builtin dispatch. Running the
identical script on pristine master (barn-base, port 7789) to diff the failure sets
before concluding anything. Failed lists in scratchpad failed-new.txt / failed-base.txt.

### Checkpoint 15g — conformance parity + bench_differ
Pristine master on the same script/fixture: 78 failed / 12464 passed — IDENTICAL failure
set to the branch (diff empty). The 78 are pre-existing local-run failures (int-width /
shift / wide-control-flow / dump persistence), not this change; CI on Linux is the gate.
Branch pushed: origin/perf/prop-access-read-path (2c9ce9c). bench_differ running with
`--barn-linux barn-prop-linux` → scratchpad/bd-prop; master baseline build barn-base-linux
next → bd-base, so before/after ratios are same-day.

### Checkpoint 15h — PR #264 open
Same-day bench_differ: prop_access 1.96x → 1.72x, builtin_abs 1.78x → 1.58x,
builtin_tostr 1.07x → 0.98x; others noise. Reports + benchstat committed under
`experiments/2026-09-01-prop-access/`. PR https://github.com/MongooseMoo/barn/pull/264
(closes #262). barn-base worktree removed; barn-prop kept on perf/prop-access-read-path.
Follow-ups noted in PR: fold the two RLocks on GET_PROP direct path; 88 B Result returned
through 3 layers per builtin call; ToLower in IsBuiltinProperty for non-builtin names.
