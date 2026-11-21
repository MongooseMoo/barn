# Native suspended-eval fixture

Produced by ToastStunt (lisdude/toaststunt 20668a7, Linux build) from the
conformance suite's packaged Test.db, using
tests/mongoose-conformance/server/suspended_eval_checkpoint.yaml under the
managed `moo-conformance` harness. A wrapper around the Toast command copied the
database each boot started from; this file is the checkpoint the restart booted
from. Toast passed the complete restart test (2 passed, including the result
`{1, 73}` after restart).

SHA256: d2d4529ee4fbf988006881ceb142447b2c08cd69253a19e18be16e45fdd54419

The checkpoint was captured while one task was suspended inside code run by
eval(). Frame 0 is the forked driver, paused in its `o:start()` call. Frame 1
is `start`, paused at its `eval(...)` call. Frame 2 is the activation eval()
pushed, paused in suspend(); Toast saved builtin `eval` with `bi_func_pc` 2 on it,
which is how `bf_eval(next=2)` later wraps the return in `{1, value}`.
The object has before=1, after=0, result=0, finished=0. Resuming must produce
before=1, after=1, result={1, 73}, finished=1.

## call_function("eval", ...)

`toast-suspended-call-function-eval.db` (SHA256
ad1bd19d6162be3e0fb0de43700e8bc5c5e5c9d5f9d13189db4ed20b7e256259) was written
on 2026-11-21 by stock Toast 03af4fa2 (binary SHA256
61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd) from the
conformance suite's Test.db (SHA256
1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e), by the
suite's `server/suspended_call_function_eval_checkpoint.yaml` under the managed
harness (Toast to Toast: 2 passed), with the same copying wrapper; it is the
checkpoint the restart booted from. `start` runs
`this.result = call_function("eval", this.source);`, and the eval'd program
stores the verb names from `callers()` in `stack` before `suspend(20)`. The
eval activation ends with the pc line `47 2 45`, `call_function` and
`bf_call_function data: fname = eval`, with no eval data after it. The
checkpoint holds before=1, after=0, result=0, finished=0,
stack={"call_function", "start", ""}; resuming must give before=1, after=1,
result={1, 73}, finished=1.

Only the test-owned public Test.db objects and producer code are present. No
Mongoose production checkpoint or player data is included.
