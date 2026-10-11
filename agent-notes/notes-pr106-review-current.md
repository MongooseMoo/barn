# PR 106 current review checkpoint

## Current findings

- PR #106 exact head is `08a2de3498c8b888e0d0244cc12700bce3bd0830`, based on Barn `aa82abf05a6a23783ba69105d0c393f926304a13`; the exact-head worktree was clean at gate start.
- The final diff remains limited to `builtins/crypto.go` and `builtins/crypto_bcrypt_prefix_test.go` across three issue-focused commits.
- The correction now accepts numeric `salt()` cost tokens of at least two digits, so `$2a$004$` and `$2y$004$` canonicalize to `$2a$04$` and `$2y$04$`; one-digit cost `4` remains rejected.
- `crypt()` still requires exactly two digits. Its validation/range, permission, and backend-format ordering is unchanged from the prior audited head.
- Final-head prefix handling keeps `$2a$`/`$2y$`, rejects `$2b$`/`$2x$` in `salt()`, returns `*0` for unsupported `$` crypt tags, and does not fake historical `$2x$` semantics.
- The tests assert exact known-answer hashes and explicit prefix preservation/rejection, rather than prefix-only output.
- Focused bcrypt tests repeated ten times passed at the exact head.
- Managed Toast conformance PR #40 run `30995941201`, scenario head `610906da361713ae81e45245de9883e410049ac7`, is fully green across the harness and three pinned Toast profiles. Its scenarios pin the required one-digit rejection, three-digit canonicalization, exact crypt width, prefix matrix, ordering, and high-byte `$2x$` distinction.
- PR #106 includes literal `Closes #72`; issue #72 remains open while the PR is unmerged.

## State and blocker

- No PR implementation or test file has been modified by this verifier. This checkpoint note is an uncommitted runtime-required artifact outside the PR worktree.
- Barn unit CI is green. Barn MOO conformance CI run `30996803831` failed before scenario execution: all 11,545 executed rows raised `CapabilityProbeError` because `OUTBOUND_NETWORK` returned `1` instead of `ON`, `OFF`, or `#-1`; 21 rows were skipped. This is not a bcrypt assertion failure, but the required check is red.
- Local exact-head gates all passed: focused bcrypt tests repeated ten times, focused race, `go test ./... -count=1`, `go build ./...`, `gofmt -l` on both touched files, and `git diff --check`.

## Next action

- Reconfirm final PR head stability, draft/check state, and the uncommitted-note boundary; then report CODE MERGE separately from CI NO-MERGE.
