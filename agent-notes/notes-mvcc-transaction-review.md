# MVCC Transaction Review Work Log

## Current state

- Branch: `work/mvcc-concurrent-moo`
- Tracking checklist committed as `cd57f85` in `review-mvcc-transaction-findings.md`.
- Pre-existing tracked worktree was clean. Extensive pre-existing untracked files remain untouched.
- All ten review findings have been implemented and committed one at a time.

## Completed findings

- Finding 1: direct old-snapshot verb lookup reproduced shared-map corruption red, then passed after detaching all collections before composing a COW image. Package tests passed. Commit: `4df1cd1`.
- Finding 2: locking a read-set object's slot did not block a disjoint-write commit before the fix, proving the read slot was not held. The commit now locks the sorted union of read-set and write-set slots through validation and publish. Targeted and package tests passed. Commit: `ffe80a6`.
- Finding 3: a task read one property, performed and adopted an unrelated live verb mutation, then ignored a concurrent change to the original property because every read was rebased. The blanket rebase was removed; targeted adoption remains. Store and builtin package tests passed. Commit: `9342b20`.
- Finding 4: a conflict after `server_log` caused 65 log executions across retries. Builtin dispatch now marks immediate non-buffered effects as irreversible, and the scheduler refuses whole-task retry after the marker. The regression records one log execution and zero retries; builtin and scheduler tests passed. Commit: `8eae579`.
- Finding 5: the extracted ticker dequeue branch processed disconnect directly and left an existing connection lane registered. Routing it through `dispatch` retires the worker after disconnect. Targeted and full server package tests passed. Commit: `089b95a`.
- Finding 6: delete-then-redefine failed in both decentralized and coarse commit paths. Definition deletes now apply before defines, paired replacement validation is allowed, and later inherited writes cancel earlier staged descendant deletes. Both targeted paths and the store package passed. Commit: `5f036b6`.
- Finding 7: the scheduler regression was red because `boot_player(#7); notify(#7, ...)` produced notification output before disconnect. The four category queues were replaced by one concrete ordered `PendingEffects` log, all scheduler paths now flush it once in call order, and old category-specific fields/functions were removed. Kernel, builtin, and scheduler packages pass. Commit: `e6e5405`.
- Finding 8: 64 staged definitions committed with `property_09` first instead of `property_00`. Both decentralized grouping and coarse apply now derive definitions from the transaction object's `propOrder`. The focused test passed ten times and the store package passed. Commit: `5ff4436`.
- Finding 9: both read-only and write terminal tasks left one active read timestamp registered. A run-scoped deferred release now deregisters the task's current transaction on every return while retry replacement remains explicit. The targeted regression, store package, and scheduler package pass. Commit: `ab26425`.
- Finding 10: a failed deferred notification send converted a committed `return 42` task into uncaught `E_INVARG`. Deferred-effect flush now logs the first host error, continues later effects, and has no task-result error channel. The focused regression and builtin/scheduler packages pass. Commit: `467d314`.

## Current state / next action

- All ten review findings are checked in the durable review checklist.
- `go test ./... -count=1` passed after all ten fixes.
- The first documented managed conformance attempt could not resolve the editable dependency because `pyproject.toml` and `uv.lock` used `../../moo-conformance-tests`; the actual checkout is the documented sibling `C:\Users\Q\code\moo-conformance-tests`.
- The user explicitly directed use of the checkout under `code`. The path was corrected to `../moo-conformance-tests`, `uv lock` resolved successfully, and the correction was committed as `f8a483e`.
- Managed full run `20260721_102601` collected 11,558 tests. After initial parser passes, connection-lifecycle cases timed out and every subsequent unrelated case also failed while Barn remained listening on port 7788. The run exceeded the recorded managed failure-run baseline and was stopped; it did not establish a conformance result.
- Focused managed run `20260721_103418` selected `audit_listener_handler_do_login_command` and reproduced one actionable red failure in 7.65 seconds: the conformance cleanup expression timed out in `SocketTransport._receive()` waiting for Barn.
- Tracked worktree was clean after the focused red run. Managed run artifacts and this notes file remain untracked/unstaged.
- Current blocker: project instructions require live verification of this exact behavior against the documented stock WSL Toast oracle before Barn-side diagnosis.
- Next action: use `scripts/run_toast_wsl.sh` with the exact focused selector from the plan-managed oracle workflow, confirm Toast green, then diagnose and TDD-fix the Barn regression one source slice at a time. After Barn-focused green, rerun the relevant managed family, full managed conformance suite, `go test ./... -count=1`, and `git diff --check`; only then check and commit the final checklist gates.

This file is a required uncommitted checkpoint and must not be staged or committed.

## Conformance-timeout investigation update (2026-07-21 11:12 MDT)

- The focused failure occurs entirely on Windows Barn at `127.0.0.1:7788`; WSL is not involved in the failing path, so restarting WSL cannot fix this symptom.
- The focused YAML row is unchanged since conformance commit `d1a68aa7` on 2026-05-05.
- Windows firewall has an enabled inbound allow rule for the current `barn.exe`, and Barn accepted/read the loopback connections. There is no evidence of firewall packet loss.
- Managed diagnostic run `20260721_110542` used the runner's documented `-ExtraServerArgs` to expose Barn's built-in pprof endpoint. During the timeout, the TCP reader, connection lane, and scheduler workers were idle; nothing was blocked on socket output or a store lock.
- The managed server log captured the real failure: a recovered nil-pointer panic in `StoreTxn.Commit` at `db/store/store_txn.go:1621` while executing the primary conformance eval.
- Finding 8 commit `5ff4436` introduced the dereference. It iterates all `tx.objects` entries for property-definition insertion order, but missing-object reads intentionally cache `nil` in that map. A transaction containing both a cached miss and a valid property definition therefore panics at commit; recovery suppresses the eval response markers, producing the apparent network timeout.
- The facts-versus-theories record is committed as `6275b2c` in `investigations/mvcc-conformance-timeout.md`.
- Current blocker: none. The next authorized action is the red half of a single TDD source slice: add a minimal store regression for cached-missing-object plus valid property definition, prove the panic, then implement the smallest nil-safe iteration fix.

## First conformance repair slice completed

- Added separate decentralized and coarse store regressions for a transaction that caches missing object `#999`, defines a valid property on `#0`, and commits. Before the fix they panicked at `store_cow.go:464` and `store_txn.go:1621`, respectively.
- Both property-definition ordering loops now skip nil cached misses before reading `propOrder`; no helper or interface was added.
- Both regressions, the original insertion-order regression, and the full `db/store` package pass.
- Managed focused run `20260721_111040` passes `audit_listener_handler_do_login_command` 1/1, proving the apparent network timeout was fixed without firewall or WSL changes.
- The source repair is committed as `83b54f8` (`fix(store): ignore cached misses when ordering definitions`).

## Second conformance failure isolated

- Managed family run `20260721_111108` passed the first four `connection_lifecycle_toast_oracle` rows, then failed `audit_user_connected_hook_on_first_login` with expected `1`, actual `0`; subsequent rows cascaded into timeouts. Family result: 4 passed, 19 failed.
- Focused managed run `20260721_111457` reproduces only `audit_user_connected_hook_on_first_login`: clean semantic assertion failure (`#0.audit_connected_seen` does not equal the expected hook frame), no focused network timeout.
- The test installs `do_login_command` returning a new player and `user_connected` assigning `{this, player, caller, args, argstr}` to `#0.audit_connected_seen`; after 500 ms Barn reports the comparison as false.
- No panic or task-error record appears in the family server log for this first failure.
- Current blocker: none. Next action is to inspect the server hook task transaction lifecycle and determine which exact field/write differs or is missing before adding the next TDD regression.

## Server-hook caller repair completed

- A direct server regression recorded `user_connected` frame `{#0, #2, #2, {#2}, ""}`; Toast requires `{#0, #2, #-1, {#2}, ""}`. The hook write committed; only `caller` differed.
- `RunServerVerbTask` is restricted to server lifecycle/error hooks. It now assigns `t.Caller = types.ObjNothing` while preserving the player argument; no API/helper was added.
- The direct regression and complete `server`/`scheduler` packages pass. Managed focused run `20260721_111924` passes `audit_user_connected_hook_on_first_login` 1/1.
- The source fix is committed as `92bf74f`; investigation update is committed as `24c7e21`.

## Ordered-family contamination narrowed

- Managed family run `20260721_111949` passes the first five lifecycle rows, then reports `audit_user_connected_continues_after_zero_delay_fork` as a timeout and cascades through later rows.
- Managed focused diagnostic run `20260721_112342` passes that same zero-delay-fork row 1/1 in 3.95 seconds.
- Therefore the fork row is not independently broken. Its family failure depends on state left by one of the preceding rows, most likely cleanup/order interaction after the newly fixed simple first-login row.
- Current blocker: none. Next action is to run the minimal ordered selector containing the preceding simple hook row and the fork row, inspect cleanup/server state, and identify the first contaminating transition before editing source.

## Live verb deletion repair completed

- The minimal ordered selector proved the simple hook row contaminated the following zero-delay-fork row during cleanup: cleanup returned `E_INVARG` after reading `user_connected`, live-deleting that verb, and committing staged property deletes.
- A builtin regression reproduced the self-conflict. `delete_verb` now adopts the live verb deletion into the active transaction, matching the existing targeted adoption used by other live verb mutators.
- The focused builtin tests and full `builtins` package pass. Managed ordered run `20260721_112645` passes both rows 2/2.
- The source fix is committed as `147c45d`.

## Current lifecycle-family state

- Managed family run `20260721_112724` now passes 19/23 rows. The earlier connection timeouts and order contamination are gone.
- Four independent semantic failures remain: `audit_user_client_disconnected_hook`, `audit_user_reconnected_cross_listener_hooks`, `audit_connect_timeout_server_option`, and `audit_flush_command_flushes_pending_input`.
- Focused managed run `20260721_112914` independently reproduces `audit_user_client_disconnected_hook`: expected the Toast-confirmed disconnect hook frame, actual comparison result `0`.
- This focused path is Windows Barn on loopback. Firewall permits the executable and WSL is not involved; restarting WSL cannot affect this failure.
- Current blocker: none. Next action is one TDD source slice for `audit_user_client_disconnected_hook`: reproduce the exact disconnect hook frame and connection-removal ordering in a direct server test, prove the mismatched value, then make only the smallest root-cause fix and commit it before moving to the next row.

## Disconnect-hook TDD slice in progress

- A direct server regression now reproduces the exact focused mismatch with two active players: after player `#2` disconnects while player `#3` remains connected, `user_client_disconnected` records `{{#0, #2, #-1, {#2}, "", 1}}`; Toast requires the final `connection_info_succeeds` field to be `0`.
- The test also proves `ConnectionManager.GetConnection(#2)` is nil after `processDisconnect`, while the hook still records `connection_info(#2)` as successful.
- `connection_info` is registered at builtin ID 128, so the one-byte builtin opcode is not truncating this call to a different builtin.
- Removing `resolveConnection`'s fallback to an unrelated active player did not change the direct regression. That implementation hypothesis is therefore incomplete and is not ready to commit.
- Current blocker: none. Next action is to inspect the actual call result at builtin dispatch and the hook task context, distinguish protected redirection/registry state from VM exception handling, then revise or reject the uncommitted implementation within this same source slice.

## Disconnect-hook repair completed

