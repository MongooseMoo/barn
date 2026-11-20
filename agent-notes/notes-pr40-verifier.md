# PR #40 verifier checkpoint

- Scope: read-only final merge gate for `MongooseMoo/moo-conformance-tests` PR #40.
- Pinned expected head: `610906da361713ae81e45245de9883e410049ac7`.
- Current PR state: open, draft, mergeable; current remote head matches the pin.
- Final diff surface reported by GitHub: exactly two new YAML files, 109 additions total.
- Commits: four focused commits ending at the pinned head.
- Linkage: PR body contains literal `Closes #39`; issue #39 remains open while PR is unmerged.
- Managed run `30995941201`: completed successfully at the pinned SHA; harness gate, three Toast profile jobs, and Full suite all report success.
- Next action: inspect exact YAML content, schema/template expansion, requirement isolation, bcrypt semantic strength and ordering; inspect job logs/artifacts/ledger; re-check remote head before verdict.
- Current blocker: none identified yet. Draft status means the PR is not presently ready to merge even if the code earns a technical MERGE verdict.

## Content and harness observations

- Table expansion recursively substitutes every embedded `{key}` with its row value. Therefore `${variant}` becomes `$2x` or `$2y` for the crypt rows, while names using `{variant}` become `2x` or `2y`.
- The high-byte identity compares slices `[4..60]` from `$2x$` and `$2y$` results, excluding the differing variant character and comparing the remaining cost, salt, and hash body. The companion ASCII identity expects those same slices to match.
- Requirements are isolated by suite: the broad salt/cost/ASCII suite requires only `crypt` and `salt`; the single raw-byte identity requires `crypt` and `chr`.
- The execution ledger fails closed on missing/extra identities per profile, failed/error outcomes, unreviewed never-executed identities, stale baseline entries, and skip-reason drift.
- Remaining work: inspect exact job logs/artifacts for expanded identity execution and exact ledger counts, audit commit patches and duplicate evidence, then re-check head/draft state.

This checkpoint note is intentionally uncommitted.
