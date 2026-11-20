# Native suspended-in-finally fixtures

Seven checkpoints written by the managed canonical WSL Toast server
(`/root/src/toaststunt/build-release/moo`, the source revision named in
profiles/toast/stock-wsl-testdb.json, binary SHA256
61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd) from the
conformance suite's packaged Test.db (SHA256
1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e), using
tests/mongoose-conformance/server/suspended_finally_checkpoint.yaml under the
managed `moo-conformance` harness. A wrapper around the Toast command copied
the database each boot started from; each file is the checkpoint one test's
restart booted from. Toast passed canonical capability admission and all seven
restart tests in that run (8 passed in 175.18s).

Each checkpoint holds one task suspended inside a `finally` body. Toast keeps
why the body is running, and a value, as the two operands at the bottom of the
body's stack (execute.cc `Finally_Reason`, `unwind_stack`).

| File | SHA256 | Reason and value saved | Entered by |
|---|---|---|---|
| toast-suspended-finally-fall-through.db | 4b6a3a1564b4e230fa6659b3a88a2d52f52a974d204c57cdf3c983af35309c39 | 0, 0 | the try body ending |
| toast-suspended-finally-called-verb.db | 679bc6b2f520fe46aa5b8d979e3326243b6035b6692966285f7c86e90f160bf2 | 3, 5 | `return 5`; the suspension is in a verb the finally body calls |
| toast-suspended-finally-raise.db | 9044f35900904ba2da2c07766c6fd999a1145c83700a3098990b971bafce4a27 | 1, {E_PERM, "finally raise", 7, stack} | an error an enclosing `except` handles |
| toast-suspended-finally-uncaught.db | 247d0350bb0ca36dcd2f70b83ae6b148d133e4619fe741b14c65d46e80023c73 | 2, 0 | an error nothing handles, raised in a verb whose caller also has a finally |
| toast-suspended-finally-return.db | fd12b742469e8fe607e7219f43b0a1c1501aebdd0018dc79051ca3c155abcff5 | 3, 5 | `return 5` |
| toast-suspended-finally-break.db | d466f7040b8d55a8fefa79da5051407c68e55a626b096243f867bfd88b8cfaed | 5, {0, 77} | `break` out of a list loop |
| toast-suspended-finally-continue.db | 40439f212e59b770e95cce163e40546f0cbfbe2c7208d9d485826541807056f2 | 5, {2, 2} | `continue` in a range loop |

What resuming must produce is in each test of the producer and in
`toastFinallyFixtures` (engine/toast_suspended_finally_import_test.go). For the
uncaught one: the task runs the rest of its finally body and its caller's
finally body, then ends, and `#0:handle_uncaught_error` is not called for it.
That fixture's `#0:handle_uncaught_error` is the producer's recorder, which had
already recorded "finally control" from a task that raised the same error
without suspending.

The regression test makes each task due in the test without changing the
serialized native program, PC, locals or operand stacks.

Only the test-owned public Test.db objects and producer code are present. No
Mongoose production checkpoint or player data is included.
