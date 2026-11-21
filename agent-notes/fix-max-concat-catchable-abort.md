# Fix max_concat_catchable abort (barn #1089)

## Goal

When a value grows past a size limit and `$server_options.max_concat_catchable`
is unset or false, ToastStunt aborts the task; Barn raised a catchable E_QUOTA
and never read the option. Pin Toast behavior in conformance tests, then make
Barn match at every place Toast calls make_space_pack or its interpreter
equivalent.

## State

- Conformance: `server/limits_concat_catchable.yaml` on
  test/max-concat-catchable-abort (27 tests). Barn fix on
  fix/max-concat-catchable-abort. The team lead pushes; no PRs from me.
- Managed runs (WSL, `-k "capability_admission or (limits_concat_catchable)"`
  unless stated):
  - First 13 tests: stock Toast 14 passed with admission (load 15.17 /
    28.70); Barn before the fix 9 failed (catchable E_QUOTA where Toast
    aborts; map index assignment ignored max_map_value_bytes; the hook test
    saw "caught") (load 32.73 / 16.16).
  - Barn with the VM/builtins fix only: the player traceback said "Too many
    verb calls" (the E_MAXREC message) instead of Toast's "Task ran out of
    seconds". Fixed in `engine/task_runtime.go` `reportUncaughtException`
    (applies to real timeouts too).
  - Full suite then failed
    `server/limits.yaml::mapdelete_uses_list_not_map_max_value_bytes`
    (Toast-passing): Toast builds call arguments as a list and checks it
    (emergency-mode probe on Test.db, max_list_value_bytes 1021, 200-entry
    map x: `mapdelete(x, 1)`, `{x, 1}`, `maphaskey(x, 1)` E_QUOTA; `{x}` and
    `length(x)` fine). Base Barn passed it by accident (map index assignment
    used the list limit). Barn now checks argument lists of builtin, verb and
    pass() calls; two suite tests pin it. A later full run failed
    `server/limits.yaml::setadd_checks_list_max_value_bytes_small` because
    the check refused exactly the limit; Toast's OP_LIST_ADD_TAIL refuses
    only more, fixed.
  - Remaining make_space_pack sites (chr, value_hash, value_hmac,
    encode_base64, decode_base64; both option states) and "a list result of
    exactly the limit is allowed" (decode_binary, `{@x, 1}`): 12 more tests.
    Stock Toast: 28 passed with admission (load 21.02 / 14.00).
  - Full managed suite results for the final build: see "Final runs".
- Go: `engine/concat_catchable_test.go`, `types/list_bytes_test.go`,
  `vm/bytecode_execution_test.go` (argument list at the limit),
  `builtins/crypto_test.go` (decode_base64); each failed on the code before
  its fix. Existing builtins/vm tests that pinned old Barn behavior (E_QUOTA
  without the option, the list limit applied to maps, a list of exactly the
  limit refused) were moved to Toast's behavior. gofmt, go vet, staticcheck,
  golangci-lint (0 issues), `go test ./...` pass.

## Final runs

(filled at the end; see the bottom of this section)

## Performance

