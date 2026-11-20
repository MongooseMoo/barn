# Issue 40 map tree cache checkpoint

## Current findings

- `goMap.toastRoot()` rebuilt the complete Toast-compatible red-black tree on every ordered read.
- The TDD regression failed before implementation because repeated calls returned distinct roots and a 1,000-entry case-insensitive lookup allocated 1,000 tree nodes per read.
- `goMap` is immutable/copy-on-write: `set` and successful `delete` construct fresh maps, so a per-map lazy cache does not require mutation invalidation.
- The implementation now uses `sync.Once` to publish one cached tree to concurrent readers.
- Case-insensitive string lookup uses the existing folded-string hash index; case-sensitive strings and all non-string keys use the cached Toast tree.
- A blanket hash fast path for every key type was rejected because it changed the stable mixed boolean/integer map reachability contract.

## Oracle and test evidence

- Managed WSL Toast command selected canonical admission plus `boolean_and_equal_integer_mixed_map_reachability` using `/root/src/toaststunt/build-release/moo`.
- Result: `test_capability_admission` passed and the exact boolean/integer reachability row passed; 2 passed, 11,582 deselected, exit 0.
- Targeted Barn tests and the concurrent-reader race test passed before the string-only refinement.
- Benchmark after caching: case-insensitive lookup stayed about 108-123 ns/op across 100, 1,000, and 5,000 entries; cached case-sensitive lookup allocated 0 B/op and 0 allocs/op.

## State and blocker

- Branch: `agent/issue-40-cache-map-tree`, based on current `origin/master` at `2130283`.
- Intended PR files: `types/map.go` and `types/map_cache_test.go` only.
- The checkout contains many unrelated untracked files. They must remain untouched and unstaged.
- No current blocker. The earlier full-suite failure identified the over-broad hash shortcut and has been corrected with the Toast-backed string-only boundary.

## Next action

1. Rerun targeted tests, race test, full `go test ./... -count=1`, `go vet ./...`, formatting, and diff checks.
2. Audit the two-file diff and commit only those files.
3. Push, open a ready PR with literal `Closes #40`, watch required CI, audit final files/commits/scope/closing phrase, normal-merge with `gh pr merge --merge`, and verify a two-parent merge commit plus issue closure.