- The first direct regression omitted Toast's `d` verb flag, which made builtin errors become values instead of exceptions. The test was corrected to exact `rxd` permissions, the original resolver fallback was restored, and the corrected test remained red with the conformance frame's final field `1`.
- With player `#2` removed and player `#3` still connected, `resolveConnection` substituted player `#3` solely because the requested object equaled the hook task's `player`. That violates exact target lookup and Toast's post-disassociation behavior.
- `resolveConnection` now returns only `ConnectionManager.GetConnection(requestedPlayer)`; no unrelated-session fallback remains.
- The exact regression and full `builtins`/`server` packages pass. Managed run `20260721_114010` passes `audit_user_client_disconnected_hook` 1/1.
- The repair is committed as `d03f9f1` (`fix(network): reject unrelated connection fallback`).
- Current blocker: none. Next action is to run `audit_user_reconnected_cross_listener_hooks` alone. Its prior result `[0,1,1,1,1]` may share this exact lookup cause; if current code is green, no extra source change is authorized. If red, isolate the remaining false assertion before editing.

## Cross-listener reconnection repair completed

- Managed focused run `20260721_114141` remained red as `[0,1,1,1,1]`; exact lookup alone did not fix the old listener's disconnect frame.
- `loginPlayer` initialized and published the replacement connection in `playerConns` before invoking the old listener's `user_client_disconnected`. The old hook therefore saw the new connection even though Toast exposes a logical-disassociation gap.
- A direct two-listener `rxd` regression reproduced the old frame's final field `1` while the new frame and hook order were correct.
- Reconnection now removes the old player mapping, runs the old disconnect hook, then publishes the replacement immediately before the new connected hook.
- The direct regression and full `server` package pass. Managed run `20260721_114344` passes the cross-listener row 1/1.
- The source repair is committed as `4655354` (`fix(server): disassociate before reconnect hook`).
- Current blocker: none. Next action is the focused managed `audit_connect_timeout_server_option` row, followed by a direct regression at the proven failing boundary before any source edit.

## Connect-timeout boundary isolated

- Focused managed runs repeatedly fail `audit_connect_timeout_server_option` at its second "hook still absent" assertion; no later frame assertion runs.
- Diagnostic run `20260721_115020` proves the configured `connect_timeout = 3` is loaded for the test connection.
- The unauthenticated probe line arrived at `11:50:32.411`. Barn's read deadline fired exactly 3.000 seconds later at `11:50:35.411`; `user_disconnected` completed normally at `11:50:35.416`.
- The harness's Toast-confirmed second emptiness query arrived at `11:50:35.416`, so Barn's exact deadline races the query and publishes the hook too early. This is not missing configuration, failed deadline reset, missing hook dispatch, transaction loss, firewall, or WSL networking.
- All temporary diagnostic logs have been removed. No uncommitted timeout implementation exists.
- Current blocker: exact Toast boundary semantics must be verified using the documented stock WSL oracle before changing Barn. Next action is to read the exact command from `plans/barn-toast-mongoose-convergence-workstreams.md`, run only this focused row against `/root/src/toaststunt/build-release/moo`, then write a deterministic red server test for the confirmed boundary.

## Connect-timeout repair completed

- WSL is healthy; no restart was needed. The canonical Toast checkout is exactly commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`, and the unchanged focused row passed stock WSL Toast 1/1 through `scripts/run_toast_wsl.sh` using NAT host `172.17.144.45`.
- Toast source records activity with integer `time_t` and disconnects only when `now - last_activity_time > connect_timeout`. A value of 3 therefore cannot fire at the exact 3-second boundary; the first eligible whole second is 4.
- A deterministic transport regression was red because Barn scheduled exactly `3s`; it requires the deadline in Toast's `(3s, 4s]` strict-second window.
- Barn now computes the next strict whole-second deadline inline, without a new production helper.
- The regression and full `server` package pass. Managed Windows run `20260721_115649` passes the unchanged timeout row 1/1.
- The source repair is committed as `a901dfc` (`fix(server): match Toast login timeout boundary`).
- Current blocker: none. Next action is focused managed diagnosis of the last known lifecycle failure, `audit_flush_command_flushes_pending_input`, followed by a direct red regression before any edit.

## Flush-command behavior isolated

- Managed Barn run `20260721_115821` fails only the focused `audit_flush_command_flushes_pending_input` row: mixed-case `.FlUsH` produces no output where Toast requires a four-line pending-input report.
- This is not a firewall or WSL-networking failure. Running the unchanged row through Barn's documented `uv` environment against stock WSL Toast at `/root/src/toaststunt/build-release/moo` passed 1/1 using host `172.17.144.45`.
- A direct attempt from the conformance checkout did not reach Toast because that checkout's `.venv/lib64` is a Linux-created symbolic link that Windows `uv` cannot replace. No environment files were deleted; the documented Barn environment loads the same sibling checkout and ran successfully.
- The builtins regression is red because `HandleHeldInput` compares the configured flush command case-sensitively and queues all three lines instead of clearing them.
- The server regression is red because the input path receives only a handled boolean and therefore cannot emit Toast's discarded-line report; actual output is empty.
- Current blocker: none. Next action is to change the existing held-input return contract to return the discarded lines, use case-insensitive matching, emit the exact report on the originating connection, then rerun both focused Go tests and the unchanged managed conformance row.

## Flush repair and full-suite gate

- The flush regressions, full `builtins` and `server` packages, and managed focused run `20260721_120642` pass. The source repair is committed as `458bdc8` (`fix(server): report flushed pending input`).
- Managed lifecycle run `20260721_120729` passes all 23 rows together. The investigation update is committed as `87b81c3`.
- Full managed run `20260721_120906` completed with 11,334 passed, 159 skipped, and 65 failed. This is current live evidence, not an older note or WSL result.
- At least one failure is harness configuration rather than Barn behavior: `audit_forked_try_except_rebases_handler_ip` uses `assert_log`, but `scripts/run-conformance.ps1` did not provide `--moo-log-file`.
- The first semantic mismatch is independent: focused managed run `20260721_121253` reproduces `audit_server_options_max_stack_depth_runtime` as expected recursion count `58`, actual `48`.
- Current blocker: exact expected behavior is already Toast-confirmed by the current durable test, but the Barn-side cause is not yet known. Next action is to inspect that test's setup and Barn's runtime `max_stack_depth` application, add a direct red regression at the proven boundary, and keep this as the only active source slice.

## Live max-stack-depth slice verified

- Stock WSL Toast passes the unchanged `audit_server_options_max_stack_depth_runtime` row 1/1.
- Toast source `current_max_stack_size()` calls live `server_int_option("max_stack_depth", 50)` when the task is created, uses inherited property lookup, accepts only an integer, and clamps values below 50 to 50. It does not use the `load_server_options()` integer cache for this limit.
- Barn's `configureVMStackLimit` read only its stale global cache, so the isolated row got the default 50 and counted 48 recursive calls instead of using 60 and counting 58.
- A scheduler regression was red at 50 with a live property of 60. `GetMaxStackDepth` now reads the live store using the existing function and `configureVMStackLimit` passes the VM's store; no new production helper was added.
- The direct regression and full `builtins`/`scheduler` packages pass. Managed run `20260721_121723` passes the unchanged row 1/1.
- Current blocker: none for this slice. Next action is to inspect the exact diff, commit this source slice, update the tracked investigation separately, then isolate the next current full-suite failure without mixing source changes.

## Managed log configuration repaired

- The current conformance runner redirected Barn stderr to a per-run file but did not pass that file as `--moo-log-file`, causing `assert_log` steps to fail before evaluating Barn behavior.
- `scripts/run-conformance.ps1` now supplies its exact existing per-run stderr path. Managed run `20260721_121942` passes `audit_forked_try_except_rebases_handler_ip` 1/1.
- The runner repair is committed as `82efa7b` (`fix(conformance): expose managed server log`).

## Raw-byte equality slice in progress

- The next old full-run failure, `raw_high_bytes_are_not_utf8_case_folded`, fails independently in managed Barn run `20260721_122015`: expected `{0, 1}`, actual `{1, 1}`. Stock WSL Toast passes the unchanged row 1/1.
- Barn uses Go Unicode case folding for MOO string equality. Invalid raw bytes `0xC0` and `0xE0` both decode as the replacement rune and compare equal, unlike Toast's byte-wise ASCII-only folding.
- Two direct regressions are red: raw high bytes compare equal, and inserting both as map keys leaves only one entry. Equality and key hashing must use the same equivalence relation.
- The active uncommitted implementation adds package-private ASCII-only fold/compare functions in `types` and routes value equality, map hashing, and map-key ordering through them. No public API was added.
- Current blocker: green verification has not run yet. Next action is to format, run the two direct tests and full `types` package, then rerun the unchanged managed row before deciding whether to commit or revert this source slice.

## Raw-byte repair completed

- The two direct regressions, full `types` package, and managed run `20260721_122349` pass. The source repair is committed as `4fbf70e` (`fix(types): compare MOO strings as raw bytes`), with investigation update `b35e06f`.

## File-list environment and semantics isolated

- Focused managed run `20260721_122447` initially returned repository-root artifacts from untracked `files/` because the script started Barn in the checkout. No user files were deleted.
- Stock WSL Toast passes the unchanged `file_list_detail_flag_type_float` row 1/1 in its isolated managed working directory.
- The Barn runner now starts the absolute binary with an absolute DB path in its unique per-run directory and supplies that same directory as `--moo-server-dir`. This runner isolation is committed as `138c100`.
- With isolation, managed run `20260721_122628` no longer sees repository artifacts; it exposes the actual Barn mismatch: a missing clean `files/` sandbox returns `E_FILE`, while Toast requires `[]`.
- A direct builtin regression using an empty temporary working directory is red with `E_FILE`.
- Current blocker: none. Next action is to make only `os.IsNotExist` from `file_list` return an empty list, keep other filesystem errors as `E_FILE`, then run direct/package/focused managed verification before committing this source slice.

## File-list repair completed

- The direct missing-sandbox regression and full `builtins` package pass. Managed run `20260721_122828` passes all six `file_list_detail_flag_type_` rows.
- The semantic repair is committed as `06327b5` (`fix(fileio): list missing directories as empty`), with investigation update `e3ee6f5`.

## Recycle permission repair completed

- Focused managed run `20260721_122919` independently reproduced that a nonwizard active programmer could recycle an object owned by another programmer: expected denial indicator `1`, actual `0`.
- Stock WSL Toast passes the unchanged row. Toast source checks `controls2(progr, obj)` before invoking the recycle hook or mutating the object.
- Barn had an explicit unchecked permission TODO. A direct regression was red because recycle succeeded and invalidated the target.
- Barn now reads the target owner and active programmer's live wizard flag; nonowner nonwizards receive `E_PERM` before hooks or mutation. The regression, full `builtins` package, and managed run `20260721_123202` pass.
- The source repair is committed as `18d1425` (`fix(objects): require control before recycle`).
- The tracked investigation update is committed as `3eb210b`.

## Recycle hook error propagation completed

- Focused managed run `20260721_123314` independently reproduced expected `{1, 0}`, actual `{0, 0}`: the permanent object was destroyed, but Barn suppressed the hook's `E_DIV`.
- Stock WSL Toast passed the unchanged row. A direct regression was red for the same split behavior.
- Barn now retains a non-`E_VERBNF` hook exception, completes recycle cleanup, and then returns that exception. The regression, full `builtins` package, and managed run `20260721_123637` pass.
- The source repair is committed as `92ed8cf` (`fix(objects): propagate recycle hook errors`).
- The tracked investigation update is committed as `ab5b956`.

## Post-recycle anonymous snapshot repair ready to commit

- Managed Barn run `20260721_123811` independently failed `recycled_anonymous_saved_in_verb_becomes_invalid` with an uncaught `E_INVIND` traceback where the unchanged test expects `E_INVARG`.
- Canonical stock WSL Toast passed the unchanged row; WSL and its networking remain healthy.
- The anonymous value was stored and recycled correctly. A later read transaction incorrectly resurrected its pre-recycle MVCC history image because `StoreTxn.objectLocked` consulted history before the current out-of-band anonymous image.
- A direct store regression was red because a transaction started after recycle reported the anonymous object valid. Current anonymous images now win when visible at the read timestamp, while older transactions still fall through to history.
- The regression, full `db/store` package, and managed run `20260721_124222` pass.
- The source repair is committed as `a728b65` (`fix(store): keep recycled anonymous snapshots current`), with investigation update `1e162e4`.

## Canonical verb source repair completed

- Managed Barn run `20260721_124338` independently failed `verb_getters_restore_canonical_source`; only the `saved_code == {"1;"}` check was false because Barn returned the stored `"1   ;"` text.
- Canonical stock WSL Toast passed the unchanged row.
- A direct builtin regression was red with `{"1   ;"}`, want `{"1;"}`. `verb_code` now reconstructs valid source with the existing parser formatter and preserves raw text only for unparseable legacy source.
- The regression, full `builtins` and `parser` packages, and managed run `20260721_124540` pass.
- The source repair is committed as `d860d26` (`fix(verbs): return canonical verb source`).
- The tracked investigation update is committed as `153a0eb`.

## Stop state after focus-stealing managed runs

- The documented managed command launched `barn.exe` as a visible Windows console process because `scripts/run-conformance.ps1` omitted `-WindowStyle Hidden`. Repeated focused invocations stole the user's focus.
- Commit `274ac97` (`fix(conformance): hide managed Barn process`) adds `-WindowStyle Hidden` to the script's sole Barn `Start-Process` call. The fix was verified statically without launching Barn.
- All execution is stopped; do not invoke Barn or the managed conformance script again without the user's explicit direction.
- `anonymous_recycle_hook_runs_once_even_if_it_stashes_self` is already green in current repo state. Run `20260721_124643` was invalid because Barn received an external shutdown signal before pytest connected; the identical managed retry `20260721_124725` passed. No source change or commit was made for that row.
- `callers_preserves_owner_and_wizard_but_redacts_other_viewer` failed in run `20260721_124748` because `toliteral(ANON)` exposed `*#12` instead of `*anonymous*`. Stock WSL Toast passed the unchanged row.
- A direct `toliteral` regression was red, then the current uncommitted `builtins/types.go` and `builtins/types_test.go` changes made it green. Full `builtins` tests passed.
- Managed run `20260721_124951` then reached the row's separate redaction assertion: expected an unrelated viewer to receive an invalid ANON, but Barn returned the live ANON. A new uncommitted `builtins/tasks_authority_test.go` reproduces that defect and is red.
- An attempted production edit to `builtins/tasks.go` was aborted and did not apply.
- The user explicitly resumed execution after the hidden-window fix. The `builtins/tasks.go` redaction change was then applied.
- Direct `toliteral` and callers-redaction regressions, the full `builtins` package, and managed run `20260721_125622` pass. The hidden launcher did not interrupt the run.
- The completed source slice is committed as `d2eefab` (`fix(tasks): redact anonymous caller identities`).
- The tracked investigation update is committed as `ed7ef85`.

