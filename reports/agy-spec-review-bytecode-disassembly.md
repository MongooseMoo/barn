# Agy Review: Bytecode Disassembly Fixed Point

Reviewed on 2026-07-11 against the current implementation and specification
diff.

## First verdict: REJECT

The first adversarial pass identified two uncovered regressions:

1. Map range reads with non-integer keys would receive key-valued first/last
   markers even though range execution requires positional bounds.
2. Map range assignments with integer keys outside the positional range would
   misinterpret those keys as positions.

It also identified a dead duplicated context check in
`compileIndexBoundary`. The slice was not committed in that state.

## Correction

The index-marker operand now distinguishes index context from range context.
Map indexing resolves boundaries to keys; every range resolves them to
positional `1` and collection length. Both operand forms remain actual bytecode
and disassemble as `FIRST` or `LAST`. The dead branch was deleted, Toast map
boundary behavior was recorded, and VM regressions cover string-keyed reads
and non-positional integer-keyed assignments.

## Final verdict: APPROVE

The follow-up review approved the corrected slice and specifically confirmed:

1. Map range reads and assignments now use positional boundaries.
2. Map indexing still resolves first/last using sorted keys.
3. Context-free boundary fallbacks remain reachable and deterministic.
4. Toast mnemonics are decoded without a source or frontend-IR dependency.
5. The scheduler ID-collision failure remains independent of this diff.
