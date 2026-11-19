# Native indefinitely-suspended fixture

One checkpoint written by the managed canonical WSL Toast server
(`/root/src/toaststunt/build-release/moo`, the source revision named in
profiles/toast/stock-wsl-testdb.json, binary SHA256
61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd) from the
conformance suite's packaged Test.db (SHA256
1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e), using the
suite's `server/restart_task_semantics.yaml::indefinitely_suspended_task_across_checkpoint_restart`
under the managed `moo-conformance` harness. A wrapper around the Toast command
copied the database each boot started from; this file is the checkpoint the
test's restart booted from. Toast passed canonical capability admission and the
restart test in that run (2 passed).

SHA256: ad6b96ec1d9fa2dd0eb10b6f04d6974fc28adeedd6aa4cf891f339ec8a6a9ada

The checkpoint holds one task, forked by `#10:hold`, suspended in `suspend()`
with no argument. Toast gives such a task start time INTNUM_MAX (tasks.cc
`enqueue_suspended_task`) and writes it as `9223372036854775807`
(`write_suspended_task`). `#10.resumed` is `{}`. After load the task must still
be suspended with no wake deadline, and `#10.resumed` must stay `{}`.

Only the test-owned public Test.db objects and producer code are present. No
Mongoose or ChatMUD database or player data is included.