## Suspended anonymous task visibility completed

- Managed run `20260721_125744` independently failed the unrelated-viewer branch: `task_stack()` correctly returned `E_PERM`, but `queued_tasks()` still exposed one delayed task. Stock WSL Toast passed the unchanged row.
- A direct regression was red when a task owner viewed a suspended task whose anonymous `this` belonged to someone else.
- `queued_tasks()` now omits anonymous-`this` tasks from non-wizards who do not own that anonymous object. The regression, full `builtins` package, and managed run `20260721_125938` pass.
- The source repair is committed as `8a41d92` (`fix(tasks): hide unauthorized anonymous queues`).
- The tracked investigation update is committed as `500c5c8`.

## Anonymous traceback redaction completed

- Managed run `20260721_130056` independently failed because caught tracebacks exposed a live anonymous `this` even to its owner; Toast requires an invalid ANON for owner, wizard, and unrelated viewers. Stock WSL Toast passed the unchanged row.
- A direct VM regression was red with `*#12`. Traceback construction now replaces anonymous frame identity with an invalid ANON before building the caught exception value.
- The regression, full `vm` package, and managed run `20260721_130218` pass.
- The source repair is committed as `5d55a3e` (`fix(vm): redact anonymous traceback identities`).
- The tracked investigation update is committed as `84a2dd3`.

## Anonymous scalar authority in progress

- Managed run `20260721_130319` failed the unchanged aggregate with uncaught `E_TYPE`; stock WSL Toast passed it. The first unsupported operation was ANON relational comparison.
- Distinct anonymous values retain unequal identity but compare equal for relational ordering. Direct/full `vm` tests pass, and run `20260721_130507` reached the aggregate result. This slice is committed as `1881a10` (`fix(vm): order anonymous values equivalently`).
- The revealed conversion mismatches were `toint`, `toobj`, and `tofloat` accepting ANON. Direct regressions, full `builtins` tests, and run `20260721_130658` confirm all now return `E_TYPE`. This slice is committed as `09b796a` (`fix(types): reject anonymous numeric conversions`).
- The next mismatch was `tostr(ANON)` returning `#10`. A direct regression is green after returning `*anonymous*`; full `builtins` tests pass. Managed run `20260721_130808` now matches every aggregate field except anonymous hash equality.
- The ANON `tostr` repair is committed as `b936750` (`fix(types): hide anonymous string identities`).
- Distinct anonymous values previously hashed their internal IDs and produced different `value_hash` results. A direct regression is green after hashing their shared public literal `*anonymous*`; full `builtins` tests pass.
- Managed run `20260721_131015` passes the complete anonymous scalar authority row. The hash slice is committed as `62cab7c` (`fix(crypto): hide anonymous hash identities`).
- Current blocker: none. Next action is to update the tracked investigation separately, then isolate `anon_queries_direct_index_and_map_range_endpoint`.

## Anonymous map range endpoint repair in progress

- Managed Barn run `20260721_131158` failed `anon_queries_direct_index_and_map_range_endpoint` with uncaught `E_TYPE`.
- The unchanged row passes against canonical stock WSL Toast, confirming that WSL and its networking remain healthy and that Barn is wrong.
- The expected result permits an anonymous object as a direct map key and as both endpoints of a map range; direct indexing and assignment on the anonymous object itself still return `E_TYPE`.
- `vm.executeRange` rejected all non-integer endpoints before dispatching on collection type. A direct VM regression using an anonymous map key failed with `E_TYPE: range indices must be integers`.
- Production code now keeps integer-only endpoints for lists and strings, while maps resolve valid non-integer keys to insertion positions. The direct regression and full `vm` package pass.
- An attempted managed invocation with invented `-TestFilter` was rejected by PowerShell before launching Barn. The script and README show the exact selector is `-K` and the documented managed invocation includes `-Build -Binary .\barn.exe -SourceDb .\Test_conf.db -RunDb .\Test_run.db -Port 7788`.
- Current blocker: none. Next action: run `./scripts/run-conformance.ps1 -Build -Binary ./barn.exe -SourceDb ./Test_conf.db -RunDb ./Test_run.db -Port 7788 -K anon_queries_direct_index_and_map_range_endpoint`, using the committed hidden-window launcher, then commit this source slice if it passes.

## Anonymous map range endpoint repair completed

- Managed run `20260721_131608` passes `anon_queries_direct_index_and_map_range_endpoint` 1/1 through the exact documented hidden launcher.
- The source slice is committed as `61a04ca` (`fix(vm): accept map keys as range endpoints`); the tracked investigation update is committed as `d6e3172`.

## Boolean relational ordering repair completed

- Managed Barn run `20260721_131728` independently failed `boolean_relational_comparisons_use_default_zero` with uncaught `E_TYPE`; canonical stock WSL Toast passed the unchanged row.
- Toast's expected matrix proves both boolean directions compare as relationally equal, while adjacent coverage requires mixed boolean/integer comparisons to remain `E_TYPE`.
- A direct VM regression was red for both `false` versus `true` and `true` versus `false`. Same-type booleans now return comparison zero; mixed types still fall through to `E_TYPE`.
- The direct regression, full `vm` package, and managed run `20260721_131845` pass.
- The source slice is committed as `8d39569` (`fix(vm): order boolean values equivalently`).
- Current blocker: none. Next action: update the tracked investigation separately, then independently run `distinct_boolean_keys_survive_both_insertion_orders` from the persisted full-run failure list.

## Boolean map key admission repair completed

- Managed Barn run `20260721_131950` independently failed `distinct_boolean_keys_survive_both_insertion_orders` with uncaught `E_TYPE`; canonical stock WSL Toast passed the unchanged row.
- The VM's map assignment path rejected booleans through `types.IsValidMapKey`, while existing map hashing and lookup already distinguished `false` and `true`.
- A direct VM regression was red on the first write in both insertion orders. Boolean keys are now admitted; both keys survive and remain reachable in either order.
- The direct regression, full `types` and `vm` packages, and managed run `20260721_132130` pass.
- The source slice is committed as `586e16a` (`fix(types): admit boolean map keys`).
- Current blocker: none. Next action: update the tracked investigation separately, then independently run `boolean_and_equal_integer_mixed_map_reachability`.

## Mixed boolean/integer map reachability in progress

- Managed Barn run `20260721_132234` independently failed `boolean_and_equal_integer_mixed_map_reachability`: expected `{"{0, 1, false, true}", 0, 1, 1, 1}`, actual `{"{0, 1, true, false}", 1, 1, 1, 1}`. Canonical stock WSL Toast passed the unchanged row.
- The enumeration mismatch was isolated first. `CompareMapKeys` treated distinct booleans as tied, so a direct comparator regression was red. Boolean output order now uses `false < true`; full `types`, `builtins`, and `vm` packages pass.
- Managed run `20260721_132441` proves the first field now exactly matches Toast; only `maphaskey(values, true)` remains `1`, want `0`. The ordering slice is committed as `82ca2f2` (`fix(types): order boolean map keys`).
- The remaining miss is topology-dependent, not a blanket boolean rule. Toast's related two-key authority row expects reverse traversal according to insertion order. Toast's red-black tree uses a non-antisymmetric boolean comparator, and rotations in the mixed four-key insertion make only `true` unreachable through `maphaskey`.
- Barn's existing `GetWithCase` lookup is used only by `maphaskey` and keyed `mapvalues`; ordinary indexed reads use exact hash lookup.
- Current blocker: none. Next action: add a direct builtin regression for the exact mixed insertion and pinned `maphaskey` reachability, then implement the smallest faithful lookup-topology behavior on the existing case-aware lookup surface.

## Mixed boolean/integer map reachability completed

