# GC cyclic-chain repair checkpoint

## Current findings

- The current conformance suite defines the failing cases in `builtins/gc.yaml`: `cyclic_chain_length_3`, `cyclic_chain_length_5`, and `cyclic_chain_length_6`.
- Each case builds an anonymous-object cycle, calls `run_gc()`, recycles the cycle head, drops task references, calls `run_gc()` again, then recycles the common parent and calls `run_gc()` once more.
- The required WSL Toast oracle binary exists and is executable at `/root/src/toaststunt/build-release/moo`.
- Managed WSL Toast passes canonical admission plus all three exact cases: lengths 3, 5, and 6.
- Managed Barn from exact `origin/master` fails those same three cases with `E_INVARG` while admission passes.
- A scheduler-level regression is being developed. A bare-store fixture failed earlier with unrelated `E_PROPNF`; loading `Test_conf.db` and evaluating as object `#0` failed with `E_PERM` because the fixture's Wizard is `#3`, not `#0`.
- The active common-cause hypotheses are stale task transaction validation after live anonymous recycling, or incorrect nested anonymous recycle/adoption state.
- The faithful command-eval regression now reproduces `E_INVARG` for all three lengths.
- Diagnostic stage markers prove the error occurs at `recycle(y)` because the immediately preceding first `run_gc()` has already made `valid(y)` false even though `y` remains in the active eval frame.
- Direct non-transactional eval passes, so the common cause is transaction visibility: anonymous `next` links are staged in `StoreTxn`, while GC expands roots through only the live store. It sees intermediate nodes as unreachable, then recursive recycle crosses the staged cycle and invalidates the live root `y`.
- The fix supplements live anonymous reachability with the active transaction's staged property graph; live expansion remains in place for sibling-task references newer than the transaction snapshot.
- Focused VM/store and command-eval regressions pass; both pass under `go test -race`.
- Managed Barn conformance passes admission plus lengths 3, 5, and 6 (`4 passed`).
- `go test ./...` and `go build ./...` pass.
- Repository-wide `go vet ./...` reaches the unchanged base warning `vm/stack.go:49: ReadByte() byte`; exact base `911b97f` has the same signature. Scoped vet passes for `db/store` and `scheduler`, and `vm` passes with only the baseline `stdmethods` analyzer disabled.
- Targeted `gofmt -l` and `git diff --check` are clean.

## State

- Repair worktree: `C:/Users/Q/code/.worktrees/barn-gc-cyclic-chain`.
- Branch: `agent/fix-gc-cyclic-chain-einvarg`.
- Exact base: `911b97f324bd3238fc223530598d2908fa94ed57` (`origin/master`).
- Intended changed files are `db/store/store_reachability.go`, `vm/anonymous_gc.go`, `vm/anonymous_gc_test.go`, and `scheduler/eval_fork_test.go`.

## Blocker

- No blocker.

## Next action

- Remove only generated worktree artifacts, restore the uv lockfile changed by the managed harness, inspect the final diff, and commit without pushing.
