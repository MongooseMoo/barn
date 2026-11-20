# Fully execute campaign - current checkpoint

- Current campaign scope is Barn plus `moo-conformance-tests`, using current code, live GitHub state, current CI, and managed WSL Toast only. Historical project Markdown is excluded.
- `moo-conformance-tests` PR #34 merged as two-parent commit `d7eeee341a51aa4a25ad46033ddd2c72e47946ac`, but its main push run `30982938666` failed the 32-bit Toast profile and protected `Full suite`.
- Exact failure: all eight new `slice({}, "x"[, default])` 32-bit cases expected `E_RANGE` but pinned Toast returned success `[]`; totals were 11,476 passed, 90 skipped, 8 failed. The immediately preceding PR run `30982355955` on the same tree/pinned Toast passed those eight as `E_RANGE`.
- Current Toast source shows `bf_slice` admits string indices and then reads `index.v.num` from the union for all non-list indices. That makes string-index behavior dependent on pointer/union representation and therefore not a stable 32-bit semantic contract.
- Issue #17 was reopened because current main is red and the merged assertions are not reproducible. Failed jobs are being rerun to characterize stability; no Barn-side behavior work will proceed on this uncertain surface.
- Live conformance issues: #12, #13, #15, #17, #18, #23, #24. Draft PR #22 targets #12 but conflicts with current main and must wait for a green/stable base.
- Live Barn has 77 open issues. Active PRs: #94/#93, #96/#95, #98/#97, #99/#91, #101/#100, #81/#4. Current fail-closed conformance exposes a common base blocker: Barn master reports `OUTBOUND_NETWORK` as integer `1`; PR #98 is the isolated fix. Feature branches must update only after #98 lands.
- PR #98 local exact-head gates are green: focused formatting/diff check, `go test ./... -count=1`, and `go build ./...`. Its managed CI rerun is in progress.
- Barn CI itself has a current delivery gap: it tracks mutable conformance main but is not triggered by conformance changes and omits profile manifests/strict unexpected-skip accounting. Conformance issue #18 tracks paired candidate CI.
- Managed local wrapper also lacks profile manifests/strict skip accounting and a long-lived orphan canonical Toast process exists; it has not been stopped manually.
- Immediate blocker: the managed 32-bit Toast oracle gives contradictory results for the same head.
- Next action: complete rerun/source investigation, replace the invalid 32-bit assertions with a fail-closed representation of the actual supported contract, restore conformance main green through a scoped PR closing #17, then resume the Barn base-repair and dependent PR sequence.