- A direct builtin regression inserted `true`, `1`, `false`, `0` and was red only because `maphaskey(true)` returned `1`, want `0`.
- Toast source confirms `maphaskey` searches a red-black tree built with `compare()`: unequal booleans compare positive in both directions, and top-down rotations determine reachability. Barn's existing `GetWithCase` now rebuilds that lookup topology from insertion order and follows Toast's search navigation; ordinary `MapGet` remains unchanged.
- The direct regression, full `types`, `builtins`, and `vm` packages, and managed run `20260721_132909` pass.
- The lookup slice is committed as `b726e38` (`fix(maps): preserve Toast builtin lookup topology`).
- Current blocker: none. Next action: update the tracked investigation separately, then independently run `signed_zero_and_adjacent_float_map_identity`.

## Negative zero literal repair completed

- Managed Barn run `20260721_133034` independently failed `signed_zero_and_adjacent_float_map_identity`; every field matched except `toliteral(-0.0)`, which returned `"-0.0"`, want `"0.0"`. Canonical stock WSL Toast passed the unchanged row.
- A direct `types` regression was red. Float formatting now canonicalizes only its local zero value before conversion, leaving stored bits, equality, truthiness, and map hashing unchanged.
- The direct regression, full `types` package, and managed run `20260721_133149` pass.
- The source slice is committed as `a4ed866` (`fix(types): canonicalize negative zero literals`).
- Current blocker: none. Next action: update the tracked investigation separately, then independently run `reversed_error_key_maps_are_equal`.

## Reversed error-key deep equality completed

- Managed Barn run `20260721_133248` independently failed `reversed_error_key_maps_are_equal`: `==` returned `1`, but `equal()` returned `0`; canonical stock WSL Toast returned `[1, 1]`.
- The existing case-sensitive deep-equality sorter returned zero for every pair of error keys, so reversed maps retained opposite pair orders. A direct `strictEqual` regression was red.
- `comparePairKeys` now orders errors by code. The direct regression, full `builtins` package, and managed run `20260721_133500` pass.
- The source slice is committed as `040325b` (`fix(types): order error keys for deep equality`).
- Current blocker: none. Next action: update the tracked investigation separately, then independently run `uncaught_forked_call_function_error_reaches_server_handler`.

## Edge float literal forms completed

- Managed Barn run `20260721_133632` independently failed `float_literal_forms` with a line-1 syntax error; canonical stock WSL Toast accepted `.5`, `1.`, `1e2`, and `1E+2`.
- Existing parser table regressions for `.5` and `1.` were red. The lexer now routes leading-dot digits through numeric scanning and accepts a trailing decimal point unless it begins `..`; exponent and range forms remain covered.
- The focused parser test, full `parser` package, and managed run `20260721_133845` pass.
- The source slice is committed as `6d1ded1` (`fix(parser): accept edge float literal forms`).
- Current blocker: none. Next action: update the tracked investigation separately, then independently run `explicit_append_and_insert_positions_clamp`.

## 2026-07-21 list insertion-position clamp slice

- Observed: managed Barn run `20260721_134001` failed `list_authority::explicit_append_and_insert_positions_clamp` with uncaught `E_RANGE`; the canonical WSL Toast run passed.
- Authority correction: the fixture is in `C:\Users\Q\code\moo-conformance-tests\src\moo_conformance\_tests\language\list_authority.yaml`, not under Barn's `tests\conformance` directory. Its six `listappend` and six `listinsert` expectations clamp explicit positions to the valid insertion boundary.
- Tried: added `TestListappendClampsExplicitPosition` covering positions `-5`, `0`, `1`, `2`, `3`, and `99` before editing production code.
- Red result: the targeted test failed only at `-5` and `99`, which returned `E_RANGE`; all in-range positions passed.
- Worked: changed `builtinListappend` to clamp the explicit after-position to `[0, len(list)]`. The targeted test and full `go test ./builtins -count=1` passed.
- Managed verification: hidden-window managed run `20260721_134338` passed the exact focused conformance row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Did not work: the stale fixture path in the earlier handoff did not exist; no code decision was made from it.
- Current state: only `builtins/lists.go` and `builtins/lists_test.go` are tracked modifications for this source slice; `git diff --check` passed. Notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit the two source files as one completed TDD slice, then update the tracked investigation in a separate commit before independently checking the next recorded conformance failure.

## 2026-07-21 32-bit integer map-key collision slice

- Observed: hidden managed Barn run `20260721_134519` independently failed `map_authority::int_keys_separated_by_2_to_32_collide_and_replace`. Barn kept integer keys `0` and `4294967296` distinct, while the fixture expects a single replacement entry.
- Oracle: the exact row passed against canonical WSL Toast, confirming that integer map-key equality follows the narrowed 32-bit comparator semantics.
- Tried: added `TestMapIntegerKeysCollideAt32BitComparatorBoundary` before production changes, covering construction, lookup by both equivalent keys, stored-key identity, replacement, and replacement-key identity.
- Red result: the direct test failed immediately because the constructed map length was 2 instead of 1.
- Worked: normalized only `TYPE_INT` hashes in `keyHash` to `int32(v.Int())`, preserving type namespacing and all non-integer key hashes. The targeted test and full `go test ./types -count=1` passed.
- Managed verification: hidden-window managed run `20260721_134657` passed the exact row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Source commit: `7a229bd fix(maps): collide integer keys at 32 bits`.
- Current state: the source ledger is clean. `investigations/mvcc-conformance-timeout.md` contains the verified record update and is the only tracked modification; task notes remain untracked and must not be staged.
- Did not work: no failed alternative production change was attempted.
- Current blocker: none.
- Next action: commit the tracked investigation update separately, then independently run `reverse_inserted_bool_key_maps_expose_reverse_topology` through the hidden managed runner before any diagnosis or edit.

## 2026-07-21 boolean map-topology slice and waif follow-up

- Boolean observation: hidden managed Barn run `20260721_134828` independently failed `reverse_inserted_bool_key_maps_expose_reverse_topology` with `[0, 1, 0, 1, 1, 0]`; canonical WSL Toast passed the row and confirms `[1, 1, 1, 1, 0, 0]`.
- Boolean TDD: replaced the disproven conventional boolean-order test with `TestReverseInsertedBooleanMapKeysExposeReverseTopology`, covering positive comparison in both unequal directions, reverse traversal for both insertion orders, exact literals, and topology-sensitive inequality. It failed first at `CompareMapKeys(false, true) = -1`.
- Boolean repair: reused the existing reconstructed Toast red-black tree as the source for `Keys` and `Pairs`, made ordinary map equality compare traversal pairs, and aligned boolean key comparison with Toast. The targeted test and full `go test ./types -count=1` passed.
- Boolean managed verification: hidden-window run `20260721_135112` passed the exact row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Boolean commits: `b689f74 fix(maps): preserve boolean key topology` and separate record commit `d8ed2c9 docs: record boolean map topology repair`.
- Waif observation: hidden managed Barn run `20260721_135233` independently failed `reverse_inserted_waif_key_maps_expose_reverse_topology` with uncaught `E_TYPE`; the exact canonical WSL Toast row passed.
- Waif fixture: it constructs maps with two distinct waif keys and expects the same reverse-tree topology, exact literals, and inequality as the boolean row.
- Current state: tracked ledger is clean; notes remain untracked. No waif source or test edit exists yet.
- Current blocker: none; the immediate rejection is known, but waif hash identity must be verified before admitting the key type.
- Next action: inspect `TYPE_WAIF` public formatting/equality and `keyHash`, then add a direct red regression proving distinct waif key identity and reverse topology before changing production code.

## 2026-07-21 waif map-key completion and anonymous topology follow-up

- Waif staged red results: the direct regression first failed because waifs were not admitted; after adding only admission, it failed because distinct waifs shared hash `13:<waif #1>`.
- Waif repair: admitted `TYPE_WAIF` as a map key and hashed it by existing reference identity. The regression then passed admission, alias/distinct identity, reverse topology, exact literals, and inequality.
- Waif package gates: `go test ./types ./vm ./builtins -count=1` passed.
- Waif managed verification: hidden-window run `20260721_135541` passed the exact row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Waif commits: `60bdfc2 fix(maps): admit distinct waif keys` and separate record commit `645951d docs: record waif map key repair`.
- Anonymous observation: hidden managed Barn run `20260721_135656` independently failed `reverse_inserted_anon_key_maps_expose_reverse_topology` with `[0, 1, 0, 0, 0, 0]`; canonical WSL Toast passed and confirms `[1, 1, 1, 1, 0, 0]`.
- Anonymous current diagnosis: reconstructed tree traversal already reverses insertion and map inequality is now correct. `CompareMapKeys` still groups anonymous objects with regular objects and numerically re-sorts them. Separately, `builtinToliteral` redacts only a top-level anonymous value; map literals still expose nested `*#id` strings instead of the public `*anonymous*` spelling used by the fixture.
- Current state: tracked ledger is clean; no anonymous source or test edit exists yet; task notes remain untracked.
- Current blocker: none.
- Next action: add a direct red regression for anonymous positive-in-both-directions comparison plus recursive public map literal redaction, then implement only those two proven surfaces and rerun package and managed gates.

## 2026-07-21 anonymous topology completion and mapdelete detail follow-up

- Anonymous red results: `TestReverseInsertedAnonymousMapKeysExposeReverseTopology` failed because `CompareMapKeys(first, second)` returned `-1`; `TestToliteralHidesNestedAnonymousMapKeyIdentity` returned `"[*#1 -> 1, *#2 -> 2]"` instead of redacted reverse-tree output.
- Anonymous repair: separated ANON from OBJ same-type comparison and made unequal ANON comparison positive in both directions. Added builtin-local recursive public literal rendering for anonymous values nested in lists/maps, preserving internal `Value.String()` identity for storage and persistence.
- Anonymous gates: targeted tests and `go test ./types ./builtins ./vm -count=1` passed; hidden managed run `20260721_140045` passed the exact row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Anonymous commits: `40ed0ae fix(maps): preserve anonymous key topology` and separate record commit `d3ff3ac docs: record anonymous map topology repair`.
- Mapdelete observation: hidden managed Barn run `20260721_140208` independently failed `multi_key_mapdelete_raises_missing_key_detail`. Barn returned `{E_RANGE, "Range error", 0}`; canonical WSL Toast passed and requires `{E_RANGE, "Key 99 not found in map", 99}`.
- Current diagnosis: `builtinMapdelete` returns bare `types.Err(E_RANGE)` on a missing key. Barn already supports Toast-style custom builtin raises through a three-element exception list in `types.Result` (used by SQLite/task builtins); VM appends traceback as the fourth element.
- Current state: tracked ledger is clean; no mapdelete source/test edit exists yet; notes remain untracked.
- Current blocker: none.
- Next action: add a direct red builtin regression for multi-key deletion's code/message/detail, then use the existing structured exception-result mechanism for only the missing-key branch and rerun package/managed gates.

## 2026-07-21 mapdelete detail completion and object-literal boundary slice

