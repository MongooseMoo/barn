# Fix max_concat_catchable abort (barn #1089)

## Goal

When a value grows past a size limit and `$server_options.max_concat_catchable`
is unset or false, ToastStunt aborts the task; Barn raises a catchable E_QUOTA
and never reads the option. Pin Toast behavior in conformance tests, then make
Barn match.

## State

- Conformance: `server/limits_concat_catchable.yaml` (14 tests incl.
  admission) committed on test/max-concat-catchable-abort, not pushed.
- Managed runs (WSL, `-k "capability_admission or (limits_concat_catchable)"`):
  - stock Toast (`make conformance-toast`): 14 passed (load 15.17 before,
    28.70 after).
  - Barn before the fix (binary built from the unfixed tree): 9 failed,
    5 passed (load 32.73 / 16.16). Failures: catchable E_QUOTA where Toast
    aborts; map index assignment ignored max_map_value_bytes; the hook test
    saw "caught".
  - Barn with the VM/builtins fix only: 7 failed; the abort happened but the
    player traceback said "Too many verb calls" (the E_MAXREC message)
    instead of Toast's "Task ran out of seconds". Fixed in
    `engine/task_runtime.go` `reportUncaughtException` (applies to real
    seconds timeouts too).
  - Barn fixed: 14 passed (load 8.81 after).
  - Full managed suite on that build: 1 failed, 13833 passed, 325 skipped
    (load 10.33 before, 11.07 after). The failure,
    `server/limits.yaml::mapdelete_uses_list_not_map_max_value_bytes`, is
    Toast-passing: Toast builds call arguments as a list and checks it
    (emergency-mode probe on Test.db with max_list_value_bytes 1021 and a
    200-entry map x: `mapdelete(x, 1)`, `{x, 1}`, `maphaskey(x, 1)` all
    E_QUOTA; `{x}` and `length(x)` fine). Base Barn passed it only by
    accident (its map index assignment used the list limit). Fixed: Barn
    now checks argument lists of builtin, verb and pass() calls
    (`vm/error.go` checkCallArgsLimit / checkArgListLimit /
    checkSplicedArgsLimit). Two suite tests added for it.
  - Toast and fixed Barn on the 16 new tests plus the three limits.yaml
    map/list tests: 19 passed each (load 10.06 before Toast, 14.55 after
    Barn).
  - Full managed suite with the argument-list check: 1 failed, 13835
    passed, 325 skipped (load 7.87 / 12.48):
    `server/limits.yaml::setadd_checks_list_max_value_bytes_small`. The
    check refused an argument list of exactly the limit (it used Barn's
    `>=`); Toast's OP_LIST_ADD_TAIL refuses only `>`. Changed to `>`, with a
    VM boundary test. The limits, crash_prevention and
    server_options_same_task suites then passed on Barn (64 passed,
    1 skipped; load 9.70 / 34.19).
  - Full managed suite on the final build: 13836 passed, 325 skipped, 0
    failed (load 34.43 before, 17.17 after).
- Performance of the argument-list check: `BenchmarkVM/builtin_two_args_200k`
  (new workload, `maphaskey(m, i)` in a loop) measured 10 interleaved runs:
  base median 30.0 ms, fixed 33-36 ms on a loaded machine (the check reads
  each argument's cached size; `types.ListBytes` reads list/map caches in
  place, ~6 ns for two arguments in a microbenchmark). One-argument calls
  (`builtin_abs_200k`) unchanged. Real-workload gate
  (`TestMongooseRealWorkload`, mongoose.db.new, 3 interleaved runs each,
  load 5-29): 1 player base 84/87/84 vs fixed 88/84/75 cmd/s, 16 players
  base 422/354/331 vs fixed 408/351/404 cmd/s: no difference outside the
  noise. The harness reports FAIL on both builds (failed/uncaught commands
  from the dump's own verbs, same counts on both).
- Go: new `engine/concat_catchable_test.go` (failed before, passes after);
  existing builtins/vm tests that expected E_QUOTA without setting the option
  now set `MaxConcatCatchable` (they pinned the old wrong default); the map
  index/range tests and the keyed-map rebase test now use the map limit.
  gofmt, go vet, staticcheck, golangci-lint (0 issues), `go test ./...` all
  pass.

## Toast source (origin/master)

- Option: `src/include/server.h:230` `SVO_MAX_CONCAT_CATCHABLE`, kind flag,
  default 0, cached by `load_server_options()` like `include_rt_vars`
  (truthiness of `$server_options.max_concat_catchable`).
- Builtins: `make_space_pack()` returns `make_error_pack(E_QUOTA)` when the
  flag is set, else `make_abort_pack(ABORT_SECONDS)` (copies in list.cc:620,
  base64.cc:41, crypto.cc:391, numbers.cc:821). Callers: list.cc setadd,
  setremove, listinsert/listappend (insert_or_append), listdelete, listset,
  decode_binary (list limit); strsub, tostr, toliteral, substitute,
  encode_binary, chr (stream past `stream_alloc_maximum` =
  max_string_concat+1); base64.cc encode_base64/decode_base64; crypto.cc
  binary_hash, value_hash, string_hmac, binary_hmac, value_hmac (streams);
  numbers.cc random_bytes. No map builtin calls it.
- Interpreter: `execute.cc:899` `PUSH_ERROR_UNLESS_QUOTA`: when the error is
  E_QUOTA and the flag is off, `abort_task(ABORT_SECONDS)` and return
  OUTCOME_ABORTED, regardless of the verb's `d` flag. Used by OP_MAP_INSERT
  (map literal, map limit), OP_LIST_ADD_TAIL and OP_LIST_APPEND (list
  literal and `@` splice, list limit), OP_INDEXSET (list/map), OP_ADD
  (string concat vs max_string_concat, list + vs list limit), EOP_RANGESET
  (map/list/string).
- The issue's reproducer `{@r, i}` goes OP_LIST_APPEND/OP_LIST_ADD_TAIL ->
  `PUSH_ERROR_UNLESS_QUOTA`, so the issue's description (same option decides
  in the interpreter) is right; the mechanism is the macro, not make_space_pack.
