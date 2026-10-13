# Bcrypt conformance issue 39 checkpoint

- Task: implement MongooseMoo/moo-conformance-tests issue 39 without changing Barn product code.
- Worktree: `C:\Users\Q\code\moo-conformance-bcrypt-39-20260805`
- Branch: `agent/bcrypt-prefix-semantics-39`
- Base: current `origin/main` at `7af1f02`.
- Scope found: focused cases belong in `src/moo_conformance/_tests/builtins/algorithms.yaml`; the managed Toast workflow is `.github/workflows/toast-conformance.yml` and runs quality gates plus three pinned Toast profiles and an exact-union execution ledger.
- Existing coverage: bcrypt `$2a$` salt/crypt behavior, cost bounds, wizard-only non-default cost, malformed cost characters, and `$2b$` crypt failure marker.
- Missing coverage per issue: genuine high-byte `$2x$` versus corrected `$2y$`, `$2y$` salt preservation, `$2x$` and `$2b$` salt rejection, exact two-digit cost width, and permission/error ordering.
- Blocker: exact new oracle outputs must be established by the managed Toast GitHub workflow; no manual Toast or Barn process may be launched.
- Next action: add narrowly scoped algorithm identities, run local non-server gates, commit/push/open a draft PR with literal `Closes #39`, then use managed workflow evidence to correct any candidate assertion.

## Source inspection update

- The pinned Toast commit is `aecc51e9449c6e7c95272f0f044b5ba38948459e` from the managed workflow.
- Its `src/crypto.cc` parser recognizes bcrypt `$2a$`, `$2x$`, and `$2y$`, parses cost before the final `$`, and accepts numeric values 4 through 31 before backend validation.
- `crypt()` checks programmer permission after parsing but before `_crypt_blowfish_rn`; a non-default parsed cost therefore yields `E_PERM` for a programmer even if backend layout validation would later reject malformed cost width for a wizard.
- `salt()` routes parsed bcrypt prefixes to `_crypt_gensalt_blowfish_rn`; the issue records the effective generator/parser intersection as `$2a$` and `$2y$`, with `$2x$` and `$2b$` rejected.
- High-byte construction still needs a conformance expression that reaches `crypt()` as genuine high bytes. MOO binary text such as `~80` is not automatically decoded by `crypt()`; source-level non-ASCII or another existing runtime construction pattern must be used and proved in the managed Toast job.

## Scope correction

- `chr(128)` is confirmed by current conformance coverage to create one raw high byte, and bcrypt hash comparison uses `[4..60]`, which excludes character 3 containing the `x` or `y` variant.
- Initial commit `1b3c110` added `chr` to the existing `algorithms.yaml` suite-level requirements. Review correctly identified that this would make every legacy algorithm identity depend on `chr`.
- Draft PR `MongooseMoo/moo-conformance-tests#40` was opened with literal `Closes #39`; its first managed run was stopped while full Toast profiles were still executing.
- Correction in progress: restore `algorithms.yaml` exactly and move all new identities to `builtins/bcrypt_prefix_semantics.yaml` with focused requirements `[crypt, salt, chr]`.
- Next action: eliminate residual line-ending-only differences in `algorithms.yaml`, rerun all local gates, commit and push the focused-suite correction, then use the replacement managed Toast run as oracle proof.

## Focused suite complete

- Commit `7aa3368` restores `algorithms.yaml` exactly to `origin/main` and adds `src/moo_conformance/_tests/builtins/bcrypt_prefix_semantics.yaml` with focused requirements `[crypt, salt, chr]`.
- Final branch diff from `origin/main` is one new YAML file with 85 lines; no existing suite or harness file differs.
- The focused suite expands to 17 unique identities, including a combined `$2a$`/`$2y$` preservation assertion and no skip conditions.
- Revalidation is green: 267 harness tests, complete packaged schema collection, exact duplicate baseline, Ruff, and mypy.
- Next action: push `7aa3368`, confirm PR #40 final diff/body, then wait for the replacement managed Toast workflow and exact-union ledger.
