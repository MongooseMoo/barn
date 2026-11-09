# Native suspended-call fixture

Produced by the managed canonical WSL Toast server from Test.db using
tests/mongoose-conformance/server/suspended_call_checkpoint.yaml. The producer
passed canonical capability admission and the complete restart test; retained
XML reports errors=0, failures=0, tests=2, time=24.158 seconds.

SHA256: 51158bee3d37e0b59225611e3a138dd274261e4c5993a8846f73fc74d229f0d8

The checkpoint was captured while task 1656954001 was suspended. Its forked
driver calls start(), which saves the left operand 100 while pause() suspends.
The object has before=1, after=0, result=0, finished=0. Resuming must produce
before=1, after=1, result=173, finished=1. The task's original absolute wake
deadline is 1794030538; the focused regression makes it due in the test without
changing the serialized native program, PC, locals or operand stacks.

Only the test-owned public Test.db objects and producer code are present. No
Mongoose production checkpoint or player data is included.