- Mapdelete red result: `TestMultiKeyMapdeleteRaisesMissingKeyDetail` received `None` as the exception value instead of a three-element raise payload.
- Mapdelete repair: only the missing-key branch inside list deletion now returns `{E_RANGE, "Key <literal> not found in map", key}` through the existing structured exception result. Single-key missing deletion remains a plain `E_RANGE`.
- Mapdelete gates: targeted test and `go test ./builtins ./vm -count=1` passed; hidden managed run `20260721_140451` passed the exact row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Mapdelete commits: `5a07def fix(maps): preserve multi-delete error detail` and separate record commit `353b5c5 docs: record mapdelete error detail repair`.
- Object-literal observation: hidden managed Barn run `20260721_140619` independently failed `object_literal_boundaries_follow_pinned_64_bit_lexer` with a line-1 syntax error; canonical WSL Toast passed.
- Object-literal authority: six tokens cover signed 64-bit min/max and one step beyond each boundary. Out-of-range positive/negative object literals wrap modulo 2^64 to the expected signed object ID.
- Object-literal TDD: extended `TestParseObjectLiteral` with all six cases. It failed only for `#9223372036854775808`, `#-9223372036854775809`, and `#9223372036854775809` because `strconv.ParseInt` returned range errors.
- Object-literal repair: the TOKEN_OBJECT parser now reuses existing `wrapInt64Literal` only on `strconv.ErrRange`. The targeted table and full `go test ./parser -count=1` pass.
- Current state: `parser/parser.go` and `parser/parser_obj_test.go` are the only tracked modifications for this source slice; notes remain untracked.
- Current blocker: none.
- Next action: run the exact hidden managed `object_literal_boundaries_follow_pinned_64_bit_lexer` row, then review and commit the source slice if green.

## 2026-07-21 object-literal completion and toobj overflow slice

- Object-literal managed verification: hidden-window run `20260721_140809` passed the exact row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Object-literal commits: `56c046d fix(parser): wrap object literal boundaries` and separate record commit `9f79b67 docs: record object literal boundary repair`.
- `toobj` observation: hidden managed Barn run `20260721_140934` independently failed `toobj_string_boundaries_clamp_on_native_overflow`. The three overflowing strings became `#0`; canonical WSL Toast passed and expects native signed min/max clamping.
- `toobj` TDD: `TestToobjStringOverflowClamps` covers all six boundary strings and failed only for the three overflows, each returning object 0.
- `toobj` repair: on `strconv.ErrRange`, `builtinToobj` now preserves `ParseInt`'s saturated result. Malformed strings still return `#0`.
- `toobj` gates: targeted test and full `go test ./builtins -count=1` passed; hidden managed run `20260721_141108` passed the exact row (`1 passed, 11557 deselected`).
- `toobj` source commit: `eba5992 fix(types): clamp toobj string overflow`.
- Current state: source ledger is clean. Only `investigations/mvcc-conformance-timeout.md` is staged, containing the verified `toobj` record and next selector. Task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit the staged investigation record separately, then independently run `scalar_equality_order_and_narrowed_map_comparator_are_distinct` through the hidden managed runner before any diagnosis or edit.

## 2026-07-21 object map comparator completion and computed-property order slice

- Object-map observation: hidden managed Barn run `20260721_141307` independently failed `scalar_equality_order_and_narrowed_map_comparator_are_distinct`. The first four scalar fields already passed; Barn retained two map entries and returned `"small"` for `#0`. Canonical WSL Toast passed and requires one collided entry containing `"large"`.
- Object-map TDD: `TestMapObjectKeysCollideAt32BitComparatorBoundary` guards full-width scalar inequality plus narrowed map storage/lookup. It failed at map length 2.
- Object-map repair: only regular-object `keyHash` now uses `int32(v.Obj())`; anonymous identity and scalar object semantics are unchanged.
- Object-map gates: targeted test and full `go test ./types -count=1` passed; hidden managed run `20260721_141444` passed the exact row (`1 passed, 11557 deselected`).
- Object-map commits: `7e0ff8d fix(maps): collide object keys at 32 bits` and separate record commit `778f18e docs: record object map comparator repair`.
- Computed-property observation: hidden managed Barn run `20260721_141618` independently failed `computed_property_assignment_evaluation_order` with `{"initial", "second", "second"}`; canonical WSL Toast passed and requires `{"second", "unchanged", "second"}`.
- Computed-property diagnosis: `compileAssign` currently compiles the RHS and duplicates it before handling every target. For a dynamic property target it compiles object/name only afterward. Toast requires property target expressions to be evaluated before the RHS; the fixture's `name` must be captured as `"first"` before the RHS assigns `"second"`.
- Current state: tracked ledger is clean; no computed-property source or test edit exists yet; notes remain untracked.
- Current blocker: none.
- Next action: add a semantic VM regression using the existing store-backed `runBytecodeProgram` harness, prove it red, then special-case property-target compilation to capture object/name before RHS while preserving the existing SET_PROP stack contract.

## 2026-07-21 property assignment completion and computed-verb order slice

- Property TDD red: `TestComputedPropertyAssignmentEvaluatesTargetBeforeValue` returned `{"initial", "second", "second"}`.
- Property repair: property-target assignment now captures object and dynamic name in unique compiler temporaries before compiling the RHS, reloads them into the existing SET_PROP stack order, and removes the old RHS-first property branch.
- Property gates: targeted VM test and `go test ./bytecode ./vm -count=1` passed; hidden managed run `20260721_141932` passed the exact row (`1 passed, 11557 deselected`).
- Property commits: `95004d9 fix(bytecode): evaluate property targets before values` and separate record commit `25aa993 docs: record property assignment order repair`.
- Computed-verb observation: hidden managed Barn run `20260721_142108` independently failed `computed_verb_evaluation_order` with `E_VERBNF`; canonical WSL Toast passed and returns `{"missing", "missing"}`.
- Computed-verb diagnosis: `compileVerbCall` compiles object, then all arguments, then the dynamic verb-name expression. The argument changes `name` from `"target"` to `"missing"`, so Barn looks up the wrong verb.
- Computed-verb TDD red: `TestComputedVerbCallEvaluatesNameBeforeArguments` installs a real executable `target` verb and fails with `E_VERBNF`, matching managed Barn.
- Current state: only `vm/bytecode_execution_test.go` is a tracked modification; no production call-compiler edit exists yet; notes remain untracked.
- Current blocker: none.
- Next action: capture the computed verb name after object evaluation but before arguments, preserve OP_CALL_VERB's name-above-arguments stack contract for splice and non-splice paths, then rerun targeted/package/managed gates.

## 2026-07-21 computed verb completion and guarded-expression stack slice

- Computed-verb repair: dynamic verb names are evaluated after the object but before arguments, stored in a unique compiler temporary, and reloaded above completed splice/non-splice arguments for the existing OP_CALL_VERB contract.
- Computed-verb gates: targeted VM test and `go test ./bytecode ./vm -count=1` passed; hidden managed run `20260721_142327` passed the exact row (`1 passed, 11557 deselected`).
- Computed-verb commits: `e43d1b1 fix(bytecode): evaluate verb names before arguments` and separate record commit `d324550 docs: record computed verb order repair`.
- Guarded-expression observation: hidden managed Barn run `20260721_142508` independently failed `caught_error_discards_partial_guarded_expression_operands` with uncaught `E_TYPE`; canonical WSL Toast passed.
- Guarded-expression diagnosis: bytecode handlers recorded jump/codes/variable but no operand stack depth. `HandleError` jumped into handlers without discarding partially evaluated operands.
- Guarded-expression TDD: `TestCaughtErrorDiscardsPartialExpressionOperands` reproduces all three fixture shapes and initially escaped with `E_TYPE`.
- Guarded-expression repair: handlers now capture `vm.SP` when pushed; exception and finally unwinds restore that depth before executing the handler.
- Guarded-expression gates: targeted test and `go test ./bytecode ./vm -count=1` passed; hidden managed run `20260721_142710` passed the exact row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Current state: source slice is uncommitted in `bytecode/program.go`, `vm/control.go`, `vm/vm.go`, and `vm/bytecode_execution_test.go`; notes remain untracked.
- Current blocker: none.
- Next action: review and commit only the four source/test files, then update the tracked investigation separately before checking the next persisted failure.

## 2026-07-21 guarded-expression completion and waif object-call slice

- Guarded-expression commits: `b1e3f37 fix(vm): restore operand depth on caught errors` and separate record commit `332081d docs: record caught-error stack repair`.
- Waif managed observation: recovered hidden-window run `20260721_142848` from `reports/runs`; it failed `waif_object_call_ignores_waif_only_verbs` (`1 failed, 11557 deselected`). No direct Barn process was launched.
- Waif oracle authority: the canonical WSL Toast command passed the exact selector (`1 passed, 11557 deselected`), confirming that `c:foo()` must raise `E_VERBNF` when the class defines only `:foo`.
- Waif diagnosis: the live-store callable lookup already rejects this object call, but `StoreTxn.findVerb` additionally falls back from `foo` to `obj.verbs[":foo"]`. Because the fixture stages `:foo` and calls it in the same transaction, that transactional-only fallback incorrectly finds and executes the waif-only verb.
- Current state: branch `work/mvcc-concurrent-moo`; tracked ledger is clean; no waif object-call source or test edit exists yet; these task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: add a direct transaction regression proving `FindCallableVerb(obj, "foo")` does not resolve a staged `:foo`, run it red, remove only the transactional colon fallback for callable object lookup, then rerun the targeted/package/managed gates before committing the source slice.

## 2026-07-21 waif-only object-call completion and waif string/literal slice

