# PR #99 freshness audit checkpoint

- Task: read-only freshness and architecture audit of Barn PR #99 at exact head `2719014d16e777c97979f71a6e68638d2469c288` against current `master` and pending PR #115.
- Scope: issue #91 fidelity; Go 1.24 necessity/correctness; unrelated semantic changes; overlap with PR #115, PR #110, and future issue #13 `vm/op_property` work; PR body and closing phrase; CI failure attribution; exact update, gates, and review needed.
- Constraints: current code, tests, and live GitHub only; no historical Markdown or plans; no edits or external-state changes; do not merge.
- Current state: audit complete; verdict `APPROVE-TO-UPDATE`, not approval to merge the stale/red head.
- Findings:
  - PR #99 is one exact commit `2719014d16e777c97979f71a6e68638d2469c288`, 13 files, and contains literal `Closes #91`.
  - Current master is `911b97f324bd3238fc223530598d2908fa94ed57`; it descends from PR #99's base and changed none of PR #99's 13 paths.
  - Pending PR #115 exact head `f00082ea1a2f4497fcf296a516b756ee5440a796` still has exactly the three expected `gofmt` paths and full vet fails only on `VM.ReadByte() byte`.
  - PR #99 source changes are three gofmt-only rewrites plus the exhaustive `ReadByte` to `FetchByte` rename (38 definition/call references, no old VM references). No behavior semantics change.
  - PR #115 overlaps only `.github/workflows/ci.yml`; three-way merge output retains its managed default-profile conformance block and adds the formatting/vet steps without conflict markers.
  - PR #110 has no file overlap. If PR #99 lands first, PR #110's body/gates must drop the now-stale vet-blocker statement.
  - Future issue #13 overlaps only the two `FetchByte` call sites in `vm/op_property.go`; PR #99 does not alter property permission behavior. Land/update #99 before beginning #13 to avoid stale-name conflict.
  - PR #99's failed conformance job is unrelated legacy workflow behavior: `OUTBOUND_NETWORK` probed as raw `1`, causing 11,545 capability-probe errors. PR #115's corrected managed job is green at 11,175 passed and 410 declared skipped.
  - Exact PR #99 local head is clean and passes `gofmt -l .`, `go vet ./...`, `go test ./... -count=1`, `go build ./...`, and `git diff --check`; its GitHub build job passed the same hygiene gates under Go 1.24.6.
  - No existing PR reviews or inline review comments.
- Blocker: PR #99 must not merge at its stale head with a red conformance job. PR #115/base repair must land first.
- Next action: after PR #115 is normal-merged, normally merge latest master into PR #99; confirm the diff remains the same 13 scoped paths and the workflow retains both corrected conformance and new hygiene gates; refresh the verification/body; run all local Go gates plus the documented managed default conformance job; require both GitHub workflow jobs green and an independent exact-head review before normal merge.
