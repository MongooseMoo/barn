# PR 105 atomic delete authority repair checkpoint

- Requested branch/worktree: `agent/fix-105-atomic-delete-authority` at `C:\Users\Q\code\.worktrees\barn-fix-105-atomic-delete-authority`.
- Exact base verified: `911b97f324bd3238fc223530598d2908fa94ed57`.
- Current finding: `delete_verb()` resolves authority through a transaction snapshot, then performs an irreversible live deletion whose store-locked generation check does not validate object authority. A concurrent authority revocation can therefore leave a verb deleted even when later transaction validation fails.
- Constraints: retain descriptor validation/error precedence, exact `ResolvedVerb` identity/generation behavior, owner/wizard/write authorization, and staged-write behavior. Deterministic tests must use synchronization, not sleeps.
- State: worktree and branch created cleanly; no implementation or test edits yet.
- Blocker: none.
- Next action: inspect current test helpers and store APIs, add deterministic failing regressions, capture red evidence, then implement one store-locked authority-and-delete operation.
- This checkpoint note is intentionally untracked and must not be staged, committed, or pushed.

## Transaction-staged redesign checkpoint

- Review rejected the intermediate live-preflight/live-delete implementation because it still bypassed transaction atomicity and ignored same-task staged authority.
- Deterministic red proof captured: staged owner/flag grants were denied, staged revocations were ignored, live deletion occurred before commit, unrelated conflicts did not validate, and changed verb generations could not conflict.
- Current implementation stages ordered verb-list indices on `StoreTxn`, mutates only its private view, drops a staged code write for the exact deleted target, and preserves survivor code writes.
- `Commit` now treats verb deletion as a write, validates scalar/verb/unrelated reads plus all shifted indices before mutation, and applies deletion while holding the existing commit gate/store or object-slot locks.
- Focused tests are green for authority revocation then serial `E_PERM`, unrelated conflict with no deletion, staged owner/FlagWrite grants and revocations, two shifted-index deletes, survivor `SetVerbCode`, multi/overlapping aliases, and concurrent list-generation conflict.
- `HasStagedTopology` includes verb deletion. `FlushStagedToLive` uses the commit gate and complete read validation whenever a verb deletion is present; conflict returns before mutation. New explicit flush tests were just added and have not yet been run.
- No-transaction fallback remains one store lock; an explicit builtin test was just added and has not yet been run.
- Blocker: none known. Remaining work: run the new focused tests, inspect/fix regressions, then focused race, full test/build/vet/gofmt/diff gates, final review, and commit/amend without push.
