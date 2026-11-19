# Native suspended range-bound fixture

Produced by the managed stock WSL Toast server
(`/root/src/toaststunt/build-release/moo`, ToastStunt source 03af4fa) from the
conformance suite's packaged Test.db, using
tests/mongoose-conformance/server/suspended_range_bound_checkpoint.yaml under
the managed `moo-conformance` harness. A wrapper around the Toast command
copied the database each boot started from; this file is the checkpoint the
restart booted from. Toast passed canonical capability admission and the
complete restart test (2 passed in 41.12s).

SHA256: 8454828352c2833042c79789516d9e0f8ca32c26e07cebef40a534991bc4baff

The checkpoint was captured while two tasks were each suspended in `limit`,
called from a range loop's upper bound. Toast evaluates the lower bound, then
the upper bound, then runs OP_FOR_RANGE (code_gen.cc STMT_RANGE), so the
looping frame holds the lower bound as an ordinary operand:

- Task 1372396205: the forked driver, `flat` paused at `this:limit(4, "flat")`
  in `for n in [low..this:limit(4, "flat")]` holding the operand 2, and `limit`
  paused in suspend().
- Task 1579663818: the forked driver, `nested` paused at
  `this:limit(10 * a + 1, "nested")` in the inner loop of
  `for a in [1..2] for b in [10 * a..this:limit(...)]` holding the operands
  2, 2, 10 (the outer loop's next value and limit, then the inner lower bound),
  and `limit` paused in suspend().

The object has flat_paused=1, nested_paused=1, seen={}, pairs={} and every
other property 0. Resuming must produce seen={2, 3, 4},
pairs={{1, 10}, {1, 11}, {2, 20}, {2, 21}}, and flat_resumed, flat_finished,
nested_resumed and nested_finished all 1.

Only the test-owned public Test.db objects and producer code are present. No
Mongoose or ChatMUD database or player data is included.
