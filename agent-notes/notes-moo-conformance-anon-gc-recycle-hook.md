# Anonymous GC recycle-hook conformance checkpoint

## Findings and observations

- The narrow default-profile home is `src/moo_conformance/_tests/builtins/gc.yaml`.
- That suite already exercises `run_gc()`, orphan anonymous collection, cycles, and anonymous recycle hooks.
- Current multi-step and cleanup schema supports captured setup objects, persistent root properties, and guaranteed cleanup without runner changes.
- Existing anonymous-hook patterns install a `recycle` verb on a numbered class object and use class properties to observe hook behavior.
- The managed WSL Toast authority remains `/root/src/toaststunt/build-release/moo` under the harness-owned server lifecycle.

## State

- The noisy root checkout was restored to its exact tracked baseline after confirming it was clean at task start.
- An isolated worktree now owns the change at `C:\Users\Q\code\.worktrees\moo-conformance-gc-recycle-hook`, based on `origin/main` commit `49f08f5`.
- Its sole diff is `src/moo_conformance/_tests/builtins/gc.yaml`, exactly 41 insertions and zero deletions.
- Forced-fresh collection named the exact new node.
- The managed Windows harness selected canonical admission plus the exact node; admission passed and the new Toast case failed with `E_TYPE` in its combined post-sweep assertion.

## Blocker

- The combined `{hook_count, anonymous_type, valid(stash)}` expression does not identify which post-sweep invariant produced Toast's `E_TYPE`; the test needs separate assertions before its semantic result can be trusted.

## Next action

- Split the post-sweep observation into exact hook-count, stash-type, and stash-validity steps, rerun only admission plus the named case through the requested Barn WSL wrapper, and keep the final conformance diff insertion-only without committing or pushing.
