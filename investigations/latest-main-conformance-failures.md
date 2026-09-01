# Investigation: Latest-main conformance failures

## Facts (verified)
- Managed conformance at test head `9bdfd2ece7316fa3a28cdfa71b0624f9e92552f4` against Barn `49485169380dfe46f285a1a803f1a67aea01e877` completed with 18 failures, 12,499 passes, and 420 skips.
- Six failures exercise negative `rindex()` offsets; two exercise Boolean/integer equality; ten exercise Boolean/integer membership.
- The final fix changes reverse-search bounds and shares Boolean/integer MOO equivalence across equality and membership surfaces.

## Theories (plausible)
1. Barn normalizes negative `rindex()` offsets differently from Toast, explaining the six string-search failures.
2. Barn's general equality relation aliases Boolean and integer values, explaining both equality and membership failures.
3. Membership has an independent comparison defect, so repairing equality alone will not fix all ten membership failures.
4. One or more new expectations do not match the documented Toast oracle and must be rejected rather than implemented.

## Tests Run

| Test | Hypothesis | Result | Rules Out | Supports |
|------|------------|--------|-----------|----------|
| Full managed Barn suite | Establish complete failure inventory | 18 failures in three semantic groups | Six-case-only inventory | Multiple root causes |
| Managed WSL Toast: admission plus all three failing families | Validate the new expectations against the pinned oracle | 772 passed, 12,165 deselected | Theory 4: incorrect expectations | The failures are Barn divergences |
| Barn source trace and focused red unit tests | Distinguish shared equality from independent membership defects | `rindex` returned 1; equality/membership returned `{1,0,2,2,2,1,0,2,2,2}` | Theory 3: independent membership defect | One shared bool/int equivalence plus one rindex bound defect explain all failures |
| Focused Go tests after two production changes | Predict both roots before managed verification | `builtins` and `vm` passed | Symptom-only patches | The two-root model |
| Focused managed Barn run | Test the two-root prediction across every affected case | 772 passed, 12,165 deselected | Remaining affected-family divergence | Both fixes close their predicted scope |
| Complete managed Barn run | Detect regressions or additional latest-main failures | 12,517 passed, 420 skipped, exit 0 | Additional conformance roots | The worktree is clean across the complete suite |

## Current Best Theory
Two roots explain the complete inventory: `rindex()` clamped an impossible search window to index zero, while builtin `equal()` and both membership paths bypassed the VM's existing Boolean/integer equivalence.

## Open Questions
- None.

## Next Action
Investigation complete. Preserve the two regression tests and the shared equivalence helper with the production fixes.
