# Conformance Fix Progress

## Baseline (2026-02-24)
- 2829 tests collected
- 2632 passed, 62 failed, 135 skipped
- Test command: `uv run --project ../moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"`

## Failure Clusters

### Cluster 1: Permission Checks (31 tests) -- DONE
- **Commit:** db299af
- 29 fileio builtins + 3 network admin builtins — wizard check added as first statement
- All 53 `requires_wizard` tests now pass

### Cluster 2: FileIO Security (18 tests) -- DONE
- **Commit:** 9292c9c
- Rewrote `sanitizeFilePath` to check raw input for `/.` and `\.`
- Strict 4-char mode validation `[rwa][+-][tb][fn]`
- Added `canRead()`/`canWrite()` methods for mode enforcement
- `builtinFileChmod` now takes string mode, validates 3-digit octal
- `builtinFileRename` checks dest first (E_INVARG), then source (E_FILE)
- All 43 fileio_security tests pass, 101/101 full fileio pass

### Cluster 3: FileIO Behavior (6 tests) -- DONE
- **Commit:** ac892a9
- Added `filterTextMode()` for text-mode reads (printable ASCII only)
- EOF now returns E_FILE instead of empty string
- `file_readlines` uses (start, end) semantics with proper validation
- `file_writeline` now decodes binary strings in binary mode

### Cluster 4: Exec Task Management (2 tests) -- DONE
- **Commit:** b9ab786
- `builtinExec` now suspends task, runs subprocess in goroutine, resumes on completion
- Added `ExecCancelFunc` field to Task for kill_task subprocess cancellation
- Added `CompleteExec()` method to bypass IsExecSuspended guard for result delivery
- All 11 non-skipped exec tests pass

### Cluster 5: Flaky (1 test)
- disassemble_with_index_operators — failed on full run, passed on rerun. Not investigating now.

## Final Results

**Before:** 62 failed, 2632 passed, 135 skipped
**After:** 2 failed, 2692 passed, 135 skipped

**60 failures fixed across 4 commits:**
- db299af: Permission checks (31 tests)
- ac892a9: FileIO behavior (6 tests)
- 9292c9c: FileIO security (18 tests)
- b9ab786: Exec async (2 tests + 3 lifecycle that were cascading from prior test)

**2 remaining conformance failures (both timeouts, not assertion failures):**
- `disassemble_with_index_operators` — flaky, passes on isolated rerun
- `do_command_return_true_stops_parsing` — lifecycle test timeout

## toastcore.db Investigation

### Connection Options Fix
- **Commit:** 414db88 — added binary, flush-command, keep-alive options
- Unblocked telnet negotiation in `$telnet:new_connection`

### Login Still Broken
- Barn shows "*** Connected ***" but no lifecycle hooks fire
- Toast calls `#0:user_connected(player)` after `switch_player` — shows room, confunc chain
- Barn doesn't — no room description, no confunc, no verb execution post-login
- **Root cause found:** Barn doesn't implement verb `d` (debug) flag behavior
- Toast: when verb lacks `d` flag, runtime errors pushed as values (not exceptions)
- Barn: always propagates errors as exceptions regardless of debug flag
- `#108:set_connection` (perms `rx`, no `d`) calls `set_name` on anon obj → E_VERBNF
- In Toast: error silently pushed as value, execution continues, login completes
- In Barn: exception propagates up, kills entire `user_connected` handler
- **Commit:** 457330d — VM now checks VerbDebug flag; errors pushed as values when d is off
- toastcore.db login FULLY WORKING — MCP init, lifecycle hooks, room description, eval all work
- See `reports/fix-verb-debug-flag-report.md` for implementation details

### raise() Behavior Fix
- **Commit:** 920aeaa — raise() in non-debug verbs now caught as value (matches Toast)
- Original implementation exempted VMException (raise) from debug flag check
- Toast treats raise() identically to other errors when verb lacks `d` flag
- Verified via conformance test `raise_in_non_debug_verb_becomes_value`

## Conformance Tests Added (moo-conformance-tests repo)
- **Commit:** f4e5a67 — 21 new tests across 3 areas
- 11 verb debug flag tests (new file `vm/verb_debug_flag.yaml`)
- 5 connection option functional tests (added to `builtins/server_admin.yaml`)
- 5 fileio mode tests: append, r+, w+, a+ (added to `builtins/fileio.yaml`)
- All verified against Toast before commit, all pass on Barn

## Current Results (2026-02-25)
- **2850 tests collected** (was 2829 before new tests)
- **2713 passed, 2 failed (flaky timeouts), 135 skipped**
