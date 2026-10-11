# PR 98 verifier checkpoint

## Current findings

- Barn issue #97 requires known boolean `server_version()` option keys to return Toast-compatible `"ON"` / `"OFF"` strings for dot and slash spellings, preserve the unknown-key contract, cover enabled/disabled combinations, and pass unit/build/managed capability gates.
- PR #98 is open and ready, head `9d22061dac4bd6adcb4ba11d6c8e8fc410c38e68`, with one commit and two scoped files: `builtins/system.go` and `builtins/runtime_options_test.go`.
- The body contains literal `Closes #97` and describes the correct root cause and red/green matrix.
- Implementation changes only `OUTBOUND_NETWORK` and `PROMOTE_NUMBERS` option results from integer booleans to `"ON"` / `"OFF"`; unknown paths still fall through to `E_INVARG`.
- Tests cover enabled/disabled values and dot/slash spellings for both known options, plus unknown dot/slash keys.
- Exact-head required Build/unit and MOO conformance checks are currently running.

## Current state and blocker

- The two-file implementation is narrowly correct for the direct option contract. Canonical Toast source serializes `OUTBOUND_NETWORK` values 0/1 as `"OFF"`/`"ON"`; the helper is isolated and no removed integer helper callers remain.
- Pinned Toast does not advertise `PROMOTE_NUMBERS`, but Barn already exposes it as a Barn runtime option. Normalizing the existing Barn extension to the boolean string shape is not required by the current conformance probe but is covered symmetrically.
- Independent exact-head temporary-clone verification passed `go test ./... -count=1`, `go build ./...`, gofmt, and `git diff --check`.
- The managed fail-closed gate now passes the repaired `OUTBOUND_NETWORK` option probe, then fails on the next pristine capability probe: `ephemeral_listen` returns `E_INVARG`.
- Exact-head GitHub Build and unit tests passed. Exact-head MOO conformance failed with 21 skips and 11,545 setup errors, all routed through `Failed to probe feature ephemeral_listen: MooError.E_INVARG`.
- Issue #97 explicitly requires the managed capability gate to pass, and the required conformance check is red. Verdict is `NO-MERGE`.

## Next action

- Repair the separate `ephemeral_listen` admission/profile mismatch outside PR #98, update PR #98 from the repaired base if needed, and require a green exact-head conformance rerun before reconsidering merge. Do not fold unrelated listener behavior into the option-contract PR.
