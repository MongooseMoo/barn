# Investigation: Mongoose Login Input Admission

## Scope

Active behavior row: the managed real-Mongoose trusted-PROXY, account login,
post-login eval, and `look` scenario in
`src/moo_conformance/_tests/integration/mongoose_login_look.yaml`.

The unchanged row is green on the pinned WSL Mongoose Toast oracle and red on
Windows Barn built from `85308f1d5ad973d5c7cda628581b4bd8341eeda1`.

## Facts (verified)

- The selected fixture is `mongoose_fresh2.db`, 101,244,108 bytes, SHA-256
  `33201970097d3d2d2bfc0d5f875f087d587601bf8255ef31ef19b416d65ac925`.
- WSL Mongoose Toast is at
  `72e3c7f96ce7a41fdeba793aef8818dc4408072e`; its executable SHA-256 is
  `a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098`.
- The managed Toast row passed: `1 passed, 11461 deselected in 48.12s`.
- The unchanged managed Windows Barn row timed out on the first post-login eval
  after the scenario's 30-second startup wait: `SocketTransport._receive()`
  received no response.
- In `.tmp/mongoose-convergence/barn-m2-first/logs/latest.jsonl`, Barn loaded
  26,386 objects, listened, accepted the transport connection, and rewrote its
  address to `203.0.113.5` 13 seconds after accepting it. No error was logged.
- In the debug rerun under `.tmp/mongoose-convergence/barn-m3-diag-1`, the
  pre-connection snapshot had `tasks_started=8`, `tasks_live=8`, and
  `connections_live=0`. After the harness connection, those task counts were
  unchanged while `connections_live=2`.
- The debug rerun never logged `proxy name rewritten`. Thirty-five seconds
  after connection acceptance it logged `E_INVARG` in
  `#1584:bf_call_function`, player `#0`, on
  `return call_function(func, @rest);`. The managed row again timed out at its
  first post-login eval.
- The Windows Barn manifest records the expected fixture checksum and both
  `option.OUTBOUND_NETWORK=true` and `option.PROMOTE_NUMBERS=true`.
- `barn_logs -level error` found no errors in the first run. The second run's
  only error was the `bf_call_function` exception above.

## Theories (plausible, untested)

1. Startup or restored background tasks keep command input held. The eight
   startup tasks remain live, and the connection path does not admit a new
   input task until a later release condition. This predicts unchanged task
   counts while login lines wait and late processing after a startup timeout or
   task-state transition.
2. The connection/input processor leaves the new connection in an input-held
   state independently of scheduler load. This predicts queued socket lines
   with no task admission even if the eight startup tasks are otherwise
   runnable or intentionally suspended.
3. The static login lines are admitted but delivered to the wrong login/read
   state or in the wrong order. This predicts input-task creation before the
   timeout and a trace tied to `q` or the password, rather than task counts
   remaining fixed immediately after connection.

## Tests Run

| Test | Hypothesis | Result | Rules Out | Supports |
| --- | --- | --- | --- | --- |
| Managed WSL Mongoose Toast row | The scenario and fixture are valid | Passed unchanged | Invalid test/fixture as the Barn explanation | Barn-specific boundary |
| Managed Windows Barn row with stable logs | Barn can complete the same login contract | First eval timed out; PROXY rewrite was late | Barn-green/no-delta premise | Input/startup delay |
| Debug rerun with `/debug/vars` before and during login | Input creates runnable tasks promptly | Connections rose 0 -> 2; tasks stayed 8/8 | Prompt task admission | Theories 1 or 2 |
| Error-level log correlation | A visible runtime exception explains the initial stall | First run had none; second had a late pre-auth `bf_call_function` error | Immediate exception as common cause | Late/misordered processing |

## Current Best Theory

The failure is before ordinary post-login command execution. The strongest
current explanation is that connection input is not admitted while the eight
startup/restored tasks remain live, either because of startup-wide input hold
or because the new connection retains a hold state. Login ordering remains a
secondary theory until the admission owner is read and the pending-input state
is observed directly.

## Open Questions

- Which source condition decides whether `InputEvent` becomes a task?
- What are the eight live startup tasks doing while input is not admitted?
- What event releases the PROXY line in the first run, and why was it absent in
  the second?
- Which exact input line produced the late `bf_call_function` exception?

## Next Action

Read the connection input queue, `InputProcessor`, startup task restoration,
and scheduler admission paths as one ownership slice. Identify the condition
that can keep input pending, then add only the instrumentation or focused test
needed to distinguish startup-wide hold from connection-local hold.
