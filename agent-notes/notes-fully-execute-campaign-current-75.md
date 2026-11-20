# Current campaign checkpoint 75

## State

- Data PR #63 is open at exact head `5f31f11722c112975bec35095460c54f2beedd91`, base `e9b6532c5dd9f46104739f4671137e38bc95363e`.
- PR #63 has one commit, one YAML file, 17 insertions, zero deletions, and literal `Closes #12`.
- Its complete Toast workflow run `31057331207` is executing all three profiles after green classification and trusted controller validation.
- Exact paired run `31057380765` was dispatched for Barn `aa82abf05a6a23783ba69105d0c393f926304a13` and conformance `5f31f11722c112975bec35095460c54f2beedd91`, expecting packaged success with no bad identities.

## Findings

- Current Barn master is still `aa82abf05a6a23783ba69105d0c393f926304a13` and is already known to fail `admission::feature.ephemeral_listen`; paired success for #12 therefore depends on Barn #100.
- Barn issue #100 requires runtime `listen(..., 0)` to retain MOO descriptor zero while owning the OS-assigned bound port, reject duplicate descriptor zero, and preserve startup-owned port-zero behavior.
- Draft Barn PR #101 is based exactly on current master and is mergeable. Head `df0f8ac8b77ae9ab8458d78341f71218766e13c8` contains two commits and four files: `builtins/network_test.go`, `server/connection_manager.go`, `server/connection_manager_test.go`, and `server/tls_listener_test.go`.
- PR #101 has literal `Closes #100`; unit/build CI was green, while its old conformance check failed because then-current conformance admission stopped earlier.

## Observation

- The exact expected-success pair for PR #63 is useful proof even if it fails: a failure at `ephemeral_listen` confirms the current dependency; any different result must be analyzed from the managed evidence before Barn-side work.
- Barn PR #101 should not absorb preceding issue #97 or unrelated base fixes. Current master now includes those prior repairs, so #101 can be revalidated on its existing exact base.

## Blocker

- PR #63 cannot satisfy issue #12's Barn acceptance criterion until an exact pair succeeds. Barn #100 is the likely blocker pending current managed run evidence.

## Next action

- Finish current Toast and paired runs. Independently audit PR #101 source/tests, rerun focused/full Go gates, refresh conformance evidence through managed workflows, then normal-merge #101 only if exact scope and checks pass. Re-dispatch the exact PR #63 Barn pair against the repaired Barn merge before merging #63.