- `abort_task(ABORT_SECONDS)` (execute.cc:569): saves handler info
  `handle_task_timeout` with args `{"seconds", stack, traceback}` where the
  traceback message is "Task ran out of seconds", then
  `unwind_stack(FIN_ABORT)`, which skips `finally` bodies and except
  handlers. After the task ends (execute.cc:3193) `#0:handle_task_timeout`
  runs if it exists; if it returns true or suspends, nothing else; otherwise
  the traceback lines are notified to the player. Exactly the path of a real
  seconds timeout.
- Minimum limits: 1021 for max_string_concat, max_list_value_bytes,
  max_map_value_bytes (smaller positive values are raised to 1021).

## Barn source (before the fix)

- No production Go code reads `max_concat_catchable`
  (`builtins/limits.go` `collectServerOptions` does not load it).
- Barn's seconds abort: `vm/vm.go` `secondsLimitResult()` returns
  E_MAXREC with `ExceptionReasonSecondsBudget` straight out of the run loop
  (no handlers, no finally); `engine/task_runtime.go`
  `reportUncaughtException` calls `callTaskTimeoutHook(..., "seconds")`.
- Interpreter size sites: vm/op_arith.go (string +, list +), vm/op_list.go
  (list literal, map literal, splice, add tail, OP_CHECK_MAP_LIMIT),
  vm/op_index.go (indexset, rangeset), vm/collection_helpers.go.
- Builtin size sites: builtins/lists.go, strings.go, types.go, crypto.go,
  plus Barn-only sites (maps.go, unicode.go, wasm.go, fileio.go, pcre.go)
  that have no make_space_pack counterpart in Toast.

## Decisions and why

- One signal for "the task ran out of seconds": builtins return
  `builtins.SecondsAbortResult()` (E_MAXREC, `ExceptionReasonSecondsBudget`,
  the same result `vm.secondsLimitResult` builds); `executeCallBuiltin` turns
  that into the `secondsAbort` sentinel and the run loop sends it to the
  existing `secondsLimit` exit, before the `d`-flag branch. So the limit abort
  takes exactly the path of a real seconds timeout (no handler, no finally,
  `#0:handle_task_timeout` with "seconds", the same traceback). Only typed
  metadata sets that reason, so `raise(E_MAXREC, ...)` cannot trigger it.
- `Session.SpaceResult` / `VM.quotaError` are make_space_pack /
  PUSH_ERROR_UNLESS_QUOTA. Converted only sites that are Toast
  make_space_pack or PUSH_ERROR_UNLESS_QUOTA sites. Barn-only size checks
  (implode, map builtins, encode_chars/decode_chars/graphemes, wasm, fileio,
  pcre_replace, the `{a..b}` range literal guard) keep E_QUOTA: Toast has no
  such check to copy.
- Map index assignment and map range assignment checked the list limit
  (`vm/collection_helpers.go`, `vm/op_index.go`); Toast's OP_INDEXSET and
  EOP_RANGESET check max_map_value_bytes. Fixed on the same path (the new map
  tests lower only the map limit, and Barn completed the loop). The keyed
  property writeback (`vm/op_property.go`) now hands the store the map limit
  as its first refused size (limit + 1, since the store refuses `>=`).
- `max_concat_catchable` is read like `include_rt_vars` (defined on
  $server_options, truthiness) into the cached snapshot, including the
  same-task pending view.

## Fuzz generator reach

- `internal/fuzzseed/structured.go` never names a $server_options property
  (its property names are name/owner/p/location/contents), so every generated
  program runs with `max_concat_catchable` unset, i.e. false. It can reach a
  size limit only in that state, and only by repeated doubling in a loop
  (`acc = acc + acc`; strings are at most 6 characters and lists 4 elements)
  against the 64537861-byte defaults. It cannot reach the true state; that
  would need the generator to define and set the option and call
  `load_server_options()`, which it does not do.
