# Investigation: MVCC review conformance timeout

## Facts (verified)

- Branch `work/mvcc-concurrent-moo` was tracked-clean before this investigation.
- The documented Windows managed command collected 11,558 tests from the sibling `C:\Users\Q\code\moo-conformance-tests` checkout.
- Managed focused run `20260721_103418` selected `audit_listener_handler_do_login_command` and timed out in `SocketTransport._receive()` during cleanup.
- That run used Windows Barn at `127.0.0.1:7788`; WSL Toast was not launched.
- Barn remained listening on Windows TCP port 7788 while later full-run tests timed out.
- The current plan records that `connection_lifecycle_toast_oracle` previously passed 23/23 on both stock WSL Toast and committed Windows Barn.
- Barn received ten review-fix commits today, including `089b95a` changing ticker-dequeued input to route through connection lanes.
- The conformance checkout also received commits today and is currently on clean `main` at `f9f8503`.
- `audit_listener_handler_do_login_command` lines 34-66 are unchanged since conformance commit `d1a68aa7` on 2026-05-05.
- Windows firewall has an enabled inbound allow rule for the current `barn.exe`; the managed server accepted and read loopback connections during the failure.
- Managed diagnostic run `20260721_110542` exposed Barn's built-in pprof endpoint. During the timeout, Barn's TCP reader, connection lane, and scheduler workers were idle; no goroutine was blocked on a socket write or store lock.
- That run's server log captured a recovered nil-pointer panic in `StoreTxn.Commit` at `db/store/store_txn.go:1621` while the primary conformance eval ran.
- Finding 8 commit `5ff4436` added a loop over every `tx.objects` entry and dereferences `obj.propOrder`. `StoreTxn` intentionally caches missing objects as `nil` in that map, so an invalid-object lookup followed by a property definition can panic during commit.
- Separate decentralized and coarse store regressions reproduced both panic sites. Nil cached misses are now skipped in both ordering loops; the store package and focused managed row pass. Fix commit: `83b54f8`.
- Managed connection-lifecycle family run `20260721_111108` passed its first four rows, then `audit_user_connected_hook_on_first_login` returned semantic comparison `0` instead of `1`; later family cases cascaded into timeouts.
- Focused managed run `20260721_111457` reproduces only the semantic `user_connected` failure in 3.89 seconds.
- `RunServerVerbTask` assigns `t.Caller = player`, but that assignment predates the current review work. The conformance boolean does not reveal whether the hook write is absent or one recorded frame field differs.
- Direct server regression recorded `{#0, #2, #2, {#2}, ""}` versus Toast's `{#0, #2, #-1, {#2}, ""}`. Server lifecycle tasks now use `caller = #-1`; focused managed run `20260721_111924` passes. Fix commit: `92bf74f`.
- Managed family run `20260721_111949` passes the first five rows, then `audit_user_connected_continues_after_zero_delay_fork` times out; later cases cascade. The next active surface is server-hook fork continuation.
- A minimal ordered selector proved the zero-delay-fork row passes alone but fails after the simple hook row because that row's cleanup returns `E_INVARG`.
- Cleanup reads `user_connected`, live-deletes it, then commits staged property deletes. `delete_verb` did not adopt its own live verb mutation, so the transaction conflicted with itself after blanket read-set rebasing was correctly removed.
- A builtin regression reproduced the conflict. `delete_verb` now calls targeted `AdoptLiveVerbs`; the two-row managed sequence passes. Fix commit: `147c45d`.
- Managed family run `20260721_112724` passes 19/23. Four independent semantic failures remain: client-disconnected hook, cross-listener reconnection hooks, connect timeout, and flush command.
- Focused managed run `20260721_112914` independently reproduces `audit_user_client_disconnected_hook` as expected `1`, actual `0`.
- A direct `rxd` hook regression reproduced the exact bad frame: after player `#2` was removed while player `#3` remained connected, `connection_info(#2)` resolved player `#3` through `resolveConnection`'s fallback.
- Direct connection lookup is now exact: a missing requested player returns no connection instead of substituting any active session. Affected packages pass, and managed run `20260721_114010` passes the focused disconnect row. Fix commit: `d03f9f1`.

## Theories (plausible)

