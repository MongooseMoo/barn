# Current campaign checkpoint 8

## Current findings

- Ordinary conformance PR/push CI already runs all 11,566 packaged identities against pinned Toast in three profiles and enforces the exact union with the reviewed never-executed baseline.
- The issue #18 paired-candidate workflow is being extended to call that same reusable Toast workflow before executing the explicit Barn/conformance pair.
- The Toast workflow now accepts a required exact `conformance_sha` on `workflow_call`, and all three jobs check out that SHA while ordinary PR/push events fall back to `github.sha`.
- A new `validate-inputs` job validates lowercase 40-hex Barn and conformance SHAs and the expected-result contract before either candidate checkout. The Toast job and Barn pair consume only validated job outputs.
- Full local proof before the latest fail-closed refinement: 253 harness tests passed, 11,566 packaged cases collected, duplicate baseline matched 45 name groups and 28 content groups, Ruff passed, mypy passed across 19 files, and diff check passed.

## State and blocker

- The latest focused run has one test assertion failure only: the test looks for shell text `barn_sha=$BARN_SHA`, while the workflow writes the same output safely with `printf 'barn_sha=%s\\n' "$BARN_SHA"`. This is a test-string mismatch, not a workflow failure.
- Independent reviewer is re-auditing after the validation job was added and requires a stable snapshot.
- No project historical Markdown was read.

## Next action

- Correct the overly literal test assertion to verify the safe `printf` output, rerun focused/full gates, freeze the exact diff, then have the independent reviewer issue a MERGE/NO-MERGE verdict on that snapshot before commit or push.