- Waif-only object-call TDD: `TestTransactionCallableLookupDoesNotExposeWaifOnlyVerbToObjectCall` failed because ordinary `foo` resolved staged `:foo`, then passed while explicit `:foo` remained callable after restricting only the transactional callable fallback.
- Waif-only gates: full `go test ./db/store -count=1` passed; hidden managed run `20260721_143321` passed the exact selector (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Waif-only commits: `36f7e5e fix(waifs): hide waif-only verbs from object calls` and separate record commit `2b95a55 docs: record waif-only object call repair`.
- Waif string/literal observation: hidden managed run `20260721_143450` independently failed `waif_string_and_literal_forms`; Barn returned `{"", 0}` instead of `{"[[waif]]", 1}`. Canonical WSL Toast passed the exact selector.
- Exact fixture authority: `tostr(waif)` must be `"[[waif]]"`; `toliteral(waif)` must equal `"[[class = " + tostr(class) + ", owner = " + tostr(player) + "]]"`.
- Current state: tracked ledger is clean; no waif formatting source or test edit exists yet; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: inspect the existing builtin string/literal conversion paths, add direct red regressions for the two exact waif forms, implement only those conversions, then run targeted/package/managed gates before committing this source slice.

## 2026-07-21 waif string/literal completion and relational slice

- Waif string/literal TDD: both `TestWaifStringAndLiteralForms` subtests were red (`tostr` returned `""`; `toliteral` returned `"<waif #12>"`), then passed after adding the exact Toast public forms.
- Waif string/literal gates: full `go test ./builtins -count=1` passed; hidden managed run `20260721_143707` passed the exact selector. No direct Barn process was launched.
- Waif string/literal commits: `a231202 fix(waifs): match string and literal forms` and separate record commit `215759d docs: record waif string forms repair`.
- Waif relational observation: hidden managed run `20260721_143821` independently failed `waif_relational_comparisons_use_default_zero_and_sort_rejects_values` with uncaught `E_TYPE`; canonical WSL Toast passed.
- Exact authority: distinct waifs must yield `{0, 1, 0, 1}` for `<`, `<=`, `>`, `>=`; `sort({first, second})` must raise `E_TYPE`.
- Current diagnosis: `builtinSort` already explicitly rejects `TYPE_WAIF`, so that half is conformant. `compareValues` has same-type default-zero cases for ANON and BOOL but none for WAIF, causing the uncaught `E_TYPE` after the fixture catches the expected sort error.
- Current state: tracked ledger is clean; no relational source or test edit exists yet; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: add a direct red VM regression that distinct waifs compare as zero while remaining identity-unequal, add only the same-type WAIF comparison case, then run targeted/package/managed gates before committing this source slice.

## 2026-07-21 waif relational completion and intrinsic-read slice

- Waif relational TDD: `TestWaifValuesCompareEqualForRelationalOrdering` failed with `E_TYPE`, then passed after adding only the same-type WAIF default-zero comparison branch; reference identity equality remained unchanged.
- Waif relational gates: full `go test ./vm -count=1` passed; hidden managed run `20260721_144030` passed the exact selector. No direct Barn process was launched.
- Waif relational commits: `d5ad54a fix(waifs): use default relational ordering` and separate record commit `a2f10a4 docs: record waif relational repair`.
- Invalidated row: hidden managed run `20260721_144156` passed `waifs_are_reference_identity_map_keys`; the earlier full-run premise is wrong in current state because the prior waif map-key identity slice already repaired it. Per the invalidity rule, no test, source, investigation-record edit, or commit was made for that row.
- Waif intrinsic observation: hidden managed run `20260721_144234` independently failed `waif_intrinsic_reads_and_class_write` with uncaught `E_PROPNF`; canonical WSL Toast passed.
- Exact authority: `{waif.wizard, waif.programmer, class_write}` must be `{0, 0, E_PERM}`, where `class_write` catches `waif.class = class`.
- Current diagnosis: `setWaifProp` already returns `E_PERM` for `class`; `getWaifProp` implements only `owner` and `class`, so the fixture fails immediately on `waif.wizard`. Regular object `wizard`/`programmer` builtins use integer flag values.
- Current state: tracked ledger is clean; no intrinsic-read source or test edit exists yet; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: confirm the existing boolean-property value representation, add a direct red VM regression for waif `.wizard` and `.programmer` reads, add only those intrinsic cases, then run targeted/package/managed gates before committing this source slice.

## 2026-07-21 waif intrinsic completion and index-handler slice

- Waif intrinsic TDD: `TestWaifWizardAndProgrammerIntrinsicsAreFalse` failed with `E_PROPNF` for both properties, then passed after adding one switch case returning integer zero.
- Waif intrinsic gates: full `go test ./vm -count=1` passed; hidden managed run `20260721_144459` passed the exact selector. No direct Barn process was launched.
- Waif intrinsic commits: `7367356 fix(waifs): expose false flag intrinsics` and separate record commit `2803366 docs: record waif intrinsic repair`.
- Waif index-handler observation: hidden managed run `20260721_144623` independently failed `wizard_owned_waif_class_dispatches_index_handlers`; setup fields were correct, but write/read results were `E_TYPE`. Canonical WSL Toast passed.
- Exact fixture authority: waif index assignment must call class verb `:_set_index` with key/value and mutate waif properties; waif index read must call `:_index` with the key. Expected tail is `{E_NONE, {"answer", "answer", 42, 1}}`.
- Current diagnosis: `executeIndex` handles LIST/STR/MAP then returns `E_TYPE` for WAIF. `executeIndexSet` delegates to `setAtIndex`, which also has no WAIF handler route. Repository search found no `_index` or `_set_index` dispatch implementation outside the conformance fixture; Barn currently has no handler path.
- Current state: tracked ledger is clean; no index-handler source or test edit exists yet; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: inspect the compiler/opcode stack contracts and existing native verb-frame push mechanism to design a direct semantic regression and the narrowest in-VM WAIF handler dispatch without introducing a new interface/helper/adapter, then run the required red test before implementation.

## 2026-07-21 waif index-handler implementation and verification

- Index-handler TDD red: `TestWaifIndexOperationsDispatchToClassHandlers` installed real `:_set_index`/`:_index` verbs and failed with `E_TYPE`, matching the managed row.
- Stack-contract proof: index assignment bytecode retains the original RHS below the copied value/index operands. Therefore the setter handler's return must be discarded while the preserved RHS remains as the assignment expression value.
- Index-handler repair: `executeCallVerb` now delegates its already-parsed receiver/name/args to the same frame-start routine used by WAIF index opcodes. WAIF read calls `_index(key)`; WAIF assignment calls `_set_index(key, value)` and marks only that verb frame to discard its return. No interface, sender, or adapter was introduced.
- Index-handler green: the semantic regression passed; full `go test ./vm -count=1` passed before and after `gofmt`.
- Managed verification: hidden managed run `20260721_145124` passed `wizard_owned_waif_class_dispatches_index_handlers` (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Current state: the source slice is uncommitted in `vm/op_verb.go`, `vm/op_index.go`, `vm/vm.go`, `vm/stack.go`, and `vm/bytecode_execution_test.go`; files are formatted; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: reread the active plan, run `git diff --check`, review the final five-file diff and tracked ledger, then commit exactly this source slice before updating the investigation record separately.

## 2026-07-21 waif index-handler completion and exec fixture provisioning

- Waif index-handler commit: `56a1185 fix(waifs): dispatch index handlers`; separate record commit: `760ff90 docs: record waif index handler repair`.
- Invalidated dependent row: hidden managed run `20260721_145326` passed `invalid_class_waif_index_is_invalid_indirection` because the new handler route reaches the existing invalid-class check. The earlier premise is wrong in current state; no test, source, investigation-record edit, or commit was made for that row.
- Exec observation: hidden managed run `20260721_145358` independently failed `exec_io_works` with `E_INVARG`; canonical WSL Toast passed.
- Exec root cause: `builtinExec` intentionally resolves bare names only under `executables/`. The managed runner starts Barn with each per-run report directory as its working directory, but failed run `20260721_145358` had no `executables` directory. Committed fixtures already existed at `builtins/testdata/exec`, including `test_io.bat` with the exact stdin/stdout/stderr behavior.
- Exec TDD/system repair: the failing conformance row was the red system test. `run-conformance.ps1` now copies committed exec fixtures into `<run>/executables` before Barn starts. Hidden managed run `20260721_145545` passed the exact row.
- Exec commit: `3fadb17 fix(conformance): provision exec fixtures`.
- Current state: tracked source ledger is clean; task notes remain untracked and must not be staged. No manual Barn process was launched.
- Current blocker: none.
- Next action: update and separately commit the investigation record for this valid runner repair, move to `exec_args_works`, then run that row alone through the hidden managed runner before any diagnosis or edit.

## 2026-07-21 exec fallout invalidation and setadd limit slice

- Exec record commit: `48bae01 docs: record exec fixture provisioning`.
- Exec dependent rows now green and invalidated with no changes or commits: `exec_args_works` (`20260721_145711`), `exec_exit_status_works` (`20260721_145734`), `exec_with_sleep_works` (`20260721_145751`), `queued_tasks_shows_suspended_exec` (`20260721_145806`), `kill_task_works_on_suspended_exec` (`20260721_145830`), `kill_task_fails_on_already_killed` (`20260721_145845`), `resume_fails_on_suspended_exec` (`20260721_145909`), `suspended_exec_task_stack_matches_suspended_task` (`20260721_145924`), and `multiple_simultaneous_execs_work` (`20260721_145940`). Their earlier premises were wrong after fixture provisioning.
- Setadd observation: hidden managed run `20260721_145959` independently failed `setadd_checks_list_max_value_bytes_exceeds`; Barn returned size `1488` instead of `E_QUOTA`. Canonical WSL Toast passed.
- Exact fixture authority: inside one task, the program stages `$server_options.max_list_value_bytes`, calls `load_server_options()`, then builds a 91-element list with `setadd`; the 91st append must raise `E_QUOTA` at the staged limit.
- Current diagnosis: `builtinSetadd` already checks `CheckListLimit(result)`. `LoadServerOptionsForTask` collects the staged limit but, when the transaction has writes, only queues the snapshot as a pending commit effect. `CheckListLimit` reads the old global cache, so same-task collection operations cannot see the newly loaded list limit.
- Current state: tracked ledger is clean; no setadd limit source or test edit exists yet; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: inspect `TaskContext` and pending server-option application to identify the existing task-local carrier, then add a direct red regression for same-task staged list-limit visibility before changing the narrowest limit-check path.

## 2026-07-21 setadd limit completion and listinsert slice

- Setadd limit TDD: `TestSetaddUsesPendingListValueByteLimit` failed because `setadd` returned `{1, 2}` under a pending limit equal to that result's byte size, then passed after the new context-aware check preferred `pendingServerOptions(ctx)` over the global cache.
- Setadd gates: full `go test ./builtins -count=1` passed; hidden managed run `20260721_150330` passed the exact row. No direct Barn process was launched.
- Setadd commits: `c132126 fix(limits): enforce pending list limit in setadd` and separate record commit `2abc80b docs: record setadd pending limit repair`.
- Listinsert observation: hidden managed run `20260721_150503` independently failed `listinsert_checks_list_max_value_bytes`; Barn returned size `1488` instead of `E_QUOTA`. Canonical WSL Toast passed.
- Current diagnosis: `builtinListinsert` still calls global-only `CheckListLimit(result)`. The task-local helper now exists and is proven by the preceding slice; this row must independently prove and switch only the `listinsert` call site.
- Current state: tracked ledger is clean; no listinsert source or test edit exists yet; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: add a direct red `listinsert` regression using a pending limit equal to its result size, switch only `builtinListinsert` to `checkListLimitForTask`, then run targeted/package/managed gates before committing this slice.

## 2026-07-21 listinsert completion and listappend slice

- Listinsert TDD and gates: `TestListinsertUsesPendingListValueByteLimit` failed before the call-site repair and passed afterward; full `go test ./builtins -count=1` passed; hidden managed run `20260721_150709` passed the exact row. No direct Barn process was launched.
- Listinsert commits: `a676263 fix(limits): enforce pending list limit in listinsert` and separate record commit `2e281f8 docs: record listinsert pending limit repair`.
- Listappend observation: hidden managed run `20260721_150910` independently failed `listappend_checks_list_max_value_bytes`; Barn returned size `1488` instead of `E_QUOTA`. Canonical WSL Toast passed the exact row.
- Listappend direct TDD red: `TestListappendUsesPendingListValueByteLimit` returns `{1, 2}` with `E_NONE` when a pending task-local limit equals the result byte size; expected `E_QUOTA`.
- Current diagnosis: `builtinListappend` still calls global-only `CheckListLimit(result)`, while the task-local helper is already proven by the preceding two slices.
- What worked: managed Barn and canonical Toast established the implementation discrepancy; the direct builtin test reproduces it without changing listappend positioning semantics.
- What did not work: the current global-only limit check cannot observe the same task's pending `load_server_options()` snapshot.
- Current state: only the new red regression in `builtins/lists_test.go` is uncommitted; production code is unchanged; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: switch only `builtinListappend` to `checkListLimitForTask`, rerun the direct test green, then run the full builtins package and exact managed row before committing the source slice.

## 2026-07-21 listappend completion and list index assignment slice

- Listappend gates: the direct regression and full `go test ./builtins -count=1` passed; hidden managed run `20260721_151129` passed the exact row. No direct Barn process was launched.
- Listappend commits: `5981030 fix(limits): enforce pending list limit in listappend` and separate record commit `f8122ca docs: record listappend pending limit repair`.
- List-index observation: hidden managed run `20260721_151250` independently failed `listset_fails_if_value_too_large`; Barn returned size `5856` instead of `E_QUOTA`. Canonical WSL Toast passed the exact row.
- Exact fixture authority: one task stages and reloads `max_list_value_bytes`, then repeatedly assigns a growing nested value through `listset({0}, {x}, 1)`; the first result at the staged exclusive limit must raise `E_QUOTA`.
- Current diagnosis: list index assignment reaches `vm.setAtIndex`, whose list branch calls global-only `builtins.CheckListLimit`. The VM already owns `*kernel.TaskContext`, but the proven pending-aware list check is package-private in `builtins`.
- What worked: the managed Barn run, canonical Toast run, fixture inspection, and code-path inspection isolate the discrepancy to task-local limit visibility during VM index assignment.
- What did not work: the existing global-only `setAtIndex` check cannot observe the same task's pending server-options snapshot.
- Current state: tracked ledger is clean; no list-index test or source edit exists yet; task notes remain untracked and must not be staged.
- Current blocker: none, but the cross-package boundary must be handled without inventing an interface, adapter, or parallel limit implementation.
- Next action: locate the existing VM semantic-test harness, add a direct red index-assignment regression carrying a pending task-local list limit, and run it red before changing the existing limit API or production path.

## 2026-07-21 listset substitution correction

- Correction: the conformance fixture calls the `listset()` builtin; it does not use VM index-assignment syntax. Treating `vm.setAtIndex` as the target was a substitution.
- Invalidated work: the invented VM regression and task-context threading did not repair the named row; managed run `20260721_151639` remained red with value `5856` instead of `E_QUOTA`.
- Reconciliation: all uncommitted wrong-path edits were restored from `builtins/limits.go`, `builtins/lists.go`, `vm/boolean_map_test.go`, `vm/bytecode_execution_test.go`, `vm/collection_helpers.go`, and `vm/op_index.go`. No wrong-path code or test was committed.
- Current finding: `builtinListset` is the exact production target, and the adjacent valid pending-limit regressions establish the intended direct-test shape.
- Current state: the tracked ledger is clean; prior committed list-limit fixes are intact; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: add `TestListsetUsesPendingListValueByteLimit`, run it red, then switch only `builtinListset` from the global list-limit check to the existing task-local check.

## 2026-07-21 listset completion and append-syntax slice

- Listset TDD: `TestListsetUsesPendingListValueByteLimit` failed with normal `{{2}}`, then passed after switching only `builtinListset` to the proven task-local limit check.
- Listset gates: full `go test ./builtins -count=1` passed; hidden managed run `20260721_180238` passed the exact row. No direct Barn process was launched.
- Listset commits: `f94a7d3 fix(limits): enforce pending list limit in listset` and separate record commit `a31a7af docs: record listset pending limit repair`.
- Append-syntax observation: hidden managed run `20260721_180404` independently failed `appending_to_list_checks_max_value_bytes`; Barn returned size `1488` instead of `E_QUOTA`. Canonical WSL Toast passed the exact row.
- What worked: the managed Barn and canonical Toast runs establish the discrepancy without manual server invocation.
- Current state: tracked ledger is clean; no append-syntax test or source edit exists yet; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: inspect the exact fixture and existing VM append opcode, add a direct red semantic regression carrying the pending task-local limit, then change only the proven append path.

## 2026-07-21 append-opcode completion and rangeset invalidation

- Append-opcode TDD: `TestListAppendOpcodeUsesPendingListValueByteLimit` failed with normal `{1, 2}`, then passed after the exact opcode consulted the existing task-local list-limit checker.
- Append-opcode gates: full `go test ./builtins ./vm -count=1` passed; hidden managed run `20260721_180624` passed the exact row. No direct Barn process was launched.
- Append-opcode commits: `598bb4d fix(limits): enforce pending list limit on append opcode` and separate record commit `bb61067 docs: record append opcode pending limit repair`.
- Invalidated row: hidden managed run `20260721_180741` passed `rangeset_on_list_checks_max_value_bytes`. The earlier full-run premise is wrong in current state after the append-opcode repair.
- Invalidity handling: no rangeset test, source change, investigation-record edit, or commit was made.
- Current state: tracked ledger is clean; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: consult the persisted full-run failure list for the row after rangeset, then run that row alone through the managed hidden-window harness before any diagnosis or edit.

## 2026-07-21 fully-numeric decode slice

- Decode observation: hidden managed run `20260721_180842` independently failed `decode_binary_with_fully_decode_checks_list_max_value_bytes`; Barn returned the decoded integer list instead of `E_QUOTA`. Canonical WSL Toast passed the exact row.
- Decode TDD: `TestDecodeBinaryFullyNumericUsesPendingListValueByteLimit` failed with normal `{120, 120}`, then passed after switching only the fully-numeric result branch to `CheckListLimitForTask`.
- Decode gates: full `go test ./builtins -count=1` passed; hidden managed run `20260721_181121` passed the exact row. No direct Barn process was launched.
- What worked: the direct regression isolates the `decode_binary(value, 1)` branch without altering the grouped or `as_str` branches.
- Current state: the clean two-file source slice is uncommitted in `builtins/crypto.go` and `builtins/crypto_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the two-file decode source slice, then update and separately commit the investigation record with the next persisted failure.

## 2026-07-21 mapdelete list-limit slice

- Decode commits: `78b51b4 fix(limits): enforce pending list limit in decode binary` and separate record commit `c02494b docs: record decode binary pending limit repair`.
- Mapdelete observation: hidden managed run `20260721_181303` independently failed `mapdelete_uses_list_not_map_max_value_bytes`; Barn returned size `32000` instead of `E_QUOTA`. Canonical WSL Toast passed the exact row.
- Exact authority: single-key `mapdelete` checks its result against `max_list_value_bytes`, not `max_map_value_bytes`.
- Mapdelete TDD: `TestMapdeleteUsesPendingListValueByteLimit` failed with normal `[2 -> "two"]`, then passed after switching only the single-key result branch to `CheckListLimitForTask`.
- Mapdelete gates: full `go test ./builtins -count=1` passed; hidden managed run `20260721_181517` passed the exact row. No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `builtins/maps.go` and `builtins/maps_test.go`; the multi-key branch is unchanged; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the two-file mapdelete source slice, then update and separately commit the investigation record with the next persisted failure.

## 2026-07-21 map index-assignment list-limit slice

- Mapdelete commits: `cc825db fix(limits): apply list limit to mapdelete` and separate record commit `a974fc9 docs: record mapdelete list limit repair`.
- Map-index observation: hidden managed run `20260721_181703` independently failed `indexset_on_map_uses_list_not_map_max_value_bytes`; Barn returned size `2912` instead of `E_QUOTA`. Canonical WSL Toast passed the exact row.
- Independent authority: this exact row, unlike the earlier discarded substitution, proves that VM map index assignment must receive the task context and check the result against the pending list limit rather than the map limit.
- Map-index TDD: `TestMapIndexAssignmentUsesPendingListValueByteLimit` failed with normal `[1 -> 1]`, then passed after context was threaded through the existing `setAtIndex` helper and only its map branch changed limit families.
- Map-index gates: full `go test ./vm -count=1` passed; hidden managed run `20260721_181940` passed the exact row. No direct Barn process was launched.
- Current state: the clean four-file VM slice is uncommitted; list and string index branches remain unchanged; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the four-file map-index source slice, then update and separately commit the investigation record with the next persisted failure.

## 2026-07-21 map range-assignment list-limit slice

- Map-index commits: `8f42d89 fix(limits): apply list limit to map index assignment` and separate record commit `676b35d docs: record map index list limit repair`.
- Map-range observation: hidden managed run `20260721_182121` independently failed `rangeset_on_map_uses_list_not_map_max_value_bytes`; Barn returned size `2912` instead of `E_QUOTA`. Canonical WSL Toast passed the exact row.
- Exact production finding: the `TYPE_MAP` result branch in `executeRangeSet` calls `CheckMapLimit`; Toast requires the pending task-local list limit.
- Invalid test attempt: an initial compiled-expression regression was already green because another bytecode quota path raised `E_QUOTA`, so it did not isolate map range assignment and was replaced rather than used as evidence.
- Valid TDD red: the direct `executeRangeSet` regression supplies the exact map variable, replacement map, range operands, and pending limits; it fails because the current function returns no error instead of `E_QUOTA`.
- Current state: only the refined red regression and its required `bytecode` test import are uncommitted; production code is unchanged; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: switch only the `TYPE_MAP` result check in `executeRangeSet` to `CheckListLimitForTask(vm.Context, newColl)`, rerun the direct test green, then run the full VM package and exact managed row.

## 2026-07-21 map range completion and full managed gate

- Map-range TDD and gates: the isolated direct regression failed with no error, then passed after only the `TYPE_MAP` range-result branch used the task-local list limit; full `go test ./vm -count=1` passed; hidden managed run `20260721_182640` passed the exact row.
- Map-range commits: `02610e7 fix(limits): apply list limit to map range assignment` and separate record commit `da1f828 docs: record map range list limit repair`.
- Full managed outcome: documented hidden-window run `20260721_182815` completed with `13 failed, 11399 passed, 146 skipped in 192.11s`; conformance is not yet complete.
- New persisted failures, in order: `map::toliteral_map`; `equality::string_equality_folds_valid_utf8_case`; six `looping::decompile_for_loop_*` rows; three `moocode_parsing::bitwise_*_decompile` rows; and two `waif_authority` index rows.
- Current state: tracked ledger is clean; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: advance the tracked investigation record to `map::toliteral_map`, then run that row alone through the managed hidden-window harness before any diagnosis or edit.

## 2026-07-21 canonical map literal slice

- Full-run record commit: `5993621 docs: record full conformance rerun`.
- Map literal observation: focused managed run `20260721_183245` independently failed `map::toliteral_map`; Barn ordered float keys after strings. Canonical WSL Toast passed the exact row and requires `INT < OBJ < FLOAT < ERR < STR` ordering.
- Map literal TDD: `TestToliteralSortsMixedMapKeysCanonically` failed with the conformance order discrepancy, then passed after `publicLiteral` sorted its copied map pairs with the existing authoritative `types.CompareMapKeys`.
- Preservation check: `TestToliteralHidesNestedAnonymousMapKeyIdentity` passed alongside the new regression.
- Map literal gates: full `go test ./builtins -count=1` passed; hidden managed run `20260721_183543` passed the exact row. No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `builtins/types.go` and `builtins/types_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the two-file map-literal source slice, then update and separately commit the investigation record to the next persisted failure from run `20260721_182815`.

## 2026-07-21 valid UTF-8 string equality slice

- Map-literal commits: `23d0883 fix(maps): sort public literals canonically` and separate record commit `f1f4070 docs: record canonical map literal repair`.
- Equality observation: hidden managed run `20260721_183728` independently failed `equality::string_equality_folds_valid_utf8_case`; Barn returned `{1, 0}` for `{"A" == "a", "À" == "à"}` instead of `{1, 1}`. Canonical WSL Toast passed the exact unchanged row.
- Exact compatibility boundary: valid UTF-8 strings require Unicode case folding, while invalid raw high-byte strings must retain the already-tested byte-wise ASCII-only comparison.
- Equality TDD: `TestStringEqualityFoldsValidUTF8Case` failed before the change and passed after valid UTF-8 equal-folded strings were recognized before the existing raw-byte comparison. `TestStringEqualityFoldsASCIIWithoutFoldingRawHighBytes` passed alongside it.
- Equality gates: full `go test ./types -count=1` passed; hidden managed run `20260721_183948` passed the exact row (`1 passed, 11557 deselected`). No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `types/str.go` and `types/string_equality_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: verify and commit exactly the two-file UTF-8 equality source slice, then update and separately commit the investigation record to the first persisted loop-decompiler failure from run `20260721_182815`.

## 2026-07-21 compound verb-code line slice

- UTF-8 equality commits: `83c6e4d fix(strings): fold valid UTF-8 equality` and separate record commit `193cbae docs: record UTF-8 equality repair`.
- Loop observation: hidden managed run `20260721_184322` independently failed `looping::decompile_for_loop_duplicate_vars_break`; `verb_code()` returned one string containing `~0A`-escaped internal newlines instead of three source-line strings. Canonical WSL Toast passed the exact unchanged row.
- Exact path: despite the conformance row's decompile name, its fixture calls `verb_code()`. `parser.FormatMOO` advertised source lines but appended each multiline compound top-level statement as one element.
- Loop TDD: `TestVerbCodeReturnsCompoundStatementAsSeparateLines` failed with the exact one-element result and passed after `FormatMOO` flattened each rendered statement on its embedded newline boundaries.
- Package and managed gates: full `go test ./parser ./builtins -count=1` passed; hidden managed run `20260721_184603` passed the exact row. No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `parser/format.go` and `builtins/verbs_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the two-file compound-line source slice, then update and separately commit the investigation record to the next persisted failure from run `20260721_182815`.

## 2026-07-21 bitwise-and formatter slice

- Compound-line commits: `fe918c5 fix(parser): split formatted compound statements` and separate record commit `5d583b6 docs: record compound verb-code line repair`.
- Invalidated persisted loop rows: managed runs `20260721_184744`, `20260721_184831`, `20260721_184857`, `20260721_184924`, and `20260721_184951` each pass their exact remaining loop selector after the compound-line repair. Their earlier full-run failure premise is no longer current; no test, source change, tracked investigation entry, or commit was created for any of them.
- Bitwise observation: hidden managed run `20260721_185021` independently failed `moocode_parsing::bitwise_and_decompile`; Barn returned `{"1 & 2;"}` instead of `{"1 &. 2;"}`. Canonical WSL Toast passed the exact unchanged row.
- Exact production finding: the lexer and AST retain `BinaryBitAnd`, but `unparseBinaryOp` emits the invalid/canonicality-losing spelling `&` rather than MOO's `&.`.
- Bitwise TDD: `TestFormatMOOPreservesCanonicalBitwiseAndSpelling` failed with `FormatMOO() = "1 & 2;", want "1 &. 2;"`, then passed after changing only the formatter token.
- Bitwise gates: full `go test ./parser -count=1` passed; hidden managed run `20260721_185226` passed the exact row. No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `parser/format.go` and `parser/format_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the two-file bitwise-and formatter slice, then update and separately commit the investigation record to the next persisted failure from run `20260721_182815`.

## 2026-07-21 bitwise-or formatter slice

- Bitwise-and commits: `eb75a6a fix(parser): preserve dotted bitwise and` and separate record commit `b3fa532 docs: record bitwise-and formatter repair`.
- Bitwise-or observation: hidden managed run `20260721_185415` independently failed `moocode_parsing::bitwise_or_decompile`; Barn returned `{"1 | 2;"}` instead of `{"1 |. 2;"}`. Canonical WSL Toast passed the exact unchanged row.
- Bitwise-or TDD: `TestFormatMOOPreservesCanonicalBitwiseOrSpelling` failed with the exact undotted token and passed after changing only `BinaryBitOr`'s formatter spelling to `|.`.
- Bitwise-or gates: full `go test ./parser -count=1` passed; hidden managed run `20260721_185608` passed the exact row. No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `parser/format.go` and `parser/format_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the two-file bitwise-or formatter slice, then update and separately commit the investigation record to the next persisted failure from run `20260721_182815`.

## 2026-07-21 bitwise-xor formatter slice

- Bitwise-or commits: `8b8b554 fix(parser): preserve dotted bitwise or` and separate record commit `713d9e7 docs: record bitwise-or formatter repair`.
- Bitwise-xor observation: hidden managed run `20260721_185750` independently failed `moocode_parsing::bitwise_xor_decompile`; Barn returned `{"1 ^ 2;"}` instead of `{"1 ^. 2;"}`. Canonical WSL Toast passed the exact unchanged row.
- Bitwise-xor TDD: `TestFormatMOOPreservesCanonicalBitwiseXorSpelling` failed with the exact undotted token and passed after changing only `BinaryBitXor`'s formatter spelling to `^.`.
- Bitwise-xor gates: full `go test ./parser -count=1` passed; hidden managed run `20260721_185941` passed the exact row. No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `parser/format.go` and `parser/format_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the two-file bitwise-xor formatter slice, then update and separately commit the investigation record to the next persisted failure from run `20260721_182815`.

## 2026-07-21 waif missing-index-handler slice

- Bitwise-xor commits: `caf5f94 fix(parser): preserve dotted bitwise xor` and separate record commit `edc472f docs: record bitwise-xor formatter repair`.
- Waif observation: hidden managed run `20260721_190138` independently failed `waif_authority::waif_index_without_handler_is_type_error`; Barn returned `E_VERBNF` instead of `E_TYPE`. Canonical WSL Toast passed the exact unchanged row.
- Exact compatibility boundary: only WAIF indexing with no `:_index` handler translates the missing-handler result to `E_TYPE`; ordinary verb calls and other handler failures retain their existing errors.
- Waif TDD: `TestWaifIndexWithoutHandlerReturnsTypeError` failed with `E_VERBNF` and passed after `executeIndex` translated only `startVerbCall`'s exact missing-`_index` result.
- Waif gates: full `go test ./vm -count=1` passed; hidden managed run `20260721_190448` passed the exact row. No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `vm/op_index.go` and `vm/bytecode_execution_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: reread the active records, verify and commit exactly the two-file waif missing-handler slice, then update and separately commit the investigation record to the final persisted failure from run `20260721_182815`.

## 2026-07-21 nonwizard-owned waif index slice

- Missing-handler commits: `9e3529b fix(vm): type-error missing waif index handler` and separate record commit `3da6041 docs: record missing waif index handler repair`.
- Ownership observation: hidden managed run `20260721_190647` independently failed `waif_authority::nonwizard_owned_waif_class_cannot_dispatch_index`; a nonwizard-owned class dispatched `:_index` and returned `"key"` instead of `E_TYPE`. Canonical WSL Toast passed the exact unchanged row.
- Exact authority boundary: WAIF index handlers are eligible only when the waif class owner has the wizard flag. Invalid class/owner reads retain `E_INVIND`; wizard-owned handler dispatch and wizard-owned missing-handler translation remain separate behaviors.
- Ownership TDD: `TestNonwizardOwnedWaifClassCannotDispatchIndex` failed by returning `"key"` and passed after the WAIF index branch read the class owner through the active transaction and required that owner's wizard flag.
- Preservation tests: the successful-handler and missing-handler fixtures now explicitly model wizard-owned classes; all three direct waif-index tests pass together.
- Package and managed gates: full `go test ./vm -count=1` passed; hidden managed run `20260721_191104` passed the exact final row. No direct Barn process was launched.
- Current state: the clean two-file source slice is uncommitted in `vm/op_index.go` and `vm/bytecode_execution_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: commit exactly the two-file waif ownership slice, then update and separately commit the investigation record before the required final full-suite gates.

## 2026-07-21 final full managed gates

- Waif ownership commits: `3e32167 fix(vm): restrict waif index handlers to wizard owners` and separate record commit `c21cb82 docs: record waif index authority repair`.
- First full gate: hidden managed run `20260721_191328` completed with `1 failed, 11411 passed, 146 skipped in 196.57s`; the only failure was `audit_connect_timeout_server_option`. Record commit: `913dd36 docs: record one-failure full conformance run`.
- Independent isolation: hidden managed run `20260721_191815` passed the unchanged timeout row alone. The premise that it remained an independent current defect was wrong; no test, source change, tracked record entry, or commit was created for it.
- Current state: tracked ledger is clean; second unfiltered hidden managed run `20260721_191904` is in progress; task notes remain untracked and must not be staged.
- Current blocker: the final full managed gate has not yet completed green.
- Next action: let run `20260721_191904` complete without intervention, then use its persisted summary to decide the final gate.

## 2026-07-21 reproducible full-order timeout investigation

- Second full gate: hidden managed run `20260721_191904` completed with `1 failed, 11411 passed, 146 skipped in 190.70s`; the same `audit_connect_timeout_server_option` row was the only failure. Investigation record commit: `3e7a51e docs: record reproducible full-run timeout`.
- Canonical oracle: the documented WSL Toast command passed all 23 `connection_lifecycle_toast_oracle` rows together, including the timeout row.
- Current Barn isolation: hidden managed run `20260721_192842` passed all 23 lifecycle rows together. The trigger therefore precedes that family in the unfiltered suite; the lifecycle family alone is not sufficient.
- Timing evidence: in both full failures, the unauthenticated connection closed on the expected whole-second boundary. `callUserHook` is synchronous and precedes disconnect cleanup/logging, yet the following assertion observed no hook write. The isolated and family passes show the hook setup and timeout mechanism work without earlier full-suite state.
- Current state: tracked ledger is clean; task notes remain untracked and must not be staged. No timeout source slice has begun.
- Current blocker: the exact earlier full-suite row or state mutation that prevents the timeout hook write is not yet isolated.
- Next action: inspect the exact tests preceding the lifecycle family in unfiltered collection order, then use the smallest managed ordered selector that can distinguish the contaminating suite before writing a red direct regression.

## 2026-07-21 timeout input-ordering slice

- Isolation: managed runs `20260721_193035`, `20260721_193203`, `20260721_193307`, and `20260721_193434` passed the lifecycle family after the command-parser group, queued-task group, SQLite row, and exact combined prefix respectively. No predecessor state is required; the two full failures track wall-clock phase.
- Timing proof: the green prefix run's timeout connection lived 5.844s, while both full-run failures lived 5.020/5.025s and their control assertion completed about 0.2s after the timeout hook.
- Toast source proof: its main loop calls `network_process_io()`, then `run_ready_tasks()`, then performs the strict-second timeout sweep. Barn previously invoked the hook directly from the socket-read goroutine, allowing it to overtake already-arrived input.
- Record commit: `333e898 docs: isolate timeout input ordering`.
- TDD: `TestLoginTimeoutWaitsForPreviouslyDispatchedInput` first failed to build because `InputProcessor` had no in-flight input accounting. It now passes after timeout is represented as an input-barrier event. The existing strict-second deadline regression remains green.
- Package gate: `go test ./server -count=1` passes.
- Current state: an uncommitted three-file slice modifies `command/input.go`, `server/input_processor.go`, and `server/input_timeout_test.go`; task notes remain untracked and must not be staged.
- Current blocker: none.
- Next action: run the exact managed timeout row, then inspect and commit only the three source/test files if green.

## 2026-07-21 completed MVCC review campaign

- Timeout source commit: `85e4d58 fix(server): order login timeouts after input`; separate record commit: `793b2e3 docs: record timeout ordering repair`.
- Focused managed gate: run `20260721_194243` passed the exact timeout row.
- Full managed gate: run `20260721_194411` passed with `11412 passed`, `146 skipped`, and `0 failed` in 190.41s; record commit: `69ffef4 docs: record green full conformance gate`.
- Repository gate: `go test ./... -count=1` passed across all packages.
- Diff gate: `git diff --check` passed.
- Checklist commit: `4b87e60 docs: complete MVCC review checklist`; final investigation commit: `cf3687d docs: close MVCC conformance investigation`.
- Current state: all ten original review findings and all final gates are complete. Task notes remain untracked and must not be staged or committed.
- Current blocker: none.
- Next action: none.
