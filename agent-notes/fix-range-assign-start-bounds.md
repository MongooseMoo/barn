# fix/range-assign-start-bounds

## Goal
List and string range assignment with a start below 1 must follow Toast
instead of raising E_RANGE (`l = {1, 2}; l[0..1] = {9}` gives `{9, 2}`).

## State
Done locally, not pushed. Conformance cases are on moo-conformance-tests branch
`test/range-assign-start-bounds` (`language/range_assign_bounds.yaml`, 22 cases).

## Toast rule
- src/execute.cc EOP_RANGESET (stock aecc51e, ~2353 list, ~2370 string):
  E_RANGE iff `from > len + 1 || to < 0`.
- src/list.cc listrangeset (~302) and strrangeset (~551): keep
  base[1..from-1] (none when from < 1), then value, then base[to+1..$]
  (none when to >= len).

## Decisions and why
- Barn had one implementation, `vm/op_index.go` executeRangeSet; both compiler
  paths (compileRangeAssign, compileNestedRangeAssign) emit OP_RANGE_SET to it.
- The old check rejected every start below 1 except `[0..0]`, which had its own
  prepend branch. Replaced by `checkRangeSetBounds` (Toast's rule) shared by the
  list and string paths; the `[0..0]` branch is subsumed. The string prefix cut
  is clamped at 0.

## Evidence
- Managed stock WSL Toast: 23 passed (22 cases + capability_admission), Test.db
  sha256 1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e.
- Pre-fix Windows Barn: 10 of the 22 failed with E_RANGE. Fixed Barn: 23 passed.

## Surprises
- `--moo-host 172.17.144.45` (the plan's NAT route) did not accept connections
  within 30s on 2026-11-11; `--moo-host 127.0.0.1` reached WSL Toast.
- Not pursued: Toast's listrangeset/strrangeset take 32-bit `int from, int to`,
  so indexes beyond 2^31 truncate in Toast.

## Next step
Push both branches and open the two PRs (conformance first).
