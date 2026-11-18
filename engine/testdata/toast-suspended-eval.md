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

Only the test-owned public Test.db objects and producer code are present. No
Mongoose production checkpoint or player data is included.