1. A Barn regression panics after consuming connection input; this predicts an internal task panic with idle transport and scheduler goroutines after recovery.
2. Windows firewall or endpoint filtering permits TCP accept but disrupts subsequent loopback traffic; this predicts failures outside Barn on a trivial Windows loopback exchange or firewall evidence for port 7788.
3. WSL routing or reverse-DNS behavior is broken; this predicts Windows-only Barn remains healthy and the failure appears only when the managed WSL Toast path is exercised.
4. A current conformance harness/client change sends an input sequence Barn does not handle; this predicts the pre-review Barn revision also fails the unchanged current focused test.
5. Terminal task transaction release drops or invalidates the `user_connected` hook write; this predicts the recorded property remains its default value in a direct server test.
6. The hook write commits but its server-initiated frame differs from Toast; this predicts a direct server test records a concrete non-default list with one or more mismatched fields.

## Tests Run

| Test | Hypothesis | Result | Rules Out | Supports |
|------|------------|--------|-----------|----------|
| Managed focused Windows Barn run `20260721_103418` | 1, 2, 3, 4 | Timed out waiting for cleanup response | WSL-only cause for this observed failure | 1, 2, or 4 |
| Blame focused YAML row | 4 | Row is unchanged since 2026-05-05 | Today's test-row changes | 1 or a runner-wide change |
| Inspect active firewall policy | 2 | Current `barn.exe` has inbound allow; server accepted/read loopback connections | Simple inbound firewall block | 1 |
| Managed pprof/log run `20260721_110542` | 1, 2 | Connections and workers idle; log shows recovered nil panic at `StoreTxn.Commit:1621` | Firewall, WSL, socket-write stall, lane deadlock | 1 |
| Inspect `StoreTxn.objects` contract and line 1621 | 1 | Missing objects are cached as nil; finding 8 loop dereferences every entry | Unexplained runtime/environment failure | Finding 8 nil handling defect |
| Red/green cached-miss regressions and managed focused rerun | 1 | Both commit paths no longer panic; focused row passes 1/1 | Firewall/WSL and lane routing as cause of the original timeout | Nil dereference root cause and fix |
| Managed connection family and focused first-login row | 5, 6 | First four lifecycle rows pass; isolated hook comparison returns 0 | Remaining global transport failure | Hook transaction or frame regression |
| Direct server hook frame regression | 5, 6 | Write commits with only `caller` wrong (`#2`, want `#-1`) | Terminal transaction release dropping the simple hook write | Server-hook caller defect |
| Focused hook rerun and next family run | 6 | Simple hook passes; family reaches the zero-delay-fork continuation row then times out | Simple hook frame as remaining cascade source | Separate fork-continuation defect |
| Focused fork row and minimal ordered selector | Cleanup/order contamination | Fork row passes alone and fails only after simple-hook cleanup; cleanup returns `E_INVARG` | Independent fork scheduling defect | Live mutation self-conflict in cleanup |
| Red/green `delete_verb` adoption regression and ordered rerun | Targeted live mutation adoption | Conflict reproduced before adoption; ordered managed rows pass 2/2 after fix | Network or scheduler continuation cause | Missing `AdoptLiveVerbs` after deletion |
| Managed family `20260721_112724` and focused disconnect row | Remaining cascade versus independent failures | Family passes 19/23; disconnect row fails alone with semantic `0` | Global timeout cascade | Four independent lifecycle semantics defects |
| Direct `rxd` disconnect frame regression and managed rerun | Unrelated connection fallback | Hook recorded `connection_info_succeeds = 1` with another player connected; exact lookup records `0`; managed row passes 1/1 | Disconnect removal timing and hook frame defects | `resolveConnection` substituted an unrelated session |

## Current Best Theory

The original timeout was Finding 8's nil dereference and is fixed by `83b54f8`. The simple first-login hook failure was server-hook `caller` and is fixed by `92bf74f`. The ordered fork timeout was a `delete_verb` transaction self-conflict and is fixed by `147c45d`. The client-disconnected frame failure was unrelated-session substitution and is fixed by `d03f9f1`. The same lookup may explain the first false element in the cross-listener row, but that must be rerun rather than assumed. Firewall and WSL are not causal for these Windows-managed failures.

## Open Questions

- Does the cross-listener row now pass after exact connection lookup?
- If not, which of its five recorded assertions remains false?

## Next Action

Run the cross-listener row alone with the managed harness. If it remains red, isolate its first false assertion in a direct server regression before editing; if it passes, record that the exact-lookup repair resolved the shared cause and move to the connect-timeout row without another source change.