- The first argument-list check measured `types.ListBytes` on every call with
  two or more arguments: `BenchmarkVM/builtin_two_args_200k` (new workload,
  `maphaskey(m, i)` in a loop) +6% with alignment controlled (12 interleaved
  rounds: base 30.9-31.7 ms, fixed 33.2-33.8 ms; load 22-25). Removed: the
  check now reads one atomic flag (`Session.ListLimitLowered`, set by
  `load_server_options()`, or the task's own pending options) and measures
  nothing while max_list_value_bytes is at or above its default 64537861.
  Same benchmark after: base 32.44 ms, fixed 32.80 ms (12 rounds, load 25.28
  / 24.20). Knowing divergence: with the default list limit Barn does not
  refuse an argument list over 64537861 bytes; Toast does. That needs more
  than 61 MiB of arguments in one call.
- The BenchmarkVM set showed a second effect unrelated to any changed code:
  every workload, including int_arith, nested loops and while_lt, ran 5-15%
  slower in the default `go test` build. Bisect: reverting either of two
  cold, unrelated edits (builtins/crypto.go or builtins/lists.go) removed it;
  reverting vm/vm.go, task, types did not. In all eight builds measured,
  `vm.(*VM).executeLoop` started on a 64-byte boundary in every fast build and
  at 32 mod 64 in every slow one (`go tool nm`). The Go linker aligns
  functions to 32 bytes, so any edit anywhere can move the interpreter loop
  across a 64-byte line. With `-ldflags=-funcalign=64` on both builds, the
  whole BenchmarkVM set is within 2% (5 interleaved rounds, load 23.01 /
  16.76): builtin_abs 28.84/28.87, builtin_two_args 31.26/31.95, float_arith
  48.70/48.33, if_chain 96.73/95.93, int_arith 45.74/45.59, list_append
  2.44/2.30, list_index 70.96/71.27, list_iter 44.26/44.37, nested
  45.58/45.44, prop_access 200.3/201.3, prop_defined 212.4/207.3, prop_name
  114.6/113.7, string_concat 1.58/1.52, tostr 63.97/64.82, typeof
  119.4/119.2, while_lt 45.72/45.85 ms (base/fixed medians). The deployment
  build (`GOOS=linux GOAMD64=v3`, PGO from cmd/barn/default.pgo) puts
  executeLoop on a 64-byte boundary for both base and fixed. The default
  test build of the fixed tree does not (32 mod 64), so a plain
  `go test -bench BenchmarkVM` comparison still shows the layout shift.
  Fixing that for every future change would mean building benchmarks with
  `-ldflags=-funcalign=64`; that is a build-policy change I did not make.
- Real-workload gate before the flag (`TestMongooseRealWorkload`,
  mongoose.db.new, 3 interleaved runs, load 5-29): 1 player base 84/87/84 vs
  fixed 88/84/75 cmd/s; 16 players base 422/354/331 vs fixed 408/351/404:
  inside noise. The harness reports FAIL on both builds (failed/uncaught
  commands from the dump's own verbs, same counts).

## Toast source (origin/master)

- Option: `src/include/server.h:230` `SVO_MAX_CONCAT_CATCHABLE`, kind flag,
  default 0, cached by `load_server_options()` like `include_rt_vars`.
- `make_space_pack()` returns E_QUOTA when the flag is set, else
  `make_abort_pack(ABORT_SECONDS)` (copies in list.cc:620, base64.cc:41,
  crypto.cc:391, numbers.cc:821). Callers: list.cc setadd, setremove,
  listinsert/listappend, listdelete, listset, decode_binary (result
  `value_bytes(r) <= max_list_value_bytes` passes); strsub, tostr, toliteral,
  substitute, encode_binary, chr (stream past `stream_alloc_maximum` =
  max_string_concat + 1); base64.cc encode_base64 (`(len / 3) * 4 >
  stream_alloc_maximum`), decode_base64 (`(len / 4) * 3 >
  stream_alloc_maximum`, after input validation); crypto.cc value_hash and
  value_hmac (fresh stream holding the unparsed value), binary_hash,
  string_hmac, binary_hmac (only `binary_to_raw_bytes`, see below);
  numbers.cc random_bytes. No map builtin calls it.
- Interpreter: `execute.cc:899` `PUSH_ERROR_UNLESS_QUOTA`: E_QUOTA with the
  flag off is `abort_task(ABORT_SECONDS)`, regardless of the `d` flag. Used
  by OP_MAP_INSERT (map limit), OP_LIST_ADD_TAIL and OP_LIST_APPEND (list
  limit; this is also how call arguments are built), OP_INDEXSET (list/map),
  OP_ADD (string vs max_string_concat, list vs list limit), EOP_RANGESET.
  Every one passes `value_bytes(r) <= limit`.
- The issue's reproducer `{@r, i}` goes OP_LIST_APPEND/OP_LIST_ADD_TAIL ->
  `PUSH_ERROR_UNLESS_QUOTA`; the issue's description is right.
- `abort_task(ABORT_SECONDS)` (execute.cc:569): `handle_task_timeout` gets
  `{"seconds", stack, traceback}` ("Task ran out of seconds"),
  `unwind_stack(FIN_ABORT)` skips finally and except. After the task
  (execute.cc:3193) the hook runs; true or suspended means nothing else,
  otherwise the traceback lines go to the player. Same as a real timeout.
- Minimum limits: 1021 for all three (smaller positive values are raised).

## Decisions and why

- One signal for "the task ran out of seconds": builtins return
  `builtins.SecondsAbortResult()`; `executeCallBuiltin` turns it into the
  `secondsAbort` sentinel and the run loop takes the existing `secondsLimit`
  exit before the `d`-flag branch. Only typed metadata sets that reason, so
  `raise(E_MAXREC, ...)` cannot trigger it.
- `Session.SpaceResult` / `VM.quotaError` are make_space_pack /
  PUSH_ERROR_UNLESS_QUOTA, used only at Toast sites. Barn-only size checks
  (implode, map builtins, encode_chars/decode_chars/graphemes, wasm, fileio,
  pcre_replace, the `{a..b}` range guard) keep E_QUOTA.
- Map index and range assignment check max_map_value_bytes (Toast
  OP_INDEXSET/EOP_RANGESET); the keyed property writeback hands the store the
  map limit + 1 because the store refuses `>=`.
- `CheckListLimitForTask` now refuses only more than the limit (Toast `<=`
  passes); it had refused exactly the limit.
- Not changed: binary_hash, string_hmac and binary_hmac. Their only stream is
  the static one inside `binary_to_raw_bytes` (utils.cc:907), which throws
  only when it has to grow; once any earlier call has grown it, a later call
  never throws. Toast's result there depends on what earlier calls in the
  process decoded, so no deterministic Toast test can pin it, and copying a
  plain length check would differ from Toast whenever the buffer is already
  large.

## Fuzz generator reach

- `internal/fuzzseed/structured.go` never names a $server_options property,
  so every generated program runs with `max_concat_catchable` unset (false)
  and the default limits. It can reach a size limit only in that state, by
  repeated doubling in a loop (strings are at most 6 characters, lists 4
  elements) against the 64537861-byte defaults. The true state, and the
  argument-list check (which needs a lowered list limit), are unreachable.
- `vm/fuzz_test.go` `checkLiteralRoundTrip` treats any toliteral exception as
  a result, so the abort does not disturb it.

## Surprises

- Toast's grammar has no `try ... except ... finally`; except and finally
  must be nested. A plain YAML scalar `run: return #0.x;` loses everything
  after `#`; quote such runs.
- Barn applied the list limit to maps on purpose in July 2026; Toast 03af4fa
  and master use the map limit, and the Toast run with only the map limit
  lowered confirms it.
- `git stash` is shared by every worktree of the barn repository: another
  agent's push/pop pair took my entry. Restored; never use stash in a shared
  repo. The session scratchpad is shared too: another agent overwrote my
  `run_barn.sh`, so scratch scripts need task-specific names.
- `files/` in the Barn worktree (log.sqlite, news.sqlite, sound.sqlite) was
  written by my TestMongooseRealWorkload runs (`files/sqlite/sound.sqlite`
  is where the harness copies the catalog; the others are the dump's own
  sqlite files). It held Mongoose data, so I deleted it; nothing of it was
  committed.

## Merge order

Barn's CI runs the suite's main branch. The new suite tests fail on unfixed
Barn, so the Barn PR merges first and the suite PR waits for it. Fixed Barn
against the suite's current main: see "Final runs".

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
assignment (only the map limit lowered), `listappend()`, `tostr()`, `chr()`,
`value_hash()`, `value_hmac()`, `encode_base64()` and `decode_base64()`, plus
the unset option, a verb without the `d` flag, the timeout hook, the
argument-list check (Toast builds call arguments as a list, so two or more
arguments whose list passes `max_list_value_bytes` are refused; a lone
argument is not), and a list result of exactly the limit being allowed.
Limits are 1100 and the values grow far past them, so nothing depends on
value width. Stock WSL Toast: 28 passed with `capability_admission`.

Merge after the Barn PR (Barn CI runs this repository's main, and these
tests fail on Barn without it).

## Barn PR body draft

Title: Abort the task at a size limit unless max_concat_catchable is set

Fixes #1089

- Barn reads `$server_options.max_concat_catchable` in
  `load_server_options()`, cached like `include_rt_vars`, including the
  same-task pending view.
- Every Toast `make_space_pack` builtin site (setadd, setremove, listappend,
  listinsert, listdelete, listset, decode_binary, strsub, substitute, tostr,
  toliteral, encode_binary, chr, encode_base64, decode_base64, value_hash,
  value_hmac, random_bytes) and every interpreter `PUSH_ERROR_UNLESS_QUOTA`
  site (list/map literals, splice, `+`, index and range assignment) raises
  E_QUOTA only when the option is true. Otherwise the task takes the
  out-of-seconds exit: no handler, no finally, whatever the `d` flag, and
  `#0:handle_task_timeout` with "seconds". chr, value_hash, value_hmac and
  decode_base64 had no size check before; encode_base64 now uses Toast's
  `(len / 3) * 4` rule.
- An unhandled timeout reports Toast's "Task ran out of seconds" (or ticks)
  traceback, not the E_MAXREC message "Too many verb calls".
- Map index and range assignment check `max_map_value_bytes`, as Toast does.
- List size checks refuse only more than `max_list_value_bytes`; a list of
  exactly the limit is allowed, as in Toast.
- Builtin, verb and `pass()` calls hold two or more arguments (or a spliced
  argument list) to `max_list_value_bytes` while that limit is below its
  default, as Toast does by building the arguments as a list. At the default
  limit nothing is measured (Toast would refuse an argument list over
  64537861 bytes; Barn does not).
- binary_hash, string_hmac and binary_hmac are left alone: Toast's check
  there depends on a static buffer's history (`binary_to_raw_bytes`).

Performance: with both builds at `-ldflags=-funcalign=64`, the BenchmarkVM
set (including the new `builtin_two_args_200k`) is within 2% of master. The
default `go test` build of this tree places `executeLoop` 32 bytes off a
64-byte line and runs every BenchmarkVM workload 5-15% slower, also loops
that touch no changed code; reverting either of two cold, unrelated edits
removes it. The deployment build (PGO) is 64-byte aligned for both. Numbers
and the bisect are in the task note.

Merge before the suite PR.
