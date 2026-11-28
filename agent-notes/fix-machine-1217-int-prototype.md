# Match the missing integer prototype verb error

## Goal

Resolve the review finding on Barn PR #1217 and verify the integer-prototype behavior with the managed Toast oracle. The companion regression belongs in suite PR #297.

## State

- The managed Toast run completed with `5 passed`, including capability admission and the four tests in the affected file.
- The original Barn PR head returned `1 failed, 4 passed`. Its `-1` call reported `Verb not found: -1:conformance_no_such_verb()` where Toast reported `Verb not found: conformance_no_such_verb`.
- The corrected Barn build returned `5 passed` using the same managed selection and pinned fixture.

## Decisions and why

- Use the existing missing-verb exception function and preserve the original receiver and verb in the error value. Verify a second integer receiver to distinguish the special message from the ordinary one.
- Place the durable behavioral test in the companion suite PR. It installs a temporary integer prototype and restores any prior value.

## Surprises

- The first managed harness blocked in `p9_client_rpc` on the Windows-mounted source. Isolated Linux-native snapshots let the same documented invocation complete without changing the database, oracle, or selected cases.

## Next step

Commit the corrected formatter and hand the two exact tested commits to the parent agent. The suite regression follows the Barn merge.
