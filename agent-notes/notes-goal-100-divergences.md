# Goal: 100 Toast/Barn divergences captured as conformance tests (Toast-green, Barn-red)

Started 2026-08-30. Branch: agent/issue-222-typed-call-stack (barn).

## Strategy
1. Build a high-throughput differential eval driver:
   - Toast side: WSL emergency mode (`moo -e scratch out < ';EXPR' lines`) against a scratch
     copy of bundled conformance Test.db — batches N expressions per process.
     Oracle: /root/src/toaststunt/build-release/moo (stock aecc51e).
   - Barn side: `barn.exe -db Test.db -eval` takes 1.8s/expr (db load dominates).
     Adding `-eval-file` batch mode per "Fix Tooling First". EvalExpression is at
     internal/dbtool/dbtool.go:204; dispatch in internal/app/app.go:233.
2. Generate probe corpora (builtin edge cases, type coercions, error codes, ranges,
   scatter, string/list/map ops, etc.), diff Toast vs Barn outputs.
3. Convert each real divergence into a conformance YAML test in
   C:/Users/Q/code/moo-conformance-tests (new file(s), e.g. audit/divergence_*.yaml).
4. Verify each via managed harness: Toast pass, Barn fail. Goal counts only
   tests that pass Toast and fail Barn.

## Key facts
- Bundled Test.db: C:/Users/Q/code/moo-conformance-tests/src/moo_conformance/_db/Test.db
  (SHA-256 1a3f23eb... per plan).
- Toast eval output: lines `=> VALUE` (emergency mode prints `(#2): => V` or `=> V`).
- wsl_oracle.sh pattern: copies db to /tmp scratch, pipes `;EXPR` lines + quit,
  greps `^=>`. Default DB is mongoose db — must override DB=Test.db copy.
- Managed harness: moo-conformance CLI in C:/Users/Q/code/moo-conformance-tests,
  `--server-command "wsl -e bash scripts/run_toast_wsl.sh {db} {port}"` for Toast;
  run-conformance.ps1 for Barn (Windows). Toast wrapper: barn/scripts/run_toast_wsl.sh.
- Barn built OK at barn.exe from current branch.

