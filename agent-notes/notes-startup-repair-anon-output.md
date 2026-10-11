# Startup repair anonymous checkpoint

## State

- Fresh repair worktree: `C:\Users\Q\code\.worktrees\barn-startup-repair`
- Branch: `agent/fix-startup-repair-anon-output`
- Base: `origin/master` at `911b97f324bd3238fc223530598d2908fa94ed57`
- No repair edits have been made yet.

## Verified facts

- Managed WSL Toast, using `scripts/run_toast_wsl.sh` and the packaged startup fixture directory through `--server-db-dir`, passes canonical admission plus `startup_repair_anon1` through `startup_repair_anon6`: 7 passed.
- The same seven-test slice through Barn's managed `scripts/run-conformance.ps1` creates every expected `.new` file on this machine. Anon1 and Anon3 through Anon6 pass; Anon2 fails because its output says `0 values pending finalization` instead of Toast's `1`.
- Open issue #95 describes this exact base defect: preserve the cyclic anonymous value pending finalization through shutdown snapshot. Its scope is separate from PR #112's all-six missing-file CI symptom.
- Anon2's `server_started` creates two anonymous objects, makes their `next` properties cyclic, and calls `shutdown()`.
- Barn's `CollectPendingFinalizationValues` inspects only composite VM locals/maps/lists and deliberately ignores bare anonymous locals. Anon1 uses a composite list and passes; Anon2 keeps the two objects in bare locals and fails.
- Barn's snapshot serializer currently seeds anonymous reachability only from regular persistent property values; pending-finalization values are copied but are not used as roots or rewritten by the anonymous serialization plan.

## Current theories

1. Primary: shutdown pending-finalization collection needs to preserve an unreachable anonymous cycle reachable from bare VM locals as one pending root, while retaining the existing Toast behavior that ordinary bare anonymous locals are not independently preserved.
2. Secondary: once such a root is recorded, snapshot planning must seed and rewrite pending-finalization references so the anonymous graph is emitted with dump-local IDs.
3. PR #112's all-six absent outputs are likely a distinct managed-launch/output-path symptom because clean `origin/master` produced all six files locally.

## Next action

- Add the narrowest red Go regression that executes or models the Anon2 shutdown path, then determine whether pending-cycle selection, snapshot rooting/rewriting, or both are required for a Toast-matching round trip.

## Blocker

- None.

## Final audit update

- PR #112's six missing `.new` files are caused by its server command passing `--checkpoint-interval 0`. Current `server.shutdown()` skips the final checkpoint when the interval is zero. Adding only that flag to the otherwise-green managed seven-case run reproduced the CI shape exactly: admission passed and all six startup cases failed because their output files were absent.
- PR #96 already contains the scoped #95 repair. Its remote head is `0450cf22c84d0c3ebdb96a9172b6072db1a7f56a`; it cherry-picked without conflict onto current `origin/master` as local audit commit `825fc6c5390f50fafef98706d04085ee826ef399`.
- No source changes to PR #96 were needed. Its five-file diff remains 172 insertions and 20 deletions, and its PR body contains literal `Closes #95`.
- On the current-master audit commit, admission plus all six startup cases passed 7/7 with normal checkpointing. Full Go tests, build, and focused race tests passed. Full vet reports only the unchanged `vm/stack.go` `ReadByte` signature warning; scoped vet passed.
- Required follow-up: update/rebase PR #96 to current master without changing its source scope, and remove `--checkpoint-interval 0` from PR #112's conformance server command.
- PR #96 was updated without history rewriting: normal merge commit `346203e5ea7660282e44022a8330230dfb040a9b` has parents `0450cf22c84d0c3ebdb96a9172b6072db1a7f56a` and `911b97f324bd3238fc223530598d2908fa94ed57`, and was pushed as the exact remote head. The PR remains open/draft and unmerged; both CI jobs were triggered on that head.
- Independent audit found a cross-task duplication gap: two shutdown VMs holding opposite members of one anonymous cycle could each append a root. Commit `8bd26306dc8f0860a12e1bb9722139998ea6924d` fixes accumulation in `Store.AppendPendingFinalizations` without changing VM collection or adding an exported API.
- The new server regression proves one cycle representative plus one disconnected graph survive snapshot/reload, including concurrent sink calls; the store regression proves later covering-root promotion while retaining a distinct pending value. Both were red before the fix and are green afterward; the concurrent case passed 50 iterations and focused race testing.
- PR #96 remote head is now exactly `8bd26306dc8f0860a12e1bb9722139998ea6924d`. Managed focused conformance remains 7/7, full Go tests and build pass, scoped vet passes, and full vet still reports only the unchanged `vm/stack.go` warning. The PR remains open/draft and unmerged; both CI jobs were triggered on the new head.
