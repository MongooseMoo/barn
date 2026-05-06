# Mongoose Login Investigation

## Phase 1: Toast Reference Behavior (Toast on port 9451)

### Test Results
1. `welcome` → Shows banner, then calls `$account_login:login()` which uses `read()` for interactive prompts
2. `co q canefan` → Shows player selection [1-7], waits for input via state machine
3. `co q canefan` → `1` → **Login succeeds**, `(***) WELCOME! (***)` appears, but `#43:ddmmyy` has E_INVARG (date formatting issue in DB)
4. `login` → Prompts "Username:", waits via state machine (interception pattern)
5. `login` → `q` → `canefan` → Authenticates, shows player selection, same ddmmyy error
6. `connect wizard` → "Usage: connect <existing-player-name> [password] [option]" (no bare wizard in mongoose)

### Date formatting issue
Q noted: "the time/date formatting sounds like a windows toast issue we should fix"
- `#43:ddmmyy` line 5 throws E_INVARG
- Called from title chains during `user_connected`
- This is a Toast-on-Windows issue with date builtins, not a Barn issue

## Phase 2: Login Flow Analysis

### Two login paths:
1. **welcome/account_login path** (uses `read()`):
   - `$login:welcome` (#10) → `$account_login:login()` (#1414)
   - `$account_login:_read()` calls `read(player)` to suspend task and wait for input
   - **REQUIRES** `read()` builtin to work

2. **co/connect path** (state machine, no `read()`):
   - `$login:co*nnect` (#10) → handles auth, shows player list, sets state to 5
   - `$login:parse_command` checks interception → routes subsequent commands to `$login:login`
   - `$login:login` (#10) uses state machine in `this.state` property
   - **Does NOT use** `read()` — uses interception pattern instead

### Key objects:
- #0: `do_login_command` - entry point for all pre-login commands
- #10: `$login` - login commands (connect, login, welcome, parse_command, state machine)
- #1414: `$account_login` - account-based login using `read()`

## Phase 3: Barn Behavior (Barn on port 9450)

### Test Results
1. `welcome` → Shows banner but then "Oh, dear. something's gone wrong." because `read()` returns E_INVARG
2. `co q canefan` → Shows player selection [1-7] correctly! Interception/state machine works!
3. `co q canefan` → `1` → **SILENT**. No output. Login doesn't complete.
4. `login` → Not tested yet (likely uses state machine)

## Phase 4: Identified Divergences

### Divergence 1: welcome/read() path
- **Toast**: `$account_login:login()` calls `read()`, suspends task, prompts user
- **Barn**: `read()` stub returns E_INVARG immediately, `_read` catches it and kills task
- **Fix needed**: Implement `read()` properly (suspend task, wait for next input line)

### Divergence 2: co q canefan → selection fails
- **Toast**: After "1", login completes, player connects
- **Barn**: After "1", silence. No output.
- **Root cause**: Unknown - need to trace. Could be:
  a) `$login:login` state 5 processing fails silently
  b) `toint(option)` handling issue
  c) `this.players[pos][3][toint(option)]` fails
  d) Return value from `$login:login` not being handled correctly in `do_login_command`
  e) `$telnet_utils:switch_connection` or `switch_player` failing

### Divergence 3: "I don't understand that"
- Q reports sometimes getting command list as if unlogged
- Could be the interception not being set up, or parse_command falling through to `bogus_command`

## Phase 5: Root Cause & Fix

### Root Cause: CallVerb didn't handle FlowFork

The bytecode VM yields `FlowFork` when it encounters a `fork` statement. The scheduler
is supposed to create the child task, set the fork variable, and call `vm.Resume()`.

The regular task execution path (`runTask`) and `EvalCommand` both handled this correctly.
But `CallVerb` (used for server hooks like `do_login_command`, `user_connected`) just
returned the FlowFork result without resuming, causing the verb to abort mid-execution.

In `$login:login` state 5 (player selection), lines 147-154 fork a cleanup task, then
line 155 `return who`. The fork caused the verb to exit before reaching `return who`.

### Fix: `drainForks` helper

Added `drainForks(t, bcVM, result)` method that loops over FlowFork yields, creating
child tasks and resuming the parent VM. Used in:
- `CallVerb` (new)
- `runTask` (refactored)
- `EvalCommand` (refactored)

**File changed:** `server/scheduler.go`

### Conformance test results after fix:
- 2571 passed, 2 failed, 183 skipped
- 2 failures are pre-existing (ctime, kill_task) — not related to fork fix

## Phase 6: Object Comparison Bug

### Symptom
`#0:user_disconnected` line 4 (`if (args[1] < #0)`) threw E_TYPE on disconnect.

### Root Cause
Bytecode VM `compareValues()` in `vm/operations.go` handled Int, Float, String
comparisons but was missing OBJ comparison. The tree-walker `compare()` in
`vm/operators.go` correctly handled OBJ comparison by comparing `.ID()` values.

### Fix
Added OBJ comparison block to `compareValues()` in `vm/operations.go`:
```go
aObj, aIsObj := a.(types.ObjValue)
bObj, bIsObj := b.(types.ObjValue)
if aIsObj && bIsObj {
    // compare by .ID()
}
```

### Verification
- `#5 < #0` → 0, `#0 < #249` → 1 (matches Toast)
- `user_disconnected` no longer throws E_TYPE
- Conformance: still 2571 passed, 2 failed, 183 skipped (no regressions)

## Remaining Issues

### 1. `welcome` command still broken (needs read())
- `$account_login:login()` calls `read()` which is stubbed as E_INVARG
- Needs proper read() implementation: suspend task, route next input to suspended task

### 2. ANSI color code rendering garbled
- Room descriptions show garbled characters where ANSI codes should be
- Probably a telnet/connection encoding issue

### 3. DB-specific tracebacks (not Barn bugs)
- `#2235:parse_parties` line 10: Variable not found
- `#2700:go` line 9: Type mismatch
- `#410:say` line 12: Permission denied
- `#43:ddmmyy` line 5: Invalid argument (date formatting, also fails on Toast/Windows)
- `$gmcp:user_disconnected` E_VERBNF (verb doesn't exist in DB, runs in fork)
- `#2700:disfunc` E_ARGS (wrong arg count, runs in fork)
