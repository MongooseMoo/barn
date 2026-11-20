# Value payload tag checks

## Goal
Make a wrong-type access to a Value's heap payload panic instead of reading
one payload type as another. Q chose "always check tag" 2026-11-02 after the
unsafe audit (plan: barn master checkout agent-notes/fuzzing-expansion.md).

## State
types/value.go: strRep/sliceList/goMap/waifRep index a one-element array with
`tag ^ expected`, so any other tag panics (index out of range). Draft PR; perf
gate not yet run.

## Evidence
- Only STR, LIST, MAP and WAIF values carry ref (types/list.go:138,
  types/map.go:663, types/str.go:33-118, types/waif.go:72,
  types/waif_image.go:142), so each cast has exactly one valid tag.
- The typed methods (Str, Get/Set/Append/Elements/Slice, MapGet/MapSet/Keys,
  waif Class/GetProperty/...) call the casts without their own tag check.

## Decisions and why
- Bounds-check form, not `if tag != X { panic(...) }`: the panic call cost 16
  methods their inlining (Str, Get, Len, MapGet...). The bounds-check form
  costs about 8 per cast; 8 methods that were within 1-7 of the budget of 80
  no longer inline without PGO: Append, MapDelete, MapGet, MapSet, Set, Slice
  (cost 81-82), StrAppend (93), StrIsSingleByte (88). Each already calls a
  non-inlined worker, so the loss is one call frame. Production builds use
  cmd/barn/default.pgo, which raises the budget at hot sites.

## Next step
Perf gate on an idle machine: BenchmarkVM before/after with benchstat, and
TestMongooseRealWorkload 1p and 16p, 3 runs each, load logged. Fix any
slowdown before merge. Fuzz FuzzSourceExecution with the check to hunt for
callers that skip Type().