- `vm/fuzz_test.go` `checkLiteralRoundTrip` treats any exception from
  toliteral (now the seconds abort when over the limit) as a result, so it is
  unaffected.

## Surprises

- Toast's grammar has no `try ... except ... finally`; the first Toast runs
  failed my own tests with "syntax error" until except and finally were
  nested. A plain YAML scalar `run: return #0.x;` loses everything after `#`
  (YAML comment); such runs must be quoted.
- Barn checked map index assignment and map range assignment against
  max_list_value_bytes (deliberately, in July 2026: "apply list limit to map
  index assignment"); Toast 03af4fa and master check max_map_value_bytes. The
  Toast run of `map_indexset_past_limit_raises_when_catchable` (only the map
  limit lowered) passed, which confirms the source.

## Found, not fixed here

- `chr()` has no string-limit check in Barn (`builtins/math.go`); Toast
  `bf_chr` builds under TRY_STREAM and calls make_space_pack. No conformance
  test pins it yet.
- Barn's binary_hash/value_hash/hmac builtins and decode_base64 have no
  stream-limit check; Toast's do (make_space_pack). Same.
- `CheckListLimitForTask` refuses a list of exactly the limit (`>=`), while
  Toast's interpreter and list builtins refuse only `>`; existing Toast-passing
  tests (server/limits.yaml) pass, so value_bytes accounting likely differs by
  the same margin. Not touched.

- `git stash` is shared across all worktrees of the barn repository. While I
  used push/pop to build an unfixed binary, another agent (branch
  fix/reserved-property-names) pushed its own stash between my push and pop;
  my stash entry (engine/task_runtime.go only) was popped by someone else and
  my pop hit theirs (it aborted and kept theirs). I restored my change from
  the unreachable stash commit. Their worktree may now hold my
  engine/task_runtime.go diff. Build a base binary from a separate worktree,
  never with stash.

## Next step

- Push both branches and open the PRs (team lead's call), suite PR first;
  the Barn PR's conformance job needs the suite PR merged.

## Suite PR body draft

Title: Pin max_concat_catchable: size limits abort the task unless it is set

Adds `server/limits_concat_catchable.yaml`. In ToastStunt a value that grows
past `max_string_concat`, `max_list_value_bytes` or `max_map_value_bytes`
raises E_QUOTA only when `$server_options.max_concat_catchable` is true
(`list.cc` `make_space_pack`, `execute.cc` `PUSH_ERROR_UNLESS_QUOTA`).
Otherwise the task aborts as out of seconds: no except or finally runs, the
player gets the "Task ran out of seconds" traceback and
`#0:handle_task_timeout` gets "seconds".

The tests cover both settings for list splice, string `+`, map index
assignment (only the map limit lowered), `listappend()` and `tostr()`, plus
the unset option, a verb without the `d` flag, and the timeout hook. A second
commit pins the argument-list check: Toast builds call arguments as a list, so
two or more arguments whose list passes `max_list_value_bytes` abort (or raise
E_QUOTA when catchable); a lone argument is never checked. That is what
`server/limits.yaml::mapdelete_uses_list_not_map_max_value_bytes` already
observes. Limits are 1100 and the loops grow far past them, so nothing depends
on value width. Stock WSL Toast: all 16 new tests pass with
`capability_admission`.

## Barn PR body draft

Title: Abort the task at a size limit unless max_concat_catchable is set

Fixes #1089

- Barn reads `$server_options.max_concat_catchable` in
  `load_server_options()`, cached like `include_rt_vars`, including the
  same-task pending view.
- Every Toast `make_space_pack` builtin site (setadd, setremove, listappend,
  listinsert, listdelete, listset, decode_binary, strsub, substitute, tostr,
  toliteral, encode_base64, encode_binary, random_bytes) and every interpreter
  `PUSH_ERROR_UNLESS_QUOTA` site (list/map literals, splice, `+`, index and
  range assignment) now raises E_QUOTA only when the option is true.
  Otherwise the task takes Barn's existing out-of-seconds exit: no handler,
  no finally, whatever the `d` flag, and `#0:handle_task_timeout` with
  "seconds".
- An unhandled timeout now reports Toast's "Task ran out of seconds" (or
  ticks) traceback, not the E_MAXREC message "Too many verb calls".
- Map index and range assignment check `max_map_value_bytes` as Toast does,
  not `max_list_value_bytes`.
- Builtin, verb and `pass()` calls hold two or more arguments (or a spliced
  argument list) to `max_list_value_bytes`, as Toast does by building the
  arguments as a list. On `BenchmarkVM/builtin_two_args_200k` (new) this costs
  about 3-6 ms per 200k two-argument calls on a loaded machine; the
  real-workload harness showed no difference outside its noise (numbers in
  the task note).

Checked: conformance `server/limits_concat_catchable.yaml` (suite PR): Toast
passes all 16; Barn failed 9 of the first 13 before and passes all 16 after;
full managed suite on the fixed build; `go test ./...`, go vet, staticcheck, golangci-lint.
