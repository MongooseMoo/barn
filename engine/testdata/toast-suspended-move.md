# Native suspended-move fixtures

Three checkpoints written by the managed canonical WSL Toast server (stock
ToastStunt at `03af4fa2b53b418999f03566b646ee42bc7ea647`, the
`implementation_ref` of `profiles/toast/stock-wsl-testdb.json`) from the
conformance suite's packaged Test.db (SHA256
1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e), one per
producer in tests/mongoose-conformance/server/, each run in its own session of
the managed `moo-conformance` harness together with capability admission. The
server command was a wrapper that copied the database each boot started from
and then ran `/root/src/toaststunt/build-release/moo {db} {db}.new {port}`; each
file here is the checkpoint the producer's `restart_server` step booted from.
Toast passed all three producers (2 passed per session).

| File | Producer | Hook frame | SHA256 |
|---|---|---|---|
| toast-suspended-move-accept.db | suspended_move_accept_checkpoint.yaml | `accept`, pc line `22 2 20` | 430c730bfc4c722b2eb26e8b98a5f85683e7bcabfa72aa2e71c7851cc80f59ea |
| toast-suspended-move-exitfunc.db | suspended_move_exitfunc_checkpoint.yaml | `exitfunc`, pc line `22 3 20` | 80fa40bafb261708520efa6647b2241af1cf6b8bcbf3ac247badf3de938ea205 |
| toast-suspended-move-enterfunc.db | suspended_move_enterfunc_checkpoint.yaml | `enterfunc`, pc line `22 4 20` | 4e3a934cefa184bec8c86968993d2ba2b20a539590a0a041aca2bd5af9675d17 |

In each, a non-wizard object #9 owns the driver #10, the moved object #11, its
first location #12 and the destination #13, and every verb. `#10:start` runs
`this.result = {move(this.thing, this.dest)};` and then counts `finished`.
`#13:accept`, `#12:exitfunc` and `#13:enterfunc` each append their verb name to
`#10.log` and return 1; the hook under test also calls `suspend(20)`, then
appends `"<verb> resumed"` and stores the verb names from `callers()` in
`#10.stack`.

Each checkpoint holds one suspended task of three frames. Frame 0 is the forked
driver, paused in its `o:start()` call. Frame 1 is `start`, paused at its
`move(...)` call. Frame 2 is the hook verb, paused in `suspend()`; Toast saved
builtin `move` on it with the `bi_func_pc` do_move continues at when the hook
returns (2 after accept, 3 after exitfunc, 4 after enterfunc) and the line
`bf_move data: what = 11, where = 13, position = 0`.

| File | Saved `log` | #11 is in | Resuming must give `log` |
|---|---|---|---|
| accept | `{"accept"}` | #12 | `{"accept", "accept resumed", "exitfunc", "enterfunc"}` |
| exitfunc | `{"accept", "exitfunc"}` | #13 | `{"accept", "exitfunc", "exitfunc resumed", "enterfunc"}` |
| enterfunc | `{"accept", "exitfunc", "enterfunc"}` | #13 | `{"accept", "exitfunc", "enterfunc", "enterfunc resumed"}` |

Every checkpoint has stack={}, result=0, finished=0. Resuming must leave #11
in #13 with stack={"move", "start", ""}, result={0}, finished=1. Because the
verbs' owner is not a wizard, the accept checkpoint moves the object only if
move() receives the 1 that accept returns after the restart.

Only the test-owned public Test.db objects and producer code are present. No
Mongoose or ChatMUD database or player data is included. The tests change only
the task's absolute wake deadline, to make the saved task due.
