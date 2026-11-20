# Toast conformance checks checkpoint

- Current objective: make the required checks execute and account for the complete conformance corpus against canonical managed WSL Toast.
- PR #32 (`748680f6779ca666be2155f75f396269a813d0c8`) passed independent source and CI audit: 11,566 collected, 11,475 passed, 91 skipped, 0 failures/errors; all eight target cases passed exactly once.
- PR #32 final audit: exactly eight YAML fixture files plus `.github/workflows/toast-conformance.yml`; three scoped commits; `Closes #31`; `Full suite` run 30973994484 succeeded; mergeable and ready.
- Authorized merge method for PR #32: `gh pr merge 32 --repo MongooseMoo/moo-conformance-tests --merge` only. Afterward verify a two-parent merge commit and issue #31 closure.
- Issue #14/#16 branch now has separate commits: `6f09943` requirements/admission and `6125bcf` entrypoint unification, followed by a normal merge of main at `d89ac408`.
- A full managed Toast run for issue #14/#16 is currently active as exec session 4628 using the canonical WSL binary and packaged entrypoint. It started without startup/transport/collection failure.
- Next actions: merge and verify PR #32; let the active managed run finish; merge the new main containing #32 into issue #14/#16; rerun the entire managed Toast suite; derive and commit the exact skip baseline; publish and gate the #14/#16 PR.
- No current blocker. These notes are local-only and must remain untracked.

## Issue #14/#16 live findings

- First full managed run on main through PR #30 collected all 11,566 cases but failed closed: 11 failures, 11,094 passes, 461 skips. Eight failures were listener capability admissions returning `E_INVARG`; three were unavailable `PROMOTE_NUMBERS` metadata.
- PR #32 merged as normal merge `f9b8a91b2522f76ab37c64b879f24bfccbd4fd46`, verified parents `d89ac408...` and `748680f...`; issue #31 closed.
- Issue #14/#16 branch merged current main and now includes PR #32.
- `PROMOTE_NUMBERS` now uses direct mixed numeric equality semantics and fails closed on malformed/error results; unit and managed Toast proof show pinned Toast reports false, yielding three exact declared skips.
- Admission now selects/reconnects the suite database before capability probes. A unit regression proves database switch precedes probing.
- Mutable listener capabilities are snapshotted before conformance bodies to prevent lazy admission from depending on test order.
- Exact managed red proof: `listen_call_shapes` followed by listener consumers produced 8 failures, then 4 after correcting descriptor-0 cleanup. Root cause was the two-argument call-shape fixture using descriptor `0`; that Toast add/remove cycle poisoned later listener runtime state.
- Isolation fix: the two-argument listen call-shape case now allocates a disposable explicit integer descriptor and unlistens that descriptor. Other descriptor-0 cleanups and the semantic probe now remove descriptor `0`, not the returned ephemeral port.
- Exact managed green proof on the same contaminator-first 151-case surface: 147 passed, 4 declared skips, 0 failures/errors.
- Current local gates: 199 unit tests passed; scoped Ruff clean; scoped mypy with skipped imports clean; diff check pending after final inspection.
- An obsolete full managed run (session 9418) remains active because this exec backend cannot interrupt it; it loaded the pre-isolation tree and must not be used as final evidence.
- Next: commit the isolation/admission repair as its own logical commit; wait for obsolete run to exit; run the complete final managed Toast suite on the exact current tree; derive the exact workflow skip baseline from JUnit; commit CI accounting separately; push and open the #14/#16 PR with literal closing phrases.

## PR #33 final state

- Final isolation/admission commit: `8fd684e4444ceb5d813423fb339991d02c275668`.
- Authoritative local managed Toast/JUnit at that commit: 11,566 tests, 11,107 passed, 459 skipped, 0 failures/errors, 576.96 seconds.
- Exact natural-order former listener cluster and exit-only startup-repair cluster both passed in the full run.
- CI baseline changed only from 91 to 459 in standalone commit `47567cc5114f02b413f05201e73328070f01e4dc`.
- Final non-live gates after the fix: 200 unit tests passed; selected Ruff clean; selected mypy clean after ephemeral `types-PyYAML` install; worktree/diff clean.
- Draft PR: https://github.com/MongooseMoo/moo-conformance-tests/pull/33, head `47567cc`, base `f9b8a91`, with literal `Closes #14` and `Closes #16`.
- GitHub Full suite run 30976158265 passed at exact head in 4m40s. Independent artifact/JUnit verifier is still running.
- Next: require verifier MERGE; mark PR ready; repeat final files/commits/scope/check/closing-phrase/head audit; normal merge only with `gh pr merge 33 --repo MongooseMoo/moo-conformance-tests --merge`; verify a two-parent merge and closure of both issues.
