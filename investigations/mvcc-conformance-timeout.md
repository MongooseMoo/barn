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

## Theories (plausible)

1. A Barn regression panics after consuming connection input; this predicts an internal task panic with idle transport and scheduler goroutines after recovery.
2. Windows firewall or endpoint filtering permits TCP accept but disrupts subsequent loopback traffic; this predicts failures outside Barn on a trivial Windows loopback exchange or firewall evidence for port 7788.
3. WSL routing or reverse-DNS behavior is broken; this predicts Windows-only Barn remains healthy and the failure appears only when the managed WSL Toast path is exercised.
4. A current conformance harness/client change sends an input sequence Barn does not handle; this predicts the pre-review Barn revision also fails the unchanged current focused test.

## Tests Run

| Test | Hypothesis | Result | Rules Out | Supports |
|------|------------|--------|-----------|----------|
| Managed focused Windows Barn run `20260721_103418` | 1, 2, 3, 4 | Timed out waiting for cleanup response | WSL-only cause for this observed failure | 1, 2, or 4 |
| Blame focused YAML row | 4 | Row is unchanged since 2026-05-05 | Today's test-row changes | 1 or a runner-wide change |
| Inspect active firewall policy | 2 | Current `barn.exe` has inbound allow; server accepted/read loopback connections | Simple inbound firewall block | 1 |
| Managed pprof/log run `20260721_110542` | 1, 2 | Connections and workers idle; log shows recovered nil panic at `StoreTxn.Commit:1621` | Firewall, WSL, socket-write stall, lane deadlock | 1 |
| Inspect `StoreTxn.objects` contract and line 1621 | 1 | Missing objects are cached as nil; finding 8 loop dereferences every entry | Unexplained runtime/environment failure | Finding 8 nil handling defect |

## Current Best Theory

Finding 8's insertion-order commit loop assumes every cached transaction object is non-nil. Missing-object reads deliberately cache nil. The resulting commit panic is recovered by the task runtime without emitting eval response markers, which makes the conformance client report a receive timeout. Firewall and WSL are not causal for this failure.

## Open Questions

- Does a minimal store regression reproduce the nil dereference without networking?
- After the nil-safe ordering fix, does the focused connection row pass without any networking changes?

## Next Action

Add a minimal store regression that caches a missing object, defines a property on a valid object, and commits. Confirm it fails by panic before implementing a nil-safe insertion-order loop.
