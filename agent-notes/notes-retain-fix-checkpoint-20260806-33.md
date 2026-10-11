# Retain/fix checkpoint 33

## Current findings and state

- Barn PR #99 passed exact-head build/unit and default-profile conformance CI. The uploaded report contained 11,587 tests: 11,177 passed, 410 skipped, 0 failures, and 0 errors; all admission probes passed.
- PR #99 was audited for its 13-file scope, two commits, open issue #91, literal `Closes #91`, matching local/remote head, and green required checks. It was merged with `gh pr merge 99 --repo MongooseMoo/barn --merge`.
- PR #99's GitHub merge commit is `26ffaa21fcbbabd8c52f7c44b40c9cbed99defce`, exactly matches `origin/master`, and has two parents. Issue #91 is closed by PR #99.
- PR #94 now includes the new master through normal merge commit `08b2a60d79b709226e8c23bf3a22a3625202f98a`, whose parents are repaired head `13098599a5d6e7911ef6da627c004c3fa9f6700b` and master `26ffaa21fcbbabd8c52f7c44b40c9cbed99defce`. The merge was clean and retained the untracked `reports/runs/` directory.
- PR #94 independent review is still running. Exact post-master-merge gates and PR-body correction remain pending; nothing is pushed.
- PR #110 worker is finishing its current race gate, then will normal-merge master `26ffaa21...` and rerun gates. Nothing is pushed or edited remotely.

## Blockers

- PR #94 requires exact-head post-merge gates, independent approval, corrected provenance/verification text, fresh CI, and literal pre-merge audit.
- PR #110 requires post-master-merge gates, independent approval, fresh CI, and literal pre-merge audit.

## Next action

1. Run PR #94 exact-head formatting, vet, build, full tests, focused framing tests, and diff audit; obtain independent approval.
2. Push and accurately update PR #94, then require fresh exact-head CI before normal merge and two-parent verification.
3. Complete the same sequence for PR #110 after its worker reports.

This note is intentionally uncommitted and must not be staged, committed, or pushed.
