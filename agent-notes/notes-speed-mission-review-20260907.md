# Speed-mission review checkpoint (2026-09-07)

- Task: Q asked why the beat-Toast mission failed at its first step; review issues/PRs/notes. Read-only.
- Plan: plans/barn-beat-toast-2026-09-01.md (PR #281). Sequence said A0 (#265 Toast ceiling) + B6 (#276 corpus) FIRST, then A1 (#266 retry policy).
- What happened: A0 and B6 never run (#265 open, no comments). Agent went straight to A1 (#266) on 2026-09-04.
- A1 result (PR #293 merged 2026-09-05, experiments/2026-09-04-retry-policy.md): all three mechanisms + A+C fail every gate.
  Baseline 16p 91/s 63% abort p99 3.9s. A 82/s 40% p99 2.2s p50 8->28ms. B 81/s 73%. C 111/s 52% p99 4.1s. A+C 81/s 37% p99 2.0s; 1p 90/s.
- Why A lost throughput: rejectReservedWriteConflict takes retryReservationMu.RLock() and holds it via defer through the ENTIRE Commit incl. publication; Acquire/Release take the write lock on every retry. RWMutex writer-preference => reservation churn serializes commits. A global lock came back through the side door. Also fresh tasks still run the whole verb, then get synthetic-rejected at commit.
- Deeper: census (2026-07-27) says 100% of conflicts are property RMWs on a handful of global objects: #24.traceback_log/next_perm_index via #0:handle_uncaught_error (mostly Barn-manufactured errors, #268/A3), @who writing every player's .aliases, output-chain appends. Retry policy cannot make 16p > 1p while every task writes #24. Amdahl.
- Side effect: PR #293 added two experiment-derived rules to AGENTS.md (reservation publication invariant). Cruft for a prototype that never shipped.
- PR #282 (repo hygiene) still open, checks green, no reviews.
- Local master was behind origin by 6 commits (fetched, not merged).
- Recommendation: A3 (#268) then A0 (#265), re-census, then decide. Not another retry-policy round.

## Checkpoint 2 (execution started, Q said "do the needful")
- master fast-forwarded to origin (4e86408). barn.exe built.
- PR #294 opened: drops the two prototype rules from AGENTS.md (worktree ../barn-worktrees/agents-revert).
- PR #282 (hygiene): `gh pr merge --merge` refused, master uses merge queue; must enqueue with plain `gh pr merge 282`.
- #268 premise partly wrong: harness comment (engine/mongoose_real_bench_test.go:300-306) says `say`→#3882::execute line 10 "This database is not open" is a GENUINE snapshot error (#2585.sql orphaned waif), Toast raises identically; @who E_PROPNF on #410 cohort also genuine. Verb :execute line 10 confirmed = raise(E_INVARG,"This database is not open").
- Remaining candidates: #55:map_builtin line 12 call_function(builtin,...) E_INVARG (need caller/builtin name from fresh traceback); #2700:process_players line 21 idle_seconds(person) (harness benchConnection has IdleSeconds, so maybe person not connected -> E_INVARG in both engines?); #20:regexp_quote line 5 rmatch(s, "[][$^.*+?%].*") — bracket class with leading ']'.
- barn -eval takes a bare expression (no `return`); my first probes failed for that reason.
- Fresh 16p harness run in background -> scratchpad census16.log (task bkfi0oria); will grep tracebacks for current uncaught sites.
- Blocker: none.

## Checkpoint 3 — fresh 16p census (BARN_DEBUG_RETRY=1, scratchpad census16-debug.log)
- 16p: goodput 84/s, abort 62.7%, p99 4.2s (matches 09-04 baseline). 352 uncaught errors in ~10s.
- ONLY two sites remain: 287x say -> #2585:length line 7 -> #3882::execute line 10 "This database is not open" (E_INVARG); 65x @who -> #410:@who line 11 -> #55:map_builtin line 12 E_PROPNF #36.cloaked.
- July sites look->#2700:process_players and home->#20:regexp_quote are GONE (Barn rmatch handles "[]...]" correctly; probed).
- Harness comment says both remaining sites are genuine snapshot errors Toast raises identically (orphaned waif #2585.sql; #36 lacks .cloaked). So #268 has no Barn divergence left to fix.
- Implication: the harness's dominant #24 contention (say -> handle_uncaught_error -> $wiz_utils.traceback_log RMW) is a SNAPSHOT ARTIFACT; production say writes sqlite (out of store) and never touches #24. Workload fidelity question for Q, not a Barn bug.
- barn.uncaught_exceptions only increments in logTraceback (unhandled path); handled-by-#0 errors are invisible. #268 wants a per-run harness count.
- Next: inspect #2585 / :length to see what say does; decide harness fix; comment on #268; then A0 (#265).

## Checkpoint 4 — decision on #268
- Census 07-28 follow-up table (experiments/2026-07-27-mongoose-contention-census.md:130-160) already resolved all four sites: regex = Barn bug FIXED a91809f; idle = harness artifact FIXED; @who .cloaked and say sqlite = genuine snapshot state (Toast identical). Its "actionable" line: re-register #2585.sql with $sql_utils, give #36 a .cloaked prop (db-side).
- #268 therefore has no Barn divergence left. Remaining work from its "done when": per-run uncaught-error count in the harness summary.
- Plan: (1) harness prints uncaught=N per level via metrics.UncaughtExceptions, moving the Add(1) to the handler-dispatch site so handled errors count too (no tests pin the counter); (2) harness applies the two census-recommended admin repairs at boot (open #2585.sql as wizard, define #36.cloaked), env BARN_MONGOOSE_REPAIR=0 to disable; measure 16p before/after to quantify how much #24 contention was snapshot artifact; (3) comment on #268 with the census and close or re-scope.
- Tools: engine/eval.go EvalCommandOutput(player, code) exists; barn -eval is not wizard (E_PERM on :path()).
- Blocker: none.

## Checkpoint 5 — harness edits applied (uncommitted, master checkout)
- engine/task_runtime.go: metrics.UncaughtExceptions.Add(1) now at the handle_uncaught_error dispatch site (counts handled errors too); removed from logTraceback (logCallVerbTraceback keeps its own). No tests pin the counter.
- engine/mongoose_real_bench_test.go: BARN_MONGOOSE_REPAIR (default on) runs as #2 via EvalCommandOutput: `#2585.sql:open()` + add_property(#36,"cloaked",0,{#36,"r"}) if missing; logs result. Summary line gains uncaught=N per level.
- Gotcha: the Bash tool unescapes backslashes inside quoted heredocs; use chr(92) in python for literal backslashes.
- go vet ./engine clean. Next: 16p runs repaired vs REPAIR=0 with BARN_DEBUG_RETRY=1 to confirm repair took and see residual uncaught sites; then commit on a branch + PR; comment on #268.

## Checkpoint 6 — repair run exposed two Barn sqlite divergences
- Repair eval returned {opened=1 (handle 1), is_open=0, cloaked added}. @who E_PROPNF gone; say errors persist (273/run) because #3882::is_open compares sqlite_info(h)["path"] == "files/"+path.
- Barn bug 1: builtins/fileio.go resolveFilePath = filepath.Join("files", rel) -> backslashes on Windows; MOO-visible sqlite_info path must be Toast-shaped "files/<path>" with forward slashes (Toast file_resolve_path prefixes file_subdir).
- Barn bug 2: Toast sqlite_open returns the EXISTING handle when the resolved path is already open (database_already_open, sqlite.cc:140-150, 249); Barn always creates a new handle.
- Boot noise in repaired run: 1x E_ARGS from #0:server_started line 19 -> $logger:exception(e,"Failed starting ",service) (#2478:exception scatter takes fewer args) => some service failed to start; likely my :open() racing $sql_utils forks. Handle id 1 suggests $sql_utils had not opened anything yet at +2s (or prod=0). Verify.
- Plan: Toast-first conformance tests for both bugs in moo-conformance-tests, then Barn fixes; harness repair should re-point #2585.sql to the registered waif instead of :open().

## Checkpoint 7 — Toast semantics pinned from source
- file_resolve_path (fileio.cc:318): "files/" + pathname with a leading "/" stripped; no cleaning. sqlite_info path == that string.
- Duplicate open (sqlite.cc:250): Toast RAISES E_INVARG "Database already open with handle: N" with value N. It does not return the existing handle.
- Barn: no dedupe; path via filepath.Join -> backslashes on Windows (and filepath.Clean in sanitizeFilePath converts slashes on Windows).
- Next: conformance tests (files dir availability in managed harness TBD), then Barn fixes in builtins/sqlite.go, then harness repair re-points #2585.sql to the registered waif.

## Checkpoint 8 — conformance tests Toast-green and committed
- moo-conformance-tests: 4 new tests in builtins/sqlite.yaml (path under files/, leading slash stripped, duplicate open raises E_INVARG w/ handle value, :memory: never dedupes). 97 passed on managed WSL Toast (fast lane /root/mct). Committed locally (not pushed).
- Next: run them on Barn (expect reds on path + duplicate), fix builtins/sqlite.go (MOO-visible path = "files/"+TrimPrefix(raw,"/"); dedupe by path -> raise E_INVARG msg+value like maps.go:87 idiom), unit test, rerun conformance on Barn, then harness repair.

## Checkpoint 9 — Barn fix applied (uncommitted on master checkout)
- builtins/sqlite.go: MOO-visible handle path = "files/"+TrimPrefix(raw,"/") (Toast file_resolve_path verbatim), driver opens the OS path; duplicate resolved path raises E_INVARG "Database already open with handle: N" with value N (lowest existing id); :memory:/"" never dedupe.
- builtins/compat_sqlite_test.go: two new tests; first run failed only on Windows TempDir cleanup (open handles) -> added sqliteCloseAllHandles cleanup helper.
- Barn conformance lane: run from barn dir: `uv run --frozen pytest --pyargs moo_conformance --server-command="C:/Users/Q/code/barn/barn.exe -db {db} -port {port}" -k ...` (barn pyproject has editable path dep on ../moo-conformance-tests). Running `uv run` inside the conformance checkout fails on .venv/lib64 removal.
- Still pending: rerun conformance subset on fixed barn.exe; harness repair re-point #2585.sql; 16p A/B; branch+PR; #268 comment; A0 (#265).

## Checkpoint 10 — harness forks never run; $sql_utils:server_started raises E_TYPE
- Repaired-run diag as #2 after boot+2s: sqlite_handles()={} and queued_tasks()=10 -> the fork(0) service starts from #0:server_started sit in the queue; NO forked task runs in the harness (idle probe "attempts=0" agrees). Fidelity gap: Mongoose output chains fork heavily. Check whether newRuntimeWithWorkerCount starts the scheduler loop.
- Direct `$sql_utils:server_started()` as #2 opened 2 of 3 dbs then raised E_TYPE (probably x:schema() / saved_schemas map assign). Rule Zero target: a production Barn bug if Toast completes it.
- After the direct call, sound db was open and the 1p run had uncaught=0 (say works), goodput 111/s p50 6ms.
- Harness file currently has TEMPORARY diag code; pristine copy at scratchpad harness.bak (post-repair-rewrite version).

## Checkpoint 11 — harness now drives the scheduler
- Root cause of dead forks: server/input_processor.go processRuntimeTick calls runtime.ProcessReadyTasks() on a 10ms ticker; harness never did. Added a goroutine mirroring it (stops before s.Stop()). Restored harness from harness.bak first (diag code dropped), repair poll now waits up to 30s for the registry forks to open sound.sqlite, then re-points #2585.sql and adds #36.cloaked.
- EvalCommandOutput DOES drive ProcessReadyTasks during suspend (eval.go:145-168); the E_TYPE inside direct $sql_utils:server_started() remains unexplained — parked, not production path.
- DEBUG-CALLFN in repaired16b: 1116x sqlite_info E_INVARG = :is_open probing a stale handle inside try; expected.
- runRealCommandLine comment says #0:do_command raises E_INVARG under Barn (July lead, notes/mongoose-perf-hunt-2026-07-27.md:66-104) — via CallVerbWithArgstr, not counted by DEBUG-UNCAUGHT. Re-verify later; would be a per-command Barn divergence.
- Running forks16.log (16p, debug, forks live). Then: rerun unit/engine tests, branch+commit+PR, #268 comment, A0.

## Checkpoint 12 — forks-live 16p run (forks16.log) and new leads
- With scheduler loop: $sql_utils opened 2/3 dbs via forks (1 service start failed: E_ARGS in $logger:exception w/ 3 args = db bug masking the real error). Sound db re-pointed OK. 16p: 80/s, abort 57%, uncaught=345, p99 4.0s; say attempts fell 13.5k -> 3k (sqlite call suspends => early commit).
- say now fails differently: files/sqlite/sound.sqlite is an EMPTY db created by Barn (files/ untracked) -> "no such table: assets" (171), plus 95 E_INVARG at ::execute line 13 (BEGIN TRANSACTION on a shared handle under concurrency) and 9 E_TYPE line 20. Harness needs fixture provisioning (assets table) + Toast comparison of concurrent shared-handle behavior.
- NEW Barn bug class: 176 frameless E_INVARG in handle_uncaught_error tasks + 3 in exitfunc: task_runtime.go:308 falls through to `result = types.Err(errCode)` when a validation loss is not retryable (irreversible builtin / LiveStoreMutated / canRetry false / 64 attempts). MOO sees a spurious E_INVARG Toast never raises. handle_uncaught_error tasks WERE retried up to attempt 10 before this. Cause of non-retryability still unknown (not notify: pending effects are reversible).
- NEW divergence: #1:loc_enter line 7 suspend(0) -> E_INVARG when reached via enterfunc from move(). Toast objects.cc:191 calls enterfunc via call_verb in-task with a continuation (case 4) so suspend works. Barn call_verb.go:112 rejects nested-hook suspend. Needs conformance test + fix; separate issue.
- Barn+engine full tests running in background (b5ttwxvi3).

## Checkpoint 13 — shipping state
- Conformance: PR MongooseMoo/moo-conformance-tests#91 (branch sqlite-path-and-dup-open). Accidental push to merged branch agent/toast-conformance-1000-useful was reset back to 9bdfd2e on origin.
- Barn: branch fix/sqlite-path-dedupe-harness-fidelity committed (sqlite fix+tests, harness scheduler loop, uncaught counter, repair). PR to open next. PR #294 (AGENTS revert) open; PR #282 queued.
- Final harness (repaired, forks live): 1p 77/s abort 12.8% uncaught 188; 16p 74/s abort 59% uncaught 297.
- Top uncaught now: #1033:find_urls line 6 pcre_match(text, #1033.url_re, 0, 1) -> E_INVARG (~380/run) from #2700:say line 25. Rule Zero in progress (Barn eval probe + Toast pcre source).
- Other leads to file: (a) frameless E_INVARG on non-retryable conflicts (task_runtime.go:308 fallthrough) 137/run; (b) suspend(0) in enterfunc-called verb -> E_INVARG (Toast objects.cc:191 in-task call_verb); (c) sqlite BEGIN in ::execute line 13 -> E_INVARG 108/run, suspect throwaway do_command task can't suspend (runSQLiteAsync taskManagerOf nil?); (d) $logger:exception scatter E_ARGS masks a failed service start.

## Checkpoint 14 — pcre lookbehind divergence
- #1033.url_re = (?:\w+://|www\.)[^ ,.?!#%=+][^ ]*(?<![.,;:!?)]) — negative lookbehind. Barn pcre_match -> E_INVARG on any text; simple pattern works. Toast pcre_moo.cc uses PCRE2 (lookbehind supported); E_INVARG only for empty pattern / compile error. Likely Barn uses RE2-style Go regexp without lookaround. Every Mongoose `say` hits this (#2700:say line 25 -> $url_utils:find_urls).
- Barn branch pushed: fix/sqlite-path-dedupe-harness-fidelity @6817b1a; PR to open.

## Checkpoint 15 — pcre pinned on Toast
- 7 pcre lookaround/backreference tests Toast-green (gotcha: the Bash tool strips one backslash level in heredocs AND MOO string literals eat unknown escapes; YAML needs `\` per literal backslash — verified by SHOWME harvest).
- Conformance PR for pcre opening on branch pcre-lookaround-backrefs. Barn PR #295 open (sqlite+harness). Conformance PR #91 open (sqlite).
- Barn pcre uses stdlib regexp (RE2) in builtins/regexcache.go cachedPCREPattern; regexp2 (github.com/dlclark/regexp2) is reachable via go list. Plan: back pcre_match/pcre_replace with regexp2 (RE2 option off), keep match/rmatch (MOO legacy regex translation) on stdlib.

## Checkpoint 16 — Barn pcre on regexp2 (uncommitted on branch fix/sqlite-path-dedupe-harness-fidelity)
- builtins/regexcache.go: cachedPCREPattern now returns *pcrePattern{re *regexp2.Regexp (RE2 compat mode + IgnoreCase), groups []string in PCRE order via pcreCaptureGroups scanner}; MatchTimeout 2s -> E_INVARG. Entries share regexpCache (pcre field) so cache tests still count.
- builtins/pcre.go: FindStringMatch/FindNextMatch loop; PCRE numbering keys (unnamed k-th -> GroupByNumber(k), named -> GroupByName); byte offsets via pcreByteOffsets for non-ASCII; Replace(count 1 or -1).
- builtins/pcre_syntax_test.go: 6 tests green. regexcache_test adjusted (type change). go.mod: github.com/dlclark/regexp2 v1.12.0.
- Gotcha: $CLAUDE_SCRATCH is EMPTY in this shell; earlier "scratchpad" logs went to MSYS root (/census16.log etc). Use the absolute scratchpad path.
- Next: full pcre conformance on Barn, commit as second commit on the branch (PR #295), update PR body, then file issues for the leads and comment on #268.

## Checkpoint 17 — post-pcre harness (final2.log)
- 1p: 114/s, abort 0.22%, p50 5.8ms, p99 19ms, uncaught=1 (was 188). say works end to end.
- 16p: 77/s, abort 61%, p99 4.3s, uncaught=92 (was 297-350). Remaining: 70 frameless E_INVARG (handle_uncaught_error non-retryable conflict fallthrough), 64 sqlite_query BEGIN -> E_INVARG at ::execute line 13 (0 at 1p => concurrency on one shared handle), 5 E_TYPE splice (sqlite_execute returned an error value), 3 exitfunc frameless, 3 loc_enter suspend, 1 boot E_ARGS.
- Structural contention now dominates: look 30k attempts/228 ok, @who 33k/70. That is the real A1/A2 target.
- PR #295 updated (sqlite + pcre + harness), 2 commits. Conformance PRs #91, #92. PR #294 (AGENTS) open. PR #282 queued.
- Next: file issues (frameless E_INVARG; enterfunc suspend; sqlite concurrent-handle E_INVARG), comment on #268, final report.

## Checkpoint 18 — filed and reported
- Issues filed: #296 frameless E_INVARG on non-retryable conflict; #297 enterfunc suspend; #298 sqlite E_INVARG under contention (async-op vs retry hypothesis).
- Comments: #268 re-census + resolution (done-when met by #295); #266 harness caveat + A-prototype RWMutex finding.
- Still open from the plan: A0 (#265, Toast goodput) not started; A1/A2 structural contention (look/@who) is the remaining target.
- Working tree: branch fix/sqlite-path-dedupe-harness-fidelity checked out in the main barn dir (2 commits, pushed). Worktree ../barn-worktrees/agents-revert holds PR #294.
