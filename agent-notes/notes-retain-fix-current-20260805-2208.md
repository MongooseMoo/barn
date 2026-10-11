# Retain/fix recovery checkpoint

- User authorized retaining landed PRs and fixing confirmed defects.
- `origin/master` is `911b97f324bd3238fc223530598d2908fa94ed57`.
- Issues #72 and #102 were reopened with exact regression comments.
- PR #98 was retained; its merged PR title/body now clarify that dotted option keys and `PROMOTE_NUMBERS` exposure are Barn extensions, not exact Toast key-surface claims.
- #72 corrective branch `agent/fix-106-bcrypt-raw-dollar` has commit `0a9ecea5c6788a91bd81f109943ed0a31d5a175b`; first-delimiter parsing and overflow-safe conversion are green. A follow-up is expanding raw-salt boundary/binary tests and long-leading-zero parser coverage.
- #102 corrective branch `agent/fix-105-atomic-delete-authority` has partial commit `9b31114f09b1c6a4729fe3ecbe192f8aadae88a1`. Independent review blocked the live-preflight/live-delete design because it ignores same-task staged authority and can publish irreversible state before later transaction validation.
- Required #102 direction: transaction-owned staged verb-topology deletion; scalar and verb reads validate before commit-time publication; preserve E_VERBNF-before-E_PERM, exact identity, repeated deletes, staged authority, aliases, and no denied-path publication.
- No corrective branch has been pushed.
- Next: finish both worker follow-ups, independently review exact heads, run local gates, publish separate draft PRs, obtain managed conformance evidence, then audit and normal-merge with two-parent verification.
- This checkpoint file must remain untracked and must never be staged, committed, or pushed.

## Progress update

- #72 branch now has tests-only follow-up `cad9edfab0eac9f2fd0d1fbf15b3bf0d90eb0651`; independent review APPROVED the exact two-commit head.
- Draft PR #109 is open from that head with exact two-file scope and literal `Closes #72`; build/unit CI is green and the default-profile conformance job is running.
- PR #98's merged title/body were clarified to describe Barn ON/OFF reporting and explicitly label dotted paths/PROMOTE_NUMBERS as Barn extensions.
- #102 live-delete partial commit remains blocked. The worker is now adding red tests for transaction-staged deletion; current unstaged changes are tests only in `builtins/object_metadata_permissions_test.go` and `builtins/verbs_txn_test.go`.
- Transaction design uses ordered deletion indices from the successively mutated private verb list, validates scalar/verb/unrelated reads before apply, removes deleted-target staged verb writes, preserves survivor writes, and retains a one-lock no-transaction fallback.
- Next: capture #102 red proof, implement staged commit/apply path, run exact-head gates and independent review; meanwhile finish PR #109 conformance/merge lifecycle.