## Q direction (mid-session)
- Q wants LIFECYCLE divergences too (like the merged move-hooks PR #231), not just
  expression/builtin stuff. Two lanes:
  Lane 1: expression differ (bulk, cheap).
  Lane 2: lifecycle scenarios authored directly as candidate conformance YAML
  (move hooks enterfunc/exitfunc/accept with suspend/fork/errors inside, connection
  hooks, task scheduling, recycle/chparent mid-hook, read(), do_command, etc.),
  batch-validated Toast-first, then run on Barn; keep the reds.
- Windows ~/src/toaststunt updated to lisdude master eaaa197 (branch json-v20-db
  preserved). WSL oracle stays pinned aecc51e; read source via `git show aecc51e:...`
  when matching oracle behavior.

## Tooling built
- barn -eval-file FILE: batch eval, one expr per line, prints "=> V" / "Error: E_X" /
  "Compile error: ..." / "-- skipped" (blank or ##comment) per line, in order.
  Files: internal/dbtool/dbtool.go (EvalFile), internal/app/app.go, cmd/barn/main.go.
  5 exprs in 1.7s (db load dominated). NOT yet committed.
- Toast batch: wsl -d Debian -u root: copy Test.db to /tmp scratch, pipe ";EXPR" lines
  + "quit" into /root/src/toaststunt/build-release/moo -e scratch out; grep "=>".
  Verified working. Errors uncaught in emergency mode print "=> *Aborted*";
  wrap probes in `(E) ! ANY' on BOTH sides to normalize errors to values.

## Harness invocations (verified from docs)
- Toast (all inside WSL, avoids Wall 1/2):
  wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc
    'uv run moo-conformance --server-command="/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}" -k SEL -v'
- Barn (Windows): cd barn; powershell scripts/run-conformance.ps1 -Build -K SEL
  (uses barn_parity.exe, Test_conf.db) OR direct uv run pytest with
  --server-command="...barn.exe -db {db} -port {port}" --server-db=copy.

## State
- Differ WORKS: scratchpad/{gen_corpus.py,differ.py}; run1: 935 probes, 57 mismatches.
- Driver details: probes wrapped {"KNNNN", `(E) ! ANY'}; Toast side resumes past
  SIGFPE crashes (MININT/-1 and MININT%-1 CRASH TOAST — cannot be conformance tests).
- Barn -eval-file context is NOT wizard: E_PERM noise on object_bytes/getenv/usage/
  argon2; callers() shows eval frames. Those need harness (wizard) confirmation.

## Lane 1 candidates from run1 (need harness Toast-green/Barn-red confirmation)
1. -0.0 literal/tostr/toliteral: Toast preserves "-0.0", Barn prints "0.0"
2. 1.0/0 (float/int): Toast E_TYPE, Barn E_DIV
3. floatstr(1.5, 20): Toast OK 20 digits, Barn E_INVARG (precision cap)
4. exp(-1000.0): Toast E_FLOAT, Barn 0.0 (underflow)
5. index("", ""): Toast 1, Barn 0
6. chr(0)→"" and chr(255)→str on Toast; Barn E_INVARG both
7. parent(#999999): Toast E_INVARG, Barn E_INVIND
8. parse_json("1.5e2"): Toast 150.0, Barn 150
9. (perm-suspect, recheck via harness): object_bytes, getenv, usage, argon2, callers()
- Unknown-builtin parse failures matched on both sides (tonum/ord/pad/etc absent in
  both) — not divergences.
- SUSPECT DRIVER ARTIFACT: all sort()/all_members() probes #689-703, #711-713 range/
  assoc, #778-784 toerr/error_message, #852 show TOAST-NO-OUTPUT — investigating
  (possibly Toast emergency parse of `{}` second arg or line-length/buffering).

## Pipeline CONFIRMED (pilot run 2026-08-30)
- YAML: moo-conformance-tests src/moo_conformance/_tests/audit/divergence_numeric_string_edges.yaml
- Toast lane: wsl -d Debian -u root -e bash -lc 'cd /mnt/c/Users/Q/code/moo-conformance-tests &&
    uv run moo-conformance --server-command="/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}"
    -k "capability_admission or SEL" -q'   (MUST include capability_admission; ~3.3min, collection-bound)
- Barn lane (fast, 11s): cd barn && uv run pytest --pyargs moo_conformance
    --server-command="C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
    --server-db=C:/Users/Q/code/barn/Test_divhunt.db --moo-host=127.0.0.1 --moo-port=7788
    -k "capability_admission or SEL" -q     (forward slashes! backslashes get eaten)
- Toast: 14/14 passed. Barn: 8 FAILED = 8 CONFIRMED divergences:
  1 negative_zero_toliteral_preserves_sign
  2 negative_zero_tostr_preserves_sign
  3 float_divided_by_int_zero_is_type_error (1.0/0: Toast E_TYPE, Barn E_DIV)
  4 floatstr_precision_twenty_allowed
  5 exp_underflow_raises_e_float
  6 index_empty_needle_in_empty_string (Toast 1, Barn 0)
  7 parent_of_out_of_range_object_is_invarg (Toast E_INVARG, Barn E_INVIND)
  8 argon2_default_parameters_hash (Barn params m=65536,t=1,p=2 vs Toast m=4096,t=3,p=1)
- Barn-green kept as coverage: chr_zero, chr_255, parse_json_exponent, object_bytes,
  getenv, usage (eval-file prefilter had false positives — dbtool eval != server; harness is truth).

## Count: 37 / 100 confirmed
- divergence_builtin_edges.yaml: 50 Toast-green, 24 Barn-red (create #-2/#-3,
  chparent/parents/delete_property/is_clear_property/verbs x2/properties x2 invalid-obj
  E_INVARG, object_bytes E_INVIND, verb_info idx 0/-1, set_verb_info bad perms,
  parse_json dup-keys + maxint, crypt $6$salt$, string_hmac md5, ctime maxint/negative,
  ftime(2)/ftime(-1), buffered_output_length, create($nothing,-1) anon).
  Barn-green coverage: notify/boot_player/force_input/flush_input/queue_info/
  task_local/server_log/load_server_options/reset_max_object/crypt-random-salt/
  create($nothing,2)/add_property+add_verb validation family/respond_to/clear_property.

## Count history: 8 -> 13 -> 37 -> 51 (14 inventory reds: Barn-only builtins
  background_test capitalize connection_option downcase finished_tasks implode
  ltrim malloc_stats mapmerge read_stdin rtrim trim unique upcase —
  file audit/divergence_builtin_inventory.yaml)

## GOAL COMPLETE: 100/100 — verified in ONE combined run per engine
- Toast (WSL aecc51e, managed harness): 200/200 divergence_ tests PASS.
- Barn (Windows, current build): exactly 100 FAIL / 100 pass (coverage).
- Conformance commits: 853516e (b1-7), 4110569 (b8), 82ab409 (b9-10).
- Barn tooling commit: 963411b (-eval-file batch eval).
- Ten suites, all in src/moo_conformance/_tests/audit/: divergence_numeric_
  string_edges, divergence_move_recycle_lifecycle, divergence_builtin_edges,
  divergence_builtin_inventory, divergence_parenting_task_lifecycle,
  divergence_introspection_runtime, divergence_utility_builtins,
  divergence_waif_statement_semantics, divergence_command_connection_lifecycle,
  divergence_messages_decompile_dispatch, divergence_input_lifecycle_decompile.

## Old marker: 98/100
- Batch 9 (divergence_messages_decompile_dispatch.yaml): Toast 14/14 green,
  Barn red x11 (all msg_* texts, unknown_error_identifier E_VARNF, all 4
  decompile tests, duplicate_verb_names_first_wins). Green: both wildcard tests.
  Gotcha: set_verb_code by literal name "foo*bar" is E_VERBNF even on Toast
  (query * is literal vs stored pattern) — use verb INDEX.
- run8: elseif decompile divergence (Toast keeps elseif, drops empty else;
  Barn nests if inside else) → batch 10.
- Batch 10 (divergence_input_lifecycle_decompile.yaml, written): elseif
  decompile, E_ARGS got-2 text, read()-in-command consumes next line,
  do_login_command survives suspend. Toast-validate next, then Barn.
- Batch 8 Barn-red x3: cmd_tofloat_hex_string_parses,
  cmd_out_of_band_quote_prefix_unquotes_to_inband, cmd_program_intrinsic.
  Green: OOB-to-listener-handler, PREFIX/SUFFIX, hold-input, multi-cmd order,
  say/emote shortcuts.
- Harness transport lesson: send() consumes pending output (0.5s quiet window);
  attach expect.output to the SEND step, not a later read_connection.
- run7 (error messages + decompiler): batch 9 candidates:
  E_TYPE detail "(expected string; got integer)"; E_PROPNF names prop;
  E_ARGS "(expected 1; got 0/2)"; E_ASSERT → runtime E_VARNF "Variable not
  found: E_ASSERT" (Barn: syntax error!); E_FILE "File error"; E_INTRPT
  "Interrupted"; verb_code parenthesize flag (1+ (2*3), ((1==2)&&3)||4, (!1)+2);
  decompiled float literal "15000000000.0" not "1.5e+10".
  Plus verb wildcard dispatch tests (foo*bar, "*", duplicate names).

## Old marker: 84/100 — COMMITTED in moo-conformance-tests 853516e (Q asked for
   commits as I go; batch 8 will be a follow-up commit once validated)

## Batch 8 command/connection debugging state:
- Harness transport sends an INITIAL BLANK LINE on connect → extra
  do_login_command("") call; expectations must account for it.
- send.text does NOT template {captures} → used "$dcg_target" sysprop form
  for .program (now Toast-green). multi-command order Toast-green with
  leading "".
- OOB #$# lines dispatch to the LISTENER HANDLER's do_out_of_band_command
  (not #0) — fixed, revalidating.
- prefix/suffix test: marco produced NO socket output on Toast even though
  say/emote (prop-logged) commands run; added #0.dcp_ran marker to see if
  marco runs; read_connection returned []. Investigating.
- Batch 7 waif/statements: Toast 14/14 green (waif-as-map-key ALLOWED, literal
  "[[class = #N, owner = #N]]"); Barn red x3: rangeset_zero_zero_prepends,
  finally_return_overrides_try_return, waif_recycle_verb_called_on_release.
  Barn waif support otherwise green (dispatch, :props, owner, typeof, map key).
- run6 formatting sweep: only tofloat("0x10")==16.0 (Toast hex strtod) usable;
  float tostr/toliteral formatting fully aligned. Expression lane near-exhausted.
- Batch 8 (writing now): connection/command lifecycle via listener-handler
  pattern + read_connection/send_bytes: out-of-band #$# dispatch, #$" quote
  prefix, PREFIX/SUFFIX framing, hold-input option, multi-command order,
  .program built-in command, "/:/; parser shortcuts. tofloat hex goes in too.
- read_connection expect idiom: expect: {output: {exact: [lines...]}} (see
  connection_lifecycle_toast_oracle.yaml:1621).

## Old count marker: 81/100
- Batch 5 introspection: 16 red (9 function_info signatures: chparent chparents curl
  generate_json shutdown simplex_noise spellcheck url_encode url_decode; raise(1.5)
  and raise("boom") value-raise; new_waif E_INVIND; memory_usage floats; exp tostr
  fmt; server_version(1) 9 sections; db_disk_size>0). Green coverage: ticks/seconds
  budgets, listeners maps, gc_stats/pcre_cache_stats/sqlite_handles/threads/
  reseed_random/run_gc, callers shape (3 frames in harness eval).
- Batch 6 utility: 19 Toast-green, 13 red (distance {} and mismatched-len, relative_
  heading E_TYPE, simplex_noise 1-4D exact values, salt $1$/$5$/$6$/bogus encodings,
  argon2_verify garbage->0, decode_binary ~09 tab-in-string). Green: argon2_verify
  valid hash, locate_by_name, switch_player, thread_pool (perm artifacts in differ).
- run4 leftover NOT captured: parse_ansi("[random]x") color choice (flaky/random).
- Next: corpus5 statements-via-eval() differ (bash heredoc mangled backslashes —
  write generator via Write tool); then waif lifecycle YAML batch; then connection/
  command batch if needed.

## Old count marker: 52/100 (batch 4 parenting/task: Toast 21/21 green, Barn red only
   fork_variable_matches_child_task_id; rest kept as coverage)

## Run3 arity sweep (624 probes, 38 mismatches) — key finds for batch 5:
- function_info metadata WRONG in Barn for: chparent, chparents, curl,
  generate_json, shutdown, simplex_noise, spellcheck, url_encode, url_decode
  (Barn reports 0/-1 arity; Toast has typed rows) → 9 exact-value tests.
- raise(1.5): Toast returns 1.5 (non-ERR first arg returned!), Barn E_TYPE.
- new_waif(): Toast E_INVIND, Barn E_INVARG.
- ticks_left() in fg eval: Toast ~60000 budget, Barn ~30000 → test > 30000.
- seconds_left(): Toast 5, Barn 1000 → test <= 10 (validate Toast first).
- memory_usage(): Toast list of FLOATS, Barn ints → typeof test.
- exp(1.5) tostr: Toast "4.48168907033806" vs Barn "...07" (float fmt).
- server_version(1): Toast 9 fields (incl os/options/source), Barn 6.
- listeners()[1]: Toast MAP shape.
- db_disk_size() > 0; queue_info() == {} (validate); gc_stats/pcre_cache_stats/
  sqlite_handles/threads/reseed_random/run_gc wizard access (may be green).
- Perm/dbtool artifacts to ignore: caller_perms/callers/task_id/random/ftime/frandom.

## Batch 4 in flight: audit/divergence_parenting_task_lifecycle.yaml (21 tests)
Toast corrections learned on first validate:
- chparent REMOVES child's explicit override of a prop inherited from old parent
  (value comes from new parent: "parent2") — fixed expectation.
- pass() with parentless definer raises E_INVIND (not E_VERBNF).
- tostr(err) = message ("Invalid argument"); use toliteral(err) for "E_INVARG".
- multi-parent pass traversal: redesigning test empirically (o+p1+p2 all with
  chain verb catching ANY).
- audit/divergence_numeric_string_edges.yaml: 8 red (see pilot list below)
- audit/divergence_move_recycle_lifecycle.yaml: 19 tests Toast-green; 5 red on Barn:
  move_nonwizard_no_accept_is_enacc, move_accept_recycling_what_returns_silently,
  recycle_moves_contents_out_with_exitfunc_on_recycled_object,
  recycle_runs_location_exitfunc_for_recycled_object,
  recycle_suspending_recycle_verb_completes
- audit/divergence_builtin_edges.yaml: 50 tests ALL Toast-green (crypt 1-char salt
  test flipped to randomization check). Barn run pending.
- Corpus2 findings (run2, 72 mismatches): E_VARNF cascade was dbtool artifact
  (`player` var unbound in -eval-file context — do NOT trust perm/player probes
  from differ; harness-verify). Real candidate families: invalid-obj E_INVARG vs
  E_INVIND across many builtins; object_bytes inverted (E_INVIND on Toast);
  parse_json dup-keys first-wins + maxint int; crypt $6$salt$ E_INVARG;
  string_hmac md5 E_INVARG; ctime clamp/E_INVARG; ftime(n) monotonic;
  notify/boot_player leniency; create($nothing, 2/-1) anon; task_local;
  queue_info(#invalid)=0; buffered_output_length()=65536; force/flush_input=0;
  server_log/load_server_options wizard=0; reset_max_object=0.

## Toast lane FAST path (verified, 8s total):
wsl -d Debian -u root -e bash -lc 'rsync -a --delete --exclude .git --exclude .venv
  --exclude pyghidra_mcp_projects --exclude "*.log" /mnt/c/Users/Q/code/moo-conformance-tests/ /root/mct/
  && cd /root/mct && uv run moo-conformance --server-command="/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}"
  -k "capability_admission or SEL" -q'
(ALWAYS rsync before Toast run.)

## Lane 2 batch 1 (in progress)
- File: audit/divergence_move_recycle_lifecycle.yaml — 19 move/recycle lifecycle tests
  drafted from pinned Toast source (objects.cc do_move/bf_recycle/move_to_nothing).
  Key semantics targeted: accept/E_NACC/wizard bypass, accept-recycles-what silent 0,
  exitfunc-after-location-change, exitfunc-moves-object suppresses enterfunc,
  suspend in exitfunc/recycle verb, position arg contents ordering, same-loc
  reorder without hooks, recycle: :recycle hook, self-recycle E_INVARG, contents
  exitfunc on recycled obj, reparent children to grandparent.
- Schema gotchas: test-level `setup:` must be a MAPPING not list (inlined instead);
  never expect raw object literals in YAML values — compare with == in MOO.
- Loop: run Toast → harvest actuals for wrong guesses → fix YAML → Toast green →
  run Barn → reds are divergences.

## Next
- Toast-validate lane 2 batch 1, then Barn.
- Corpus 2 for lane 1 (strings/lists/maps/verbs/props mutation).

## Divergences confirmed so far: 0 / 100
(list will go here as name -> yaml file -> toast result vs barn result)
