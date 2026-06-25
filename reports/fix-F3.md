# Fix F3 — unauthenticated connection could get instant wizard

## The bug
`server/input_login.go:45-48` returned a hardcoded `types.ObjID(2)` (the wizard)
whenever the listener object had no `do_login_command` verb. Any connection to a
listener lacking that handler was logged in as wizard — a critical auth bypass.

## Toast's authoritative behavior (no handler / invalid result)
`C:/Users/Q/src/toaststunt/src/tasks.cc` `do_login_task`:

- Line 884: `result.type = TYPE_INT;` — initialized "In case #0:do_login_command
  does not exist or does not immediately return." When the verb is absent,
  `run_server_task_setting_id` leaves `result` at this default int 0.
- Lines 914-916: the verb is invoked via `run_server_task_setting_id`.
- Line 921: login happens ONLY when
  `tq->connected && tq->player < 0 && result.type == TYPE_OBJ && is_user(result.v.obj)`.
- Lines 959-965 (`else`): otherwise the task is just cleaned up — no player is
  assigned, `player_connected` is not called, the connection stays
  un-logged-in. Toast does NOT substitute a default wizard.

So: no handler (or a non-object / non-user / negative return) ⇒ connection
remains unauthenticated, no player assigned. No wizard fallback.

## What I changed
`server/input_login.go` — `callDoLoginCommand`: replaced the hardcoded
`conn.Send("Welcome! (No login handler defined)"); return types.ObjID(2), nil`
with `return types.ObjID(-1), nil` (login refused) plus a comment citing
tasks.cc:884 and :921. The caller (`input_processor.go:413-416`) only logs in
when `player > 0`, so a negative return correctly leaves the connection
unauthenticated, matching Toast's `else` branch.

## Test assertion (tightened, with citation)
`server/review_server_test.go`
`TestReview_FallbackLoginReturnsWizardWithNoLoginHandler`:
the original assertion only failed on the exact value `player == 2`. Tightened to
`player >= 0` to assert Toast's precise behavior: ANY non-negative result is
wrong — the connection must not be logged in as any player when no handler
exists (tasks.cc:884,921). Tests remain fully in-process; no ports bound.

## Green test output
```
=== RUN   TestReview_FallbackLoginReturnsWizardWithNoLoginHandler
--- PASS: TestReview_FallbackLoginReturnsWizardWithNoLoginHandler (0.00s)
PASS
ok      barn/server     0.870s
```

## Server package failure list (before -> after)
Before: F3 test failed, plus the two sibling review tests.
After (`go test ./server/...`):
- `TestReview_FallbackLoginReturnsWizardWithNoLoginHandler` — now PASS (fixed).
- `TestReview_ConnectionNameLookupNeverErrors` — still FAIL (pre-existing F-finding,
  `connection_manager.go`, untouched).
- `TestReview_WebSocketWakeInputReaderDoesNotSetDeadline` — still FAIL (pre-existing
  F-finding, `websocket_transport.go`, untouched).
No NEW failures introduced; the two remaining are the other intentionally-red
review findings, unrelated to this change.

## Commit
COMMIT_HASH_PLACEHOLDER
